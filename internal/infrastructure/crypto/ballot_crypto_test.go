package crypto

import (
	"encoding/hex"
	"testing"

	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/stretchr/testify/require"
)

func TestBallotNullifierGenerator_GenerateNullifier(t *testing.T) {
	generator := NewBallotNullifierGenerator()

	nullifier, err := generator.GenerateNullifier()
	require.NoError(t, err)
	// 规范编码：32 字节大端 hex，固定 64 位。
	require.Len(t, nullifier, 64)
	raw, err := hex.DecodeString(nullifier)
	require.NoError(t, err)
	require.Len(t, raw, 32)

	// 必须是标量域内的规范元素（能被解析回来，且字节表示不变）。
	scalar, err := ballot.ParseScalarHex(nullifier)
	require.NoError(t, err)
	require.Equal(t, nullifier, ballot.EncodeScalarHex(scalar))

	// 两次生成不应相同。
	other, err := generator.GenerateNullifier()
	require.NoError(t, err)
	require.NotEqual(t, nullifier, other)
}

func TestBallotThresholdCrypto_KeyPairAndThresholdRecover(t *testing.T) {
	tc := NewBallotThresholdCrypto()

	privateKey, publicKey, err := tc.GenerateKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, privateKey)
	require.NotEmpty(t, publicKey)

	// 3 份份额、门限 2：任取 2 份都能还原。
	shares, err := tc.SplitPrivateKey(privateKey, 2, 3)
	require.NoError(t, err)
	require.Len(t, shares, 3)

	recovered, err := tc.RecoverPrivateKey(shares[:2], 2, 3)
	require.NoError(t, err)
	require.Equal(t, privateKey, recovered)

	recovered, err = tc.RecoverPrivateKey(shares[1:], 2, 3)
	require.NoError(t, err)
	require.Equal(t, privateKey, recovered)
}

func TestBallotThresholdCrypto_EncryptDecrypt(t *testing.T) {
	tc := NewBallotThresholdCrypto()

	privateKey, publicKey, err := tc.GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, err := tc.Encrypt(publicKey, 2)
	require.NoError(t, err)
	require.NoError(t, tc.ValidateCiphertext(ciphertext))

	index, err := tc.Decrypt(privateKey, ciphertext, 3)
	require.NoError(t, err)
	require.Equal(t, 2, index)

	// 密文不是合法结构时拒绝。
	require.Error(t, tc.ValidateCiphertext([]byte("not-a-ciphertext")))
	require.Error(t, tc.ValidateCiphertext(nil))
}

func TestBallotThresholdCrypto_EncryptRejectsNegativeIndex(t *testing.T) {
	tc := NewBallotThresholdCrypto()

	_, publicKey, err := tc.GenerateKeyPair()
	require.NoError(t, err)

	_, err = tc.Encrypt(publicKey, -1)
	require.Error(t, err)
	require.ErrorContains(t, err, "must not be negative")
}
