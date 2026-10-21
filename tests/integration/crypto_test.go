package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// scalarBytes 返回域元素的 32 字节规范编码。
func scalarBytes(element fr.Element) []byte {
	raw := element.Bytes()
	return append([]byte(nil), raw[:]...)
}

// TestCryptoPipeline_Integration 走通整条密码学链路：选票零知识证明的
// 生成与验证、选票密文的加解密与门限计票密钥，以及盲签名签发。
func TestCryptoPipeline_Integration(t *testing.T) {
	// 选票电路密钥：proving key 只在客户端侧用，verifying key 部署到服务端。
	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	dir := t.TempDir()
	pkPath := filepath.Join(dir, "proving.key")
	vkPath := filepath.Join(dir, "verifying.key")
	require.NoError(t, os.WriteFile(pkPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(vkPath, verifyingKey, 0o600))

	prover, err := ballot.NewProver(pkPath)
	require.NoError(t, err)

	// 提案的计票密钥与选票密文
	keys, err := ballot.GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	const optionIndex = 1

	ciphertext, nonce, err := ballot.EncryptBallot(keys.Public, optionIndex)
	require.NoError(t, err)

	// nullifier 由投票人的秘密与提案 ID 推导
	secretBytes, err := ballot.GenerateNullifierSecret()
	require.NoError(t, err)
	var secret fr.Element
	secret.SetBytes(secretBytes)
	nullifier := ballot.DeriveNullifier(secret, ballot.DeriveProposalField(proposalID))

	statement := entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	}

	// 证明生成（客户端侧）
	inputs, err := ballot.BuildPublicInputs(statement)
	require.NoError(t, err)
	proof, err := prover.Prove(inputs, optionIndex, nonce, secret)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	// 证明验证（服务端侧，公开输入由服务端自行拼装）
	verifier, err := cryptoinfra.NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(&entity.Proof{Data: proof}, statement))

	// 换一份密文后，同一份证明必须验证失败
	otherCiphertext, _, err := ballot.EncryptBallot(keys.Public, 2)
	require.NoError(t, err)
	tampered := statement
	tampered.Ciphertext = otherCiphertext
	require.Error(t, verifier.Verify(&entity.Proof{Data: proof}, tampered))

	// 选票密文：门限计票密钥的加解密，用的是证明里的同一把提案计票密钥
	encrypter := cryptoinfra.NewBallotThresholdCrypto()
	ballotCiphertext, err := encrypter.Encrypt(keys.Public, 1)
	require.NoError(t, err)
	require.NoError(t, encrypter.ValidateCiphertext(ballotCiphertext))

	option, err := encrypter.Decrypt(keys.Private, ballotCiphertext, numOptions)
	require.NoError(t, err)
	require.Equal(t, 1, option)

	// 篡改密文任一字节 → 结构校验或解密必须失败
	tamperedCiphertext := append([]byte(nil), ballotCiphertext...)
	tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0x01
	validateErr := encrypter.ValidateCiphertext(tamperedCiphertext)
	_, decryptErr := encrypter.Decrypt(keys.Private, tamperedCiphertext, numOptions)
	require.True(t, validateErr != nil || decryptErr != nil, "tampered ciphertext must be rejected")

	// 版本号不是 0x03 的旧密文 → 校验失败
	legacyCiphertext := append([]byte{0x02}, ballotCiphertext[1:]...)
	require.Error(t, encrypter.ValidateCiphertext(legacyCiphertext))

	// 用别的私钥解密 → 失败
	otherPrivateKey, _, err := encrypter.GenerateKeyPair()
	require.NoError(t, err)
	_, err = encrypter.Decrypt(otherPrivateKey, ballotCiphertext, numOptions)
	require.Error(t, err)

	// nullifier 生成：规范编码的 64 位十六进制
	nullifierValue, err := cryptoinfra.NewBallotNullifierGenerator().GenerateNullifier()
	require.NoError(t, err)
	require.Len(t, nullifierValue, 64)

	// kryptology blind signature issue
	issuer := cryptoinfra.NewKryptologyCredentialIssuer()
	credential, err := issuer.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotEmpty(t, credential.Signature)
}
