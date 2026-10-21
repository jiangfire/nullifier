package crypto

import (
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// ballotFixture 是一次真实投票用到的全部材料：计票密钥、选票密文、
// nullifier、零知识证明与对应的公开输入声明。
type ballotFixture struct {
	// vkPath 是选票电路的 verifying key 文件路径。
	vkPath string
	// keys 是提案的计票密钥对。
	keys *ballot.KeyPair
	// statement 是服务端自行拼装的公开输入声明。
	statement entity.BallotStatement
	// proof 是与 statement 一致的零知识证明。
	proof []byte
}

// setupBallotFixture 走完整条真实链路：生成电路密钥 → 加密选票 →
// 推导 nullifier → 生成证明。
func setupBallotFixture(t *testing.T) ballotFixture {
	t.Helper()

	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	dir := t.TempDir()
	pkPath := filepath.Join(dir, "proving.key")
	vkPath := filepath.Join(dir, "verifying.key")
	require.NoError(t, os.WriteFile(pkPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(vkPath, verifyingKey, 0o600))

	prover, err := ballot.NewProver(pkPath)
	require.NoError(t, err)

	keys, err := ballot.GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	const optionIndex = 1

	ciphertext, nonce, err := ballot.EncryptBallot(keys.Public, optionIndex)
	require.NoError(t, err)

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

	inputs, err := ballot.BuildPublicInputs(statement)
	require.NoError(t, err)

	proof, err := prover.Prove(inputs, optionIndex, nonce, secret)
	require.NoError(t, err)

	return ballotFixture{
		vkPath:    vkPath,
		keys:      keys,
		statement: statement,
		proof:     proof,
	}
}

// scalarBytes 返回域元素的 32 字节规范编码。
func scalarBytes(element fr.Element) []byte {
	raw := element.Bytes()
	return append([]byte(nil), raw[:]...)
}

// TestGnarkProofVerifier_Verify_RealBallotProof 覆盖真实证明链路：
// 生成的证明必须能通过服务端按请求材料自行拼装的公开输入验证。
func TestGnarkProofVerifier_Verify_RealBallotProof(t *testing.T) {
	fixture := setupBallotFixture(t)

	verifier, err := NewGnarkProofVerifier(fixture.vkPath)
	require.NoError(t, err)

	require.NoError(t, verifier.Verify(&entity.Proof{Data: fixture.proof}, fixture.statement))
}

// TestGnarkProofVerifier_Verify_RejectsTamperedPublicInputs 覆盖能力 B：
// 篡改任一公开输入都必须验证失败 —— 证明只能用于它被生成的那份密文。
func TestGnarkProofVerifier_Verify_RejectsTamperedPublicInputs(t *testing.T) {
	fixture := setupBallotFixture(t)

	verifier, err := NewGnarkProofVerifier(fixture.vkPath)
	require.NoError(t, err)

	t.Run("nullifier", func(t *testing.T) {
		tampered := fixture.statement
		var other fr.Element
		other.SetUint64(42)
		tampered.Nullifier = scalarBytes(other)

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "invalid ballot proof")
	})

	t.Run("ciphertext", func(t *testing.T) {
		tampered := fixture.statement
		other, _, err := ballot.EncryptBallot(fixture.keys.Public, 2)
		require.NoError(t, err)
		tampered.Ciphertext = other

		err = verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "invalid ballot proof")
	})

	t.Run("proposal", func(t *testing.T) {
		tampered := fixture.statement
		tampered.ProposalID = "proposal-2"

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "invalid ballot proof")
	})

	t.Run("option count", func(t *testing.T) {
		tampered := fixture.statement
		tampered.NumOptions = 4

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "invalid ballot proof")
	})

	t.Run("election key", func(t *testing.T) {
		tampered := fixture.statement
		other, err := ballot.GenerateKeyPair()
		require.NoError(t, err)
		tampered.ElectionPublicKey = other.Public

		err = verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "invalid ballot proof")
	})
}

// TestGnarkProofVerifier_Verify_RejectsMalformedInput 覆盖输入的形状校验：
// 公开输入不合法时必须在拼装阶段就被拒掉，而不是拿去验证。
func TestGnarkProofVerifier_Verify_RejectsMalformedInput(t *testing.T) {
	fixture := setupBallotFixture(t)

	verifier, err := NewGnarkProofVerifier(fixture.vkPath)
	require.NoError(t, err)

	t.Run("empty proof", func(t *testing.T) {
		err := verifier.Verify(&entity.Proof{Data: nil}, fixture.statement)
		require.ErrorContains(t, err, "proof is empty")
	})

	t.Run("nullifier length", func(t *testing.T) {
		tampered := fixture.statement
		tampered.Nullifier = tampered.Nullifier[:16]

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "build public inputs")
	})

	t.Run("election public key", func(t *testing.T) {
		tampered := fixture.statement
		tampered.ElectionPublicKey = []byte("not-a-compressed-point")

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "build public inputs")
	})

	t.Run("ciphertext version", func(t *testing.T) {
		tampered := fixture.statement
		tampered.Ciphertext = append([]byte{0x02}, fixture.statement.Ciphertext[1:]...)

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "build public inputs")
	})

	t.Run("option count", func(t *testing.T) {
		tampered := fixture.statement
		tampered.NumOptions = 0

		err := verifier.Verify(&entity.Proof{Data: fixture.proof}, tampered)
		require.ErrorContains(t, err, "build public inputs")
	})
}

// TestGnarkProofVerifier_NewWithEmptyPath_AllowsDevelopmentMode 保留开发
// 模式的既定行为：没有 verifying key 时只要求证明非空。
func TestGnarkProofVerifier_NewWithEmptyPath_AllowsDevelopmentMode(t *testing.T) {
	verifier, err := NewGnarkProofVerifier("")
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: []byte("invalid-binary-proof")}, entity.BallotStatement{})
	require.NoError(t, err)

	err = verifier.Verify(&entity.Proof{Data: nil}, entity.BallotStatement{})
	require.ErrorContains(t, err, "proof is empty")
}
