package persistence

import (
	"fmt"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DBConfig 数据库配置。
type DBConfig struct {
	Driver   string // sqlite | postgres
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

func NewDB(cfg *DBConfig) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("db config is nil")
	}

	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	switch driver {
	case "postgres":
		return newPostgresDB(cfg)
	case "sqlite", "":
		return newSQLiteDB(cfg)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}
}

func newPostgresDB(cfg *DBConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.Username,
		cfg.Password,
		cfg.Database,
		cfg.SSLMode,
	)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func newSQLiteDB(cfg *DBConfig) (*gorm.DB, error) {
	dsn := cfg.Database
	if strings.TrimSpace(dsn) == "" {
		dsn = ":memory:"
	}

	return gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}
