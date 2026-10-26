package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/tests/testutil"

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

// newBlindClient 建一个走 HTTP 的盲签名客户端。
func newBlindClient(t *testing.T, ts *httptest.Server) *testutil.BlindClient {
	t.Helper()
	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = "secret"
	return client
}

// 匿名领取凭证：不需要 admin token，服务端也不知道领取的是谁。
func TestClaimCredential_AnonymousClaim(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)

	// 备池需要 admin token。
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	// 领取不需要任何鉴权。
	credential, err := client.ClaimCredential(proposalID, []byte("claimant-pub"))
	require.NoError(t, err)
	require.NotEmpty(t, credential.ID)
	require.NotEmpty(t, credential.UserID)
	require.NotEmpty(t, credential.Signature)
	// 公钥是领取者自己带来的。
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("claimant-pub")), credential.UserPubKey)
}

// 预留请求里即使塞了 user_id 与 user_pub_key，服务端也会忽略 —— user_id 只能
// 来自池里的假名，而公钥**绝不能**提前告诉服务端。
func TestClaimCredential_IgnoresClientSuppliedIdentity(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim",
		map[string]any{
			"user_pub_key": base64.StdEncoding.EncodeToString([]byte("early-pub")),
			"user_id":      "i-am-the-voter",
		},
	)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var reservation testutil.Reservation
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reservation))
	_ = resp.Body.Close()

	require.NotEqual(t, "i-am-the-voter", reservation.UserID, "user_id 必须来自池里的假名")
	require.NotEmpty(t, reservation.UserID)
	// 响应里带出的是签发方公钥 —— 它本来就是公开值，客户端要拿它盲化消息。
	require.NotEmpty(t, reservation.Modulus)
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
		map[string]any{},
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
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	_, err := client.ClaimCredential(proposalID, []byte("claimant-pub"))
	require.NoError(t, err)

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

// TestClaimCredential_BlindSignRejectsUnknownUserID 验证公开的盲签名端点不能
// 为「池里没有的 user_id」签名 —— 否则签发方就是任意消息的签名预言机。
//
// 回的是 **409 CREDENTIAL_EXISTS**：槽位不在「有人正在领」的集合里，意味着
// 它已经被别人领走了 —— 客户端据此换一个槽位重来（与 attest 阶段同一个信号）。
func TestClaimCredential_BlindSignRejectsUnknownUserID(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/blind-sign",
		map[string]any{
			"user_id":            "not-in-pool",
			"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
			"issuer_key_version": 1,
		},
	)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	var failure struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
	require.Equal(t, "CREDENTIAL_EXISTS", failure.Code)
	_ = resp.Body.Close()
}

// TestClaimCredential_AttestRejectsForgedSignature 验证 attest 端点会验签：
// 随便编一个签名过不去。
func TestClaimCredential_AttestRejectsForgedSignature(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	reservation, err := client.Reserve(proposalID)
	require.NoError(t, err)

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/attest",
		map[string]any{
			"user_id":      reservation.UserID,
			"user_pub_key": base64.StdEncoding.EncodeToString([]byte("pub")),
			"signature":    base64.StdEncoding.EncodeToString([]byte("forged")),
		},
	)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}
