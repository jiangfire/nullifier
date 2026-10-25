package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"anonvote/internal/infrastructure/persistence"

	"github.com/spf13/viper"
)

// ConfigFileEnvVar 是显式指定配置文件路径的环境变量。
//
// 生产配置叫 config.production.yaml，而 viper 按名字只找 config.yaml ——
// 靠改名来切换生产配置太容易出错，所以给一条明确的路径。
const ConfigFileEnvVar = "CONFIG_FILE"

type Config struct {
	ServerAddress string
	GinMode       string
	// DebugMode 表示服务运行在开发联调模式。它只作为运行状态暴露
	// （`GET /healthz` 的 `debug` 字段）：服务端没有任何「替用户选票」的
	// 调试能力，开发模式与正式模式的行为完全一致。
	DebugMode                   bool
	CORSAllowedOrigins          []string
	AdminBearerToken            string
	VerifyingKeyPath            string
	CredentialIssuerKeyPath     string
	CredentialIssuerRotateAfter time.Duration
	// EligibilityRoot 是资格名单的 Merkle 树根（十六进制域元素）。
	//
	// 服务端只认这个根：它能验证「这个人的资格在树里」，却无法把资格对应到
	// 任何成员（服务端没有名单，也没有成员包）。
	EligibilityRoot string
	// EligibilityVerifyingKeyPath 是资格电路的验证密钥路径。
	//
	// 与 crypto.vk_path 一样，它只配了根不配密钥是不允许的 —— 那等于
	// 「不校验」，而一人一票恰恰是靠这个校验兑现的。
	EligibilityVerifyingKeyPath string
	DB                          *persistence.DBConfig
	// AutoMigrate 决定启动时是否自动改表。
	// release 模式默认关闭：多副本同时启动会争 DDL 锁，而且改表应该是
	// 一个能单独执行、能单独回滚的步骤，而不是服务启动的副作用。
	AutoMigrate bool
	// SourceFile 是实际加载的配置文件路径（没有配置文件时为空）。
	// 启动日志会打印它：最常见的上线事故是「以为加载了生产配置，
	// 实际跑的是开发配置」。
	SourceFile string
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
	// 配置文件路径：默认找名为 config.yaml 的文件；CONFIG_FILE 可以显式指定。
	//
	// 生产配置叫 config.production.yaml，viper 的名字匹配找不到它 —— 靠改名
	// 来启用生产配置太容易出错（覆盖开发配置、或忘了改回来），所以给一条
	// 明确的路径。
	explicitFile := strings.TrimSpace(os.Getenv(ConfigFileEnvVar))
	if explicitFile != "" {
		// 显式路径：由文件扩展名决定格式（yaml / json / toml 都认）。
		v.SetConfigFile(explicitFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
	}
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
	v.SetDefault("crypto.eligibility_root", "")
	v.SetDefault("crypto.eligibility_vk_path", "")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			// 显式指定的文件不存在时这里是硬错误：静默回退到开发配置，
			// 正是最危险的上线事故。
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
		DebugMode:                   isDebugMode(ginMode),
		CORSAllowedOrigins:          v.GetStringSlice("server.cors_allowed_origins"),
		AdminBearerToken:            strings.TrimSpace(v.GetString("server.admin_bearer_token")),
		VerifyingKeyPath:            v.GetString("crypto.vk_path"),
		CredentialIssuerKeyPath:     v.GetString("crypto.issuer_key_path"),
		CredentialIssuerRotateAfter: time.Duration(v.GetInt("crypto.issuer_rotate_days")) * 24 * time.Hour,
		EligibilityRoot:             strings.TrimSpace(v.GetString("crypto.eligibility_root")),
		EligibilityVerifyingKeyPath: strings.TrimSpace(v.GetString("crypto.eligibility_vk_path")),
		DB:                          dbCfg,
		AutoMigrate:                 !IsReleaseMode(ginMode),
		SourceFile:                  v.ConfigFileUsed(),
	}
	// 显式配置优先于按模式推导的默认值；环境变量优先于配置文件。
	if raw := strings.TrimSpace(v.GetString("server.auto_migrate")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("server.auto_migrate must be a boolean: %w", err)
		}
		cfg.AutoMigrate = parsed
	}
	if !forMigration && IsReleaseMode(cfg.GinMode) {
		if strings.TrimSpace(cfg.VerifyingKeyPath) == "" {
			return nil, fmt.Errorf("crypto.vk_path is required when server.mode=%q", cfg.GinMode)
		}
		// 一人一票是 release 模式的硬要求：没有名单根与资格验证密钥，服务端
		// 就无法强制「同一个人只能领一张票」—— 那时它只是个能发凭证的服务器。
		if cfg.EligibilityRoot == "" || cfg.EligibilityVerifyingKeyPath == "" {
			return nil, fmt.Errorf(
				"crypto.eligibility_root and crypto.eligibility_vk_path are required when server.mode=%q",
				cfg.GinMode,
			)
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

	// 名单根与验证密钥必须成对出现：只配根等于「不校验」，只配密钥则无从校验。
	if (cfg.EligibilityRoot == "") != (cfg.EligibilityVerifyingKeyPath == "") {
		return nil, errors.New(
			"crypto.eligibility_root and crypto.eligibility_vk_path must be configured together",
		)
	}

	return cfg, nil
}

// IsReleaseMode 判断运行模式是否属于生产模式。
//
// 生产模式要求显式配置密钥与管理员令牌，不自动改表，也不开调试端点。
func IsReleaseMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "release", "prod", "production":
		return true
	default:
		return false
	}
}

// isDebugMode 判断服务是否运行在开发联调模式。
//
// 它只影响 `GET /healthz` 报出的运行状态：服务端在两种模式下的行为
// 完全一致 —— 选票的加密与证明都在客户端完成。
func isDebugMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "debug", "dev", "development":
		return true
	default:
		return false
	}
}
