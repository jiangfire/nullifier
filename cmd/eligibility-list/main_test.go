package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// TestGenerateListProducesUsableArtifacts 覆盖名单工具的产物契约。
//
// 这条链路的另一半在服务端：运营方跑这个工具生成名单 → 把 root.json 配到
// 服务端 → 把 eligibility-<成员>.json 发给本人 → 本人用它领票。
// 中间任何一步的字段编码对不上，**要等到真实投票时才暴露**（那时已经开票了）。
// 所以这里把「工具产出的文件」当成接口来钉：文件名、JSON 字段、权限、
// 成员数量、路径长度都必须与客户端/服务端期望的一致。
func TestGenerateListProducesUsableArtifacts(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("member-b\nmember-a\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))

	// root.json：给服务端的那一份。
	var list eligibility.ListPackage
	raw, err := os.ReadFile(filepath.Join(outDir, "root.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &list))
	require.Equal(t, 2, list.Count)
	require.Equal(t, eligibility.TreeDepth, list.Depth)
	require.NotEmpty(t, list.Root)
	// root 必须是合法的域元素十六进制 —— 服务端启动时会解析它，解析不过就起不来。
	_, err = eligibility.DecodeField(list.Root)
	require.NoError(t, err)

	// 成员包：每个成员一份，文件名按成员 ID。
	for _, memberID := range []string{"member-a", "member-b"} {
		packPath := filepath.Join(outDir, "eligibility-"+memberID+".json")
		raw, err := os.ReadFile(packPath)
		require.NoError(t, err, "成员 %s 应当拿到自己的资格包", memberID)

		var pack eligibility.MemberPackage
		require.NoError(t, json.Unmarshal(raw, &pack))
		require.Equal(t, memberID, pack.MemberID)
		require.Equal(t, list.Root, pack.Root, "包里的 root 必须与服务端那一份一致")
		require.Equal(t, eligibility.TreeDepth, pack.Depth)
		require.Len(t, pack.Path, eligibility.TreeDepth, "路径长度必须等于树深")
		require.NotEmpty(t, pack.Secret)
		require.NotEmpty(t, pack.Leaf)
	}
}

// TestMemberPackagesAreSecretMaterial 钉住「成员包是秘密材料」这条要求：
// 权限必须收紧到只有所有者能读写。Windows 没有 POSIX 权限位，那一条只在
// 类 Unix 平台上检查 —— 与 cmd/keygen 的口径一致。
func TestMemberPackagesAreSecretMaterials(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("member-a\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))

	if runtime.GOOS == "windows" {
		t.Skip("Windows 没有 POSIX 权限位")
	}
	info, err := os.Stat(filepath.Join(outDir, "eligibility-member-a.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"成员包是秘密材料：谁拿到谁就能领这一票")
}

// TestGeneratedListVerifiesEndToEnd 是本文件最重要的一条：
// **工具产出的真实文件** → 客户端用它出证明 → 服务端用 root.json 验签通过。
//
// 这条链路此前没有任何测试覆盖：tests/e2e 的资格用例是在测试里手搓名单的，
// 走的是「同一个包直接构造」的路；真正部署时用的是**工具落盘的 JSON 文件**，
// 中间隔着一次序列化与一次反序列化。字段名对不上、编码方式变了、深度写错了
// —— 这些只有把工具产物真的喂给验签器才测得出来。
func TestGeneratedListVerifiesEndToEnd(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("member-a\nmember-b\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))

	// 服务端那一份：从 root.json 读出 root 与树深。
	var list eligibility.ListPackage
	raw, err := os.ReadFile(filepath.Join(outDir, "root.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &list))

	// 客户端那一份：从**文件**读回成员包（走真实的序列化往返）。
	var pack eligibility.MemberPackage
	raw, err = os.ReadFile(filepath.Join(outDir, "eligibility-member-a.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &pack))

	// 资格电路的密钥：proving key 在客户端，verifying key 在服务端。
	provingKey, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	provingKeyPath := filepath.Join(keyDir, "eligibility-proving.key")
	verifyingKeyPath := filepath.Join(keyDir, "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o600))

	prover, err := eligibility.NewProver(provingKeyPath)
	require.NoError(t, err)
	verifier, err := eligibility.NewVerifier(verifyingKeyPath)
	require.NoError(t, err)

	// 客户端：用工具给的包 + 服务端的 root 出证明。
	const (
		proposalID = "proposal-1"
		challenge  = "1f2e3d4c5b6a79887766554433221100ffeeddccbbaa99887766554433221100"
	)
	result, err := prover.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: pack,
		ProposalID:    proposalID,
		Challenge:     challenge,
		Root:          list.Root,
	})
	require.NoError(t, err)

	// 服务端：用 root.json 里的 root 验签（服务端只认自己配置的那一份）。
	statement, err := eligibility.BuildStatement(
		list.Root, proposalID, challenge, result.QualificationID,
	)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(result.Proof, statement))
}

// TestGeneratedListIsRejectedByAnotherRoot 钉住「包与 root 必须成套」这条：
// 另一份名单的 root 验不过这份证明。成员包走安全渠道分发，理论上不该和别的
// 名单混用，但这条断言保证「混了就是失败」，而不是「悄悄通过」。
func TestGeneratedListIsRejectedByAnotherRoot(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("member-a\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))

	var pack eligibility.MemberPackage
	raw, err := os.ReadFile(filepath.Join(outDir, "eligibility-member-a.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &pack))

	provingKey, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	provingKeyPath := filepath.Join(keyDir, "eligibility-proving.key")
	verifyingKeyPath := filepath.Join(keyDir, "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o600))

	prover, err := eligibility.NewProver(provingKeyPath)
	require.NoError(t, err)
	verifier, err := eligibility.NewVerifier(verifyingKeyPath)
	require.NoError(t, err)

	// 本名单的 root（服务端那一份）。
	var list eligibility.ListPackage
	raw, err = os.ReadFile(filepath.Join(outDir, "root.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &list))

	// 另一份名单的 root：同一个包配别的 root，客户端就该当场拒绝。
	var other eligibility.ListPackage
	otherLeaves := make([][]byte, 1)
	otherLeaves[0] = make([]byte, eligibility.FieldSize)
	otherLeaves[0][0] = 0x01
	canonical, err := eligibility.CanonicalSecret(otherLeaves[0])
	require.NoError(t, err)
	leaf, err := eligibility.DeriveLeaf(canonical)
	require.NoError(t, err)
	rootElement, _, err := eligibility.BuildTree(
		[]fr.Element{leaf}, eligibility.TreeDepth,
	)
	require.NoError(t, err)
	other.Root = eligibility.EncodeField(rootElement)

	const (
		proposalID = "proposal-1"
		challenge  = "1f2e3d4c5b6a79887766554433221100ffeeddccbbaa99887766554433221100"
	)
	_, err = prover.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: pack,
		ProposalID:    proposalID,
		Challenge:     challenge,
		Root:          other.Root,
	})
	require.Error(t, err, "包与 root 不成套时必须拒绝，不能出一份注定验不过的证明")

	// 本名单出的真证明，换一份名单的 root 去验，必须过不去 ——
	// root 是公开输入的一部分，证明与它绑死。
	result, err := prover.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: pack,
		ProposalID:    proposalID,
		Challenge:     challenge,
		Root:          list.Root,
	})
	require.NoError(t, err)

	crossStatement, err := eligibility.BuildStatement(
		other.Root, proposalID, challenge, result.QualificationID,
	)
	require.NoError(t, err)
	require.Error(t, verifier.Verify(result.Proof, crossStatement),
		"换一份名单的 root 就必须验不过")
}

// TestRunRefusesToOverwrite 工具不许默默覆盖已有的成员包：那些文件是秘密材料，
// 覆盖掉等于把已发出的资格作废。必须显式 -force。
func TestRunRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("member-a\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))
	require.Equal(t, 1, run([]string{"-members", membersPath, "-out", outDir}),
		"没有 -force 时不得覆盖已有的名单产物")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir, "-force"}))
}

// TestRunRejectsBadInput 名单工具的参数校验：没有名单、空名单、读不到的文件。
func TestRunRejectsBadInput(t *testing.T) {
	dir := t.TempDir()

	require.Equal(t, 2, run([]string{"-out", filepath.Join(dir, "out")}),
		"没给 -members 要直接报参数错")

	require.Equal(t, 1, run([]string{"-members", filepath.Join(dir, "missing.txt"),
		"-out", filepath.Join(dir, "out")}),
		"名单文件读不到要报错")

	emptyPath := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyPath, []byte("\n# 只有注释\n"), 0o600))
	require.Equal(t, 1, run([]string{"-members", emptyPath, "-out", filepath.Join(dir, "out")}),
		"空名单要报错")

	require.Equal(t, 2, run([]string{"-unknown"}), "未知参数要报参数错")
}
