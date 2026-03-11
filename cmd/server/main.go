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

	logger, err := di.NewLogger(cfg)
	if err != nil {
		bootstrapLogger.Fatal("create logger", zap.Error(err))
	}
	defer func() { _ = logger.Sync() }()

	server, err := di.InitializeServer(cfg)
	if err != nil {
		logger.Fatal("initialize server", zap.Error(err))
	}

	if err := server.Run(); err != nil {
		logger.Fatal("run server", zap.Error(err))
	}
}
