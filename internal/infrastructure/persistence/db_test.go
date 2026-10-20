package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// openTestDB 打开一个真实的 *sql.DB，用来观察连接池设置的效果。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pool.db")
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestApplyPoolConfig_MaxOpenConns(t *testing.T) {
	db := openTestDB(t)

	applyPoolConfig(db, &DBConfig{MaxOpenConns: 7})

	require.Equal(t, 7, db.Stats().MaxOpenConnections)
}

func TestApplyPoolConfig_MaxIdleConns(t *testing.T) {
	db := openTestDB(t)

	applyPoolConfig(db, &DBConfig{MaxIdleConns: 1})

	// 池里只留一个闲置连接：先占住一个不放，再要一个（必须新开），
	// 然后把第一个还回去 —— 池里已经有一个闲置的了，它就得被关掉。
	first, err := db.Conn(context.Background())
	require.NoError(t, err)
	second, err := db.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, first.Close())
	require.NoError(t, second.Close())

	require.Positive(t, db.Stats().MaxIdleClosed, "超过 MaxIdleConns 的闲置连接应被关闭")
}

func TestApplyPoolConfig_KeepsIdleConnections(t *testing.T) {
	db := openTestDB(t)

	applyPoolConfig(db, &DBConfig{MaxIdleConns: 2})

	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Zero(t, db.Stats().MaxIdleClosed, "MaxIdleConns=2 时连接应留在池里")
}

func TestApplyPoolConfig_ConnMaxLifetime(t *testing.T) {
	db := openTestDB(t)

	applyPoolConfig(db, &DBConfig{ConnMaxLifetime: 20 * time.Millisecond})

	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	// 等待超过寿命上限，再触发一次连接获取让库回收过期连接。
	time.Sleep(60 * time.Millisecond)
	conn2, err := db.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, conn2.Close())

	require.Positive(t, db.Stats().MaxLifetimeClosed, "超过 ConnMaxLifetime 的连接应被回收")
}

func TestApplyPoolConfig_ZeroValuesKeepDefaults(t *testing.T) {
	db := openTestDB(t)
	before := db.Stats().MaxOpenConnections

	applyPoolConfig(db, &DBConfig{})

	require.Equal(t, before, db.Stats().MaxOpenConnections, "未配置时不应改动连接池上限")
}
