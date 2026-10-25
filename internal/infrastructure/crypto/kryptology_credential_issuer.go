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

	"github.com/cloudflare/circl/blindsign/blindrsa"
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

// BlindSign 对**盲化后**的消息做盲签名。
//
// 这是签发方在真两段式盲签名里**唯一**的签名动作：服务端从头到尾没见过
// 被签消息的原文（消息是客户端用公开的签发方公钥盲化后送来的）。
//
// keyVersion 必须是客户端盲化时用的那一版：盲化用的是那一版的模数，解盲要靠
// 同一把私钥做模幂运算，版本对不上解出来的签名验不过。
func (k *KryptologyCredentialIssuer) BlindSign(
	_ context.Context,
	keyVersion int,
	blindedMessage []byte,
) ([]byte, error) {
	if len(blindedMessage) == 0 {
		return nil, errors.New("blinded message is required")
	}
	if k.initErr != nil {
		return nil, k.initErr
	}

	k.mu.RLock()
	defer k.mu.RUnlock()
	material, ok := k.keys[keyVersion]
	if !ok {
		return nil, fmt.Errorf("issuer key version %d is not available", keyVersion)
	}
	blindSignature, err := material.signer.BlindSign(blindedMessage)
	if err != nil {
		return nil, fmt.Errorf("blind sign message: %w", err)
	}
	return blindSignature, nil
}

// IssuerPublicKey 返回当前签发方公钥。
//
// 公钥是公开值 —— 客户端盲化消息本来就需要它。
//
// 这里顺带做密钥轮换：客户端每次预留槽位都要拿公钥，轮换的时机就该跟着它。
func (k *KryptologyCredentialIssuer) IssuerPublicKey(_ context.Context) (*domaincrypto.IssuerPublicKey, error) {
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
	return &domaincrypto.IssuerPublicKey{
		Version:  material.version,
		Modulus:  material.publicKey.N.Bytes(),
		Exponent: material.publicKey.E,
		Variant:  k.variant.String(),
	}, nil
}

func (k *KryptologyCredentialIssuer) VerifyCredential(
	userID string,
	userPubKey []byte,
	signature []byte,
) (int, error) {
	if userID == "" {
		return 0, errors.New("user_id is required")
	}
	if len(userPubKey) == 0 {
		return 0, errors.New("user_pub_key is required")
	}
	if len(signature) == 0 {
		return 0, errors.New("signature is required")
	}
	if k.initErr != nil {
		return 0, k.initErr
	}

	message := CredentialMessage(userID, userPubKey)

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
			return version, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no issuer key available for verification")
	}
	return 0, lastErr
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
	verifier, err := blindrsa.NewVerifier(k.variant, &privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa verifier: %w", err)
	}

	return &issuerKeyMaterial{
		version:     version,
		publicKey:   &privateKey.PublicKey,
		privateKey:  privateKey,
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

	verifier, err := blindrsa.NewVerifier(k.variant, publicKey)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa verifier: %w", err)
	}
	return &issuerKeyMaterial{
		version:     key.Version,
		publicKey:   publicKey,
		privateKey:  privateKey,
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

// CredentialMessage 构造凭证消息：sha256(user_id ‖ 0x00 ‖ user_pub_key)。
//
// 客户端与服务端必须用**同一个**函数算这条消息 —— 客户端拿它去盲化，服务端拿
// 它来验签。对不上就是验签失败。
//
// 分隔符 0x00 用来避免拼接歧义：user_id 与 user_pub_key 的边界必须是确定的。
func CredentialMessage(userID string, userPubKey []byte) []byte {
	sum := sha256.Sum256(append(append([]byte(userID), 0), userPubKey...))
	return sum[:]
}
