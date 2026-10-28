package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anonvote/internal/config"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 管理面权限分级的 HTTP 级用例（v0.4.0 · T31）。
//
// 同一台服务端配三枚令牌（admin / operator / auditor），逐条断言放行与拒绝。
// 权限不足与令牌错误同形地返回 401 —— 403 会告诉持低级令牌的人「这个路由
// 存在且需要更高权限」。
func TestServer_AdminRoles_E2E(t *testing.T) {
	tokens := config.AdminTokensConfig{
		Admin:    "tok-admin",
		Operator: "tok-operator",
		Auditor:  "tok-auditor",
	}
	ts := newServerWithTokens(t, config.Config{
		GinMode:     "test",
		AdminTokens: tokens,
	})
	defer ts.Close()

	proposalBody := map[string]any{
		"title":     "role-test",
		"options":   []string{"a", "b"},
		"starts_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	raw, err := json.Marshal(proposalBody)
	require.NoError(t, err)

	// admin：全部可达。
	code := adminRoleRequest(t, ts, "tok-admin", http.MethodPost, "/api/v1/proposals", raw)
	require.Equal(t, http.StatusCreated, code, "admin 令牌能创建提案")
	code = adminRoleRequest(t, ts, "tok-admin", http.MethodGet, "/metrics", nil)
	require.Equal(t, http.StatusOK, code, "admin 令牌能读指标")

	// operator：运营面可达，观察面可达。
	code = adminRoleRequest(t, ts, "tok-operator", http.MethodPost, "/api/v1/proposals", raw)
	require.Equal(t, http.StatusCreated, code, "operator 令牌能创建提案")
	code = adminRoleRequest(t, ts, "tok-operator", http.MethodGet, "/metrics", nil)
	require.Equal(t, http.StatusOK, code, "operator 令牌能读指标（auditor 及以上）")

	// auditor：观察面可达，运营面拒绝（401，与令牌错误同形）。
	code = adminRoleRequest(t, ts, "tok-auditor", http.MethodGet, "/metrics", nil)
	require.Equal(t, http.StatusOK, code, "auditor 令牌能读指标")
	listPath := "/api/v1/proposals/whatever/credentials"
	code = adminRoleRequest(t, ts, "tok-auditor", http.MethodGet, listPath, nil)
	require.Equal(t, http.StatusOK, code, "auditor 令牌能读凭证状态")
	code = adminRoleRequest(t, ts, "tok-auditor", http.MethodPost, "/api/v1/proposals", raw)
	require.Equal(t, http.StatusUnauthorized, code, "auditor 令牌不能创建提案")
	code = adminRoleRequest(t, ts, "tok-auditor", http.MethodPost,
		"/api/v1/proposals/whatever/credentials/pool",
		[]byte(`{"count":1}`))
	require.Equal(t, http.StatusUnauthorized, code, "auditor 令牌不能灌池")

	// 令牌错误：401（与权限不足同形，不泄露路由的权限档位）。
	code = adminRoleRequest(t, ts, "wrong-token", http.MethodGet, "/metrics", nil)
	require.Equal(t, http.StatusUnauthorized, code)
}

// TestServer_AdminRoles_LegacyTokenIsAdmin 旧的单值令牌等价于 admin：
// 存量部署零改动升级。
func TestServer_AdminRoles_LegacyTokenIsAdmin(t *testing.T) {
	ts := newServerWithTokens(t, config.Config{
		GinMode:          "test",
		AdminBearerToken: "tok-legacy",
	})
	defer ts.Close()

	proposalBody := map[string]any{
		"title":     "legacy-role-test",
		"options":   []string{"a", "b"},
		"starts_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	raw, err := json.Marshal(proposalBody)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated,
		adminRoleRequest(t, ts, "tok-legacy", http.MethodPost, "/api/v1/proposals", raw))
	require.Equal(t, http.StatusOK,
		adminRoleRequest(t, ts, "tok-legacy", http.MethodGet, "/metrics", nil))
}

// newServerWithTokens 用给定的令牌配置起一台真实服务端（内存库全链路）。
func newServerWithTokens(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()

	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: ":memory:"})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	claimRepo := persistence.NewCredentialClaimRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	encrypter := cryptoinfra.NewBallotThresholdCrypto()
	issuer := cryptoinfra.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := cryptoinfra.NewGnarkMerkleRootBuilder()
	proofVerifier, err := cryptoinfra.NewGnarkProofVerifier("")
	require.NoError(t, err)

	logger := zap.NewNop()
	registerUC := usecase.NewRegisterUseCaseWithPool(
		credentialRepo, proposalRepo, issuer, claimRepo, logger,
	)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, nil, logger)
	votingUC := usecase.NewVotingUseCase(
		voteRepo, participationRepo, credentialRepo, proposalRepo, sessionRepo,
		proofVerifier, encrypter, issuer, logger,
	)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		encrypter, merkleBuilder, logger,
	)

	cfg.ServerAddress = ":0"
	handler := NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
	server := NewServer(&cfg, handler, logger)
	return httptest.NewServer(server.Engine())
}

// adminRoleRequest 带令牌打一个管理面请求，返回状态码。
func adminRoleRequest(t *testing.T, ts *httptest.Server, token, method, path string, body []byte) int {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(context.Background(), method, ts.URL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
