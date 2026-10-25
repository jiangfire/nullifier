package eligibility

import (
	"crypto/rand"
	"testing"

	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// buildTestList 造一份测试名单：给每个成员一份秘密与成员包。
//
// 它模拟离线名单工具的行为：成员按 ID 排序 → 叶子 → Merkle 树 → 路径。
func buildTestList(t *testing.T, memberIDs []string) (root string, packages []MemberPackage) {
	t.Helper()

	secrets := make([][]byte, len(memberIDs))
	leaves := make([]fr.Element, len(memberIDs))
	for i := range memberIDs {
		secret := make([]byte, FieldSize)
		_, err := rand.Read(secret)
		require.NoError(t, err)
		canonical, err := CanonicalSecret(secret)
		require.NoError(t, err)
		secrets[i] = canonical

		leaf, err := DeriveLeaf(canonical)
		require.NoError(t, err)
		leaves[i] = leaf
	}

	rootElement, paths, err := BuildTree(leaves, TreeDepth)
	require.NoError(t, err)
	root = EncodeField(rootElement)

	packages = make([]MemberPackage, len(memberIDs))
	for i, memberID := range memberIDs {
		path := make([]string, len(paths[i]))
		for j := range paths[i] {
			path[j] = EncodeField(paths[i][j])
		}
		packages[i] = MemberPackage{
			MemberID: memberID,
			Secret:   hexString(secrets[i]),
			Leaf:     EncodeField(leaves[i]),
			Index:    uint64(i),
			Path:     path,
			Root:     root,
			Depth:    TreeDepth,
		}
	}
	return root, packages
}

func hexString(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}

func TestConstraintsAreReasonable(t *testing.T) {
	count, err := Constraints()
	require.NoError(t, err)
	// Merkle 上溯 TreeDepth 次 + 两次哈希，每个 MiMC 约 550 条约束。
	// 这个上限是「电路没有意外膨胀」的护栏，不是性能承诺。
	require.Less(t, count, 12000, "资格电路的约束数超出预期")
	require.Greater(t, count, 1000)
	t.Logf("eligibility circuit constraints: %d", count)
}

func TestProverVerifierRoundTrip(t *testing.T) {
	root, packages := buildTestList(t, []string{"member-1", "member-2", "member-3"})

	provingKey, verifyingKey, err := GenerateKeys()
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)
	verifier, err := NewVerifierFromBytes(verifyingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
	result, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[1],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Proof)
	require.NotEmpty(t, result.QualificationID)

	statement, err := BuildStatement(root, "proposal-1", challenge, result.QualificationID)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(result.Proof, statement))
}

func TestQualificationIDIsStablePerMemberAndProposal(t *testing.T) {
	root, packages := buildTestList(t, []string{"member-1", "member-2"})

	provingKey, _, err := GenerateKeys()
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
	first, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[0],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)
	second, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[0],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)
	require.Equal(t, first.QualificationID, second.QualificationID,
		"同一成员在同一提案里的去重键必须稳定 —— 否则唯一索引拦不住重复领取")

	other, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[0],
		ProposalID:    "proposal-2",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)
	require.NotEqual(t, first.QualificationID, other.QualificationID,
		"跨提案的去重键必须不同 —— 否则跨提案可关联")

	member2, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[1],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)
	require.NotEqual(t, first.QualificationID, member2.QualificationID,
		"不同成员的去重键必须不同")
}

func TestTamperedProofIsRejected(t *testing.T) {
	root, packages := buildTestList(t, []string{"member-1", "member-2"})

	provingKey, verifyingKey, err := GenerateKeys()
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)
	verifier, err := NewVerifierFromBytes(verifyingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
	result, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[0],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)

	statement, err := BuildStatement(root, "proposal-1", challenge, result.QualificationID)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(result.Proof, statement))

	// 换一个提案：公开输入变了，证明必须失效（防跨提案复用）。
	other, err := BuildStatement(root, "proposal-2", challenge, result.QualificationID)
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, other))

	// 换一个挑战：同理。
	otherChallenge, err := BuildStatement(root, "proposal-1",
		"00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		result.QualificationID)
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, otherChallenge))

	// 换一个去重键：同理。
	otherQualification, err := BuildStatement(root, "proposal-1", challenge,
		"00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, otherQualification))

	// 换一个 root：同理（证明不是这份名单的）。
	otherRoot, err := BuildStatement(
		"00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		"proposal-1", challenge, result.QualificationID)
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, otherRoot))
}

func TestMemberPackageTamperingIsRejected(t *testing.T) {
	root, packages := buildTestList(t, []string{"member-1", "member-2"})

	provingKey, _, err := GenerateKeys()
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"

	tampered := packages[0]
	tampered.Leaf = packages[1].Leaf
	_, err = prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: tampered,
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.Error(t, err, "叶子与秘密对不上的成员包必须被拒")

	foreign := packages[0]
	foreign.Root = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	_, err = prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: foreign,
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.Error(t, err, "属于另一份名单的成员包必须被拒")
}

func TestDeriveLeafMatchesSecret(t *testing.T) {
	secret := make([]byte, FieldSize)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	canonical, err := CanonicalSecret(secret)
	require.NoError(t, err)

	leaf, err := DeriveLeaf(canonical)
	require.NoError(t, err)

	// 叶子不等于秘密本身（否则名单会直接暴露成员标识）。
	secretField, err := SecretToField(canonical)
	require.NoError(t, err)
	require.False(t, leaf.Equal(&secretField))
}

func TestStatementUsesTheSameProposalDerivationAsBallot(t *testing.T) {
	// 提案域元素必须与选票侧用同一套推导 —— 否则同一个提案在两条链上
	// 会算出两个值。
	statement, err := BuildStatement(
		"00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		"proposal-1",
		"1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100",
		"00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	)
	require.NoError(t, err)
	require.True(t, statement.ProposalID.Equal(&[]fr.Element{ballot.DeriveProposalField("proposal-1")}[0]))
}
