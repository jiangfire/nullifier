package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKyberElGamal_ThresholdRecoverDecrypt(t *testing.T) {
	eg := NewKyberElGamal()
	privateKey, publicKey, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	shares, err := eg.SplitPrivateKey(privateKey, 2, 3)
	require.NoError(t, err)
	require.Len(t, shares, 3)

	recovered, err := eg.RecoverPrivateKey(shares[:2], 2, 3)
	require.NoError(t, err)

	ciphertext, err := eg.Encrypt(publicKey, []byte("candidate-A"))
	require.NoError(t, err)

	plaintext, err := eg.Decrypt(recovered, ciphertext)
	require.NoError(t, err)
	require.Equal(t, []byte("candidate-A"), plaintext)
}

func TestKyberElGamal_ThresholdRecoverRejectsInsufficientShares(t *testing.T) {
	eg := NewKyberElGamal()
	privateKey, _, err := eg.GenerateKeyPair()
	require.NoError(t, err)

	shares, err := eg.SplitPrivateKey(privateKey, 2, 3)
	require.NoError(t, err)

	_, err = eg.RecoverPrivateKey(shares[:1], 2, 3)
	require.Error(t, err)
}
