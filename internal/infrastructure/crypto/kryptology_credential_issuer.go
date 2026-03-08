package crypto

import (
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"

	"github.com/cloudflare/circl/blindsign/blindrsa"
	"github.com/google/uuid"
)

const (
	defaultRSABits = 2048
)

type persistedIssuerKey struct {
	Version     int       `json:"version"`
	PublicKey   []byte    `json:"public_key"`
	PrivateKey  []byte    `json:"private_key"`
	ActivatedAt time.Time `json:"activated_at"`
}

type persistedIssuerState struct {
	ActiveVersion int                  `json:"active_version"`
	Variant       string               `json:"variant"`
	Keys          []persistedIssuerKey `json:"keys"`
}

type issuerKeyMaterial struct {
	version     int
	publicKey   *rsa.PublicKey
	privateKey  *rsa.PrivateKey
	client      blindrsa.Client
	verifier    blindrsa.Verifier
	signer      blindrsa.Signer
	activatedAt time.Time
}

// KryptologyCredentialIssuer 使用 RFC9474 RSABSSA 盲签名签发凭证。
type KryptologyCredentialIssuer struct {
	mu          sync.RWMutex
	keys        map[int]*issuerKeyMaterial
	activeVer   int
	storePath   string
	rotateAfter time.Duration
	rsaBits     int
	variant     blindrsa.Variant
	initErr     error
}

var _ domaincrypto.CredentialIssuer = (*KryptologyCredentialIssuer)(nil)

func NewKryptologyCredentialIssuer() *KryptologyCredentialIssuer {
	return NewKryptologyCredentialIssuerWithFile("", 0)
}

func NewKryptologyCredentialIssuerWithFile(storePath string, rotateAfter time.Duration) *KryptologyCredentialIssuer {
	issuer := &KryptologyCredentialIssuer{
		keys:        make(map[int]*issuerKeyMaterial),
		storePath:   storePath,
		rotateAfter: rotateAfter,
		rsaBits:     defaultRSABits,
		variant:     blindrsa.SHA384PSSDeterministic,
	}
	if err := issuer.bootstrap(); err != nil {
		issuer.initErr = err
	}
	return issuer
}

func (k *KryptologyCredentialIssuer) bootstrap() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.storePath == "" {
		material, err := k.generateKeyMaterial(1, time.Now().UTC())
		if err != nil {
			return err
		}
		k.keys[material.version] = material
		k.activeVer = material.version
		return nil
	}

	state, err := k.loadStateFromFile(k.storePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		material, err := k.generateKeyMaterial(1, time.Now().UTC())
		if err != nil {
			return err
		}
		k.keys[material.version] = material
		k.activeVer = material.version
		return k.persistStateLocked()
	}

	if len(state.Keys) == 0 {
		return errors.New("issuer key store has no keys")
	}
	for _, key := range state.Keys {
		material, err := k.materialFromPersisted(key)
		if err != nil {
			return fmt.Errorf("load key version %d: %w", key.Version, err)
		}
		k.keys[key.Version] = material
	}
	k.activeVer = state.ActiveVersion
	if _, ok := k.keys[k.activeVer]; !ok {
		maxVer := 0
		for version := range k.keys {
			if version > maxVer {
				maxVer = version
			}
		}
		k.activeVer = maxVer
	}

	return k.rotateIfNeededLocked(time.Now().UTC())
}

func (k *KryptologyCredentialIssuer) Issue(
	_ context.Context,
	userID string,
	userPubKey []byte,
) (*entity.Credential, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	if len(userPubKey) == 0 {
		return nil, errors.New("user_pub_key is required")
	}
	if k.initErr != nil {
		return nil, k.initErr
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.rotateIfNeededLocked(time.Now().UTC()); err != nil {
		return nil, err
	}

	material, ok := k.keys[k.activeVer]
	if !ok {
		return nil, errors.New("active issuer key is missing")
	}

	message := buildCredentialMessage(userID, userPubKey)
	preparedMsg, err := material.client.Prepare(crand.Reader, message)
	if err != nil {
		return nil, fmt.Errorf("prepare blind message: %w", err)
	}
	blindedMsg, state, err := material.client.Blind(crand.Reader, preparedMsg)
	if err != nil {
		return nil, fmt.Errorf("blind message: %w", err)
	}

	blindSignature, err := material.signer.BlindSign(blindedMsg)
	if err != nil {
		return nil, fmt.Errorf("blind sign message: %w", err)
	}
	signature, err := material.client.Finalize(state, blindSignature)
	if err != nil {
		return nil, fmt.Errorf("finalize blind signature: %w", err)
	}
	if err := material.client.Verify(preparedMsg, signature); err != nil {
		return nil, fmt.Errorf("verify issued credential: %w", err)
	}

	return &entity.Credential{
		ID:               uuid.NewString(),
		UserID:           userID,
		UserPubKey:       append([]byte(nil), userPubKey...),
		Signature:        signature,
		IssuerKeyVersion: material.version,
		IssuedAt:         time.Now().UTC(),
	}, nil
}

func (k *KryptologyCredentialIssuer) VerifyCredential(userID string, userPubKey []byte, signature []byte) error {
	if userID == "" {
		return errors.New("user_id is required")
	}
	if len(userPubKey) == 0 {
		return errors.New("user_pub_key is required")
	}
	if len(signature) == 0 {
		return errors.New("signature is required")
	}
	if k.initErr != nil {
		return k.initErr
	}

	message := buildCredentialMessage(userID, userPubKey)

	k.mu.RLock()
	defer k.mu.RUnlock()
	versions := make([]int, 0, len(k.keys))
	for version := range k.keys {
		versions = append(versions, version)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	var lastErr error
	for _, version := range versions {
		if err := k.keys[version].verifier.Verify(message, signature); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no issuer key available for verification")
	}
	return lastErr
}

func (k *KryptologyCredentialIssuer) rotateIfNeededLocked(now time.Time) error {
	if k.rotateAfter <= 0 {
		return nil
	}

	active, ok := k.keys[k.activeVer]
	if !ok {
		return errors.New("active issuer key is missing")
	}
	if now.Sub(active.activatedAt) < k.rotateAfter {
		return nil
	}

	nextVersion := k.activeVer + 1
	material, err := k.generateKeyMaterial(nextVersion, now)
	if err != nil {
		return fmt.Errorf("rotate issuer key: %w", err)
	}
	k.keys[nextVersion] = material
	k.activeVer = nextVersion

	if k.storePath != "" {
		return k.persistStateLocked()
	}
	return nil
}

func (k *KryptologyCredentialIssuer) generateKeyMaterial(version int, activatedAt time.Time) (*issuerKeyMaterial, error) {
	privateKey, err := rsa.GenerateKey(crand.Reader, k.rsaBits)
	if err != nil {
		return nil, fmt.Errorf("generate rsa keypair: %w", err)
	}
	client, err := blindrsa.NewClient(k.variant, &privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa client: %w", err)
	}
	verifier, err := blindrsa.NewVerifier(k.variant, &privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa verifier: %w", err)
	}

	return &issuerKeyMaterial{
		version:     version,
		publicKey:   &privateKey.PublicKey,
		privateKey:  privateKey,
		client:      client,
		verifier:    verifier,
		signer:      blindrsa.NewSigner(privateKey),
		activatedAt: activatedAt.UTC(),
	}, nil
}

func (k *KryptologyCredentialIssuer) materialFromPersisted(key persistedIssuerKey) (*issuerKeyMaterial, error) {
	privateKey, err := x509.ParsePKCS1PrivateKey(key.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	publicKey, err := x509.ParsePKCS1PublicKey(key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}

	client, err := blindrsa.NewClient(k.variant, publicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa client: %w", err)
	}
	verifier, err := blindrsa.NewVerifier(k.variant, publicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa verifier: %w", err)
	}
	return &issuerKeyMaterial{
		version:     key.Version,
		publicKey:   publicKey,
		privateKey:  privateKey,
		client:      client,
		verifier:    verifier,
		signer:      blindrsa.NewSigner(privateKey),
		activatedAt: key.ActivatedAt.UTC(),
	}, nil
}

func (k *KryptologyCredentialIssuer) loadStateFromFile(path string) (*persistedIssuerState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state persistedIssuerState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode issuer keystore: %w", err)
	}
	return &state, nil
}

func (k *KryptologyCredentialIssuer) persistStateLocked() error {
	state := persistedIssuerState{
		ActiveVersion: k.activeVer,
		Variant:       k.variant.String(),
		Keys:          make([]persistedIssuerKey, 0, len(k.keys)),
	}
	versions := make([]int, 0, len(k.keys))
	for version := range k.keys {
		versions = append(versions, version)
	}
	sort.Ints(versions)

	for _, version := range versions {
		material := k.keys[version]
		publicBytes := x509.MarshalPKCS1PublicKey(material.publicKey)
		privateBytes := x509.MarshalPKCS1PrivateKey(material.privateKey)
		state.Keys = append(state.Keys, persistedIssuerKey{
			Version:     version,
			PublicKey:   publicBytes,
			PrivateKey:  privateBytes,
			ActivatedAt: material.activatedAt,
		})
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode issuer keystore: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(k.storePath), 0o755); err != nil {
		return fmt.Errorf("mkdir keystore dir: %w", err)
	}
	if err := os.WriteFile(k.storePath, data, 0o600); err != nil {
		return fmt.Errorf("write issuer keystore: %w", err)
	}
	return nil
}

func buildCredentialMessage(userID string, userPubKey []byte) []byte {
	sum := sha256.Sum256(append(append([]byte(userID), 0), userPubKey...))
	return sum[:]
}
