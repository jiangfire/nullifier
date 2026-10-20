package persistence

import (
	"os"
	"strconv"
	"time"
)

func GetDBConfigFromEnv() *DBConfig {
	driver := getEnv("DB_DRIVER", "sqlite")
	cfg := &DBConfig{Driver: driver}

	switch driver {
	case "postgres":
		cfg.Host = getEnv("DB_HOST", "localhost")
		cfg.Port = getEnvInt("DB_PORT", 5432)
		cfg.Database = getEnv("DB_NAME", "anonvote")
		cfg.Username = getEnv("DB_USER", "postgres")
		cfg.Password = getEnv("DB_PASSWORD", "")
		cfg.SSLMode = getEnv("DB_SSLMODE", "disable")
		cfg.MaxOpenConns = getEnvInt("DB_MAX_OPEN_CONNS", 25)
		cfg.MaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", 5)
		cfg.ConnMaxLifetime = time.Duration(
			getEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", 3600),
		) * time.Second
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
