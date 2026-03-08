package integration

import (
	"testing"

	"anonvote/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
)

func TestNewDB_SQLiteAndMigrate(t *testing.T) {
	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))
}
