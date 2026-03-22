package main

import (
	"anonvote/internal/config"
	"anonvote/internal/di"

	"go.uber.org/zap"
)

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
	if generatedAdminToken {
		bootstrapLogger.Warn(
			"server.admin_bearer_token is empty; generated a temporary admin token for this process",
			zap.String("admin_token", cfg.AdminBearerToken),
		)
	}

	logger, err := di.NewLogger(cfg)
	if err != nil {
		bootstrapLogger.Fatal("create logger", zap.Error(err))
	}
	defer func() { _ = logger.Sync() }()

	adminTokenSource := "configured"
	adminTokenLogLevel := logger.Info
	if generatedAdminToken {
		adminTokenSource = "generated"
		adminTokenLogLevel = logger.Warn
	}
	adminTokenLogLevel(
		"admin bearer token ready",
		zap.String("source", adminTokenSource),
		zap.String("admin_token", cfg.AdminBearerToken),
	)

	server, err := di.InitializeServer(cfg)
	if err != nil {
		logger.Fatal("initialize server", zap.Error(err))
	}

	if err := server.Run(); err != nil {
		logger.Fatal("run server", zap.Error(err))
	}
}
