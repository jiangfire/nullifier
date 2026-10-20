package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// anonymousPathParts 列出**不记录 client_ip** 的端点片段。
//
// 匿名领取凭证时，记录来源 IP 会让服务端多出一条能把领取者与凭证关联起来的
// 线索 —— 那正是这个端点要消除的东西。请求本身仍然会被记录（方法、路径、
// 状态、耗时），只是不记来源。
var anonymousPathParts = []string{
	"/credentials/claim",
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
