package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"anonvote/internal/infrastructure/persistence"

	"github.com/spf13/viper"
)

type Config struct {
	ServerAddress               string
	GinMode                     string
	CORSAllowedOrigins          []string
	AdminBearerToken            string
	VerifyingKeyPath            string
	CredentialIssuerKeyPath     string
	CredentialIssuerRotateAfter time.Duration
	DB                          *persistence.DBConfig
	// AutoMigrate 决定启动时是否自动改表。
	// release 模式默认关闭：多副本同时启动会争 DDL 锁，而且改表应该是
	// 一个能单独执行、能单独回滚的步骤，而不是服务启动的副作用。
	AutoMigrate bool
}

func Load() (*Config, error) {
	return load(false)
}

// LoadForMigration 加载配置，但不要求 release 模式下的 vk_path 与 admin token。
// 迁移步骤只需要数据库连接，不该被应用运行时的密钥卡住。
func LoadForMigration() (*Config, error) {
	return load(true)
}

func load(forMigration bool) (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("server.address", ":8080")
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.cors_allowed_origins", []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	})
	v.SetDefault("server.admin_bearer_token", "")
	v.SetDefault("crypto.vk_path", "")
	v.SetDefault("crypto.issuer_key_path", "issuer_keys.json")
	v.SetDefault("crypto.issuer_rotate_days", 90)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	dbCfg := persistence.GetDBConfigFromEnv()
	if driver := v.GetString("db.driver"); driver != "" {
		dbCfg.Driver = driver
	}
	if v.GetString("db.path") != "" {
		dbCfg.Database = v.GetString("db.path")
	}
	if host := v.GetString("db.host"); host != "" {
		dbCfg.Host = host
	}
	if port := v.GetInt("db.port"); port > 0 {
		dbCfg.Port = port
	}
	if name := v.GetString("db.name"); name != "" {
		dbCfg.Database = name
	}
	if user := v.GetString("db.user"); user != "" {
		dbCfg.Username = user
	}
	if pass := v.GetString("db.password"); pass != "" {
		dbCfg.Password = pass
	}
	if ssl := v.GetString("db.sslmode"); ssl != "" {
		dbCfg.SSLMode = ssl
	}
	// 连接池参数。优先级与其它 db.* 一致：环境变量高于配置文件；
	// 两边都没给时保留 GetDBConfigFromEnv 里的默认值。
	if n := v.GetInt("db.max_open_conns"); n > 0 {
		dbCfg.MaxOpenConns = n
	}
	if n := v.GetInt("db.max_idle_conns"); n > 0 {
		dbCfg.MaxIdleConns = n
	}
	if n := v.GetInt("db.conn_max_lifetime_seconds"); n > 0 {
		dbCfg.ConnMaxLifetime = time.Duration(n) * time.Second
	}

	ginMode := v.GetString("server.mode")
	cfg := &Config{
		ServerAddress:               v.GetString("server.address"),
		GinMode:                     ginMode,
		CORSAllowedOrigins:          v.GetStringSlice("server.cors_allowed_origins"),
		AdminBearerToken:            strings.TrimSpace(v.GetString("server.admin_bearer_token")),
		VerifyingKeyPath:            v.GetString("crypto.vk_path"),
		CredentialIssuerKeyPath:     v.GetString("crypto.issuer_key_path"),
		CredentialIssuerRotateAfter: time.Duration(v.GetInt("crypto.issuer_rotate_days")) * 24 * time.Hour,
		DB:                          dbCfg,
		AutoMigrate:                 !requiresVerifyingKey(ginMode),
	}
	// 显式配置优先于按模式推导的默认值；环境变量优先于配置文件。
	if raw := strings.TrimSpace(v.GetString("server.auto_migrate")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("server.auto_migrate must be a boolean: %w", err)
		}
		cfg.AutoMigrate = parsed
	}
	if !forMigration && requiresVerifyingKey(cfg.GinMode) {
		if strings.TrimSpace(cfg.VerifyingKeyPath) == "" {
			return nil, fmt.Errorf("crypto.vk_path is required when server.mode=%q", cfg.GinMode)
		}
		// 生产模式下临时 token 每次重启都会变，多副本之间还不一样，
		// 会让运营门户与 CLI 随机失联 —— 必须显式配置。
		if strings.TrimSpace(cfg.AdminBearerToken) == "" {
			return nil, fmt.Errorf(
				"server.admin_bearer_token is required when server.mode=%q (set SERVER_ADMIN_BEARER_TOKEN)",
				cfg.GinMode,
			)
		}
	}

	return cfg, nil
}

func requiresVerifyingKey(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "release", "prod", "production":
		return true
	default:
		return false
	}
}
