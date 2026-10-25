package persistence

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
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

	registerSQLiteMemTempDriver()

	return gorm.Open(sqlite.Dialector{
		DriverName: sqliteMemTempDriverName,
		DSN:        dsn,
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// sqliteMemTempDriverName 是本项目自用的 SQLite 驱动名。
//
// 与 mattn 自带的 "sqlite3" 完全一样，只多一件事：**每条新连接建立时把
// temp_store 设成 MEMORY**。
const sqliteMemTempDriverName = "sqlite3_memtemp"

var registerSQLiteMemTempDriverOnce sync.Once

// registerSQLiteMemTempDriver 注册带 ConnectHook 的 SQLite 驱动。
//
// 为什么需要它：多行 INSERT（VALUES 后跟很多组元组）在做主键去重检查时，
// SQLite 会建一个**临时索引**。本机（Windows，用户目录下的临时目录）实测：
// 一次事务里累积到几百上千行时，那个临时索引会从内存溢写到临时文件，而这个
// 临时文件建不出来 —— 整条 INSERT 就以 SQLITE_CANTOPEN 失败，接口报成一句
// 看不懂的 "unable to open database file: The system cannot find the file
// specified."（备池、投票这类批量写都会中招）。
//
// 备池接口一次要投几千行，正好踩在这条路径上 —— 所以这里直接把临时表与
// 临时索引钉在内存里：它们的大小只跟单批行数有关（几十行），不会把内存
// 吃穿；而 SQLite 的临时文件路径在这台机器上不可靠。
//
// 用 ConnectHook 而不是「启动时执行一次 PRAGMA」：database/sql 的连接是
// 按需建立的，启动时执行只会落在当时那一条连接上，后面新建的连接拿不到
// 这个设置。
func registerSQLiteMemTempDriver() {
	registerSQLiteMemTempDriverOnce.Do(func() {
		sql.Register(sqliteMemTempDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				_, err := conn.Exec("PRAGMA temp_store=MEMORY", nil)
				return err
			},
		})
	})
}
