package persistence

import (
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// closeDB 关掉连接 —— Windows 上不关的话，临时目录清不掉（文件句柄还占着）。
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func TestSQLiteReadOnlyDSN(t *testing.T) {
	t.Parallel()

	require.Equal(t, "file:anonvote.db?mode=ro", sqliteReadOnlyDSN("anonvote.db"))
	require.Equal(t,
		"file:C:/data/anonvote.db?mode=ro",
		sqliteReadOnlyDSN(`C:\data\anonvote.db`),
	)
	// 已有查询参数要合并，不能覆盖掉。
	require.Equal(t,
		"file:anonvote.db?_busy_timeout=10000&mode=ro",
		sqliteReadOnlyDSN("anonvote.db?_busy_timeout=10000"),
	)
	// 已经是 URI 且指定了 mode 的，保持原样。
	require.Equal(t,
		"file:anonvote.db?mode=rw",
		sqliteReadOnlyDSN("file:anonvote.db?mode=rw"),
	)
}

func TestNewDB_ReadOnlySQLiteCanReadButNotWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "audit.db")

	writable, err := NewDB(&DBConfig{Driver: "sqlite", Database: path})
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, writable) })
	require.NoError(t, AutoMigrate(writable))
	require.NoError(t, writable.Create(&entity.Proposal{
		ID:      "p1",
		Title:   "复算用提案",
		Options: []string{"A", "B"},
	}).Error)

	readOnly, err := NewDB(&DBConfig{Driver: "sqlite", Database: path, ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, readOnly) })

	var proposal entity.Proposal
	require.NoError(t, readOnly.Where("id = ?", "p1").First(&proposal).Error)
	require.Equal(t, "复算用提案", proposal.Title)

	// 只读不是「我们保证不写」，而是「写不了」。
	err = readOnly.Create(&entity.Proposal{ID: "p2", Title: "偷偷写入"}).Error
	require.Error(t, err)
}

func TestNewDB_ReadOnlySQLiteWorksWithWAL(t *testing.T) {
	t.Parallel()

	// WAL 是生产/压测推荐的 SQLite 形态，只读连接必须也能读它。
	path := filepath.Join(t.TempDir(), "audit-wal.db") + "?_journal_mode=WAL"

	writable, err := NewDB(&DBConfig{Driver: "sqlite", Database: path})
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, writable) })
	require.NoError(t, AutoMigrate(writable))
	require.NoError(t, writable.Create(&entity.Proposal{
		ID:      "p1",
		Title:   "WAL 提案",
		Options: []string{"A"},
	}).Error)

	readOnly, err := NewDB(&DBConfig{Driver: "sqlite", Database: path, ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, readOnly) })

	var proposal entity.Proposal
	require.NoError(t, readOnly.Where("id = ?", "p1").First(&proposal).Error)
	require.Equal(t, "WAL 提案", proposal.Title)
}

func TestNewDB_ReadOnlyRejectsInMemory(t *testing.T) {
	t.Parallel()

	_, err := NewDB(&DBConfig{Driver: "sqlite", Database: ":memory:", ReadOnly: true})
	require.Error(t, err)
}
