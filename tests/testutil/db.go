package testutil

import (
	"testing"

	"anonvote/internal/infrastructure/persistence"

	"gorm.io/gorm"
)

func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	cfg := &persistence.DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	db, err := persistence.NewDB(cfg)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}
