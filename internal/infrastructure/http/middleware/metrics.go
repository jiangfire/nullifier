package middleware

import (
	"time"

	"anonvote/internal/infrastructure/metrics"

	"github.com/gin-gonic/gin"
)

// MetricsEndpointPath 是 Prometheus 指标端点的路径。
const MetricsEndpointPath = "/metrics"

// unmatchedRouteLabel 是匹配不到路由时用的标签值。
//
// 不能用原始路径当标签：路径里带着提案 ID，拿它当标签值会让指标基数爆炸
// （每个提案一条序列，Prometheus 会被撑死）。
const unmatchedRouteLabel = "unmatched"

// MetricsMiddleware 记录 HTTP 请求数与耗时。
//
// ⚠️ 它必须注册在 ErrorHandler **之前**：gin 的中间件是「先进后出」—— 越早
// 注册的中间件，回程越晚。于是这里能拿到最终状态码（错误处理器写完响应之后）。
// 注册顺序反了，指标里就会全是 200。
func MetricsMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = unmatchedRouteLabel
		}
		if route == MetricsEndpointPath {
			// 采集自身的请求不计入 —— 否则请求数会被采集流量本身淹没。
			return
		}
		m.ObserveHTTPRequest(route, c.Request.Method, c.Writer.Status(), time.Since(start))
	}
}
