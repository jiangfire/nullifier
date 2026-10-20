package crypto

import (
	"crypto/hkdf"
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
	// 密文格式版本。v2 起用 HKDF 派生密钥并在密文里带上 salt，
	// 与 v0.1.0 的密文不兼容：v1 密文会被直接拒绝（版本号不匹配）。
	kyberCipherVersion = byte(2)
	kyberNonceSize     = 12
	kyberTagSize       = 32
	kyberSaltSize      = 32
	kyberKeySize       = 32
)

// HKDF 的 info 前缀：把协议版本与用途写进去，保证同一份共享秘密不会
// 在不同用途下派生出同一把密钥。
const (
	kdfLabelEnc  = "nullifier/kyber-elgamal/v2/encryption-key"
	kdfLabelAuth = "nullifier/kyber-elgamal/v2/authentication-key"
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
	salt := make([]byte, kyberSaltSize)
	if _, err := io.ReadFull(crand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	encKey, authKey, err := deriveKeys(sharedBytes, salt, ephemeralBytes, publicKey)
	if err != nil {
		return nil, err
	}
	cipherPayload := xorKeystream(plaintext, encKey, nonce)

	tag := computeTag(authKey, ephemeralBytes, salt, nonce, cipherPayload)

	out := make([]byte, 0, 1+2+len(ephemeralBytes)+2+kyberSaltSize+kyberNonceSize+4+len(cipherPayload)+kyberTagSize)
	out = append(out, kyberCipherVersion)
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(ephemeralBytes)))
	out = append(out, lenBuf...)
	out = append(out, ephemeralBytes...)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(salt)))
	out = append(out, lenBuf...)
	out = append(out, salt...)
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

	ephemeralBytes, salt, nonce, payload, tag, err := parseCiphertext(ciphertext)
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
	// 收件方公钥由私钥推出，两边算得到同一份上下文。
	recipientPoint := k.suite.Point().Mul(privateScalar, nil)
	recipientBytes, err := recipientPoint.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal recipient point: %w", err)
	}

	encKey, authKey, err := deriveKeys(sharedBytes, salt, ephemeralBytes, recipientBytes)
	if err != nil {
		return nil, err
	}
	if !verifyMAC(authKey, ephemeralBytes, salt, nonce, payload, tag) {
		return nil, errors.New("ciphertext authentication failed")
	}

	return xorKeystream(payload, encKey, nonce), nil
}

func (k *KyberElGamal) ValidateCiphertext(cipher []byte) error {
	ephemeralBytes, _, _, payload, _, err := parseCiphertext(cipher)
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

func parseCiphertext(cipher []byte) (ephemeral []byte, salt []byte, nonce []byte, payload []byte, tag []byte, err error) {
	if len(cipher) < 1+2+2+kyberSaltSize+kyberNonceSize+4+kyberTagSize {
		return nil, nil, nil, nil, nil, errors.New("ciphertext too short")
	}
	if cipher[0] != kyberCipherVersion {
		return nil, nil, nil, nil, nil, errors.New("unsupported ciphertext version")
	}

	offset := 1
	ephemeralLen := int(binary.BigEndian.Uint16(cipher[offset : offset+2]))
	offset += 2
	if ephemeralLen <= 0 || len(cipher) < offset+ephemeralLen {
		return nil, nil, nil, nil, nil, errors.New("invalid ephemeral length")
	}
	ephemeral = append([]byte(nil), cipher[offset:offset+ephemeralLen]...)
	offset += ephemeralLen

	saltLen := int(binary.BigEndian.Uint16(cipher[offset : offset+2]))
	offset += 2
	if saltLen <= 0 || len(cipher) < offset+saltLen {
		return nil, nil, nil, nil, nil, errors.New("invalid salt length")
	}
	salt = append([]byte(nil), cipher[offset:offset+saltLen]...)
	offset += saltLen

	nonce = append([]byte(nil), cipher[offset:offset+kyberNonceSize]...)
	offset += kyberNonceSize

	payloadLen := int(binary.BigEndian.Uint32(cipher[offset : offset+4]))
	offset += 4
	if payloadLen < 0 {
		return nil, nil, nil, nil, nil, errors.New("invalid payload length")
	}
	if len(cipher) != offset+payloadLen+kyberTagSize {
		return nil, nil, nil, nil, nil, errors.New("ciphertext length mismatch")
	}

	payload = append([]byte(nil), cipher[offset:offset+payloadLen]...)
	offset += payloadLen
	tag = append([]byte(nil), cipher[offset:offset+kyberTagSize]...)
	return ephemeral, salt, nonce, payload, tag, nil
}

// deriveKeys 用 HKDF-SHA256（RFC 5869）从共享秘密派生加密密钥与认证密钥。
//
// 相比「裸 SHA256 加个前缀」，HKDF 先做 Extract 再做 Expand，密钥材料
// 的分配是标准构造；salt 每次加密随机生成并随密文传输；info 里绑定了
// 协议版本、用途以及双方公钥（上下文绑定），因此同一份共享秘密不会在
// 不同上下文里派生出同一把密钥。
func deriveKeys(shared, salt, ephemeralPub, recipientPub []byte) (encKey, authKey []byte, err error) {
	encKey, err = hkdf.Key(
		sha256.New,
		shared,
		salt,
		kdfInfo(kdfLabelEnc, ephemeralPub, recipientPub),
		kyberKeySize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("derive encryption key: %w", err)
	}
	authKey, err = hkdf.Key(
		sha256.New,
		shared,
		salt,
		kdfInfo(kdfLabelAuth, ephemeralPub, recipientPub),
		kyberKeySize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("derive authentication key: %w", err)
	}
	return encKey, authKey, nil
}

// kdfInfo 拼出 HKDF 的 info：标签 + 长度前缀的双方公钥。
// 长度前缀避免不同字段的拼接产生歧义。
func kdfInfo(label string, ephemeralPub, recipientPub []byte) string {
	buf := make([]byte, 0, len(label)+len(ephemeralPub)+len(recipientPub)+8)
	buf = append(buf, label...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(ephemeralPub)))
	buf = append(buf, ephemeralPub...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(recipientPub)))
	buf = append(buf, recipientPub...)
	return string(buf)
}

func xorKeystream(data []byte, key []byte, nonce []byte) []byte {
	out := make([]byte, 0, len(data))
	counterBytes := make([]byte, 4)

	for offset := 0; offset < len(data); offset += sha256.Size {
		binary.BigEndian.PutUint32(counterBytes, uint32(offset/sha256.Size))

		blockHash := sha256.New()
		blockHash.Write(key)
		blockHash.Write(nonce)
		blockHash.Write(counterBytes)
		block := blockHash.Sum(nil)

		end := offset + sha256.Size
		if end > len(data) {
			end = len(data)
		}
		for i := offset; i < end; i++ {
			out = append(out, data[i]^block[i-offset])
		}
	}
	return out
}

// computeTag 计算密文认证标签。
// 覆盖临时公钥与 salt：这两项都参与密钥派生，漏掉会让攻击者有机会
// 在不触发认证失败的情况下替换它们。
func computeTag(authKey, ephemeral, salt, nonce, payload []byte) []byte {
	mac := hmac.New(sha256.New, authKey)
	mac.Write(ephemeral)
	mac.Write(salt)
	mac.Write(nonce)
	mac.Write(payload)
	return mac.Sum(nil)
}

func verifyMAC(authKey, ephemeral, salt, nonce, payload, tag []byte) bool {
	expected := computeTag(authKey, ephemeral, salt, nonce, payload)
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
