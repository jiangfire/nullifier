package main

import (
	"strings"

	"anonvote/internal/config"
	"anonvote/internal/di"

	"go.uber.org/zap"
)

// version 由构建时的 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	bootstrapLogger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = bootstrapLogger.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Fatal("load config", zap.Error(err))
	}
	generatedAdminToken, err := config.EnsureAdminBearerToken(cfg)
	if err != nil {
		bootstrapLogger.Fatal("prepare admin token", zap.Error(err))
	}

	logger, err := di.NewLogger(cfg)
	if err != nil {
		bootstrapLogger.Fatal("create logger", zap.Error(err))
	}
	defer func() { _ = logger.Sync() }()

	// 打印实际生效的配置：最常见的上线事故是「以为加载了生产配置，实际跑的是开发配置」。
	adminTokenSource := "configured"
	if generatedAdminToken {
		adminTokenSource = "generated"
	}
	logger.Info("effective configuration",
		zap.String("version", version),
		zap.String("server_mode", cfg.GinMode),
		zap.String("server_address", cfg.ServerAddress),
		zap.String("db_driver", cfg.DB.Driver),
		zap.String("db_database", cfg.DB.Database),
		zap.Int("db_max_open_conns", cfg.DB.MaxOpenConns),
		zap.Int("db_max_idle_conns", cfg.DB.MaxIdleConns),
		zap.Duration("db_conn_max_lifetime", cfg.DB.ConnMaxLifetime),
		zap.Int("cors_allowed_origins", len(cfg.CORSAllowedOrigins)),
		zap.String("admin_token_source", adminTokenSource),
		zap.Bool("proof_verification_enabled", strings.TrimSpace(cfg.VerifyingKeyPath) != ""),
	)

	// 临时 token 只对当前进程有效，必须让运维看见；
	// 但配置态的 token 是长期密钥，不能进日志。
	if generatedAdminToken {
		logger.Warn(
			"server.admin_bearer_token is empty; generated a temporary admin token for this process",
			zap.String("admin_token", cfg.AdminBearerToken),
		)
	}

	if strings.TrimSpace(cfg.VerifyingKeyPath) == "" {
		logger.Warn(
			"crypto.vk_path is empty: vote proof verification is DISABLED, any non-empty proof will be accepted",
		)
	}

	server, err := di.InitializeServer(cfg)
	if err != nil {
		logger.Fatal("initialize server", zap.Error(err))
	}

	if err := server.Run(); err != nil {
		logger.Fatal("run server", zap.Error(err))
	}
}
