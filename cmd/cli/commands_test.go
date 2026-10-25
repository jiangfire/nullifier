package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureOutput 同时接管 stdout 与 stderr。
//
// 复用 stdoutMu：它保护的是「替换进程级 stdout/stderr」这件事，
// 两个全局变量必须一起换、一起换回来。
func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())

	outData, err := io.ReadAll(outR)
	require.NoError(t, err)
	errData, err := io.ReadAll(errR)
	require.NoError(t, err)

	return strings.TrimSpace(string(outData)), strings.TrimSpace(string(errData))
}

// capturedRequest 记录服务端实际收到的请求。
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

// newRecordingServer 起一个会把请求记下来的假服务端。
func newRecordingServer(t *testing.T, status int, body string) (*httptest.Server, <-chan capturedRequest) {
	t.Helper()

	requests := make(chan capturedRequest, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests <- capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   string(raw),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, requests
}

// TestRun_HTTPCommands 覆盖所有走 HTTP 的命令：路径、方法、admin 头、输出。
func TestRun_HTTPCommands(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credentialPath := filepath.Join(dir, "cred.json")
	require.NoError(t, os.WriteFile(credentialPath, []byte(
		`{"proposal_id":"p1","user_id":"u-1","user_pub_key":"pk","signature":"sig"}`,
	), 0o600))

	cases := []struct {
		name    string
		args    []string
		method  string
		path    string
		admin   bool
		resp    string
		wantOut string
	}{
		{
			name:    "health",
			args:    []string{"health"},
			method:  http.MethodGet,
			path:    "/healthz",
			resp:    `{"status":"ok"}`,
			wantOut: `"status": "ok"`,
		},
		{
			name:    "proposals-open",
			args:    []string{"proposals-open"},
			method:  http.MethodGet,
			path:    "/api/v1/proposals/open",
			resp:    `{"proposals":[{"id":"p1"}]}`,
			wantOut: `"id": "p1"`,
		},
		{
			name: "proposal-create",
			args: []string{
				"proposal-create",
				"-title", "Board",
				"-options", "A,B",
				"-start", "2026-03-18T10:00:00Z",
				"-end", "2026-03-18T12:00:00Z",
			},
			method:  http.MethodPost,
			path:    "/api/v1/proposals",
			admin:   true,
			resp:    `{"id":"p1","title":"Board"}`,
			wantOut: `"id": "p1"`,
		},
		{
			name:    "vote-status",
			args:    []string{"vote-status", "-proposal", "p1", "-credential-file", credentialPath},
			method:  http.MethodPost,
			path:    "/api/v1/proposals/p1/vote-status",
			resp:    `{"has_voted":true}`,
			wantOut: `"has_voted": true`,
		},
		{
			name:    "credentials-list",
			args:    []string{"credentials-list", "-proposal", "p1"},
			method:  http.MethodGet,
			path:    "/api/v1/proposals/p1/credentials",
			admin:   true,
			resp:    `{"statuses":[{"user_id":"u-1"}]}`,
			wantOut: `"user_id": "u-1"`,
		},
		{
			name:    "result-get",
			args:    []string{"result-get", "-proposal", "p1"},
			method:  http.MethodGet,
			path:    "/api/v1/proposals/p1/results",
			resp:    `{"options":["A"]}`,
			wantOut: `"options"`,
		},
		{
			name:    "anchors-list",
			args:    []string{"anchors-list", "-proposal", "p1"},
			method:  http.MethodGet,
			path:    "/api/v1/proposals/p1/audit-anchors",
			resp:    `{"anchors":[{"anchor":"a1"}]}`,
			wantOut: `"anchor": "a1"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, requests := newRecordingServer(t, http.StatusOK, tc.resp)

			code := 0
			out, errOut := captureOutput(t, func() {
				code = run(append([]string{"-base-url", srv.URL, "-admin-token", "secret"}, tc.args...))
			})

			require.Equal(t, 0, code, "命令失败: %s", errOut)
			require.Empty(t, errOut)
			require.Contains(t, out, tc.wantOut)

			req := <-requests
			require.Equal(t, tc.method, req.Method)
			require.Equal(t, tc.path, req.Path)
			if tc.admin {
				require.Equal(t, "Bearer secret", req.Auth)
			} else {
				require.Empty(t, req.Auth, "公开命令不该带 admin token")
			}
		})
	}
}

// TestRun_CommandsThatWriteFiles 覆盖需要落盘的命令。
func TestRun_CommandsThatWriteFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	t.Run("credential-issue 落盘", func(t *testing.T) {
		t.Parallel()

		// 管理端签发走真两段式盲签名：服务端只见盲化消息。
		server := newBlindSigningServer(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			server.requests = append(server.requests, r.URL.Path)
			switch r.URL.Path {
			case "/api/v1/credentials":
				server.writeReservation(t, w, "p1", "u-1")
			case "/api/v1/credentials/blind-sign":
				server.writeBlindSignature(t, w, r)
			default:
				server.writeCredential(t, w, r)
			}
		}))
		t.Cleanup(srv.Close)

		outPath := filepath.Join(dir, "issued.json")

		code := 0
		out, _ := captureOutput(t, func() {
			code = run([]string{
				"-base-url", srv.URL, "-admin-token", "secret",
				"credential-issue", "-proposal", "p1", "-user", "u-1", "-out", outPath,
			})
		})

		require.Equal(t, 0, code)
		require.Contains(t, out, outPath)
		require.Equal(t, []string{
			"/api/v1/credentials",
			"/api/v1/credentials/blind-sign",
			"/api/v1/proposals/p1/credentials/attest",
		}, server.requests)

		var issued credentialFile
		raw, err := os.ReadFile(outPath)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &issued))
		require.Equal(t, 1, issued.Version)
		require.Equal(t, "p1", issued.ProposalID)
		require.Equal(t, "u-1", issued.UserID)
		require.NotEmpty(t, issued.ExportedAt)
	})

	t.Run("vote-submit 提交选票", func(t *testing.T) {
		t.Parallel()

		srv, requests := newRecordingServer(t, http.StatusOK, `{"status":"accepted"}`)

		credentialPath := filepath.Join(dir, "vote-cred.json")
		ballotPath := filepath.Join(dir, "vote-ballot.json")
		require.NoError(t, os.WriteFile(credentialPath, []byte(
			`{"proposal_id":"p1","user_id":"u-1","user_pub_key":"pk","signature":"sig"}`,
		), 0o600))
		require.NoError(t, os.WriteFile(ballotPath, []byte(
			`{"proposal_id":"p1","encrypted_vote":"cipher","proof":"proof","nullifier":"n1"}`,
		), 0o600))

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{
				"-base-url", srv.URL,
				"vote-submit", "-credential-file", credentialPath, "-ballot-file", ballotPath,
			})
		})

		require.Equal(t, 0, code, "命令失败: %s", errOut)

		req := <-requests
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/api/v1/votes", req.Path)

		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(req.Body), &body))
		require.Equal(t, "cipher", body["encrypted_vote"])
		require.Equal(t, "u-1", body["user_id"])
	})

	t.Run("decrypt-setup 落盘", func(t *testing.T) {
		t.Parallel()

		srv, requests := newRecordingServer(t, http.StatusOK,
			`{"proposal_id":"p1","threshold":2,"trustees":3,"shares":["s1","s2","s3"]}`)
		outPath := filepath.Join(dir, "shares.json")

		code := 0
		out, _ := captureOutput(t, func() {
			code = run([]string{
				"-base-url", srv.URL, "-admin-token", "secret",
				"decrypt-setup", "-proposal", "p1", "-out", outPath,
			})
		})

		require.Equal(t, 0, code)
		require.Contains(t, out, outPath)

		req := <-requests
		require.Equal(t, "/api/v1/proposals/p1/decryption/setup", req.Path)
		require.Equal(t, "Bearer secret", req.Auth)

		var setup map[string]any
		raw, err := os.ReadFile(outPath)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &setup))
		require.Equal(t, "p1", setup["proposal_id"])
	})

	t.Run("result-publish 用份额文件", func(t *testing.T) {
		t.Parallel()

		srv, requests := newRecordingServer(t, http.StatusOK, `{"status":"published"}`)
		sharesPath := filepath.Join(dir, "shares.txt")
		require.NoError(t, os.WriteFile(sharesPath, []byte("s1\ns2\ns3\n"), 0o600))

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{
				"-base-url", srv.URL, "-admin-token", "secret",
				"result-publish", "-proposal", "p1", "-shares-file", sharesPath,
			})
		})

		require.Equal(t, 0, code, "命令失败: %s", errOut)

		req := <-requests
		require.Equal(t, "/api/v1/proposals/p1/results/publish", req.Path)

		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(req.Body), &body))
		require.Equal(t, []any{"s1", "s2", "s3"}, body["shares"])
	})

	// `decrypt-setup -out` 写出来的是会话对象，发布结果时必须能直接吃这个文件。
	t.Run("result-publish 用 decrypt-setup 落盘的会话文件", func(t *testing.T) {
		t.Parallel()

		srv, requests := newRecordingServer(t, http.StatusOK, `{"status":"published"}`)
		sharesPath := filepath.Join(dir, "session.json")
		require.NoError(t, os.WriteFile(sharesPath, []byte(
			`{"proposal_id":"p1","public_key":"pk","shares":["s1","s2","s3"],"threshold":2,"trustees":3}`,
		), 0o600))

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{
				"-base-url", srv.URL, "-admin-token", "secret",
				"result-publish", "-proposal", "p1", "-shares-file", sharesPath,
			})
		})

		require.Equal(t, 0, code, "命令失败: %s", errOut)

		req := <-requests
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(req.Body), &body))
		require.Equal(t, []any{"s1", "s2", "s3"}, body["shares"])
	})
}

// TestRun_UsageErrors 覆盖参数用法错误。
func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{name: "没有命令", args: nil, wantOut: "用法:"},
		{name: "未知命令", args: []string{"nope"}, wantOut: "用法:", wantErr: "未知命令"},
		{name: "全局参数错误", args: []string{"-nope"}, wantErr: "参数错误"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code := 0
			out, errOut := captureOutput(t, func() {
				code = run(tc.args)
			})
			require.Equal(t, 2, code)
			if tc.wantOut != "" {
				require.Contains(t, out, tc.wantOut)
			}
			if tc.wantErr != "" {
				require.Contains(t, errOut, tc.wantErr)
			}
		})
	}
}

// TestRun_HelpAndVersion 覆盖不碰网络的两个命令。
func TestRun_HelpAndVersion(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		code := 0
		out, _ := captureOutput(t, func() { code = run([]string{"help"}) })
		require.Equal(t, 0, code)
		require.Contains(t, out, "用法:")
	})

	t.Run("version", func(t *testing.T) {
		t.Parallel()

		code := 0
		out, _ := captureOutput(t, func() { code = run([]string{"version"}) })
		require.Equal(t, 0, code)
		require.Equal(t, version, out)
	})
}

// TestRun_APIErrorMapping 覆盖服务端报错的映射方式。
func TestRun_APIErrorMapping(t *testing.T) {
	t.Parallel()

	t.Run("带错误码", func(t *testing.T) {
		t.Parallel()

		srv, _ := newRecordingServer(t, http.StatusBadRequest, `{"code":"VALIDATION_ERROR","error":"参数不合法"}`)

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{"-base-url", srv.URL, "health"})
		})
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "参数不合法")
		require.Contains(t, errOut, "VALIDATION_ERROR")
	})

	t.Run("只有错误文案", func(t *testing.T) {
		t.Parallel()

		srv, _ := newRecordingServer(t, http.StatusBadRequest, `{"error":"只有文案"}`)

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{"-base-url", srv.URL, "health"})
		})
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "只有文案")
		require.NotContains(t, errOut, "()")
	})

	t.Run("非 JSON 响应", func(t *testing.T) {
		t.Parallel()

		srv, _ := newRecordingServer(t, http.StatusInternalServerError, `boom`)

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{"-base-url", srv.URL, "health"})
		})
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "HTTP 500")
	})

	t.Run("服务连不上", func(t *testing.T) {
		t.Parallel()

		code := 0
		_, errOut := captureOutput(t, func() {
			code = run([]string{"-base-url", "http://127.0.0.1:1", "health"})
		})
		require.Equal(t, 1, code)
		require.NotEmpty(t, errOut)
	})
}

// TestRun_AdminCommandRequiresToken 管理面命令缺 token 时必须在发请求前就失败。
func TestRun_AdminCommandRequiresToken(t *testing.T) {
	t.Parallel()

	c := &cli{
		baseURL:    "http://127.0.0.1:1",
		adminToken: "",
		httpClient: &http.Client{},
	}

	err := c.cmdCredentialStatuses([]string{"-proposal", "p1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "admin token")
}

// TestRun_MissingRequiredFlags 各命令缺必填参数时的报错。
func TestRun_MissingRequiredFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(c *cli) error
	}{
		{name: "proposal-create", call: func(c *cli) error { return c.cmdCreateProposal(nil) }},
		{name: "credential-issue", call: func(c *cli) error { return c.cmdIssueCredential(nil) }},
		{name: "credential-show", call: func(c *cli) error { return c.cmdShowCredential(nil) }},
		{name: "vote-submit", call: func(c *cli) error { return c.cmdVoteSubmit(nil) }},
		{name: "vote-status", call: func(c *cli) error { return c.cmdVoteStatus(nil) }},
		{name: "credentials-list", call: func(c *cli) error { return c.cmdCredentialStatuses(nil) }},
		{name: "decrypt-setup", call: func(c *cli) error { return c.cmdDecryptSetup(nil) }},
		{name: "result-publish", call: func(c *cli) error { return c.cmdPublishResult(nil) }},
		{name: "result-get", call: func(c *cli) error { return c.cmdGetResult(nil) }},
		{name: "anchors-list", call: func(c *cli) error { return c.cmdListAnchors(nil) }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &cli{baseURL: "http://127.0.0.1:1", httpClient: &http.Client{}}
			require.Error(t, tc.call(c))
		})
	}
}

func TestHelpers_SplitCSV(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"A", "B"}, splitCSV("A, B"))
	require.Equal(t, []string{"A"}, splitCSV("A,, ,"))
	require.Empty(t, splitCSV("  ,  "))
}

func TestHelpers_Stringify(t *testing.T) {
	t.Parallel()

	require.Equal(t, "x", stringify("x"))
	require.Equal(t, "", stringify(42))
	require.Equal(t, "", stringify(nil))
}

func TestHelpers_FirstNonEmpty(t *testing.T) {
	t.Parallel()

	require.Equal(t, "b", firstNonEmpty("", "  ", " b "))
	require.Equal(t, "", firstNonEmpty("", "  "))
}

func TestHelpers_RandomBase64(t *testing.T) {
	t.Parallel()

	first, err := randomBase64(32)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := randomBase64(32)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestHelpers_WriteAndReadJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "data.json")
	require.NoError(t, writeJSON(path, map[string]string{"a": "b"}))

	var out map[string]string
	require.NoError(t, readJSON(path, &out))
	require.Equal(t, "b", out["a"])

	require.Error(t, readJSON(filepath.Join(t.TempDir(), "missing.json"), &out))
}

func TestHelpers_LoadShares(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	values, err := loadShares("s1, s2", "")
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2"}, values)

	jsonPath := filepath.Join(dir, "shares.json")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`["s1","s2"]`), 0o600))
	values, err = loadShares("", jsonPath)
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2"}, values)

	textPath := filepath.Join(dir, "shares.txt")
	require.NoError(t, os.WriteFile(textPath, []byte("s1 s2\ns3"), 0o600))
	values, err = loadShares("", textPath)
	require.NoError(t, err)
	require.Equal(t, []string{"s1", "s2", "s3"}, values)

	emptyPath := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyPath, []byte("  \n "), 0o600))
	_, err = loadShares("", emptyPath)
	require.Error(t, err)

	_, err = loadShares("", "")
	require.Error(t, err)

	_, err = loadShares("", filepath.Join(dir, "missing.txt"))
	require.Error(t, err)
}

func TestHelpers_EnvOrDefault(t *testing.T) {
	t.Setenv("NULLIFIER_CLI_TEST_KEY", "  ")
	require.Equal(t, "fallback", envOrDefault("NULLIFIER_CLI_TEST_KEY", "fallback"))

	t.Setenv("NULLIFIER_CLI_TEST_KEY", "value")
	require.Equal(t, "value", envOrDefault("NULLIFIER_CLI_TEST_KEY", "fallback"))
}
