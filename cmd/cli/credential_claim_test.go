package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	cryptoinfra "anonvote/internal/infrastructure/crypto"

	"github.com/stretchr/testify/require"
)

// blindSigningServer 是一个只做「服务端该做的事」的假服务端：
// 盲签名 + 验签。它**从来看不到被签消息的原文** —— 这正是两段式盲签名的意义。
type blindSigningServer struct {
	issuer *cryptoinfra.KryptologyCredentialIssuer
	// seenBlinded 记录服务端收到的盲化消息。
	seenBlinded []byte
	// requests 记录每个请求的路径。
	requests []string
}

func newBlindSigningServer(t *testing.T) *blindSigningServer {
	t.Helper()
	return &blindSigningServer{issuer: cryptoinfra.NewKryptologyCredentialIssuer()}
}

func (s *blindSigningServer) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests = append(s.requests, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/proposals/p1/credentials/claim":
			s.writeReservation(t, w, "p1", "slot-abc")
		case "/api/v1/proposals/p1/credentials/blind-sign":
			s.writeBlindSignature(t, w, r)
		case "/api/v1/proposals/p1/credentials/attest":
			s.writeCredential(t, w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (s *blindSigningServer) writeReservation(
	t *testing.T,
	w http.ResponseWriter,
	proposalID string,
	userID string,
) {
	t.Helper()
	issuerKey, err := s.issuer.IssuerPublicKey(context.Background())
	require.NoError(t, err)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"proposal_id":        proposalID,
		"user_id":            userID,
		"issuer_key_version": issuerKey.Version,
		"variant":            issuerKey.Variant,
		"modulus":            base64.StdEncoding.EncodeToString(issuerKey.Modulus),
		"exponent":           issuerKey.Exponent,
	})
}

func (s *blindSigningServer) writeBlindSignature(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var body struct {
		UserID           string `json:"user_id"`
		BlindedMessage   string `json:"blinded_message"`
		IssuerKeyVersion int    `json:"issuer_key_version"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
	blinded, err := base64.StdEncoding.DecodeString(body.BlindedMessage)
	require.NoError(t, err)
	s.seenBlinded = blinded

	blindSignature, err := s.issuer.BlindSign(
		context.Background(), body.IssuerKeyVersion, blinded,
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"blind_signature": base64.StdEncoding.EncodeToString(blindSignature),
	})
}

func (s *blindSigningServer) writeCredential(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var body struct {
		UserID     string `json:"user_id"`
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
	userPubKey, err := base64.StdEncoding.DecodeString(body.UserPubKey)
	require.NoError(t, err)
	signature, err := base64.StdEncoding.DecodeString(body.Signature)
	require.NoError(t, err)

	version, err := s.issuer.VerifyCredential(body.UserID, userPubKey, signature)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":  "INVALID_CREDENTIAL",
			"error": "invalid credential",
		})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                 "cred-1",
		"proposal_id":        "p1",
		"user_id":            body.UserID,
		"user_pub_key":       body.UserPubKey,
		"signature":          body.Signature,
		"issuer_key_version": version,
		"issued_at":          time.Now().UTC().Format(time.RFC3339),
	})
}

// TestRun_CredentialClaim_AnonymousClaim 覆盖 CLI 的匿名领取凭证命令。
//
// 关键性质：① 三步都**不带** admin token —— 领取端点是公开的；② 预留阶段
// **不发公钥**（服务端一旦知道公钥，盲化就失效了）；③ 服务端只收到盲化消息；
// ④ 落盘的凭证能直接拿去投票。
func TestRun_CredentialClaim_AnonymousClaim(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := newBlindSigningServer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		server.handler(t).ServeHTTP(w, r)
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
	// 三步都走到了。
	require.Equal(t, []string{
		"/api/v1/proposals/p1/credentials/claim",
		"/api/v1/proposals/p1/credentials/blind-sign",
		"/api/v1/proposals/p1/credentials/attest",
	}, server.requests)
	// 服务端收到的是盲化消息 —— 不是被签内容。
	require.NotEmpty(t, server.seenBlinded)

	var credential credentialFile
	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &credential))
	require.Equal(t, "p1", credential.ProposalID)
	require.Equal(t, "slot-abc", credential.UserID)
	require.NotEmpty(t, credential.UserPubKey)
	require.NotEmpty(t, credential.Signature)
	require.Equal(t, 1, credential.Version)
}

// TestRun_CredentialClaim_RejectsInvalidPubKey 验证 -pubkey 必须是 base64。
func TestRun_CredentialClaim_RejectsInvalidPubKey(t *testing.T) {
	t.Parallel()

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", "http://127.0.0.1:1",
			"credential-claim", "-proposal", "p1", "-pubkey", "%%%not-base64%%%",
		})
	})
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "base64")
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

// TestRun_CredentialIssue_AdminFlow 覆盖管理端签发：同样走真两段式盲签名。
func TestRun_CredentialIssue_AdminFlow(t *testing.T) {
	t.Parallel()

	server := newBlindSigningServer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.requests = append(server.requests, r.URL.Path)
		if r.URL.Path == "/api/v1/credentials" {
			require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
			server.writeReservation(t, w, "p1", "operator-user")
			return
		}
		if r.URL.Path == "/api/v1/credentials/blind-sign" {
			server.writeBlindSignature(t, w, r)
			return
		}
		server.writeCredential(t, w, r)
	}))
	t.Cleanup(srv.Close)

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL, "-admin-token", "secret",
			"credential-issue", "-proposal", "p1", "-user", "operator-user",
		})
	})
	require.Equal(t, 0, code, "命令失败: %s", errOut)
	require.Equal(t, []string{
		"/api/v1/credentials",
		"/api/v1/credentials/blind-sign",
		"/api/v1/proposals/p1/credentials/attest",
	}, server.requests)
}

// TestRun_CredentialClaim_RetriesWhenSlotTaken 验证槽位被别人抢走时 CLI 会换一个
// 槽位重来 —— 重试要重新盲化，服务端代劳不了，所以只能在客户端做。
func TestRun_CredentialClaim_RetriesWhenSlotTaken(t *testing.T) {
	t.Parallel()

	server := newBlindSigningServer(t)
	claimCount := 0
	attestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.requests = append(server.requests, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/proposals/p1/credentials/claim":
			claimCount++
			server.writeReservation(t, w, "p1", fmt.Sprintf("slot-%d", claimCount))
		case "/api/v1/proposals/p1/credentials/blind-sign":
			server.writeBlindSignature(t, w, r)
		default:
			attestCount++
			if attestCount == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"code":"CREDENTIAL_EXISTS","error":"凭证已存在"}`))
				return
			}
			server.writeCredential(t, w, r)
		}
	}))
	t.Cleanup(srv.Close)

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{"-base-url", srv.URL, "credential-claim", "-proposal", "p1"})
	})
	require.Equal(t, 0, code, "命令失败: %s", errOut)
	require.Equal(t, 2, claimCount, "换一个槽位重来：预留应该被调用两次")
	require.Equal(t, 2, attestCount)
}

// TestRun_CredentialClaim_CrossAttackFails 验证「拿 A 的盲签名去签 B」过不去：
// CLI 解盲出来的签名只对 A 的消息有效。
func TestRun_CredentialClaim_CrossAttackFails(t *testing.T) {
	t.Parallel()

	server := newBlindSigningServer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.requests = append(server.requests, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/proposals/p1/credentials/claim":
			server.writeReservation(t, w, "p1", "slot-abc")
		case "/api/v1/proposals/p1/credentials/blind-sign":
			server.writeBlindSignature(t, w, r)
		default:
			// attest 阶段把 user_id 换成别人的 —— 验签必然失败。
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["user_id"] = "someone-else"
			payload, _ := json.Marshal(body)
			r.Body = io.NopCloser(bytes.NewReader(payload))
			server.writeCredential(t, w, r)
		}
	}))
	t.Cleanup(srv.Close)

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{"-base-url", srv.URL, "credential-claim", "-proposal", "p1"})
	})
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "INVALID_CREDENTIAL")
}
