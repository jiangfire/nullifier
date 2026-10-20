package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

// createProposalForClaim 建一个提案，返回它的 id。
func createProposalForClaim(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	body := map[string]any{
		"title":     "Pool Election",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	}
	resp := doPostAdminJSON(t, ts.URL+"/api/v1/proposals", "secret", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposal struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposal))
	_ = resp.Body.Close()
	return proposal.ID
}

// 匿名领取凭证：不需要 admin token，服务端也不知道领取的是谁。
func TestClaimCredential_AnonymousClaim(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	// 备池需要 admin token。
	resp := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
		"secret",
		map[string]any{"count": 2},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	// 领取不需要任何鉴权。
	resp = doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
		map[string]any{"user_pub_key": base64.StdEncoding.EncodeToString([]byte("claimant-pub"))},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var credential struct {
		ID         string `json:"id"`
		UserID     string `json:"user_id"`
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&credential))
	_ = resp.Body.Close()

	require.NotEmpty(t, credential.ID)
	require.NotEmpty(t, credential.UserID)
	require.NotEmpty(t, credential.Signature)
	// 公钥是领取者自己带来的。
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("claimant-pub")), credential.UserPubKey)
}

// 领取请求里即使塞了 user_id，服务端也会忽略它 —— user_id 只能来自池里的假名。
func TestClaimCredential_IgnoresClientSuppliedUserID(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	resp := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
		"secret",
		map[string]any{"count": 1},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	resp = doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
		map[string]any{
			"user_pub_key": base64.StdEncoding.EncodeToString([]byte("claimant-pub")),
			"user_id":      "i-am-the-voter",
		},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var credential struct {
		UserID string `json:"user_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&credential))
	_ = resp.Body.Close()

	require.NotEqual(t, "i-am-the-voter", credential.UserID, "user_id 必须来自池里的假名")
	require.NotEmpty(t, credential.UserID)
}

// 池领完了要返回 4xx，而不是 5xx。
func TestClaimCredential_PoolEmptyReturns4xx(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
		map[string]any{"user_pub_key": base64.StdEncoding.EncodeToString([]byte("claimant-pub"))},
	)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}

// 备池是管理面操作，必须有 admin token。
func TestFillCredentialPool_RequiresAdminToken(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
		map[string]any{"count": 2},
	)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

// 领取之后，凭证池里的槽位必须被标记为已领取，库里不存在「谁领的」。
func TestClaimCredential_MarksSlotClaimed(t *testing.T) {
	engine, _, db := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	resp := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
		"secret",
		map[string]any{"count": 1},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	resp = doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
		map[string]any{"user_pub_key": base64.StdEncoding.EncodeToString([]byte("claimant-pub"))},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	var claims []entity.CredentialClaim
	require.NoError(t, db.Find(&claims).Error)
	require.Len(t, claims, 1)
	require.NotNil(t, claims[0].ClaimedAt, "领取过的槽位必须被标记")
	require.Equal(t, []byte("claimant-pub"), []byte(claims[0].UserPubKey))

	// 领取的凭证落库，但没有「领取者」这个字段 —— 只有池里的假名。
	var credentials []entity.Credential
	require.NoError(t, db.Find(&credentials).Error)
	require.Len(t, credentials, 1)
	require.Equal(t, claims[0].UserID, credentials[0].UserID)
}
