package crypto

import (
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
