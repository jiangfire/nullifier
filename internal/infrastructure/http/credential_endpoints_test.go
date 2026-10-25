package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anonvote/tests/testutil"

	"github.com/stretchr/testify/require"
)

// jsonBody 造一个**不是 JSON** 的请求体。
func jsonBody(raw string) io.Reader {
	return strings.NewReader(raw)
}

// 这一组覆盖凭证两段式盲签名端点的**请求校验分支**。
//
// 成功路径与安全性质在 credential_claim_test.go 与 tests/e2e 里；这里补的是
// 「客户端发来不合法的东西」时服务端怎么答 —— 每一处都必须是 4xx，不能 500。

// 公开的盲签名端点：缺字段一律 400。
func TestBlindSignCredential_RejectsMalformedRequests(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "缺少 user_id",
			body: map[string]any{
				"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
				"issuer_key_version": 1,
			},
		},
		{
			name: "缺少 blinded_message",
			body: map[string]any{
				"user_id":            "someone",
				"issuer_key_version": 1,
			},
		},
		{
			name: "缺少 issuer_key_version",
			body: map[string]any{
				"user_id":         "someone",
				"blinded_message": base64.StdEncoding.EncodeToString([]byte("blinded")),
			},
		},
		{
			name: "blinded_message 不是 base64",
			body: map[string]any{
				"user_id":            "someone",
				"blinded_message":    "%%%not-base64%%%",
				"issuer_key_version": 1,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resp := doPostJSON(
				t,
				ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/blind-sign",
				testCase.body,
			)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			_ = resp.Body.Close()
		})
	}
}

// 公开的盲签名端点：请求体根本不是 JSON 时也要 400，不能 500。
func TestBlindSignCredential_RejectsBrokenJSON(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	resp, err := http.Post(
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/blind-sign",
		"application/json",
		jsonBody("{not json"),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

// 管理端的盲签名端点：必须带 admin token。
func TestAdminBlindSignCredential_RequiresAdminToken(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	resp := doPostJSON(
		t,
		ts.URL+"/api/v1/credentials/blind-sign",
		map[string]any{
			"proposal_id":        proposalID,
			"user_id":            "operator-user",
			"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
			"issuer_key_version": 1,
		},
	)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

// 管理端的盲签名端点：缺字段与非法 base64 都是 400。
func TestAdminBlindSignCredential_RejectsMalformedRequests(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "缺少 proposal_id",
			body: map[string]any{
				"user_id":            "operator-user",
				"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
				"issuer_key_version": 1,
			},
		},
		{
			name: "缺少 user_id",
			body: map[string]any{
				"proposal_id":        proposalID,
				"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
				"issuer_key_version": 1,
			},
		},
		{
			name: "blinded_message 不是 base64",
			body: map[string]any{
				"proposal_id":        proposalID,
				"user_id":            "operator-user",
				"blinded_message":    "%%%not-base64%%%",
				"issuer_key_version": 1,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resp := doPostAdminJSON(
				t,
				ts.URL+"/api/v1/credentials/blind-sign",
				"secret",
				testCase.body,
			)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			_ = resp.Body.Close()
		})
	}
}

// 管理端的盲签名端点：可以覆盖**任意** user_id —— 它不必在凭证池里。
//
// 这是与公开端的区别：公开端必须绑定池槽位（否则签发方是任意消息的签名
// 预言机），而运营方本来就有服务端的完整权限。
func TestAdminBlindSignCredential_SignsAnyUserID(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)
	client := newBlindClient(t, ts)

	// 管理端为一个不在池里的 user_id 预留签发权。
	reservation, err := client.ReserveForUser(proposalID, "operator-user")
	require.NoError(t, err)
	require.Equal(t, "operator-user", reservation.UserID)

	blinded, err := client.Blind(reservation, []byte("operator-pub"))
	require.NoError(t, err)

	blindSignature, err := client.BlindSignAdmin(
		proposalID,
		reservation,
		blinded.BlindedMessage,
	)
	require.NoError(t, err)
	require.NotEmpty(t, blindSignature)

	// 解盲后 attest 应当通过。
	signature, err := blinded.Finalize(blindSignature)
	require.NoError(t, err)
	credential, err := client.Attest(proposalID, reservation.UserID, []byte("operator-pub"), signature)
	require.NoError(t, err)
	require.Equal(t, "operator-user", credential.UserID)
}

// attest 端点：缺字段与非法 base64 都是 400。
func TestAttestCredential_RejectsMalformedRequests(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "缺少 user_id",
			body: map[string]any{
				"user_pub_key": base64.StdEncoding.EncodeToString([]byte("pub")),
				"signature":    base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			name: "缺少 user_pub_key",
			body: map[string]any{
				"user_id":   "someone",
				"signature": base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			name: "缺少 signature",
			body: map[string]any{
				"user_id":      "someone",
				"user_pub_key": base64.StdEncoding.EncodeToString([]byte("pub")),
			},
		},
		{
			name: "user_pub_key 不是 base64",
			body: map[string]any{
				"user_id":      "someone",
				"user_pub_key": "%%%not-base64%%%",
				"signature":    base64.StdEncoding.EncodeToString([]byte("sig")),
			},
		},
		{
			name: "signature 不是 base64",
			body: map[string]any{
				"user_id":      "someone",
				"user_pub_key": base64.StdEncoding.EncodeToString([]byte("pub")),
				"signature":    "%%%not-base64%%%",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resp := doPostJSON(
				t,
				ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/attest",
				testCase.body,
			)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			_ = resp.Body.Close()
		})
	}
}

// 管理端预留：缺字段是 400，成功是 201。
func TestReserveCredentialForUser_ValidatesAndReserves(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	missing := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/credentials",
		"secret",
		map[string]any{"proposal_id": proposalID},
	)
	require.Equal(t, http.StatusBadRequest, missing.StatusCode)
	_ = missing.Body.Close()

	ok := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/credentials",
		"secret",
		map[string]any{"proposal_id": proposalID, "user_id": "operator-user"},
	)
	require.Equal(t, http.StatusCreated, ok.StatusCode)
	var reservation testutil.Reservation
	require.NoError(t, json.NewDecoder(ok.Body).Decode(&reservation))
	_ = ok.Body.Close()
	require.Equal(t, "operator-user", reservation.UserID)
	require.NotEmpty(t, reservation.Modulus)
}

// 备池：count 非法（0 / 负数 / 超过上限）是 400，成功是 200。
func TestFillCredentialPool_ValidatesCount(t *testing.T) {
	engine, _, _ := buildFullStackEngineWithVerifier(t, nil)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalID := createProposalForClaim(t, ts)

	for _, count := range []int{0, -1, 10001} {
		resp := doPostAdminJSON(
			t,
			ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
			"secret",
			map[string]any{"count": count},
		)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "count=%d 必须被拒", count)
		_ = resp.Body.Close()
	}

	ok := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/pool",
		"secret",
		map[string]any{"count": 3},
	)
	require.Equal(t, http.StatusCreated, ok.StatusCode)
	_ = ok.Body.Close()
}
