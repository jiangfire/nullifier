package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anonvote/internal/infrastructure/metrics"
	"anonvote/internal/infrastructure/ratelimit"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// newRateLimitEngine 造一个带限流中间件的引擎。
//
// 路由按提案分组：/proposals/:proposal_id/votes —— 限流键里带提案 ID，
// 不同提案的额度互不干扰。
func newRateLimitEngine(limiter *ratelimit.Limiter, m *metrics.Metrics) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/proposals/:proposal_id/votes", RateLimit(limiter, "votes", m), func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "ok"})
	})
	return engine
}

// doVote 发一个投票请求，来源地址由 remoteAddr 决定。
func doVote(engine *gin.Engine, proposalID, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/"+proposalID+"/votes", nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit_AllowsUntilQuotaThenReturns429(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 2})
	engine := newRateLimitEngine(limiter, nil)

	require.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.1:1000").Code)
	require.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.1:1000").Code)

	rec := doVote(engine, "p1", "203.0.113.1:1000")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"), "429 必须带 Retry-After")
	assert.Contains(t, rec.Body.String(), `"code":"RATE_LIMITED"`)
	assert.Contains(t, rec.Body.String(), "rate limit exceeded")
}

func TestRateLimit_DifferentClientsAreIndependent(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1})
	engine := newRateLimitEngine(limiter, nil)

	require.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.1:1000").Code)
	require.Equal(t, http.StatusTooManyRequests, doVote(engine, "p1", "203.0.113.1:1000").Code)

	// 另一个来源不受影响 —— 这条是路线图 T25 的验收「不同 IP 互不影响」。
	assert.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.2:1000").Code)
}

func TestRateLimit_DifferentProposalsAreIndependent(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1})
	engine := newRateLimitEngine(limiter, nil)

	require.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.1:1000").Code)
	require.Equal(t, http.StatusTooManyRequests, doVote(engine, "p1", "203.0.113.1:1000").Code)

	// A 提案的投票人不该吃掉 B 提案的额度。
	assert.Equal(t, http.StatusCreated, doVote(engine, "p2", "203.0.113.1:1000").Code)
}

func TestRateLimit_NilLimiterPassesThrough(t *testing.T) {
	t.Parallel()

	engine := newRateLimitEngine(nil, nil)
	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusCreated, doVote(engine, "p1", "203.0.113.1:1000").Code)
	}
}

func TestRateLimit_RecordsMetric(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1})
	engine := newRateLimitEngine(limiter, m)

	doVote(engine, "p1", "203.0.113.1:1000")
	doVote(engine, "p1", "203.0.113.1:1000")

	assert.Equal(t, 1.0, counterValue(t, m, "anonvote_rate_limited_total"))
}

// TestRateLimit_MetricUsesRouteTemplate 钉住 route 标签用**路由模板**：
// 用原始路径的话，每个提案 ID 都会变成一个标签值，标签集会爆。
func TestRateLimit_MetricUsesRouteTemplate(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1})
	engine := newRateLimitEngine(limiter, m)

	doVote(engine, "p1", "203.0.113.1:1000")
	doVote(engine, "p1", "203.0.113.1:1000")

	labels := counterLabels(t, m, "anonvote_rate_limited_total")
	require.Len(t, labels, 1)
	assert.Contains(t, labels[0], "route=/api/v1/proposals/:proposal_id/votes")
}

// TestRateLimit_MetricDoesNotLeakClientIP 守卫指标里不出现来源地址。
func TestRateLimit_MetricDoesNotLeakClientIP(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	limiter := ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1})
	engine := newRateLimitEngine(limiter, m)

	doVote(engine, "p1", "203.0.113.1:1000")
	doVote(engine, "p1", "203.0.113.1:1000")

	families, err := m.Registry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.NotContains(t, label.GetValue(), "203.0.113.1", "指标里不应当出现来源地址")
			}
		}
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, retryAfterSeconds(0), "0 秒等于让客户端立刻重试")
	assert.Equal(t, 1, retryAfterSeconds(300*time.Millisecond), "不足一秒向上取整为 1")
	assert.Equal(t, 3, retryAfterSeconds(3*time.Second))
	assert.Equal(t, 4, retryAfterSeconds(3500*time.Millisecond))
}
