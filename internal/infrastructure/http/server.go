package httpapi

import (
	"anonvote/internal/config"
	"anonvote/internal/infrastructure/http/middleware"
	"io"
	"net"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Server struct {
	engine *gin.Engine
	addr   string
	logger *zap.Logger
}

func NewServer(cfg *config.Config, handler *Handler, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = &config.Config{
			ServerAddress: ":8080",
			GinMode:       gin.ReleaseMode,
		}
	}

	gin.SetMode(cfg.GinMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	gin.DebugPrintRouteFunc = func(string, string, string, int) {}
	engine := gin.New()
	engine.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	// 指标中间件必须排在错误处理器**之前**：gin 的中间件先进后出，越早注册的
	// 回程越晚 —— 这样它拿得到最终状态码。
	engine.Use(middleware.MetricsMiddleware(handler.Metrics()))
	engine.Use(middleware.RequestLogger(logger))
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.ErrorHandler(logger))

	// Prometheus 指标端点。
	//
	// 它要管理员令牌：指标里虽然只有计数与耗时（没有任何选票内容或身份），
	// 但「多少人投了票」「多少次资格证明没验过」本身就是运营状态 ——
	// 按仓库的一贯口径，管理面接口一律要 Bearer 令牌。
	engine.GET(
		middleware.MetricsEndpointPath,
		middleware.RequireAdminBearerToken(cfg.AdminBearerToken),
		gin.WrapH(handler.Metrics().Handler()),
	)

	engine.GET("/healthz", func(c *gin.Context) {
		// debug 标志只报运行状态：服务端是否处于开发联调模式本来就是公开事实。
		// 两种模式的行为完全一致 —— 服务端没有「替用户选票」的调试能力。
		//
		// eligibility 报出服务端**是否在强制一人一票**：没开的话任何人都能
		// 反复领票，这是运营必须能一眼看到的运行状态。
		c.JSON(200, gin.H{
			"status":      "ok",
			"debug":       cfg.DebugMode,
			"eligibility": cfg.EligibilityRoot != "",
		})
	})

	v1 := engine.Group("/api/v1")
	handler.RegisterRoutes(v1)

	admin := v1.Group("/")
	admin.Use(middleware.RequireAdminBearerToken(cfg.AdminBearerToken))
	admin.POST("/credentials", handler.ReserveCredentialForUser)
	admin.POST("/credentials/blind-sign", handler.AdminBlindSignCredential)
	admin.POST("/proposals", handler.CreateProposal)
	admin.GET("/proposals/:proposal_id/credentials", handler.ListProposalCredentialStatuses)
	admin.POST("/proposals/:proposal_id/credentials/pool", handler.FillCredentialPool)
	admin.POST("/proposals/:proposal_id/decryption/setup", handler.SetupDecryptionSession)
	admin.POST("/proposals/:proposal_id/results/publish", handler.PublishResult)

	return &Server{
		engine: engine,
		addr:   cfg.ServerAddress,
		logger: logger,
	}
}

func (s *Server) Run() error {
	s.logger.Info("http server starting", zap.String("addr", s.addr))
	return s.engine.Run(s.addr)
}

// Serve 在已有的 listener 上提供服务。生产路径走 Run；测试用临时端口的
// listener 驱动一次真实的监听、请求与关停，而不用占用固定端口。
func (s *Server) Serve(l net.Listener) error {
	return s.engine.RunListener(l)
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
