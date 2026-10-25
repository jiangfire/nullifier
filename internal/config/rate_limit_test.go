package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// chdirTemp 把工作目录切到临时目录，避免测试读到仓库里的 configs/config.yaml。
func chdirTemp(t *testing.T) {
	t.Helper()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
}

// setReleaseEnv 补齐 release 模式要求的全部配置。
func setReleaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")
	t.Setenv("CRYPTO_ELIGIBILITY_ROOT", testEligibilityRoot)
	t.Setenv("CRYPTO_ELIGIBILITY_VK_PATH", "eligibility-vk.bin")
}

// TestLoad_RateLimitDefaultsByMode 钉住「没配就按模式推导」：
// release 默认开（公开端点必须有上限），debug 默认关（方便联调）。
func TestLoad_RateLimitDefaultsByMode(t *testing.T) {
	chdirTemp(t)

	t.Setenv("SERVER_MODE", "debug")
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.RateLimit.Enabled, "debug 模式默认不限流")

	chdirTemp(t)
	setReleaseEnv(t)
	cfg, err = Load()
	require.NoError(t, err)
	require.True(t, cfg.RateLimit.Enabled, "release 模式默认限流")
}

// TestLoad_RateLimitDefaultQuotas 钉住默认额度。
//
// 这些数字不是随手填的：推导过程见 docs/t25-ratelimit-design.md §3.2
// （500 人 / 5 分钟 / 共用一个出口 IP 的场景能跑完）。
func TestLoad_RateLimitDefaultQuotas(t *testing.T) {
	chdirTemp(t)
	setReleaseEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 10, cfg.RateLimit.Credentials.RPS)
	require.Equal(t, 40, cfg.RateLimit.Credentials.Burst)
	require.Equal(t, 10, cfg.RateLimit.Votes.RPS)
	require.Equal(t, 40, cfg.RateLimit.Votes.Burst)
	require.Equal(t, 50, cfg.RateLimit.Read.RPS)
	require.Equal(t, 100, cfg.RateLimit.Read.Burst)
}

// TestLoad_RateLimitQuotaFromEnv 验证额度能按端点组分别覆盖。
func TestLoad_RateLimitQuotaFromEnv(t *testing.T) {
	chdirTemp(t)
	setReleaseEnv(t)
	t.Setenv("RATE_LIMIT_VOTES_RPS", "3")
	t.Setenv("RATE_LIMIT_VOTES_BURST", "5")
	t.Setenv("RATE_LIMIT_READ_RPS", "7")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 3, cfg.RateLimit.Votes.RPS)
	require.Equal(t, 5, cfg.RateLimit.Votes.Burst)
	require.Equal(t, 7, cfg.RateLimit.Read.RPS)
	require.Equal(t, 40, cfg.RateLimit.Credentials.Burst, "没配的组保持默认值")
}

// TestLoad_RateLimitCannotBeDisabledInReleaseMode 钉住「限流是 release 模式的
// 硬要求」：显式关掉会让服务起不来，而不是悄悄地没有保护。
func TestLoad_RateLimitCannotBeDisabledInReleaseMode(t *testing.T) {
	chdirTemp(t)
	setReleaseEnv(t)
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate_limit.enabled cannot be disabled")
}

// TestLoad_RateLimitCanBeDisabledInDebugMode 确认开发模式仍能关掉限流。
func TestLoad_RateLimitCanBeDisabledInDebugMode(t *testing.T) {
	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.RateLimit.Enabled)
}

// TestLoad_RateLimitRejectsNonBoolean 拒绝看不懂的开关值。
func TestLoad_RateLimitRejectsNonBoolean(t *testing.T) {
	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("RATE_LIMIT_ENABLED", "maybe")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate_limit.enabled must be a boolean")
}

// TestLoad_RateLimitRejectsNonPositiveQuota 钉住「配错比不配更危险」：
// 额度为 0 意味着「全拒」，服务直接不可用 —— 必须在启动时就失败。
func TestLoad_RateLimitRejectsNonPositiveQuota(t *testing.T) {
	chdirTemp(t)
	setReleaseEnv(t)
	t.Setenv("RATE_LIMIT_VOTES_RPS", "0")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate_limit.votes.rps must be positive")

	chdirTemp(t)
	setReleaseEnv(t)
	t.Setenv("RATE_LIMIT_CREDENTIALS_BURST", "-1")

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate_limit.credentials.burst must be positive")
}

// TestLoad_RateLimitQuotaNotValidatedWhenDisabled 确认限流关掉时不校验额度 ——
// 那时这些数字根本不参与决策。
func TestLoad_RateLimitQuotaNotValidatedWhenDisabled(t *testing.T) {
	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	t.Setenv("RATE_LIMIT_VOTES_RPS", "0")

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.RateLimit.Enabled)
}

// TestLoad_TrustedProxies 验证可信代理网段能配。
//
// 默认空 = 不信任任何代理：客户端 IP 取连接来源地址，伪造
// X-Forwarded-For 无效。部署在反向代理后面时必须显式配置。
func TestLoad_TrustedProxies(t *testing.T) {
	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")

	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.TrustedProxies, "默认不信任任何代理")

	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_TRUSTED_PROXIES", "10.0.0.0/8 192.168.0.0/16")

	cfg, err = Load()
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, cfg.TrustedProxies)
}

// TestLoad_RejectsInvalidTrustedProxy 钉住「配错网段宁可启动失败」：
// 网段配错的后果是限流按错误的来源计数 —— 与其带着假象上线，不如说清楚。
func TestLoad_RejectsInvalidTrustedProxy(t *testing.T) {
	chdirTemp(t)
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_TRUSTED_PROXIES", "10.0.0.0/8 not-a-cidr")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "server.trusted_proxies")
}
