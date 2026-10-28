package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anonvote/internal/config"
	"anonvote/internal/infrastructure/ratelimit"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// newRateLimitTestEngine 造一个带限流的服务。
//
// 限流额度刻意压得很小（突发 1），好让用例一眼能看出「放行」与「被限」。
func newRateLimitTestEngine(t *testing.T, trustedProxies []string) *gin.Engine {
	t.Helper()

	handler := NewHandlerWithTally(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		zap.NewNop(),
	).WithRateLimits(&RateLimits{
		Credentials: ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1}),
		Votes:       ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1}),
		Read:        ratelimit.New(ratelimit.Config{Limit: rate.Limit(1), Burst: 1}),
	})

	return NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
		TrustedProxies:   trustedProxies,
	}, handler, zap.NewNop()).Engine()
}

// voteRequest 造一个投票请求。
//
// 注意请求体是**合法的**：被限流的请求根本不该走到业务层，如果业务层先报错
// （400），用例就分不清「被限流」和「请求不合法」了。
func voteRequest(t *testing.T, proposalID, remoteAddr, forwardedFor string) *http.Request {
	t.Helper()
	body := map[string]any{
		"encrypted_vote":       base64.StdEncoding.EncodeToString([]byte("cipher")),
		"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
		"nullifier":            strings.Repeat("ab", 32),
		"user_id":              "u1",
		"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
		"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
	}
	req := jsonRequest(t, http.MethodPost, "/api/v1/proposals/"+proposalID+"/votes", body)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	return req
}

// TestRateLimit_EndpointReturns429 钉住超限的端点行为：429 + Retry-After。
//
// 第一次请求会走到业务层并因为「仓储没配」而失败（500）—— 重点是它**没有**
// 被限流；第二次才被挡下来。
func TestRateLimit_EndpointReturns429(t *testing.T) {
	t.Parallel()

	engine := newRateLimitTestEngine(t, nil)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "第一次请求不该被限流")

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"), "429 必须带 Retry-After")
	assert.Contains(t, rec.Body.String(), `"code":"RATE_LIMITED"`)
}

// TestRateLimit_EndpointDifferentClientsAreIndependent 钉住「不同 IP 互不影响」。
func TestRateLimit_EndpointDifferentClientsAreIndependent(t *testing.T) {
	t.Parallel()

	engine := newRateLimitTestEngine(t, nil)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	// 另一个来源不受影响。
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.2:1000", ""))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
}

// TestRateLimit_ForgedForwardedForCannotBypass 钉住「伪造 X-Forwarded-For
// 不能绕过限流」。
//
// 默认不信任任何代理 —— gin 的默认行为是信任所有代理，那会让限流形同虚设。
func TestRateLimit_ForgedForwardedForCannotBypass(t *testing.T) {
	t.Parallel()

	engine := newRateLimitTestEngine(t, nil)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)

	// 换一个 X-Forwarded-For —— 同一个真实来源，额度不应当重置。
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", "198.51.100.7"))
	require.Equal(t, http.StatusTooManyRequests, rec.Code, "伪造 X-Forwarded-For 不应当绕过限流")
}

// TestRateLimit_TrustedProxyUsesForwardedFor 确认显式配置可信代理后，
// 来源地址取自 X-Forwarded-For —— 这才是反向代理后面的正确口径。
func TestRateLimit_TrustedProxyUsesForwardedFor(t *testing.T) {
	t.Parallel()

	engine := newRateLimitTestEngine(t, []string{"203.0.113.0/24"})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", "198.51.100.7"))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", "198.51.100.7"))
	require.Equal(t, http.StatusTooManyRequests, rec.Code, "同一个 XFF 应当共用一个额度")

	// 不同的 XFF 是不同的来源。
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", "198.51.100.8"))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
}

// TestRateLimit_DifferentRouteGroupsHaveSeparateQuota 确认不同路由组额度独立：
// 领取链路的流量不该吃掉投票端点的额度。
func TestRateLimit_DifferentRouteGroupsHaveSeparateQuota(t *testing.T) {
	t.Parallel()

	engine := newRateLimitTestEngine(t, nil)

	// 投票端点用光 votes 组的额度。
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, voteRequest(t, "p1", "203.0.113.1:1000", ""))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	// 只读组是另一个桶。
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, jsonRequest(t, http.MethodGet, "/api/v1/proposals", nil))
	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
}
