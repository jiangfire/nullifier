package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"anonvote/internal/config"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newMetricsTestEngine() *gin.Engine {
	handler := NewHandlerWithTally(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		zap.NewNop(),
	)
	return NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()
}

func TestMetricsEndpoint_RequiresAdminToken(t *testing.T) {
	t.Parallel()

	engine := newMetricsTestEngine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	// 普通计数器恒有样本（值可以是 0）；带标签的指标族在没有样本时整个不出现
	// —— 这是 Prometheus 的惯例，告警规则用 `or vector(0)` 兜底。
	assert.Contains(t, rec.Body.String(), "anonvote_votes_accepted_total")
}

// TestMetricsEndpoint_CountsVoteRejections 验证被拒的投票会进指标，
// 且错误码与响应体里的 code 是同一个口径。
func TestMetricsEndpoint_CountsVoteRejections(t *testing.T) {
	t.Parallel()

	engine := newMetricsTestEngine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, jsonRequest(t, http.MethodPost, "/api/v1/proposals/p1/votes", map[string]any{
		"encrypted_vote":       "%%%invalid-base64%%%",
		"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
		"nullifier":            "n1",
		"user_id":              "u1",
		"user_pub_key":         base64.StdEncoding.EncodeToString([]byte("pub")),
		"credential_signature": base64.StdEncoding.EncodeToString([]byte("sig")),
	}))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `anonvote_votes_rejected_total{code="INVALID_REQUEST"} 1`)
}

// TestMetricsEndpoint_ExposesNoVoteContent 守卫指标端点不泄漏票面信息。
//
// 指标里只能有计数与耗时：一旦出现 nullifier、user_id 或候选人票数，
// 它就变成了投票内容的第二个出口。
func TestMetricsEndpoint_ExposesNoVoteContent(t *testing.T) {
	t.Parallel()

	engine := newMetricsTestEngine()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	for _, forbidden := range []string{"nullifier", "user_id", "encrypted_vote", "credential_signature"} {
		assert.NotContains(t, body, forbidden)
	}
}
