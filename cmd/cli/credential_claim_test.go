package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRun_CredentialClaim_AnonymousClaim 覆盖 CLI 的匿名领取凭证命令。
//
// 关键性质：① 请求**不带** admin token —— 领取端点是公开的；② 请求体里
// 只有自己生成的公钥，没有任何身份信息；③ 落盘的凭证能直接拿去投票。
func TestRun_CredentialClaim_AnonymousClaim(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/proposals/p1/credentials/claim", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                 "cred-1",
			"proposal_id":        "p1",
			"user_id":            "slot-abc",
			"user_pub_key":       gotBody["user_pub_key"],
			"signature":          base64.StdEncoding.EncodeToString([]byte("sig")),
			"issuer_key_version": 1,
			"issued_at":          "2026-10-21T00:00:00Z",
		})
	}))
	t.Cleanup(srv.Close)

	outPath := filepath.Join(t.TempDir(), "cred.json")
	code := 0
	out, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL,
			"credential-claim", "-proposal", "p1", "-out", outPath,
		})
	})
	require.Equal(t, 0, code, "命令失败: %s", errOut)
	require.Contains(t, out, outPath)

	// 领取是公开操作：不带 admin token。
	require.Empty(t, gotAuth)
	// 请求体里只有公钥，没有 user_id 之类的身份字段。
	require.Len(t, gotBody, 1)
	require.NotEmpty(t, gotBody["user_pub_key"])

	var credential credentialFile
	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &credential))
	require.Equal(t, "p1", credential.ProposalID)
	require.Equal(t, "slot-abc", credential.UserID)
	require.Equal(t, gotBody["user_pub_key"], credential.UserPubKey)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("sig")), credential.Signature)
	require.Equal(t, 1, credential.Version)
}

// 领取失败时要把服务端的错误带出来，不能静默成功。
func TestRun_CredentialClaim_ReportsServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"CREDENTIAL_POOL_EMPTY","error":"凭证池已空"}`))
	}))
	t.Cleanup(srv.Close)

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{"-base-url", srv.URL, "credential-claim", "-proposal", "p1"})
	})
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "CREDENTIAL_POOL_EMPTY")
}

// 缺少 -proposal 时直接报错，不去打服务端。
func TestRun_CredentialClaim_RequiresProposal(t *testing.T) {
	t.Parallel()

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{"-base-url", "http://127.0.0.1:1", "credential-claim"})
	})
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "-proposal")
}
