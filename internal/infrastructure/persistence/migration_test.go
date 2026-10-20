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
