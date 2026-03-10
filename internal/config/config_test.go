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
