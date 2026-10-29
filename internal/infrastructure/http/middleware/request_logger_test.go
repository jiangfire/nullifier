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

// 整条匿名领取链路的端点都不记录 client_ip —— 记录来源 IP 会给服务端多出
// 能把领取者与凭证假名关联起来的线索（F3）：挑战值会原样回到 claim 请求体，
// blind-sign / attest 与 claim 同链路可关联到同一假名。
func TestRequestLogger_SkipsClientIPForAnonymousPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths := []string{
		"/api/v1/proposals/p-1/credentials/claim",
		"/api/v1/proposals/p-1/credentials/eligibility-challenge",
		"/api/v1/proposals/p-1/credentials/blind-sign",
		"/api/v1/proposals/p-1/credentials/attest",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			logger, logs := newObservedLogger()

			engine := gin.New()
			engine.Use(RequestLogger(logger))
			engine.POST(path, func(c *gin.Context) {
				c.Status(http.StatusCreated)
			})

			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			req.RemoteAddr = "203.0.113.9:12345"
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			require.Equal(t, 1, logs.Len())
			fields := logs.All()[0].ContextMap()
			require.NotContains(t, fields, "client_ip")
			// 其余信息照常记录，便于排障。
			require.Equal(t, http.MethodPost, fields["method"])
			require.Equal(t, int64(201), fields["status"])
		})
	}
}
