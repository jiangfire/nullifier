package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	cryptoinfra "anonvote/internal/infrastructure/crypto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAdminToken = "test-admin-token"

// fakeAPI 是压测工具用到的最小 API 面。
//
// 它模拟真实服务端的关键约束：管理端接口要 Bearer 令牌、凭证池先备槽位再领取、
// 同一 nullifier 只能落一张票。
type fakeAPI struct {
	mu           sync.Mutex
	proposalID   string
	publicKey    string
	startsAt     time.Time
	endsAt       time.Time
	nullifiers   map[string]bool
	votedUsers   map[string]bool
	credentials  []string
	poolSize     int
	claims       int
	voteRequests int
	// enforceUnique 决定是否强制「同一 nullifier 只放行一张」。
	// 关掉它用来验证压测工具自己能不能发现防重投失守。
	enforceUnique bool
	// requireAdmin 为真时，管理端接口校验 Bearer 令牌。
	requireAdmin bool
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	thresholdCrypto := cryptoinfra.NewBallotThresholdCrypto()
	_, publicKey, err := thresholdCrypto.GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := thresholdCrypto.UncompressPublicKey(publicKey)
	require.NoError(t, err)

	return &fakeAPI{
		proposalID:    "p-loadtest",
		publicKey:     base64.StdEncoding.EncodeToString(uncompressed),
		startsAt:      time.Now().UTC(),
		endsAt:        time.Now().UTC().Add(time.Hour),
		nullifiers:    make(map[string]bool),
		votedUsers:    make(map[string]bool),
		enforceUnique: true,
		requireAdmin:  true,
	}
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/proposals", f.createProposal)
	mux.HandleFunc("POST /api/v1/proposals/{id}/decryption/setup", f.setupDecryption)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/pool", f.fillPool)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/claim", f.claimCredential)
	mux.HandleFunc("GET /api/v1/proposals/{id}/election-key", f.electionKey)
	mux.HandleFunc("GET /api/v1/proposals/{id}/credentials", f.listCredentials)
	mux.HandleFunc("GET /api/v1/proposals", f.listProposals)
	mux.HandleFunc("POST /api/v1/votes", f.submitVote)
	return mux
}

func (f *fakeAPI) authorized(r *http.Request) bool {
	if !f.requireAdmin {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+testAdminToken
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeAPI) createProposal(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, ginError("UNAUTHORIZED", "unauthorized"))
		return
	}
	var payload struct {
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}
	f.mu.Lock()
	f.startsAt, _ = time.Parse(time.RFC3339, payload.StartsAt)
	f.endsAt, _ = time.Parse(time.RFC3339, payload.EndsAt)
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{"id": f.proposalID})
}

func (f *fakeAPI) setupDecryption(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, ginError("UNAUTHORIZED", "unauthorized"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ok"})
}

func (f *fakeAPI) fillPool(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, ginError("UNAUTHORIZED", "unauthorized"))
		return
	}
	var payload struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}
	f.mu.Lock()
	f.poolSize += payload.Count
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ok", "count": payload.Count})
}

func (f *fakeAPI) claimCredential(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserPubKey string `json:"user_pub_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}
	f.mu.Lock()
	f.claims += 1
	userID := fmt.Sprintf("u-%d", f.claims)
	f.credentials = append(f.credentials, userID)
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           fmt.Sprintf("c-%s", userID),
		"user_id":      userID,
		"user_pub_key": payload.UserPubKey,
		"signature":    base64.StdEncoding.EncodeToString([]byte("signature-" + userID)),
	})
}

func (f *fakeAPI) electionKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id": f.proposalID,
		"public_key":  f.publicKey,
		"encoding":    "uncompressed",
	})
}

func (f *fakeAPI) listProposals(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{{
			"id":        f.proposalID,
			"starts_at": f.startsAt.Format(time.RFC3339),
			"ends_at":   f.endsAt.Format(time.RFC3339),
		}},
	})
}

func (f *fakeAPI) listCredentials(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, ginError("UNAUTHORIZED", "unauthorized"))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]map[string]any, 0, len(f.credentials))
	for index, userID := range f.credentials {
		items = append(items, map[string]any{
			"credential_id": fmt.Sprintf("c-%d", index),
			"user_id":       userID,
			"has_voted":     f.votedUsers[userID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (f *fakeAPI) submitVote(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Nullifier string `json:"nullifier"`
		UserID    string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.voteRequests += 1
	if f.enforceUnique && f.nullifiers[payload.Nullifier] {
		writeJSON(w, http.StatusConflict, ginError("DOUBLE_VOTING", "double voting"))
		return
	}
	f.nullifiers[payload.Nullifier] = true
	f.votedUsers[payload.UserID] = true
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ok"})
}

func ginError(code string, message string) map[string]string {
	return map[string]string{"code": code, "error": message}
}

// captureStdout 接管 os.Stdout，返回读取到的内容与恢复函数。
//
// 读取函数会先关闭写端 —— 读端要等到 EOF 才会结束，而 EOF 只有写端关闭后才来。
// 顺序反了就是死锁。
func captureStdout(t *testing.T) (func() string, func()) {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = writer

	done := make(chan string, 1)
	go func() {
		raw, _ := io.ReadAll(reader)
		done <- string(raw)
	}()

	return func() string {
			os.Stdout = original
			_ = writer.Close()
			return <-done
		}, func() {
			os.Stdout = original
			_ = reader.Close()
		}
}

func newFakeServer(t *testing.T, api *fakeAPI) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)
	return server
}

func TestRun_AgainstFakeServer(t *testing.T) {
	api := newFakeAPI(t)
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", testAdminToken,
		"-votes", "12",
		"-concurrency", "4",
		"-open-delay", "0",
		"-duplicates", "6",
	})
	output := read()
	restore()

	assert.Equal(t, 0, code, "压测应当成功退出，输出：\n%s", output)
	assert.Contains(t, output, "防重投复核")
	assert.Contains(t, output, "201×12")

	api.mu.Lock()
	defer api.mu.Unlock()
	// 12 张不同的票 + 同 nullifier 并发 6 次 = 18 个请求，其中恰好 13 张入库。
	assert.Equal(t, 18, api.voteRequests)
	assert.Equal(t, 18, api.claims, "凭证池要备够 votes + duplicates 个槽位")
	assert.Len(t, api.nullifiers, 13, "13 张票对应 13 个不同的 nullifier")
	assert.Len(t, api.votedUsers, 13)
}

func TestRun_JSONReport(t *testing.T) {
	api := newFakeAPI(t)
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", testAdminToken,
		"-votes", "5",
		"-concurrency", "2",
		"-open-delay", "0",
		"-duplicates", "3",
		"-json",
	})
	output := read()
	restore()

	require.Equal(t, 0, code, "输出：\n%s", output)
	var result report
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	assert.Equal(t, "p-loadtest", result.ProposalID)
	assert.Equal(t, 5, result.Votes)
	assert.Equal(t, 1, result.DuplicateOK)
	assert.Equal(t, 3, result.DuplicateCount)
	assert.Equal(t, 6, result.VotedCredential)
	assert.Equal(t, 6, result.ExpectedVoted)
	assert.Equal(t, map[string]int{"201": 5}, result.StatusCounts)
	assert.Greater(t, result.ThroughputPerS, 0.0)
}

// TestRun_DetectsDuplicateReplay 验证压测工具能发现「防重投没守住」。
//
// 防重投失守时工具必须以非 0 退出 —— 它的结论要能当门禁用。
func TestRun_DetectsDuplicateReplay(t *testing.T) {
	api := newFakeAPI(t)
	api.enforceUnique = false
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", testAdminToken,
		"-votes", "3",
		"-concurrency", "2",
		"-open-delay", "0",
		"-duplicates", "4",
	})
	output := read()
	restore()

	assert.Equal(t, 1, code, "防重投失守必须失败退出，输出：\n%s", output)
}

func TestRun_Unauthorized(t *testing.T) {
	api := newFakeAPI(t)
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", "wrong-token",
		"-votes", "1",
		"-concurrency", "1",
		"-open-delay", "0",
	})
	read()
	restore()

	assert.Equal(t, 1, code)
}

func TestRun_MissingAdminToken(t *testing.T) {
	read, restore := captureStdout(t)
	code := run([]string{"-votes", "1", "-concurrency", "1"})
	read()
	restore()

	assert.Equal(t, 2, code, "缺少管理令牌属于用法错误")
}

func TestRun_InvalidFlags(t *testing.T) {
	read, restore := captureStdout(t)
	code := run([]string{"-votes", "0", "-concurrency", "1", "-admin-token", testAdminToken})
	read()
	restore()

	assert.Equal(t, 2, code)
}

func TestPercentile(t *testing.T) {
	sorted := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}
	assert.Equal(t, 10*time.Millisecond, percentile(sorted, 0))
	assert.Equal(t, 20*time.Millisecond, percentile(sorted, 0.5))
	assert.Equal(t, 40*time.Millisecond, percentile(sorted, 1))
	assert.Equal(t, time.Duration(0), percentile(nil, 0.5))
}

func TestStatusLabel(t *testing.T) {
	assert.Equal(t, "201", statusLabel(outcome{status: http.StatusCreated}))
	assert.Equal(t, "409/DOUBLE_VOTING", statusLabel(outcome{status: http.StatusConflict, code: "DOUBLE_VOTING"}))
	assert.True(t, strings.HasPrefix(statusLabel(outcome{status: 0, code: "boom"}), "error:"))
}

func TestFormatCounts(t *testing.T) {
	counts := map[string]int{"409/DOUBLE_VOTING": 4, "201": 1}
	formatted := formatCounts(counts)
	assert.Contains(t, formatted, "201×1")
	assert.Contains(t, formatted, "409/DOUBLE_VOTING×4")
}

func TestBuildReport(t *testing.T) {
	votePhase := phaseResult{
		outcomes: []outcome{
			{status: http.StatusCreated, latency: 5 * time.Millisecond},
			{status: http.StatusCreated, latency: 15 * time.Millisecond},
		},
		elapsed: 100 * time.Millisecond,
	}
	duplicatePhase := phaseResult{
		outcomes: []outcome{
			{status: http.StatusCreated, latency: time.Millisecond},
			{status: http.StatusConflict, code: "DOUBLE_VOTING"},
		},
	}
	result := buildReport(&options{concurrency: 2}, "p1", votePhase, duplicatePhase, 3)

	assert.Equal(t, "p1", result.ProposalID)
	assert.Equal(t, 2, result.Votes)
	assert.Equal(t, 20.0, result.ThroughputPerS)
	assert.Equal(t, 1, result.DuplicateOK)
	assert.Equal(t, 3, result.ExpectedVoted)
	assert.Equal(t, map[string]int{"201": 2}, result.StatusCounts)
}
