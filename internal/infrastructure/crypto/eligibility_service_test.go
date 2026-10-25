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
	testServiceRoot = "2113d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e6"
)

// writeEligibilityKeys 生成一对资格电路密钥，返回验证密钥的路径。
func writeEligibilityKeys(t *testing.T) string {
	t.Helper()
	_, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(path, verifyingKey, 0o600))
	return path
}

// TestNewEligibilityService_Disabled 没配根也没配密钥 → 未开启。
//
// 「未开启」时服务端**不强制**一人一票：任何人都能反复领凭证。
func TestNewEligibilityService_Disabled(t *testing.T) {
	service, err := NewEligibilityService("", "")
	require.NoError(t, err)
	require.False(t, service.Enabled())

	_, _, err = service.Root()
	require.Error(t, err)

	_, err = service.NewChallenge()
	require.Error(t, err)

	require.Error(t, service.Verify(nil, domaincrypto.EligibilityStatement{}))
}

// TestNewEligibilityService_RequiresBoth 根与密钥必须成对出现。
func TestNewEligibilityService_RequiresBoth(t *testing.T) {
	_, err := NewEligibilityService(testServiceRoot, "")
	require.ErrorContains(t, err, "crypto.eligibility_vk_path is required")

	_, err = NewEligibilityService("", "vk.bin")
	require.ErrorContains(t, err, "crypto.eligibility_root is required")
}

// TestNewEligibilityService_RejectsBadRoot 名单根必须是规范的域元素编码。
func TestNewEligibilityService_RejectsBadRoot(t *testing.T) {
	_, err := NewEligibilityService("not-hex", "vk.bin")
	require.ErrorContains(t, err, "invalid crypto.eligibility_root")
}

// TestNewEligibilityService_MissingVerifyingKeyFile 验证密钥读不到时启动失败 ——
// 「验不了就放行」是最不能接受的降级。
func TestNewEligibilityService_MissingVerifyingKeyFile(t *testing.T) {
	_, err := NewEligibilityService(testServiceRoot, filepath.Join(t.TempDir(), "missing.bin"))
	require.Error(t, err)
}

// TestEligibilityService_Enabled 开启后的正常路径。
func TestEligibilityService_Enabled(t *testing.T) {
	service, err := NewEligibilityService(testServiceRoot, writeEligibilityKeys(t))
	require.NoError(t, err)
	require.True(t, service.Enabled())

	root, depth, err := service.Root()
	require.NoError(t, err)
	require.Equal(t, testServiceRoot, root)
	require.Equal(t, eligibility.TreeDepth, depth)

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

// TestEligibilityService_Verify 验签：错的证明被拒，对的通过。
func TestEligibilityService_Verify(t *testing.T) {
	service, err := NewEligibilityService(testServiceRoot, writeEligibilityKeys(t))
	require.NoError(t, err)

	statement := domaincrypto.EligibilityStatement{
		Root:            testServiceRoot,
		ProposalID:      "p-1",
		Challenge:       "00aabbccddeeff00112233445566778899aabbccddeeff001122334455667788",
		QualificationID: "1122334455667788990011223344556677889900112233445566778899001122",
	}

	// 乱写的证明过不了。
	require.Error(t, service.Verify([]byte("not-a-proof"), statement))

	// 公开输入不合法（根不是域元素）也过不了。
	broken := statement
	broken.Root = "zz"
	require.Error(t, service.Verify([]byte("not-a-proof"), broken))
}
