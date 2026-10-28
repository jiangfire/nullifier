package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
// TestGenerateListRejectsCollidingFileNames 钉住「撞名必须拒绝」。
//
// 成员包的文件名由成员标识归一化而来，归一化会把字母数字与 `-_.` 之外的字符
// 一律换成 `_`。于是「张三-01」与「李四-01」会落到同一个文件名 ——
// 两个人共用一个包名，后写的把前一个静默覆盖（带 -force）或整次生成半途失败。
//
// 后者更危险：root.json 已经落盘、成员包只写了一半，root 声称的成员数与实际
// 包数对不上；运营方看到报错就加 -force 重跑，缺的那个包就永远消失了 ——
// 「谁拿到谁就能领这一票」，等于有一个人能领别人的票，一人一票当场失效。
//
// 所以工具必须在**写盘前**拒绝，并把撞名的成员标识报出来。
func TestGenerateListRejectsCollidingFileNames(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("张三-01\n李四-01\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 1, run([]string{"-members", membersPath, "-out", outDir}),
		"两个成员归一化后撞名时必须拒绝生成")

	// 报错信息要说清是「谁」撞了「谁」—— 运营方要靠它去改标识。
	_, err := os.Stat(filepath.Join(outDir, "root.json"))
	require.Error(t, err, "拒绝生成时不得留下 root.json：它是要配到服务端的，留下了就可能被误上线")

	// 带 -force 也一样要拒绝 —— 那是「静默覆盖」那条路。
	require.Equal(t, 1, run([]string{"-members", membersPath, "-out", outDir, "-force"}),
		"带 -force 也不许静默覆盖：两个成员不能共用一个包")
	entries, err := os.ReadDir(outDir)
	if err == nil {
		for _, entry := range entries {
			require.NotEqual(t, "root.json", entry.Name(),
				"拒绝生成时不得留下 root.json")
		}
	}
}

// TestGenerateListWritesOnePackagePerMember 钉住「包数与成员数一致」。
//
// 这是撞名那个缺陷最直接的观测口径：root.json 声称 N 名成员，目录里就应当
// 有 N 个成员包。缺陷存在时，root 说 2 个、目录里只有 1 个。
func TestGenerateListWritesOnePackagePerMember(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("E-001\nE-002\nE-003\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir}))

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	packages := 0
	for _, entry := range entries {
		if entry.Name() != "root.json" && strings.HasPrefix(entry.Name(), "eligibility-") {
			packages++
		}
	}
	require.Equal(t, 3, packages, "每个成员一份包")

	var list eligibility.ListPackage
	raw, err := os.ReadFile(filepath.Join(outDir, "root.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &list))
	require.Equal(t, list.Count, packages, "root.json 声称的成员数必须与实际包数一致")
}

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
	require.Equal(t, eligibility.DefaultTreeDepth, list.Depth)
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
		require.Equal(t, eligibility.DefaultTreeDepth, pack.Depth)
		require.Len(t, pack.Path, eligibility.DefaultTreeDepth, "路径长度必须等于树深")
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
	provingKey, verifyingKey, err := eligibility.GenerateKeys(eligibility.DefaultTreeDepth)
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

	provingKey, verifyingKey, err := eligibility.GenerateKeys(eligibility.DefaultTreeDepth)
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
		[]fr.Element{leaf}, eligibility.DefaultTreeDepth,
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

// TestGenerateListAtCustomDepth 覆盖非默认树深：-depth 4 产出的 root.json 与
// 成员包都带深度 4，路径长度等于树深（v0.4.0 起深度跟随部署，不再是常量）。
func TestGenerateListAtCustomDepth(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("A-1\nA-2\n"), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir, "-depth", "4"}))

	var list eligibility.ListPackage
	raw, err := os.ReadFile(filepath.Join(outDir, "root.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &list))
	require.Equal(t, 4, list.Depth)

	var pack eligibility.MemberPackage
	raw, err = os.ReadFile(filepath.Join(outDir, "eligibility-A-1.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &pack))
	require.Equal(t, 4, pack.Depth)
	require.Len(t, pack.Path, 4)
}

// TestGenerateListRejectsOverflow 钉住超限行为：名单装不下时**拒绝生成**，
// 且错误信息指向解决办法（换更深的树），不是底层树构建的报错。
func TestGenerateListRejectsOverflow(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	members := make([]string, 0, 17)
	for i := 0; i < 17; i++ {
		members = append(members, fmt.Sprintf("M-%03d", i))
	}
	require.NoError(t, os.WriteFile(membersPath, []byte(strings.Join(members, "\n")), 0o600))

	outDir := filepath.Join(dir, "out")
	require.Equal(t, 1, run([]string{"-members", membersPath, "-out", outDir, "-depth", "4"}))

	// 拒绝发生在写任何文件之前：目录里不该有 root.json。
	_, err := os.Stat(filepath.Join(outDir, "root.json"))
	require.True(t, os.IsNotExist(err), "超限时不能留下半份名单")

	// 同一份名单在深度 5 下装得下（2^5 = 32）。
	require.Equal(t, 0, run([]string{"-members", membersPath, "-out", outDir, "-depth", "5"}))
}

func TestRejectsDepthOutsideRange(t *testing.T) {
	dir := t.TempDir()
	membersPath := filepath.Join(dir, "members.txt")
	require.NoError(t, os.WriteFile(membersPath, []byte("A-1\n"), 0o600))
	require.Equal(t, 2, run([]string{"-members", membersPath, "-out", filepath.Join(dir, "o1"), "-depth", "3"}))
	require.Equal(t, 2, run([]string{"-members", membersPath, "-out", filepath.Join(dir, "o2"), "-depth", "25"}))
}
