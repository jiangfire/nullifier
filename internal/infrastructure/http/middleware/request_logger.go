package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// anonymousPathParts 列出**不记录 client_ip** 的端点片段。
//
// 匿名领取凭证是一条四步链路（挑战 → 预留 → 盲签名 → 落库）：任何一步记录
// 来源 IP，都会给服务端多出一条能把领取者与凭证假名关联起来的线索 ——
// 挑战值会原样回到 claim 请求体，blind-sign / attest 与 claim 同链路可关联
// 到同一假名。所以整条链路统一豁免。请求本身仍然会被记录（方法、路径、
// 状态、耗时），只是不记来源；限流不受影响（限流键独立于请求日志）。
var anonymousPathParts = []string{
	"/credentials/claim",
	"/credentials/eligibility-challenge",
	"/credentials/blind-sign",
	"/credentials/attest",
}

func isAnonymousPath(path string) bool {
	for _, part := range anonymousPathParts {
		if strings.Contains(path, part) {
			return true
		}
	}
	return false
}

func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.String("raw_path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		}
		if !isAnonymousPath(c.FullPath()) {
			fields = append(fields, zap.String("client_ip", c.ClientIP()))
		}

		logger.Info("http request", fields...)
	}
}
