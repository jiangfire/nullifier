package crypto

import (
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	domaincrypto "anonvote/internal/domain/crypto"

	"go.dedis.ch/kyber/v3/group/edwards25519"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/util/random"
)

const (
	kyberCipherVersion = byte(1)
	kyberNonceSize     = 12
	kyberTagSize       = 32
)

// KyberElGamal 使用 kyber 提供的曲线能力实现简化 ElGamal 封装（含完整性标签）。
type KyberElGamal struct {
	suite *edwards25519.SuiteEd25519
}

var _ domaincrypto.Encrypter = (*KyberElGamal)(nil)

func NewKyberElGamal() *KyberElGamal {
	return &KyberElGamal{
		suite: edwards25519.NewBlakeSHA256Ed25519(),
	}
}

func (k *KyberElGamal) GenerateKeyPair() ([]byte, []byte, error) {
	private := k.suite.Scalar().Pick(random.New())
	public := k.suite.Point().Mul(private, nil)

	privateBytes, err := private.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	publicBytes, err := public.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}

	return privateBytes, publicBytes, nil
}

func (k *KyberElGamal) SplitPrivateKey(privateKey []byte, threshold, trustees int) ([][]byte, error) {
	if len(privateKey) == 0 {
		return nil, errors.New("private key is empty")
	}
	if threshold <= 0 {
		return nil, errors.New("threshold must be positive")
	}
	if trustees < threshold {
		return nil, errors.New("trustees must be >= threshold")
	}

	secret := k.suite.Scalar()
	if err := secret.UnmarshalBinary(privateKey); err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}

	poly := share.NewPriPoly(k.suite, threshold, secret, random.New())
	rawShares := poly.Shares(trustees)
	serialized := make([][]byte, 0, len(rawShares))
	for _, priShare := range rawShares {
		data, err := encodePriShare(priShare)
		if err != nil {
			return nil, err
		}
		serialized = append(serialized, data)
	}

	return serialized, nil
}

func (k *KyberElGamal) RecoverPrivateKey(sharesData [][]byte, threshold, trustees int) ([]byte, error) {
	if threshold <= 0 {
		return nil, errors.New("threshold must be positive")
	}
	if trustees < threshold {
		return nil, errors.New("trustees must be >= threshold")
	}
	if len(sharesData) < threshold {
		return nil, errors.New("insufficient shares")
	}

	priShares := make([]*share.PriShare, 0, len(sharesData))
	for _, data := range sharesData {
		priShare, err := decodePriShare(k.suite, data)
		if err != nil {
			return nil, err
		}
		priShares = append(priShares, priShare)
	}

	secret, err := share.RecoverSecret(k.suite, priShares, threshold, trustees)
	if err != nil {
		return nil, fmt.Errorf("recover private key: %w", err)
	}
	secretBytes, err := secret.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal recovered private key: %w", err)
	}
	return secretBytes, nil
}

func (k *KyberElGamal) Encrypt(publicKey []byte, plaintext []byte) ([]byte, error) {
	if len(publicKey) == 0 {
		return nil, errors.New("public key is empty")
	}
	if len(plaintext) == 0 {
		return nil, errors.New("plaintext is empty")
	}

	publicPoint := k.suite.Point()
	if err := publicPoint.UnmarshalBinary(publicKey); err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}

	ephemeralSecret := k.suite.Scalar().Pick(random.New())
	ephemeralPublic := k.suite.Point().Mul(ephemeralSecret, nil)
	sharedPoint := k.suite.Point().Mul(ephemeralSecret, publicPoint)

	ephemeralBytes, err := ephemeralPublic.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal ephemeral point: %w", err)
	}
	sharedBytes, err := sharedPoint.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal shared point: %w", err)
	}

	nonce := make([]byte, kyberNonceSize)
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	encKey, authKey := deriveKeys(sharedBytes)
	cipherPayload := xorKeystream(plaintext, encKey, nonce)

	mac := hmac.New(sha256.New, authKey)
	mac.Write(nonce)
	mac.Write(cipherPayload)
	tag := mac.Sum(nil)

	out := make([]byte, 0, 1+2+len(ephemeralBytes)+kyberNonceSize+4+len(cipherPayload)+kyberTagSize)
	out = append(out, kyberCipherVersion)
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(ephemeralBytes)))
	out = append(out, lenBuf...)
	out = append(out, ephemeralBytes...)
	out = append(out, nonce...)
	payloadLen := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadLen, uint32(len(cipherPayload)))
	out = append(out, payloadLen...)
	out = append(out, cipherPayload...)
	out = append(out, tag...)
	return out, nil
}

func (k *KyberElGamal) Decrypt(privateKey []byte, ciphertext []byte) ([]byte, error) {
	if len(privateKey) == 0 {
		return nil, errors.New("private key is empty")
	}
	if err := k.ValidateCiphertext(ciphertext); err != nil {
		return nil, err
	}

	privateScalar := k.suite.Scalar()
	if err := privateScalar.UnmarshalBinary(privateKey); err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}

	ephemeralBytes, nonce, payload, tag, err := parseCiphertext(ciphertext)
	if err != nil {
		return nil, err
	}

	ephemeralPoint := k.suite.Point()
	if err := ephemeralPoint.UnmarshalBinary(ephemeralBytes); err != nil {
		return nil, fmt.Errorf("decode ephemeral point: %w", err)
	}

	sharedPoint := k.suite.Point().Mul(privateScalar, ephemeralPoint)
	sharedBytes, err := sharedPoint.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal shared point: %w", err)
	}

	encKey, authKey := deriveKeys(sharedBytes)
	if !verifyMAC(authKey, nonce, payload, tag) {
		return nil, errors.New("ciphertext authentication failed")
	}

	return xorKeystream(payload, encKey, nonce), nil
}

func (k *KyberElGamal) ValidateCiphertext(cipher []byte) error {
	ephemeralBytes, _, payload, _, err := parseCiphertext(cipher)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return errors.New("ciphertext payload is empty")
	}

	ephemeralPoint := k.suite.Point()
	if err := ephemeralPoint.UnmarshalBinary(ephemeralBytes); err != nil {
		return fmt.Errorf("decode ephemeral point: %w", err)
	}
	return nil
}

func parseCiphertext(cipher []byte) (ephemeral []byte, nonce []byte, payload []byte, tag []byte, err error) {
	if len(cipher) < 1+2+kyberNonceSize+4+kyberTagSize {
		return nil, nil, nil, nil, errors.New("ciphertext too short")
	}
	if cipher[0] != kyberCipherVersion {
		return nil, nil, nil, nil, errors.New("unsupported ciphertext version")
	}

	ephemeralLen := int(binary.BigEndian.Uint16(cipher[1:3]))
	minLen := 1 + 2 + ephemeralLen + kyberNonceSize + 4 + kyberTagSize
	if ephemeralLen <= 0 || len(cipher) < minLen {
		return nil, nil, nil, nil, errors.New("invalid ephemeral length")
	}

	offset := 3
	ephemeral = append([]byte(nil), cipher[offset:offset+ephemeralLen]...)
	offset += ephemeralLen

	nonce = append([]byte(nil), cipher[offset:offset+kyberNonceSize]...)
	offset += kyberNonceSize

	payloadLen := int(binary.BigEndian.Uint32(cipher[offset : offset+4]))
	offset += 4
	if payloadLen < 0 {
		return nil, nil, nil, nil, errors.New("invalid payload length")
	}
	if len(cipher) != offset+payloadLen+kyberTagSize {
		return nil, nil, nil, nil, errors.New("ciphertext length mismatch")
	}

	payload = append([]byte(nil), cipher[offset:offset+payloadLen]...)
	offset += payloadLen
	tag = append([]byte(nil), cipher[offset:offset+kyberTagSize]...)
	return ephemeral, nonce, payload, tag, nil
}

func deriveKeys(shared []byte) (encKey []byte, authKey []byte) {
	hash1 := sha256.Sum256(append([]byte("enc:"), shared...))
	hash2 := sha256.Sum256(append([]byte("auth:"), shared...))
	return hash1[:], hash2[:]
}

func xorKeystream(data []byte, key []byte, nonce []byte) []byte {
	out := make([]byte, len(data))
	var counter uint32
	used := 0

	for used < len(data) {
		counterBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(counterBytes, counter)

		blockHash := sha256.New()
		blockHash.Write(key)
		blockHash.Write(nonce)
		blockHash.Write(counterBytes)
		block := blockHash.Sum(nil)

		n := copy(out[used:], data[used:])
		for i := 0; i < n; i++ {
			out[used+i] ^= block[i]
		}
		used += n
		counter++
	}
	return out
}

func verifyMAC(authKey, nonce, payload, tag []byte) bool {
	mac := hmac.New(sha256.New, authKey)
	mac.Write(nonce)
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, tag)
}

func encodePriShare(priShare *share.PriShare) ([]byte, error) {
	if priShare == nil || priShare.V == nil {
		return nil, errors.New("private share is nil")
	}
	shareBytes, err := priShare.V.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal private share: %w", err)
	}
	if len(shareBytes) > 0xFFFF {
		return nil, errors.New("private share is too large")
	}

	out := make([]byte, 0, 4+2+len(shareBytes))
	idx := make([]byte, 4)
	binary.BigEndian.PutUint32(idx, uint32(priShare.I))
	out = append(out, idx...)
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(shareBytes)))
	out = append(out, size...)
	out = append(out, shareBytes...)
	return out, nil
}

func decodePriShare(suite *edwards25519.SuiteEd25519, data []byte) (*share.PriShare, error) {
	if len(data) < 6 {
		return nil, errors.New("private share is too short")
	}
	idx := int(binary.BigEndian.Uint32(data[:4]))
	size := int(binary.BigEndian.Uint16(data[4:6]))
	if size <= 0 || len(data) != 6+size {
		return nil, errors.New("private share length mismatch")
	}

	scalar := suite.Scalar()
	if err := scalar.UnmarshalBinary(data[6:]); err != nil {
		return nil, fmt.Errorf("decode private share: %w", err)
	}
	return &share.PriShare{
		I: idx,
		V: scalar,
	}, nil
}
