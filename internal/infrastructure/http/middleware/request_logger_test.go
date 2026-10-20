package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	return zap.New(core), logs
}

func TestRequestLogger_LogsClientIPForNormalPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, logs := newObservedLogger()

	engine := gin.New()
	engine.Use(RequestLogger(logger))
	engine.GET("/api/v1/proposals", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/proposals", nil)
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, 1, logs.Len())
	fields := logs.All()[0].ContextMap()
	require.Equal(t, "203.0.113.9", fields["client_ip"])
}

// 匿名领取凭证的端点不记录 client_ip —— 记录来源 IP 会给服务端多出一条
// 能把领取者与凭证关联起来的线索。
func TestRequestLogger_SkipsClientIPForAnonymousPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, logs := newObservedLogger()

	engine := gin.New()
	engine.Use(RequestLogger(logger))
	engine.POST("/api/v1/proposals/:proposal_id/credentials/claim", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/proposals/p-1/credentials/claim",
		strings.NewReader("{}"),
	)
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, 1, logs.Len())
	fields := logs.All()[0].ContextMap()
	require.NotContains(t, fields, "client_ip")
	// 其余信息照常记录，便于排障。
	require.Equal(t, http.MethodPost, fields["method"])
	require.Equal(t, int64(201), fields["status"])
}
