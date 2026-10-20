package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLI_CreateProposal_SendsPlainDescriptionAndOptions(t *testing.T) {
	t.Parallel()

	type createProposalRequest struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Options     []string `json:"options"`
		StartsAt    string   `json:"starts_at"`
		EndsAt      string   `json:"ends_at"`
	}

	requests := make(chan createProposalRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/proposals", r.URL.Path)
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))

		var req createProposalRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests <- req

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"id":          "proposal-1",
			"title":       req.Title,
			"description": req.Description,
			"options":     req.Options,
			"status":      "open",
			"starts_at":   req.StartsAt,
			"ends_at":     req.EndsAt,
			"created_at":  time.Now().UTC().Format(time.RFC3339),
		}))
	}))
	defer server.Close()

	c := &cli{
		baseURL:    server.URL,
		adminToken: "secret-token",
		httpClient: server.Client(),
	}

	output := captureStdout(t, func() {
		err := c.cmdCreateProposal([]string{
			"-title", "Board Election",
			"-description", "Vote for one candidate",
			"-options", "candidate-A,candidate-B",
			"-start", "2026-03-18T10:00:00Z",
			"-end", "2026-03-18T12:00:00Z",
		})
		require.NoError(t, err)
	})

	req := <-requests
	require.Equal(t, "Board Election", req.Title)
	require.Equal(t, "Vote for one candidate", req.Description)
	require.Equal(t, []string{"candidate-A", "candidate-B"}, req.Options)
	require.Contains(t, output, `"id": "proposal-1"`)
	require.NotContains(t, req.Description, `"summary"`)
}

func TestCLI_BuildURL_DoesNotDoubleEscapePath(t *testing.T) {
	t.Parallel()

	c := &cli{baseURL: "http://127.0.0.1:8080"}

	// proposal id 已经被 PathEscape 转义过一次，buildURL 不能再转义一遍。
	built, err := c.buildURL("/api/v1/proposals/a%20b%2Fc/results", nil)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/api/v1/proposals/a%20b%2Fc/results", built)
	require.NotContains(t, built, "%2520")
	require.NotContains(t, built, "%252F")
}

func TestCLI_BuildURL_KeepsBasePathPrefix(t *testing.T) {
	t.Parallel()

	c := &cli{baseURL: "http://127.0.0.1:8080/anonvote"}

	built, err := c.buildURL("/api/v1/proposals/p1/results", nil)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/anonvote/api/v1/proposals/p1/results", built)
}

func TestCLI_DecryptSetup_RequiresOutFileSoSharesAreNotPrinted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"proposal_id": "p1",
			"threshold":   2,
			"trustees":    3,
			"public_key":  "pk",
			"shares":      []string{"secret-share-1", "secret-share-2"},
		}))
	}))
	defer server.Close()

	c := &cli{
		baseURL:    server.URL,
		adminToken: "secret-token",
		httpClient: server.Client(),
	}

	var err error
	output := captureStdout(t, func() {
		err = c.cmdDecryptSetup([]string{"-proposal", "p1"})
	})

	// 门限份额只能取一次，必须落到文件里；不能默认打到 stdout。
	require.Error(t, err)
	require.Contains(t, err.Error(), "-out")
	require.NotContains(t, output, "secret-share-1")
}

func TestCLI_VoteSubmit_DoesNotSendSelectedOption(t *testing.T) {
	t.Parallel()

	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests <- body

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"status": "ok"}))
	}))
	defer server.Close()

	dir := t.TempDir()
	credentialPath := dir + "/cred.json"
	ballotPath := dir + "/ballot.json"
	require.NoError(t, os.WriteFile(credentialPath, []byte(
		`{"proposal_id":"p1","user_id":"u-1","user_pub_key":"pk","signature":"sig"}`,
	), 0o600))
	require.NoError(t, os.WriteFile(ballotPath, []byte(
		`{"proposal_id":"p1","encrypted_vote":"cipher","proof":"proof","nullifier":"n-1","selected_option":"A"}`,
	), 0o600))

	c := &cli{
		baseURL:    server.URL,
		adminToken: "secret-token",
		httpClient: server.Client(),
	}

	captureStdout(t, func() {
		require.NoError(t, c.cmdVoteSubmit([]string{
			"-credential-file", credentialPath,
			"-ballot-file", ballotPath,
		}))
	})

	body := <-requests
	require.Equal(t, "cipher", body["encrypted_vote"])
	// 明文选项不该上服务端：服务端根本不消费这个字段。
	require.NotContains(t, body, "selected_option")
}

// stdoutMu 串行化 stdout 的替换：captureStdout 改的是进程级全局变量，
// 并行测试同时调用会互相踩，导致断言读到别的测试的输出。
var stdoutMu sync.Mutex

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return strings.TrimSpace(string(data))
}
