package eligibility

import (
	"crypto/rand"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// buildTestListAtDepth 造一份指定树深的测试名单（buildTestList 的深度参数版）。
func buildTestListAtDepth(t *testing.T, memberIDs []string, depth int) (root string, packages []MemberPackage) {
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

	rootElement, paths, err := BuildTree(leaves, depth)
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
			Depth:    depth,
		}
	}
	return root, packages
}

// TestDefaultTreeDepthIsTheLegacyShape 把默认深度钉在 10。
//
// 这是跨语言锁的 Go 半边：TS 侧 `web/src/lib/eligibility-prover.ts` 的缺省
// 深度由它自己的测试钉住。两边各拿自己的常量自证测不出单边改动 —— 但任何
// 一边变红都会把人引到另一边。v0.2.0 的存量名单与密钥全是深度 10，这个值
// 不能在没人注意到的情况下变。
func TestDefaultTreeDepthIsTheLegacyShape(t *testing.T) {
	require.Equal(t, 10, DefaultTreeDepth)
}

func TestValidateTreeDepthBounds(t *testing.T) {
	require.NoError(t, ValidateTreeDepth(MinTreeDepth))
	require.NoError(t, ValidateTreeDepth(MaxTreeDepth))
	require.NoError(t, ValidateTreeDepth(DefaultTreeDepth))
	require.Error(t, ValidateTreeDepth(MinTreeDepth-1), "低于下限必须被拒")
	require.Error(t, ValidateTreeDepth(MaxTreeDepth+1), "高于上限必须被拒")
	require.Error(t, ValidateTreeDepth(0))
	require.Error(t, ValidateTreeDepth(-1))
}

// TestCircuitRoundTripAtNonDefaultDepth 证明深度真的是运行期参数：
// 用下限深度（4）生成密钥、建名单、出证明、验签 —— 全链路走通。
//
// 测试刻意用最小深度：密钥生成的开销随约束数（随深度）增长，下限深度
// 几百毫秒就能跑完，而它对「深度参数化」的证明力与深度 24 相同。
func TestCircuitRoundTripAtNonDefaultDepth(t *testing.T) {
	root, packages := buildTestListAtDepth(t, []string{"m-1", "m-2", "m-3"}, MinTreeDepth)

	provingKey, verifyingKey, err := GenerateKeys(MinTreeDepth)
	require.NoError(t, err)
	prover, err := NewProverFromBytesAtDepth(provingKey, MinTreeDepth)
	require.NoError(t, err)
	require.Equal(t, MinTreeDepth, prover.Depth())
	verifier, err := NewVerifierFromBytes(verifyingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
	result, err := prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[2],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          root,
	})
	require.NoError(t, err)

	statement, err := BuildStatement(root, "proposal-1", challenge, result.QualificationID)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(result.Proof, statement))

	// 篡改公开输入：非默认深度的证明同样绑定输入。
	other, err := BuildStatement(root, "proposal-2", challenge, result.QualificationID)
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, other))
}

// TestProverRejectsDepthMismatch 钉住「密钥、名单、深度成套」：
// 深度 4 的成员包放到默认深度（10）的证明器上必须当场被拒 ——
// 宁可在这里失败，也不要出一份注定验不过的证明。
func TestProverRejectsDepthMismatch(t *testing.T) {
	_, packages := buildTestListAtDepth(t, []string{"m-1", "m-2"}, MinTreeDepth)

	provingKey, _, err := GenerateKeys(DefaultTreeDepth)
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)

	const challenge = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
	_, err = prover.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: packages[0],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          packages[0].Root,
	})
	require.Error(t, err, "深度不匹配的成员包必须被拒")

	// 反方向同样：默认深度的包放到深度 4 的证明器上。
	provingKeySmall, _, err := GenerateKeys(MinTreeDepth)
	require.NoError(t, err)
	proverSmall, err := NewProverFromBytesAtDepth(provingKeySmall, MinTreeDepth)
	require.NoError(t, err)
	_, defaultPackages := buildTestList(t, []string{"m-1", "m-2"})
	_, err = proverSmall.ProveEligibility(ClientEligibilityRequest{
		MemberPackage: defaultPackages[0],
		ProposalID:    "proposal-1",
		Challenge:     challenge,
		Root:          defaultPackages[0].Root,
	})
	require.Error(t, err, "深度不匹配的成员包必须被拒")
}

// TestProverRejectsOutOfRangeDepth 编译入口要拒绝范围外的深度。
func TestProverRejectsOutOfRangeDepth(t *testing.T) {
	provingKey, _, err := GenerateKeys(MinTreeDepth)
	require.NoError(t, err)
	_, err = NewProverFromBytesAtDepth(provingKey, 0)
	require.Error(t, err)
	_, err = NewProverFromBytesAtDepth(provingKey, MaxTreeDepth+1)
	require.Error(t, err)
}

// TestConstraintsGrowWithDepth 约束数随深度增长 —— 这是「深度真的进了
// 电路」的间接证据（上溯层数与位分解宽度都由 Path 长度决定）。
func TestConstraintsGrowWithDepth(t *testing.T) {
	shallow, err := Constraints(MinTreeDepth)
	require.NoError(t, err)
	deeper, err := Constraints(MinTreeDepth + 1)
	require.NoError(t, err)
	require.Greater(t, deeper, shallow, "每加一层 Merkle 上溯约束必须变多")
}
