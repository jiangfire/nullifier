package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/stretchr/testify/require"
)

// testEligibilityRoot 是一枚合法的资格名单根（十六进制的域元素）。
const testEligibilityRoot = "2113d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e6"

// testEligibilityRoot2 是第二枚合法的名单根（与第一枚不同）。
const testEligibilityRoot2 = "1123d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e7"

// setReleaseEligibilityEnv 补齐 release 模式要求的资格校验配置。
//
// release 模式必须同时配名单根与资格验证密钥 —— 没有它们，服务端无法强制
// 「一人一票」。
func setReleaseEligibilityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CRYPTO_ELIGIBILITY_ROOT", testEligibilityRoot)
	t.Setenv("CRYPTO_ELIGIBILITY_VK_PATH", "eligibility-vk.bin")
}

func TestLoad_DefaultsAndEnv(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("GO_ENV", "test")
	t.Setenv("SERVER_ADDRESS", ":9090")
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret-token")
	t.Setenv("CRYPTO_ISSUER_ROTATE_DAYS", "7")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DB_PATH", "ignored.db")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.ServerAddress)
	require.Equal(t, "debug", cfg.GinMode)
	require.Equal(t, "secret-token", cfg.AdminBearerToken)
	require.Equal(t, 7*24*time.Hour, cfg.CredentialIssuerRotateAfter)
	require.NotNil(t, cfg.DB)
	require.Equal(t, "sqlite", cfg.DB.Driver)
	require.Equal(t, "ignored.db", cfg.DB.Database)
}

func TestLoad_RequiresVKInReleaseMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "crypto.vk_path is required")
}

func TestLoad_RequiresAdminTokenInReleaseMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")
	setReleaseEligibilityEnv(t)

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "server.admin_bearer_token is required")
}

// TestLoad_RequiresEligibilityInReleaseMode 钉住「release 模式必须开启一人
// 一票」这条硬要求：没有名单根与验证密钥，服务端启动就失败。
func TestLoad_RequiresEligibilityInReleaseMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")
	t.Setenv("CRYPTO_ELIGIBILITY_ROOT", "")
	t.Setenv("CRYPTO_ELIGIBILITY_VK_PATH", "")

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "crypto.eligibility_root")
}

// TestLoad_EligibilityRootAndKeyMustBePaired 钉住「根与验证密钥必须成对出现」：
// 只配根等于「不校验」，只配密钥则无从校验。
func TestLoad_EligibilityRootAndKeyMustBePaired(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_ELIGIBILITY_ROOT", testEligibilityRoot)
	t.Setenv("CRYPTO_ELIGIBILITY_VK_PATH", "")

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be configured together")
}

func TestLoad_AllowsMissingAdminTokenInDebugMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "", cfg.AdminBearerToken)
}

func TestEnsureAdminBearerToken_GeneratesWhenMissing(t *testing.T) {
	cfg := &Config{}

	generated, err := EnsureAdminBearerToken(cfg)
	require.NoError(t, err)
	require.True(t, generated)
	require.NotEmpty(t, cfg.AdminBearerToken)
}

func TestEnsureAdminBearerToken_KeepsConfiguredValue(t *testing.T) {
	cfg := &Config{AdminBearerToken: "  fixed-token  "}

	generated, err := EnsureAdminBearerToken(cfg)
	require.NoError(t, err)
	require.False(t, generated)
	require.Equal(t, "fixed-token", cfg.AdminBearerToken)
}

func TestLoad_FromConfigFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("GO_ENV", "")
	for _, key := range []string{
		"DB_DRIVER",
		"DB_PATH",
		"DB_HOST",
		"DB_PORT",
		"DB_NAME",
		"DB_USER",
		"DB_PASSWORD",
		"DB_SSLMODE",
	} {
		_ = os.Unsetenv(key)
	}

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "configs"), 0o755))
	cfgPath := filepath.Join(tempDir, "configs", "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
server:
  address: ":7001"
  mode: "debug"
  admin_bearer_token: "ops-secret"
crypto:
  vk_path: "vk.bin"
  issuer_key_path: "issuer.json"
  issuer_rotate_days: 15
db:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5544
  name: "vote_db"
  user: "dbu"
  password: "dbp"
  sslmode: "disable"
`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":7001", cfg.ServerAddress)
	require.Equal(t, "debug", cfg.GinMode)
	require.Equal(t, "ops-secret", cfg.AdminBearerToken)
	require.Equal(t, "vk.bin", cfg.VerifyingKeyPath)
	require.Equal(t, "issuer.json", cfg.CredentialIssuerKeyPath)
	require.Equal(t, 15*24*time.Hour, cfg.CredentialIssuerRotateAfter)
	require.Equal(t, "postgres", cfg.DB.Driver)
	require.Equal(t, "127.0.0.1", cfg.DB.Host)
	require.Equal(t, 5544, cfg.DB.Port)
	require.Equal(t, "vote_db", cfg.DB.Database)
	require.Equal(t, "dbu", cfg.DB.Username)
	require.Equal(t, "dbp", cfg.DB.Password)
}

func TestLoad_PoolConfigFromEnv(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_MAX_OPEN_CONNS", "42")
	t.Setenv("DB_MAX_IDLE_CONNS", "9")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "120")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 42, cfg.DB.MaxOpenConns)
	require.Equal(t, 9, cfg.DB.MaxIdleConns)
	require.Equal(t, 120*time.Second, cfg.DB.ConnMaxLifetime)
}

func TestLoad_PoolConfigDefaults(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 25, cfg.DB.MaxOpenConns)
	require.Equal(t, 5, cfg.DB.MaxIdleConns)
	require.Equal(t, time.Hour, cfg.DB.ConnMaxLifetime)
}

func TestLoad_PoolConfigFromConfigFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "")

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(`
db:
  driver: "postgres"
  max_open_conns: 11
  max_idle_conns: 3
  conn_max_lifetime_seconds: 60
`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 11, cfg.DB.MaxOpenConns)
	require.Equal(t, 3, cfg.DB.MaxIdleConns)
	require.Equal(t, 60*time.Second, cfg.DB.ConnMaxLifetime)
}

// 与其它 db.* 一致：环境变量优先于配置文件。
func TestLoad_PoolConfigEnvWinsOverConfigFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_MAX_OPEN_CONNS", "42")
	t.Setenv("DB_MAX_IDLE_CONNS", "9")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "120")

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(`
db:
  driver: "postgres"
  max_open_conns: 11
  max_idle_conns: 3
  conn_max_lifetime_seconds: 60
`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 42, cfg.DB.MaxOpenConns)
	require.Equal(t, 9, cfg.DB.MaxIdleConns)
	require.Equal(t, 120*time.Second, cfg.DB.ConnMaxLifetime)
}

func TestLoad_AutoMigrateDefaultsByMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_AUTO_MIGRATE", "")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.AutoMigrate, "开发模式应当自动迁移")
}

func TestLoad_AutoMigrateDisabledInReleaseMode(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("SERVER_AUTO_MIGRATE", "")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")
	setReleaseEligibilityEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.AutoMigrate, "release 模式不应在启动时改表")
}

func TestLoad_AutoMigrateOverriddenByEnv(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("SERVER_AUTO_MIGRATE", "true")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")
	setReleaseEligibilityEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.AutoMigrate)
}

func TestLoad_AutoMigrateOverriddenByConfigFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("SERVER_AUTO_MIGRATE", "")
	t.Setenv("CRYPTO_VK_PATH", "vk.bin")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "secret")
	setReleaseEligibilityEnv(t)

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(`
server:
  mode: "release"
  auto_migrate: true
`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.AutoMigrate)
}

func TestLoad_AutoMigrateRejectsNonBoolean(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_AUTO_MIGRATE", "maybe")

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "server.auto_migrate")
}

// 迁移步骤不该依赖只有应用运行时才需要的密钥。
func TestLoadForMigration_SkipsReleaseRequirements(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("SERVER_AUTO_MIGRATE", "")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")

	_, err = Load()
	require.Error(t, err, "普通加载仍应要求 release 模式的密钥")

	cfg, err := LoadForMigration()
	require.NoError(t, err)
	require.Equal(t, "release", cfg.GinMode)
}

func TestLoad_InvalidConfig(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte("server: ["), 0o600))
	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "read config file")
}

// CONFIG_FILE 显式指定配置文件：生产配置不叫 config.yaml，靠这条路径加载。
func TestLoad_ConfigFileEnvPointsToExplicitFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")

	explicit := filepath.Join(tempDir, "config.production.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte(`
server:
  address: ":7002"
  mode: "debug"
`), 0o600))
	t.Setenv(ConfigFileEnvVar, explicit)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":7002", cfg.ServerAddress)
	require.Equal(t, explicit, cfg.SourceFile)
}

// 显式指定的文件不存在时必须直接失败 —— 静默回退到开发配置是最危险的上线事故。
func TestLoad_ConfigFileEnvMissingFileFails(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv(ConfigFileEnvVar, filepath.Join(tempDir, "does-not-exist.yaml"))

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "read config file")
}

// 显式路径优先于自动发现：工作目录里有 config.yaml 也不能覆盖它。
func TestLoad_ConfigFileEnvWinsOverDiscoveredFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(`
server:
  address: ":1111"
`), 0o600))

	explicit := filepath.Join(tempDir, "explicit.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte(`
server:
  address: ":2222"
`), 0o600))
	t.Setenv(ConfigFileEnvVar, explicit)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":2222", cfg.ServerAddress)
	require.Equal(t, explicit, cfg.SourceFile)
}

// 自动发现到的配置文件也要记进 SourceFile，启动日志要能看出跑的是哪份配置。
func TestLoad_RecordsDiscoveredSourceFile(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")
	t.Setenv(ConfigFileEnvVar, "")

	discovered := filepath.Join(tempDir, "config.yaml")
	require.NoError(t, os.WriteFile(discovered, []byte("server:\n  address: \":3333\"\n"), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, discovered, cfg.SourceFile)
}

// 没有配置文件时 SourceFile 为空：启动日志据此说明「没读到配置文件」。
func TestLoad_WithoutConfigFileHasEmptySource(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")
	t.Setenv(ConfigFileEnvVar, "")

	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.SourceFile)
}

func TestIsReleaseMode(t *testing.T) {
	require.True(t, IsReleaseMode("release"))
	require.True(t, IsReleaseMode(" PROD "))
	require.True(t, IsReleaseMode("production"))
	require.False(t, IsReleaseMode("debug"))
	require.False(t, IsReleaseMode(""))
}

// TestLoad_EligibilityDepthDefaultsToLegacyShape 钉住默认树深：不配置时取 10
// —— v0.2.0 存量名单与密钥的口径，升级零动作。
func TestLoad_EligibilityDepthDefaultsToLegacyShape(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	t.Setenv("SERVER_MODE", "debug")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 10, cfg.EligibilityDepth)
}

// TestLoad_RejectsDepthOutsideRange 钉住深度校验：范围外的树深一律启动失败
// —— 深度的合法范围由资格电路定义，配错说明部署认知与实现脱节了。
func TestLoad_RejectsDepthOutsideRange(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("CRYPTO_ELIGIBILITY_DEPTH", "3")
	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "crypto.eligibility_depth")

	t.Setenv("CRYPTO_ELIGIBILITY_DEPTH", "25")
	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "crypto.eligibility_depth")

	// 合法值放行（显式配置优先于默认值）。
	t.Setenv("CRYPTO_ELIGIBILITY_DEPTH", "16")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 16, cfg.EligibilityDepth)
}

// writeRosterConfigFile 写一份带名单列表的配置文件，返回路径。
func writeRosterConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestLoad_EligibilityRosters 覆盖多名单配置：列表解析、深度缺省、
// release 模式只靠列表即满足硬要求。
func TestLoad_EligibilityRosters(t *testing.T) {
	configFile := writeRosterConfigFile(t, `
server:
  mode: release
  admin_bearer_token: secret
crypto:
  vk_path: vk.bin
  eligibility_roots:
    - name: acme-2026
      root: `+testEligibilityRoot+`
      vk_path: keys/acme.key
      depth: 10
    - name: default
      root: `+testEligibilityRoot2+`
      vk_path: keys/default.key
`)
	t.Setenv(ConfigFileEnvVar, configFile)

	cfg, err := Load()
	require.NoError(t, err)
	require.Len(t, cfg.EligibilityRosters, 2)
	require.Equal(t, "acme-2026", cfg.EligibilityRosters[0].Name)
	require.Equal(t, 10, cfg.EligibilityRosters[0].Depth)
	require.Equal(t, "default", cfg.EligibilityRosters[1].Name)
	require.Equal(t, eligibility.DefaultTreeDepth, cfg.EligibilityRosters[1].Depth,
		"缺省深度取默认值（存量口径）")
	// 旧单值字段保持为空：列表写法与单值写法互斥。
	require.Empty(t, cfg.EligibilityRoot)
}

// TestLoad_EligibilityRostersConflictsWithLegacy 列表与旧单值混配 → 启动失败：
// 「默认名单是哪套」会有歧义。
func TestLoad_EligibilityRostersConflictsWithLegacy(t *testing.T) {
	configFile := writeRosterConfigFile(t, `
server:
  mode: debug
crypto:
  eligibility_root: `+testEligibilityRoot+`
  eligibility_vk_path: keys/legacy.key
  eligibility_roots:
    - name: acme-2026
      root: `+testEligibilityRoot+`
      vk_path: keys/acme.key
`)
	t.Setenv(ConfigFileEnvVar, configFile)

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

// TestLoad_EligibilityRostersRejectsDuplicates 重名的名单 → 启动失败：
// 「提案绑定的是哪一套」无从回答。
func TestLoad_EligibilityRostersRejectsDuplicates(t *testing.T) {
	configFile := writeRosterConfigFile(t, `
server:
  mode: debug
crypto:
  eligibility_roots:
    - name: acme-2026
      root: `+testEligibilityRoot+`
      vk_path: keys/a.key
    - name: acme-2026
      root: `+testEligibilityRoot2+`
      vk_path: keys/b.key
`)
	t.Setenv(ConfigFileEnvVar, configFile)

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than once")
}

// TestLoad_EligibilityRostersRejectsIncomplete 缺名字或缺密钥的名单 → 启动失败。
func TestLoad_EligibilityRostersRejectsIncomplete(t *testing.T) {
	configFile := writeRosterConfigFile(t, `
server:
  mode: debug
crypto:
  eligibility_roots:
    - root: `+testEligibilityRoot+`
      vk_path: keys/a.key
`)
	t.Setenv(ConfigFileEnvVar, configFile)

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "name is required")

	configFile = writeRosterConfigFile(t, `
server:
  mode: debug
crypto:
  eligibility_roots:
    - name: acme-2026
      root: `+testEligibilityRoot+`
`)
	t.Setenv(ConfigFileEnvVar, configFile)

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "vk_path is required")
}
