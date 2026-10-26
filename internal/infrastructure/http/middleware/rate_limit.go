package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"anonvote/internal/infrastructure/metrics"
	"anonvote/internal/infrastructure/ratelimit"

	"github.com/gin-gonic/gin"
)

// RateLimit 返回一个限流中间件。
//
// 限流是**传输层**的保护措施，不是业务失败：它自己写 429 响应，
// 不经过 MapError —— 那张表映射的是领域错误（usecase.Err*），
// 把限流塞进去会污染 usecase 的语义。但**文案**仍然跟其它错误码共用
// publicErrorMessage，客户端看到的还是同一套错误体。
//
// limiter 为 nil 时直接放行：那是没有配置限流（开发模式、单元测试）的正常路径。
func RateLimit(limiter *ratelimit.Limiter, group string, m *metrics.Metrics) gin.HandlerFunc {
	if limiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		allowed, retryAfter := limiter.Allow(rateLimitKey(c, group))
		if allowed {
			c.Next()
			return
		}
		if m != nil {
			m.RecordRateLimited(routeLabel(c))
		}
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": publicErrorMessage(nil, RateLimitedCode),
			"code":  RateLimitedCode,
		})
	}
}

// rateLimitKey 拼出限流键：组 + 提案 + 来源。
//
// 提案 ID 在里面：多个提案同时开放投票时，A 提案的投票人不该吃掉
// B 提案的额度。路径里没有提案的端点（如 GET /proposals）这一段留空。
//
// 来源用 c.ClientIP() —— 它是否可信取决于 server.trusted_proxies 的配置：
// 默认不信任任何代理，伪造 X-Forwarded-For 无效。
func rateLimitKey(c *gin.Context, group string) string {
	return group + "\x00" + c.Param("proposal_id") + "\x00" + c.ClientIP()
}

// routeLabel 返回指标用的路由标签。
//
// 用**路由模板**（/api/v1/proposals/:proposal_id/votes）而不是原始路径 ——
// 否则每个提案 ID 都会变成一个标签值，标签集会爆。
func routeLabel(c *gin.Context) string {
	if full := c.FullPath(); full != "" {
		return full
	}
	return "unmatched"
}

// retryAfterSeconds 把等待时长换算成 Retry-After 头要的秒数。
//
// 向上取整且最小为 1：告诉客户端「0 秒后重试」等于让它立刻重试，
// 那只是把限流变成一个自旋。
func retryAfterSeconds(d time.Duration) int {
	seconds := int(math.Ceil(d.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}
