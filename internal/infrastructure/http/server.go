package httpapi

import (
	"anonvote/internal/config"
	"anonvote/internal/infrastructure/http/middleware"
	"io"

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
	engine.Use(middleware.RequestLogger(logger))
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.ErrorHandler(logger))

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	handler.RegisterRoutes(v1)

	admin := v1.Group("/")
	admin.Use(middleware.RequireAdminBearerToken(cfg.AdminBearerToken))
	admin.POST("/credentials", handler.IssueCredential)
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

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
