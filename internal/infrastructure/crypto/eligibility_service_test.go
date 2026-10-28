package crypto

import (
	"os"
	"path/filepath"
	"testing"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/stretchr/testify/require"
)

// 资格校验服务端的测试根与密钥。
const (
	testServiceRoot  = "2113d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e6"
	testServiceRoot2 = "1123d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e7"
)

// writeEligibilityKeys 生成一对资格电路密钥，返回验证密钥的路径。
func writeEligibilityKeys(t *testing.T) string {
	t.Helper()
	_, verifyingKey, err := eligibility.GenerateKeys(eligibility.DefaultTreeDepth)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(path, verifyingKey, 0o600))
	return path
}

// TestNewEligibilityService_Disabled 没配根也没配密钥 → 未开启。
//
// 「未开启」时服务端**不强制**一人一票：任何人都能反复领凭证。
func TestNewEligibilityService_Disabled(t *testing.T) {
	service, err := NewEligibilityService("", "", eligibility.DefaultTreeDepth)
	require.NoError(t, err)
	require.False(t, service.Enabled())

	_, _, err = service.RosterFor("")
	require.Error(t, err)

	_, err = service.NewChallenge()
	require.Error(t, err)

	require.Error(t, service.VerifyFor("", nil, domaincrypto.EligibilityStatement{}))
}

// TestNewEligibilityService_RequiresBoth 根与密钥必须成对出现。
func TestNewEligibilityService_RequiresBoth(t *testing.T) {
	_, err := NewEligibilityService(testServiceRoot, "", eligibility.DefaultTreeDepth)
	require.ErrorContains(t, err, "verifying key path is required")

	_, err = NewEligibilityService("", "vk.bin", eligibility.DefaultTreeDepth)
	require.ErrorContains(t, err, "root is required")
}

// TestNewEligibilityService_RejectsBadRoot 名单根必须是规范的域元素编码。
func TestNewEligibilityService_RejectsBadRoot(t *testing.T) {
	_, err := NewEligibilityService("not-hex", "vk.bin", eligibility.DefaultTreeDepth)
	require.ErrorContains(t, err, "invalid root")
}

// TestNewEligibilityService_MissingVerifyingKeyFile 验证密钥读不到时启动失败 ——
// 「验不了就放行」是最不能接受的降级。
func TestNewEligibilityService_MissingVerifyingKeyFile(t *testing.T) {
	_, err := NewEligibilityService(testServiceRoot, filepath.Join(t.TempDir(), "missing.bin"), eligibility.DefaultTreeDepth)
	require.Error(t, err)
}

// TestEligibilityService_Enabled 开启后的正常路径。
func TestEligibilityService_Enabled(t *testing.T) {
	service, err := NewEligibilityService(testServiceRoot, writeEligibilityKeys(t), eligibility.DefaultTreeDepth)
	require.NoError(t, err)
	require.True(t, service.Enabled())

	// 单名单（旧写法）等价于一套名为 default 的名单：空名单域解析到它。
	root, depth, err := service.RosterFor("")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot, root)
	require.Equal(t, eligibility.DefaultTreeDepth, depth)
	root, depth, err = service.RosterFor("default")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot, root)

	// 挑战必须是规范的域元素编码（64 位十六进制）。
	challenge, err := service.NewChallenge()
	require.NoError(t, err)
	require.Len(t, challenge, eligibility.FieldSize*2)
	_, err = eligibility.DecodeField(challenge)
	require.NoError(t, err)

	// 两次挑战不许相同。
	other, err := service.NewChallenge()
	require.NoError(t, err)
	require.NotEqual(t, challenge, other)
}

// TestEligibilityService_VerifyFor 验签：错的证明被拒。
func TestEligibilityService_VerifyFor(t *testing.T) {
	service, err := NewEligibilityService(testServiceRoot, writeEligibilityKeys(t), eligibility.DefaultTreeDepth)
	require.NoError(t, err)

	statement := domaincrypto.EligibilityStatement{
		Root:            testServiceRoot,
		ProposalID:      "p-1",
		Challenge:       "00aabbccddeeff00112233445566778899aabbccddeeff001122334455667788",
		QualificationID: "1122334455667788990011223344556677889900112233445566778899001122",
	}

	// 乱写的证明过不了。
	require.Error(t, service.VerifyFor("", []byte("not-a-proof"), statement))

	// 公开输入不合法（根不是域元素）也过不了。
	broken := statement
	broken.Root = "zz"
	require.Error(t, service.VerifyFor("", []byte("not-a-proof"), broken))
}

// TestEligibilityService_MultiRoster 多名单并存：按名字解析、默认规则、
// 未知名字报错、重名拒绝。
func TestEligibilityService_MultiRoster(t *testing.T) {
	vkA := writeEligibilityKeys(t)
	vkB := writeEligibilityKeys(t)
	specs := []RosterSpec{
		{Name: "acme-2026", Root: testServiceRoot, VerifyingKeyPath: vkA, Depth: eligibility.DefaultTreeDepth},
		{Name: "default", Root: testServiceRoot2, VerifyingKeyPath: vkB, Depth: 4},
	}
	service, err := NewEligibilityServiceFromSpecs(specs)
	require.NoError(t, err)
	require.True(t, service.Enabled())

	// 按名字各取各的。
	rootA, depthA, err := service.RosterFor("acme-2026")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot, rootA)
	require.Equal(t, eligibility.DefaultTreeDepth, depthA)

	rootB, depthB, err := service.RosterFor("default")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot2, rootB)
	require.Equal(t, 4, depthB)

	// 空 scope 取名为 default 的那套（不是第一套）。
	emptyRoot, _, err := service.RosterFor("")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot2, emptyRoot)

	// 存在性检查。
	require.True(t, service.ScopeKnown("acme-2026"))
	require.True(t, service.ScopeKnown(""))
	require.False(t, service.ScopeKnown("nope"))

	// 未知名字：明确报错并列出已配置的名字。
	_, _, err = service.RosterFor("nope")
	require.ErrorContains(t, err, `unknown eligibility roster "nope"`)
	require.Error(t, service.VerifyFor("nope", nil, domaincrypto.EligibilityStatement{}))
}

// TestEligibilityService_MultiRosterNoDefault 多名单且没有名为 default 的：
// 空 scope 解析失败（fail-closed），但显式名字照常工作。
func TestEligibilityService_MultiRosterNoDefault(t *testing.T) {
	specs := []RosterSpec{
		{Name: "acme-2026", Root: testServiceRoot, VerifyingKeyPath: writeEligibilityKeys(t), Depth: eligibility.DefaultTreeDepth},
		{Name: "other-2026", Root: testServiceRoot2, VerifyingKeyPath: writeEligibilityKeys(t), Depth: eligibility.DefaultTreeDepth},
	}
	service, err := NewEligibilityServiceFromSpecs(specs)
	require.NoError(t, err)

	_, _, err = service.RosterFor("")
	require.ErrorContains(t, err, "no default eligibility roster")

	// 唯一名单时空 scope 取它；两套时就必须显式绑定。
	single, err := NewEligibilityServiceFromSpecs(specs[:1])
	require.NoError(t, err)
	root, _, err := single.RosterFor("")
	require.NoError(t, err)
	require.Equal(t, testServiceRoot, root)
}

// TestEligibilityService_RejectsDuplicateRoster 名字重复 → 启动失败。
func TestEligibilityService_RejectsDuplicateRoster(t *testing.T) {
	vk := writeEligibilityKeys(t)
	specs := []RosterSpec{
		{Name: "acme-2026", Root: testServiceRoot, VerifyingKeyPath: vk, Depth: eligibility.DefaultTreeDepth},
		{Name: "acme-2026", Root: testServiceRoot2, VerifyingKeyPath: vk, Depth: eligibility.DefaultTreeDepth},
	}
	_, err := NewEligibilityServiceFromSpecs(specs)
	require.ErrorContains(t, err, "more than once")
}

// TestEligibilityService_RejectsIncompleteRoster 缺密钥或缺根的名单 → 启动失败。
func TestEligibilityService_RejectsIncompleteRoster(t *testing.T) {
	_, err := NewEligibilityServiceFromSpecs([]RosterSpec{
		{Name: "half", Root: testServiceRoot, VerifyingKeyPath: "", Depth: eligibility.DefaultTreeDepth},
	})
	require.Error(t, err)

	_, err = NewEligibilityServiceFromSpecs([]RosterSpec{
		{Name: "half", Root: "", VerifyingKeyPath: "vk.bin", Depth: eligibility.DefaultTreeDepth},
	})
	require.Error(t, err)

	_, err = NewEligibilityServiceFromSpecs([]RosterSpec{
		{Name: "", Root: testServiceRoot, VerifyingKeyPath: "vk.bin", Depth: eligibility.DefaultTreeDepth},
	})
	require.ErrorContains(t, err, "name is required")
}
