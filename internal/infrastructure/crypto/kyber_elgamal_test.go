package crypto

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKyberElGamal_EncryptDecrypt(t *testing.T) {
	eg := NewKyberElGamal()

	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, privateKey)
	require.NotEmpty(t, publicKey)

	plaintext := []byte("candidate-A")
	ciphertext, err := eg.Encrypt(publicKey, plaintext)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	decrypted, err := eg.Decrypt(privateKey, ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestKyberElGamal_EncryptDecrypt_LongPayloads(t *testing.T) {
	eg := NewKyberElGamal()

	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	// The keystream block is 32 bytes, so payloads longer than one block must
	// advance the counter instead of indexing past the block.
	for _, size := range []int{1, 31, 32, 33, 64, 65, 200, 4096} {
		plaintext := make([]byte, size)
		for i := range plaintext {
			plaintext[i] = byte('a' + i%26)
		}

		ciphertext, err := eg.Encrypt(publicKey, plaintext)
		require.NoError(t, err, "size %d", size)

		decrypted, err := eg.Decrypt(privateKey, ciphertext)
		require.NoError(t, err, "size %d", size)
		require.Equal(t, plaintext, decrypted, "size %d", size)
	}
}

func TestKyberElGamal_ValidateCiphertext(t *testing.T) {
	eg := NewKyberElGamal()
	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, err := eg.Encrypt(publicKey, []byte("candidate-B"))
	require.NoError(t, err)

	require.NoError(t, eg.ValidateCiphertext(ciphertext))

	ciphertext[len(ciphertext)-1] ^= 0xFF
	_, err = eg.Decrypt(privateKey, ciphertext)
	require.Error(t, err)
}

func TestKyberElGamal_RejectsInvalidCiphertext(t *testing.T) {
	eg := NewKyberElGamal()
	require.Error(t, eg.ValidateCiphertext([]byte("invalid")))
}

// 密文格式升到 v2 之后，v0.1.0 的旧密文必须被明确拒绝，
// 而不是解出一个错误结果。
func TestKyberElGamal_RejectsLegacyCiphertext(t *testing.T) {
	eg := NewKyberElGamal()
	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, err := eg.Encrypt(publicKey, []byte("candidate-A"))
	require.NoError(t, err)

	legacy := append([]byte(nil), ciphertext...)
	legacy[0] = 1

	err = eg.ValidateCiphertext(legacy)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported ciphertext version")

	_, err = eg.Decrypt(privateKey, legacy)
	require.Error(t, err)
}

func TestKyberElGamal_RejectsTamperedSalt(t *testing.T) {
	eg := NewKyberElGamal()
	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, err := eg.Encrypt(publicKey, []byte("candidate-A"))
	require.NoError(t, err)

	// salt 参与密钥派生，改了它就必须解不出来。
	ephemeralLen := int(binary.BigEndian.Uint16(ciphertext[1:3]))
	saltOffset := 1 + 2 + ephemeralLen + 2
	ciphertext[saltOffset] ^= 0xFF

	_, err = eg.Decrypt(privateKey, ciphertext)
	require.Error(t, err)
}

func TestDeriveKeys_BindsSaltAndContext(t *testing.T) {
	shared := []byte("shared-secret")
	salt := bytes.Repeat([]byte{0x01}, kyberSaltSize)
	ephemeral := []byte("ephemeral-public-key")
	recipient := []byte("recipient-public-key")

	encKey, authKey, err := deriveKeys(shared, salt, ephemeral, recipient)
	require.NoError(t, err)
	require.Len(t, encKey, kyberKeySize)
	require.Len(t, authKey, kyberKeySize)
	require.NotEqual(t, encKey, authKey, "加密密钥与认证密钥必须不同")

	otherSalt := bytes.Repeat([]byte{0x02}, kyberSaltSize)
	otherEnc, _, err := deriveKeys(shared, otherSalt, ephemeral, recipient)
	require.NoError(t, err)
	require.NotEqual(t, encKey, otherEnc, "salt 不同必须派生出不同的密钥")

	otherContextEnc, _, err := deriveKeys(shared, salt, ephemeral, []byte("other-recipient"))
	require.NoError(t, err)
	require.NotEqual(t, encKey, otherContextEnc, "上下文不同必须派生出不同的密钥")

	otherEphemeralEnc, _, err := deriveKeys(shared, salt, []byte("other-ephemeral"), recipient)
	require.NoError(t, err)
	require.NotEqual(t, encKey, otherEphemeralEnc, "临时公钥不同必须派生出不同的密钥")
}

func TestKdfInfo_IsUnambiguous(t *testing.T) {
	// 长度前缀：两段字节拼在一起不能与另一种拼法撞出同一个 info。
	a := kdfInfo(kdfLabelEnc, []byte("ab"), []byte("c"))
	b := kdfInfo(kdfLabelEnc, []byte("a"), []byte("bc"))
	require.NotEqual(t, a, b)
}
