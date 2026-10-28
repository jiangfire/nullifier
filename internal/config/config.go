package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"anonvote/internal/infrastructure/crypto/eligibility"
	"anonvote/internal/infrastructure/persistence"

	"github.com/spf13/viper"
)

// ConfigFileEnvVar 是显式指定配置文件路径的环境变量。
//
// 生产配置叫 config.production.yaml，而 viper 按名字只找 config.yaml ——
// 靠改名来切换生产配置太容易出错，所以给一条明确的路径。
const ConfigFileEnvVar = "CONFIG_FILE"

// RateLimitGroupConfig 是一组端点的限流额度。
type RateLimitGroupConfig struct {
	// RPS 是每秒允许的请求数（按来源计）。
	RPS int
	// Burst 是允许的突发请求数。
	Burst int
}

// RateLimitConfig 是限流配置。
//
// 额度按**路由组**分档：凭证领取、投票提交、只读三种流量的攻击面差很多，
// 用同一套额度要么挡不住投票端点，要么把只读端点也一起拖死。
type RateLimitConfig struct {
	// Enabled 决定限流是否生效。
	//
	// release 模式下它是**硬要求**：默认开启，显式关掉会让服务启动失败。
	Enabled bool
	// Credentials 覆盖凭证领取链路（challenge / claim / blind-sign / attest）。
	Credentials RateLimitGroupConfig
	// Votes 覆盖投票提交端点 —— 每张票都要验一次 proof，最贵的一组。
	Votes RateLimitGroupConfig
	// Read 覆盖只读端点。
	Read RateLimitGroupConfig
}

// EligibilityRosterConfig 是一套资格名单的配置（crypto.eligibility_roots 列表的一项）。
//
// 一台服务端可以并存多套名单：每个名单一个名字，提案创建时绑定其中之一
// （proposals.eligibility_scope）。名为 default 的一套同时充当未显式绑定
// 名单的提案的默认名单。
type EligibilityRosterConfig struct {
	// Name 是名单域的名字，创建提案时引用它。
	Name string `mapstructure:"name"`
	// Root 是名单的 Merkle 树根（十六进制域元素）。
	Root string `mapstructure:"root"`
	// VerifyingKeyPath 是该名单对应深度电路的验证密钥路径。
	VerifyingKeyPath string `mapstructure:"vk_path"`
	// Depth 是名单树的深度（缺省取默认值 10）。
	Depth int `mapstructure:"depth"`
}

// AdminTokensConfig 是管理面的分级令牌（全部可选）。
//
// 旧的单值 admin_bearer_token 等价于 admin 档 —— 存量部署什么都不用改。
// 要分权时才需要动配置：auditor 给第三方审计（只读 + 指标），
// operator 给选举工作人员（运营全量），admin 留给部署者自己。
type AdminTokensConfig struct {
	// Admin 是管理员令牌：管理面全部端点。
	Admin string
	// Operator 是运营令牌：提案生命周期、凭证池、解密 setup、发布结果。
	Operator string
	// Auditor 是审计令牌：只读端点（凭证状态、/metrics）。
	Auditor string
}

// Config 持有服务的全部运行期配置。
type Config struct {
	ServerAddress string
	GinMode       string
	// DebugMode 表示服务运行在开发联调模式。它只作为运行状态暴露
	// （`GET /healthz` 的 `debug` 字段）：服务端没有任何「替用户选票」的
	// 调试能力，开发模式与正式模式的行为完全一致。
	DebugMode          bool
	CORSAllowedOrigins []string
	// TrustedProxies 是可信反向代理的网段（CIDR）。
	//
	// **默认空 = 不信任任何代理**：客户端 IP 取连接来源地址，
	// 伪造 X-Forwarded-For 无效。部署在反向代理后面时必须显式配置 ——
	// 否则所有请求会被算到代理的同一个 IP 上（fail-closed：偏严而不是偏松）。
	//
	// 它的另一个作用：gin 的默认行为是**信任所有代理**，那会让
	// `c.ClientIP()` 完全可伪造 —— 限流与日志里的来源地址都会失真。
	TrustedProxies          []string
	AdminBearerToken        string
	// AdminTokens 是管理面的分级令牌（v0.4.0 起可选）。
	//
	// 与旧单值并存：单值 = admin 档。两者都配置了 admin 档且**值不同**时
	// 启动失败 —— 「哪枚是管理员令牌」不该有两份答案。
	AdminTokens              AdminTokensConfig
	VerifyingKeyPath         string
	CredentialIssuerKeyPath  string
	// CredentialIssuerRotateAfter 是签发方密钥的轮换周期。
	CredentialIssuerRotateAfter time.Duration
	// RateLimit 是限流配置。
	RateLimit RateLimitConfig
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
	// EligibilityDepth 是资格名单 Merkle 树的深度（v0.4.0 起可配，默认 10）。
	//
	// 名单规模上限是 2^depth 人。root / 验证密钥 / 深度是**一套**：
	// 换名单规模要成套换（`keygen -depth N` 出新密钥、`eligibility-list
	// -depth N` 重新生成名单），单项不匹配时资格证明必然验不过。
	EligibilityDepth int
	// EligibilityRosters 是多套资格名单（v0.4.0 起可选）。
	//
	// 配了它就**不能再**配旧的单值写法（crypto.eligibility_root / vk_path /
	// depth 的显式值）—— 两种写法混在一起时「默认名单是哪套」会有歧义，
	// 宁可启动失败。单值写法继续合法，等价于一套名为 default 的名单。
	EligibilityRosters []EligibilityRosterConfig
	DB                 *persistence.DBConfig
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
	v.SetDefault("crypto.eligibility_depth", eligibility.DefaultTreeDepth)

	// 限流额度。`rate_limit.enabled` **故意不设默认值** ——
	// 它要能区分「没配」（按模式推导）与「显式配了 false」（release 下是错误）。
	//
	// 默认额度偏宽松：宁可放过一次攻击，也不要在真实投票窗口里把投票人挡在门外。
	// 真实部署规模决定最终值（推导过程见 docs/t25-ratelimit-design.md）。
	v.SetDefault("rate_limit.credentials.rps", 10)
	v.SetDefault("rate_limit.credentials.burst", 40)
	v.SetDefault("rate_limit.votes.rps", 10)
	v.SetDefault("rate_limit.votes.burst", 40)
	v.SetDefault("rate_limit.read.rps", 50)
	v.SetDefault("rate_limit.read.burst", 100)

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
		AdminTokens: AdminTokensConfig{
			Admin:    strings.TrimSpace(v.GetString("server.admin_tokens.admin")),
			Operator: strings.TrimSpace(v.GetString("server.admin_tokens.operator")),
			Auditor:  strings.TrimSpace(v.GetString("server.admin_tokens.auditor")),
		},
		VerifyingKeyPath:            v.GetString("crypto.vk_path"),
		CredentialIssuerKeyPath:     v.GetString("crypto.issuer_key_path"),
		CredentialIssuerRotateAfter: time.Duration(v.GetInt("crypto.issuer_rotate_days")) * 24 * time.Hour,
		EligibilityRoot:             strings.TrimSpace(v.GetString("crypto.eligibility_root")),
		EligibilityVerifyingKeyPath: strings.TrimSpace(v.GetString("crypto.eligibility_vk_path")),
		EligibilityDepth:            v.GetInt("crypto.eligibility_depth"),
		DB:                          dbCfg,
		AutoMigrate:                 !IsReleaseMode(ginMode),
		SourceFile:                  v.ConfigFileUsed(),
	}

	// 多套资格名单（可选）。列表里的每套都要名字 / 根 / 验证密钥齐全；
	// 与旧的单值写法互斥 —— 混用「默认名单是哪套」就有歧义。
	if err := v.UnmarshalKey("crypto.eligibility_roots", &cfg.EligibilityRosters); err != nil {
		return nil, fmt.Errorf("parse crypto.eligibility_roots: %w", err)
	}
	if err := validateEligibilityRosters(cfg.EligibilityRosters); err != nil {
		return nil, err
	}
	if len(cfg.EligibilityRosters) > 0 &&
		(cfg.EligibilityRoot != "" || cfg.EligibilityVerifyingKeyPath != "") {
		return nil, errors.New(
			"crypto.eligibility_roots and crypto.eligibility_root/vk_path are mutually exclusive: " +
				"configure either the roster list or the single legacy roster, not both",
		)
	}
	// 显式配置优先于按模式推导的默认值；环境变量优先于配置文件。
	if raw := strings.TrimSpace(v.GetString("server.auto_migrate")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("server.auto_migrate must be a boolean: %w", err)
		}
		cfg.AutoMigrate = parsed
	}

	// 限流开关。没配就按模式推导：release 默认开，debug 默认关。
	// 显式配成 false 要记下来 —— release 模式下那是要被拒绝的。
	rateLimitEnabled := IsReleaseMode(ginMode)
	rateLimitDisabled := false
	if raw := strings.TrimSpace(v.GetString("rate_limit.enabled")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("rate_limit.enabled must be a boolean: %w", err)
		}
		rateLimitEnabled = parsed
		rateLimitDisabled = !parsed
	}
	rateLimit := RateLimitConfig{
		Enabled:     rateLimitEnabled,
		Credentials: loadRateLimitGroup(v, "credentials"),
		Votes:       loadRateLimitGroup(v, "votes"),
		Read:        loadRateLimitGroup(v, "read"),
	}
	if err := validateRateLimit(rateLimit); err != nil {
		return nil, err
	}
	cfg.RateLimit = rateLimit
	cfg.TrustedProxies = v.GetStringSlice("server.trusted_proxies")
	if err := validateTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	if !forMigration && IsReleaseMode(cfg.GinMode) {
		if strings.TrimSpace(cfg.VerifyingKeyPath) == "" {
			return nil, fmt.Errorf("crypto.vk_path is required when server.mode=%q", cfg.GinMode)
		}
		// 一人一票是 release 模式的硬要求：没有名单根与资格验证密钥，服务端
		// 就无法强制「同一个人只能领一张票」—— 那时它只是个能发凭证的服务器。
		// 单名单（旧写法）与多名单（列表写法）二选一，上面已保证互斥。
		if len(cfg.EligibilityRosters) == 0 &&
			(cfg.EligibilityRoot == "" || cfg.EligibilityVerifyingKeyPath == "") {
			return nil, fmt.Errorf(
				"crypto.eligibility_root and crypto.eligibility_vk_path are required when server.mode=%q",
				cfg.GinMode,
			)
		}
		// 生产模式下临时 token 每次重启都会变，多副本之间还不一样，
		// 会让运营门户与 CLI 随机失联 —— 必须显式配置。
		// 旧单值与分级令牌（admin / operator / auditor 任一）都算「显式配置」。
		if cfg.AdminBearerToken == "" && cfg.AdminTokens == (AdminTokensConfig{}) {
			return nil, fmt.Errorf(
				"server.admin_bearer_token is required when server.mode=%q (set SERVER_ADMIN_BEARER_TOKEN)",
				cfg.GinMode,
			)
		}
		// 两处都配了 admin 档且值不同 → 启动失败：哪枚是管理员令牌不该有
		// 两份答案（同值则等价于配了一处，放行）。
		if cfg.AdminTokens.Admin != "" && cfg.AdminBearerToken != "" &&
			cfg.AdminTokens.Admin != cfg.AdminBearerToken {
			return nil, errors.New(
				"server.admin_tokens.admin and server.admin_bearer_token are both configured with different values; keep one of them",
			)
		}
		// 限流是 release 模式的硬要求：公开端点上最贵的操作是投票
		// （每张票都要验一次 Groth16 proof），不设上限时一个脚本就能把
		// 验签算力打满。默认就是开的 —— 显式关掉则启动失败。
		//
		// 用「默认开 + 显式关才失败」而不是「不配就失败」：漏配的后果是
		// 「没限流」，而硬失败会让所有存量 release 部署升级即宕机。
		if rateLimitDisabled {
			return nil, fmt.Errorf(
				"rate_limit.enabled cannot be disabled when server.mode=%q",
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

	// 树深配错同样是启动失败：深度的合法范围由资格电路包定义（4–24），
	// 配了范围外的值说明部署者对名单规模的认知与实现脱节了。
	if err := eligibility.ValidateTreeDepth(cfg.EligibilityDepth); err != nil {
		return nil, fmt.Errorf("crypto.eligibility_depth: %w", err)
	}

	return cfg, nil
}

// IsReleaseMode 判断运行模式是否属于生产模式。
// 生产模式要求显式配置密钥与管理员令牌，不自动改表，也不开调试端点。
func IsReleaseMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "release", "prod", "production":
		return true
	default:
		return false
	}
}

// loadRateLimitGroup 读一组限流额度。
func loadRateLimitGroup(v *viper.Viper, group string) RateLimitGroupConfig {
	return RateLimitGroupConfig{
		RPS:   v.GetInt("rate_limit." + group + ".rps"),
		Burst: v.GetInt("rate_limit." + group + ".burst"),
	}
}

// validateTrustedProxies 校验可信代理网段。
//
// 配错网段的后果是「限流按错误的来源计数」—— 与其带着一个假象中的限流
// 上线，不如启动时就说清楚。
func validateTrustedProxies(proxies []string) error {
	for _, p := range proxies {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(trimmed); err != nil {
			return fmt.Errorf("server.trusted_proxies contains an invalid CIDR %q: %w", trimmed, err)
		}
	}
	return nil
}

// validateRateLimit 校验限流额度。
//
// 配错比不配更危险：额度为 0 意味着「全拒」（服务直接不可用），
// 负数同样没有意义 —— 所以一律启动失败，不给「静默兜底」的机会。
//
// 限流没启用时不校验：那时这些数字根本不参与决策。
func validateRateLimit(cfg RateLimitConfig) error {
	if !cfg.Enabled {
		return nil
	}
	groups := []struct {
		name  string
		group RateLimitGroupConfig
	}{
		{"credentials", cfg.Credentials},
		{"votes", cfg.Votes},
		{"read", cfg.Read},
	}
	for _, g := range groups {
		if g.group.RPS <= 0 {
			return fmt.Errorf("rate_limit.%s.rps must be positive, got %d", g.name, g.group.RPS)
		}
		if g.group.Burst <= 0 {
			return fmt.Errorf("rate_limit.%s.burst must be positive, got %d", g.name, g.group.Burst)
		}
	}
	return nil
}

// validateEligibilityRosters 校验名单列表。
//
// 每套名单要名字 / 根 / 验证密钥齐全、深度合法；名字不能重复 —— 重名时
// 「提案绑定的是哪一套」无从回答，宁可启动失败。深度缺省取默认值
// （2^10 = 1024 人的存量口径）。
func validateEligibilityRosters(rosters []EligibilityRosterConfig) error {
	seen := map[string]struct{}{}
	for i := range rosters {
		roster := &rosters[i]
		roster.Name = strings.TrimSpace(roster.Name)
		roster.Root = strings.TrimSpace(roster.Root)
		roster.VerifyingKeyPath = strings.TrimSpace(roster.VerifyingKeyPath)
		if roster.Name == "" {
			return fmt.Errorf("crypto.eligibility_roots[%d]: name is required", i)
		}
		if roster.Root == "" {
			return fmt.Errorf("crypto.eligibility_roots[%d] (%s): root is required", i, roster.Name)
		}
		if roster.VerifyingKeyPath == "" {
			return fmt.Errorf("crypto.eligibility_roots[%d] (%s): vk_path is required", i, roster.Name)
		}
		if roster.Depth == 0 {
			roster.Depth = eligibility.DefaultTreeDepth
		}
		if err := eligibility.ValidateTreeDepth(roster.Depth); err != nil {
			return fmt.Errorf("crypto.eligibility_roots[%d] (%s): %w", i, roster.Name, err)
		}
		if _, duplicated := seen[roster.Name]; duplicated {
			return fmt.Errorf("crypto.eligibility_roots: roster %q is configured more than once", roster.Name)
		}
		seen[roster.Name] = struct{}{}
	}
	return nil
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
