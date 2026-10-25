package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"anonvote/internal/config"
	"anonvote/internal/di"
	httpapi "anonvote/internal/infrastructure/http"

	"go.uber.org/zap"
)

// version 由构建时的 -ldflags "-X main.version=..." 注入。
var version = "dev"

// startServer 启动 HTTP 服务。做成包级变量，是为了让测试能换成临时端口的
// listener，驱动一次真实的启动、请求与关停。
var startServer = func(s *httpapi.Server, addr string) error {
	return s.Run()
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 解析参数并启动服务，返回进程退出码。main 里只做 os.Exit，
// 逻辑放在这里是为了让启动流程能进单元测试。
func run(args []string) int {
	fs := flag.NewFlagSet("nullifier-server", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	migrateOnly := fs.Bool(
		"migrate",
		false,
		"run database schema migrations and exit (run this before starting the server)",
	)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, "用法: nullifier-server [-migrate]")
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	if err := serve(*migrateOnly); err != nil {
		// serve 内部已经用 zap 记录了失败原因，这里只决定退出码。
		return 1
	}
	return 0
}

// serve 加载配置、按需迁移并启动服务；失败时返回错误而不是直接退出，
// 好让调用方（和测试）决定怎么处理。
func serve(migrateOnly bool) error {
	bootstrapLogger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	defer func() { _ = bootstrapLogger.Sync() }()

	// 迁移步骤只需要数据库连接，不该被 release 模式才需要的密钥卡住。
	var cfg *config.Config
	if migrateOnly {
		cfg, err = config.LoadForMigration()
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		bootstrapLogger.Error("load config", zap.Error(err))
		return err
	}

	logger, err := di.NewLogger(cfg)
	if err != nil {
		bootstrapLogger.Error("create logger", zap.Error(err))
		return err
	}
	defer func() { _ = logger.Sync() }()

	if migrateOnly {
		if err := di.Migrate(cfg); err != nil {
			logger.Error("run schema migrations", zap.Error(err))
			return err
		}
		logger.Info("schema migrations completed", zap.String("db_database", cfg.DB.Database))
		return nil
	}

	generatedAdminToken, err := config.EnsureAdminBearerToken(cfg)
	if err != nil {
		logger.Error("prepare admin token", zap.Error(err))
		return err
	}

	// 打印实际生效的配置：最常见的上线事故是「以为加载了生产配置，实际跑的是开发配置」。
	adminTokenSource := "configured"
	if generatedAdminToken {
		adminTokenSource = "generated"
	}
	logger.Info("effective configuration",
		zap.String("version", version),
		zap.String("config_file", cfg.SourceFile),
		zap.String("server_mode", cfg.GinMode),
		zap.String("server_address", cfg.ServerAddress),
		zap.String("db_driver", cfg.DB.Driver),
		zap.String("db_database", cfg.DB.Database),
		zap.Int("db_max_open_conns", cfg.DB.MaxOpenConns),
		zap.Int("db_max_idle_conns", cfg.DB.MaxIdleConns),
		zap.Duration("db_conn_max_lifetime", cfg.DB.ConnMaxLifetime),
		zap.Bool("db_auto_migrate", cfg.AutoMigrate),
		zap.Int("cors_allowed_origins", len(cfg.CORSAllowedOrigins)),
		zap.String("admin_token_source", adminTokenSource),
		zap.Bool("proof_verification_enabled", strings.TrimSpace(cfg.VerifyingKeyPath) != ""),
		// 一人一票是不是真的开着：这是运营最需要一眼看到的运行状态。
		zap.Bool("eligibility_enforced", strings.TrimSpace(cfg.EligibilityRoot) != ""),
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

	// CORS 留空只有一种合法场景：门户与后端同源部署。不同源时浏览器会拦掉
	// 所有跨域请求 —— 这类事故在启动日志里看不出来，所以要在这里喊一声。
	warnIfCORSEmpty(logger, cfg)

	// 关闭自动迁移时，运维必须知道表结构得自己先迁。
	if !cfg.AutoMigrate {
		logger.Warn(
			"schema auto-migration is DISABLED: run `nullifier-server -migrate` before starting the service",
		)
	}

	server, err := di.InitializeServer(cfg)
	if err != nil {
		logger.Error("initialize server", zap.Error(err))
		return err
	}

	if err := startServer(server, cfg.ServerAddress); err != nil {
		logger.Error("run server", zap.Error(err))
		return err
	}
	return nil
}

// warnIfCORSEmpty 在 release 模式下 CORS 白名单为空时告警。
//
// 空 CORS 只有一种合法场景：门户与后端同源部署。不同源时浏览器会拦掉所有
// 跨域请求 —— 这类事故在启动日志里看不出来，所以要在这里喊一声。
// 不阻止启动：同源部署留空是合法的。
func warnIfCORSEmpty(logger *zap.Logger, cfg *config.Config) {
	if !config.IsReleaseMode(cfg.GinMode) || len(cfg.CORSAllowedOrigins) > 0 {
		return
	}
	logger.Warn(
		"server.cors_allowed_origins is empty: a portal served from another origin will be blocked by the browser",
	)
}
