package config

import (
	"errors"
	"fmt"
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
}

func Load() (*Config, error) {
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

	cfg := &Config{
		ServerAddress:               v.GetString("server.address"),
		GinMode:                     v.GetString("server.mode"),
		CORSAllowedOrigins:          v.GetStringSlice("server.cors_allowed_origins"),
		AdminBearerToken:            strings.TrimSpace(v.GetString("server.admin_bearer_token")),
		VerifyingKeyPath:            v.GetString("crypto.vk_path"),
		CredentialIssuerKeyPath:     v.GetString("crypto.issuer_key_path"),
		CredentialIssuerRotateAfter: time.Duration(v.GetInt("crypto.issuer_rotate_days")) * 24 * time.Hour,
		DB:                          dbCfg,
	}
	if requiresVerifyingKey(cfg.GinMode) && strings.TrimSpace(cfg.VerifyingKeyPath) == "" {
		return nil, fmt.Errorf("crypto.vk_path is required when server.mode=%q", cfg.GinMode)
	}
	if requiresVerifyingKey(cfg.GinMode) && cfg.AdminBearerToken == "" {
		return nil, fmt.Errorf("server.admin_bearer_token is required when server.mode=%q", cfg.GinMode)
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
