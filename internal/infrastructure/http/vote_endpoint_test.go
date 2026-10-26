package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSubmitVote_LegacyPathReturnsNotFound 守住 v0.3.0 的破坏性变更。
//
// 投票提交端点在 v0.3.0 从 `POST /api/v1/votes` 换成了
// `POST /api/v1/proposals/:proposal_id/votes`。旧路径**不再注册** ——
// 留着它会让人以为「提案 ID 可以放在请求体里」，两套入口的结果还不一样。
//
// 契约门禁（tests/architecture/api_contract_test.go）断言的是「路由树里没有
// 这一条」，那是静态检查；这里补一条 **HTTP 级**的断言：真发一个请求，
// 确认它落到 404 而不是被别的路由接住。
func TestSubmitVote_LegacyPathReturnsNotFound(t *testing.T) {
	t.Parallel()

	engine := newMetricsTestEngine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, jsonRequest(t, http.MethodPost, "/api/v1/votes", map[string]any{
		"encrypted_vote":       "Y2lwaGVy",
		"proof":                "cHJvb2Y=",
		"nullifier":            "abababababababababababababababababababababababababababababababab",
		"user_id":              "u-1",
		"user_pub_key":         "cHVi",
		"credential_signature": "c2ln",
	}))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
