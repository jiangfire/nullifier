package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExternalPostgresSettingsFromEnv 覆盖外部 PostgreSQL 的连接参数解析。
//
// 这条路径是「本地 compose 起库跑集成测试」的入口（`make pg-test`），CI 则走
// testcontainers。解析错了会连到错误的库上，所以单测把它钉住 —— 纯逻辑，
// 不需要真的数据库。
func TestExternalPostgresSettingsFromEnv(t *testing.T) {
	t.Run("unset means no external database", func(t *testing.T) {
		t.Setenv("ANONVOTE_TEST_PG_HOST", "")
		_, ok := externalPostgresSettingsFromEnv()
		require.False(t, ok)
	})

	t.Run("host alone falls back to the compose defaults", func(t *testing.T) {
		t.Setenv("ANONVOTE_TEST_PG_HOST", "127.0.0.1")
		t.Setenv("ANONVOTE_TEST_PG_PORT", "")
		t.Setenv("ANONVOTE_TEST_PG_USER", "")
		t.Setenv("ANONVOTE_TEST_PG_PASSWORD", "")
		t.Setenv("ANONVOTE_TEST_PG_DATABASE", "")

		settings, ok := externalPostgresSettingsFromEnv()
		require.True(t, ok)
		require.Equal(t, "127.0.0.1", settings.Host)
		require.Equal(t, 5432, settings.Port)
		require.Equal(t, "test", settings.User)
		require.Equal(t, "test", settings.Password)
		require.Equal(t, "anonvote", settings.Database)
	})

	t.Run("every field can be overridden", func(t *testing.T) {
		t.Setenv("ANONVOTE_TEST_PG_HOST", "pg.internal")
		t.Setenv("ANONVOTE_TEST_PG_PORT", "6543")
		t.Setenv("ANONVOTE_TEST_PG_USER", "anonvote")
		t.Setenv("ANONVOTE_TEST_PG_PASSWORD", "s3cret")
		t.Setenv("ANONVOTE_TEST_PG_DATABASE", "anonvote_prod")

		settings, ok := externalPostgresSettingsFromEnv()
		require.True(t, ok)
		require.Equal(t, "pg.internal", settings.Host)
		require.Equal(t, 6543, settings.Port)
		require.Equal(t, "anonvote", settings.User)
		require.Equal(t, "s3cret", settings.Password)
		require.Equal(t, "anonvote_prod", settings.Database)
	})

	// 端口不是数字时按「没有外部库」处理 —— 宁可不跑，也不要连到错的地址。
	t.Run("an invalid port disables the external database", func(t *testing.T) {
		t.Setenv("ANONVOTE_TEST_PG_HOST", "127.0.0.1")
		t.Setenv("ANONVOTE_TEST_PG_PORT", "not-a-port")

		_, ok := externalPostgresSettingsFromEnv()
		require.False(t, ok)
	})
}
