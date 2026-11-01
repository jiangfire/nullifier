package persistence

import (
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := NewDB(&DBConfig{Driver: "sqlite", Database: ":memory:"})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestSchemaReady_FalseBeforeMigration(t *testing.T) {
	db := newMemoryDB(t)

	require.False(t, SchemaReady(db), "迁移之前表结构不应被视为就绪")
}

func TestSchemaReady_TrueAfterMigration(t *testing.T) {
	db := newMemoryDB(t)
	require.NoError(t, AutoMigrate(db))

	require.True(t, SchemaReady(db))
}

func TestSchemaReady_FalseForNilDB(t *testing.T) {
	require.False(t, SchemaReady(nil))
}

// v0.3.0 的库缺 v0.4.0 新列 eligibility_scope：升级忘跑 -migrate 时必须启动
// 失败，而不是等第一个建提案的请求才报「no such column」（F6）。
func TestSchemaReady_FalseWhenEligibilityScopeColumnMissing(t *testing.T) {
	db := newMemoryDB(t)
	require.NoError(t, AutoMigrate(db))
	require.NoError(t, db.Exec("ALTER TABLE proposals DROP COLUMN eligibility_scope").Error)

	require.False(t, SchemaReady(db), "缺少 v0.4.0 新列 proposals.eligibility_scope 时不应被视为就绪")
}

func TestAutoMigrate_CreatesAllTables(t *testing.T) {
	db := newMemoryDB(t)
	require.NoError(t, AutoMigrate(db))

	for _, model := range []any{
		&entity.Vote{},
		&entity.Proposal{},
		&entity.Credential{},
		&entity.VoteParticipation{},
		&entity.DecryptionSession{},
		&entity.TallyResult{},
		&entity.AuditAnchor{},
	} {
		require.True(t, db.Migrator().HasTable(model), "缺少表: %T", model)
	}
}
