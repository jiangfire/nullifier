package httpapi

import (
	"anonvote/internal/config"
	"anonvote/internal/infrastructure/http/middleware"

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
	engine := gin.New()
	engine.Use(middleware.CORS())
	engine.Use(gin.Recovery())
	engine.Use(middleware.ErrorHandler(logger))

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	handler.RegisterRoutes(v1)

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
