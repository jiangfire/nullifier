package ballot

import (
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

func writeTestKeys(t *testing.T) (string, string) {
	t.Helper()
	provingKey, verifyingKey, err := GenerateKeys()
	require.NoError(t, err)
	dir := t.TempDir()
	pkPath := filepath.Join(dir, "proving.key")
	vkPath := filepath.Join(dir, "verifying.key")
	require.NoError(t, os.WriteFile(pkPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(vkPath, verifyingKey, 0o600))
	return pkPath, vkPath
}

func TestBallotEncryptDecryptRoundTrip(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, _, err := EncryptBallot(keys.Public, 2)
	require.NoError(t, err)
	require.NoError(t, ValidateBallotCiphertext(ciphertext))

	index, err := DecryptBallotOption(keys.Private, ciphertext, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(2), index)
}

func TestBallotDecryptRejectsForeignKey(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)
	other, err := GenerateKeyPair()
	require.NoError(t, err)

	ciphertext, _, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	_, err = DecryptBallotOption(other.Private, ciphertext, 3)
	require.Error(t, err)
}

func TestBallotValidateRejectsMalformedCiphertext(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)
	ciphertext, _, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	require.Error(t, ValidateBallotCiphertext(ciphertext[:len(ciphertext)-1]))
	require.Error(t, ValidateBallotCiphertext(append([]byte{2}, ciphertext[1:]...)))
}

func TestBallotProofRoundTrip(t *testing.T) {
	pkPath, vkPath := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)
	verifier, err := NewVerifier(vkPath)
	require.NoError(t, err)

	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	ciphertext, nonce, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	secret, err := randomScalar()
	require.NoError(t, err)
	nullifier := DeriveNullifier(secret, DeriveProposalField(proposalID))

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)

	proof, err := prover.Prove(inputs, 1, nonce, secret)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(proof, inputs))
}

// TestBallotProofRejectsTamperedInputs 覆盖能力 B 的验收口径：
// 篡改任一公开输入，验证都必须失败。
func TestBallotProofRejectsTamperedInputs(t *testing.T) {
	pkPath, vkPath := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)
	verifier, err := NewVerifier(vkPath)
	require.NoError(t, err)

	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	ciphertext, nonce, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	secret, err := randomScalar()
	require.NoError(t, err)
	nullifier := DeriveNullifier(secret, DeriveProposalField(proposalID))

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)

	proof, err := prover.Prove(inputs, 1, nonce, secret)
	require.NoError(t, err)

	t.Run("nullifier", func(t *testing.T) {
		tampered := inputs
		var other fr.Element
		other.SetUint64(42)
		tampered.Nullifier = other
		require.Error(t, verifier.Verify(proof, tampered))
	})

	t.Run("proposal", func(t *testing.T) {
		tampered := inputs
		tampered.ProposalID = DeriveProposalField("proposal-2")
		require.Error(t, verifier.Verify(proof, tampered))
	})

	t.Run("ciphertext", func(t *testing.T) {
		tampered := inputs
		other, _, err := EncryptBallot(keys.Public, 2)
		require.NoError(t, err)
		tamperedInputs, err := BuildPublicInputs(entity.BallotStatement{
			ProposalID:        proposalID,
			Nullifier:         scalarBytes(nullifier),
			ElectionPublicKey: keys.Public,
			Ciphertext:        other,
			NumOptions:        numOptions,
		})
		require.NoError(t, err)
		tampered = tamperedInputs
		require.Error(t, verifier.Verify(proof, tampered))
	})

	t.Run("option count", func(t *testing.T) {
		tampered := inputs
		var count fr.Element
		count.SetUint64(numOptions + 1)
		tampered.NumOptions = count
		require.Error(t, verifier.Verify(proof, tampered))
	})

	t.Run("election key", func(t *testing.T) {
		other, err := GenerateKeyPair()
		require.NoError(t, err)
		tampered := inputs
		tampered.ElectionKey = mustParsePoint(t, other.Public)
		require.Error(t, verifier.Verify(proof, tampered))
	})
}

// TestBallotProofRejectsOptionOutOfRange 覆盖能力 A 与 B 的边界：
// 选项下标必须落在提案的选项集合内。
func TestBallotProofRejectsOptionOutOfRange(t *testing.T) {
	pkPath, _ := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)

	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	ciphertext, nonce, err := EncryptBallot(keys.Public, 5)
	require.NoError(t, err)

	secret, err := randomScalar()
	require.NoError(t, err)
	nullifier := DeriveNullifier(secret, DeriveProposalField(proposalID))

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)

	_, err = prover.Prove(inputs, 5, nonce, secret)
	require.Error(t, err)
}

// TestBallotProofRejectsForeignNullifierSecret 证明 nullifier 由秘密推导：
// 拿别人的秘密配自己的 nullifier，电路解不出来。
func TestBallotProofRejectsForeignNullifierSecret(t *testing.T) {
	pkPath, _ := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)

	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	const proposalID = "proposal-1"
	const numOptions = 3
	ciphertext, nonce, err := EncryptBallot(keys.Public, 1)
	require.NoError(t, err)

	secret, err := randomScalar()
	require.NoError(t, err)
	nullifier := DeriveNullifier(secret, DeriveProposalField(proposalID))

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         scalarBytes(nullifier),
		ElectionPublicKey: keys.Public,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)

	other, err := randomScalar()
	require.NoError(t, err)
	_, err = prover.Prove(inputs, 1, nonce, other)
	require.Error(t, err)
}

func TestNullifierBindsProposalAndSecret(t *testing.T) {
	secret, err := randomScalar()
	require.NoError(t, err)

	base := DeriveNullifier(secret, DeriveProposalField("proposal-1"))
	require.Equal(t, base, DeriveNullifier(secret, DeriveProposalField("proposal-1")))
	require.NotEqual(t, base, DeriveNullifier(secret, DeriveProposalField("proposal-2")))

	other, err := randomScalar()
	require.NoError(t, err)
	require.NotEqual(t, base, DeriveNullifier(other, DeriveProposalField("proposal-1")))
}

func TestSecretSplitAndRecover(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	shares, err := SplitSecret(keys.Private, 2, 3)
	require.NoError(t, err)
	require.Len(t, shares, 3)

	recovered, err := RecoverSecret(shares[:2], 2)
	require.NoError(t, err)
	require.Equal(t, keys.Private, recovered)

	_, err = RecoverSecret(shares[:1], 2)
	require.Error(t, err)
}

func TestScalarHexRoundTrip(t *testing.T) {
	scalar, err := randomScalar()
	require.NoError(t, err)

	encoded := EncodeScalarHex(scalar)
	parsed, err := ParseScalarHex(encoded)
	require.NoError(t, err)
	require.True(t, scalar.Equal(&parsed))

	_, err = ParseScalarHex("zz")
	require.Error(t, err)
	_, err = ParseScalarHex("00")
	require.Error(t, err)
}

func mustParsePoint(t *testing.T, raw []byte) Point {
	t.Helper()
	point, err := parsePoint(raw)
	require.NoError(t, err)
	return point
}
