package persistence

import (
	"os"
	"strconv"
	"time"
)

func GetDBConfigFromEnv() *DBConfig {
	driver := getEnv("DB_DRIVER", "sqlite")
	cfg := &DBConfig{Driver: driver}

	// 连接池参数的默认值**不能**挂在 postgres 分支里：驱动可能被配置文件里的
	// db.driver 覆盖（见 config.go），那时候 env 分支拿到的还是默认的
	// sqlite —— 池参数就全成了 0，applyPoolConfig 一个都不设，database/sql
	// 于是只保留 2 条空闲连接：高并发下每个请求都要重开一条数据库连接，
	// 量到的是连接开销，不是服务端能力。
	//
	// SQLite 不用连接池（applyPoolConfig 只在 postgres 路径上调用），
	// 所以这里设了也无副作用。
	cfg.MaxOpenConns = getEnvInt("DB_MAX_OPEN_CONNS", 25)
	cfg.MaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", 5)
	cfg.ConnMaxLifetime = time.Duration(
		getEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", 3600),
	) * time.Second

	switch driver {
	case "postgres":
		cfg.Host = getEnv("DB_HOST", "localhost")
		cfg.Port = getEnvInt("DB_PORT", 5432)
		cfg.Database = getEnv("DB_NAME", "anonvote")
		cfg.Username = getEnv("DB_USER", "postgres")
		cfg.Password = getEnv("DB_PASSWORD", "")
		cfg.SSLMode = getEnv("DB_SSLMODE", "disable")
	default:
		cfg.Driver = "sqlite"
		cfg.Database = getEnv("DB_PATH", "anonvote.db")
		if os.Getenv("GO_ENV") == "test" {
			cfg.Database = ":memory:"
		}
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func getEnvInt(key string, defaultVal int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}

	num, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return num
}
