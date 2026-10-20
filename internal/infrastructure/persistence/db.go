package persistence

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

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

	// 连接池参数。为 0 表示不改动 database/sql 的默认值。
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
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
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	applyPoolConfig(sqlDB, cfg)
	return gdb, nil
}

// applyPoolConfig 设置连接池参数。
//
// GORM 默认不限制连接数，高并发下会直接打爆 PostgreSQL 的 max_connections。
// 这里把上限、闲置数与连接最大存活时间都显式设出来，让连接能被定期回收
// （否则中间设备的连接表会被长连接占满）。
func applyPoolConfig(sqlDB *sql.DB, cfg *DBConfig) {
	if sqlDB == nil || cfg == nil {
		return
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
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
