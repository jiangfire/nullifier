package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

	_, err = Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "server.admin_bearer_token is required")
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
