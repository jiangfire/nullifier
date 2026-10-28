package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAdminToken = "test-admin-token"

// fakeAPI 是压测工具用到的最小 API 面。
//
// 它模拟真实服务端的关键约束：管理端接口要 Bearer 令牌、凭证池先备槽位再领取、
// 凭证签发走**真两段式盲签名**（claim 预留 → blind-sign 盲签 → attest 验签落库）、
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
	// claimBodies 记下每次 claim 的请求体：领取请求**不许**带公钥
	// （服务端一旦知道公钥就能算出消息原文，盲化当场失效）。
	claimBodies []map[string]any
	// issuer 是签发方：假服务端要用它对盲化消息做盲签名。
	issuer domaincrypto.CredentialIssuer
	// enforceUnique 决定是否强制「同一 nullifier 只放行一张」。
	// 关掉它用来验证压测工具自己能不能发现防重投失守。
	enforceUnique bool
	// requireAdmin 为真时，管理端接口校验 Bearer 令牌。
	requireAdmin bool
	// verifier 可选：配了就**真验签**选票证明。
	// 真证明路径的证据 —— 没有它，「出了真证明」只是工具自己说了算。
	verifier *ballot.Verifier
	// eligibilityVerifier 可选：配了就真验资格证明。
	eligibilityVerifier *eligibility.Verifier
	// eligibilityRoot 是资格名单的树根（十六进制域元素）。
	eligibilityRoot string
	// qualificationIDs 记下用过的资格去重键：一人一票的守卫。
	qualificationIDs map[string]bool
	// eligibilityBodies 记下每次领取的资格材料。
	eligibilityBodies []map[string]any
	// challenges 是签发过的领取挑战数。
	challenges int
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	thresholdCrypto := cryptoinfra.NewBallotThresholdCrypto()
	_, publicKey, err := thresholdCrypto.GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := thresholdCrypto.UncompressPublicKey(publicKey)
	require.NoError(t, err)

	return &fakeAPI{
		proposalID:       "p-loadtest",
		publicKey:        base64.StdEncoding.EncodeToString(uncompressed),
		startsAt:         time.Now().UTC(),
		endsAt:           time.Now().UTC().Add(time.Hour),
		nullifiers:       make(map[string]bool),
		votedUsers:       make(map[string]bool),
		issuer:           cryptoinfra.NewKryptologyCredentialIssuer(),
		enforceUnique:    true,
		requireAdmin:     true,
		qualificationIDs: make(map[string]bool),
	}
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/proposals", f.createProposal)
	mux.HandleFunc("POST /api/v1/proposals/{id}/decryption/setup", f.setupDecryption)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/pool", f.fillPool)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/eligibility-challenge", f.issueEligibilityChallenge)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/claim", f.claimCredential)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/blind-sign", f.blindSignCredential)
	mux.HandleFunc("POST /api/v1/proposals/{id}/credentials/attest", f.attestCredential)
	mux.HandleFunc("GET /api/v1/proposals/{id}/election-key", f.electionKey)
	mux.HandleFunc("GET /api/v1/proposals/{id}/credentials", f.listCredentials)
	mux.HandleFunc("GET /api/v1/proposals", f.listProposals)
	mux.HandleFunc("POST /api/v1/proposals/{id}/votes", f.submitVote)
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

// issueEligibilityChallenge 签发一次领取挑战。
//
// 挑战是一个 64 位十六进制的域元素：它会进资格电路的公开输入。
func (f *fakeAPI) issueEligibilityChallenge(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.challenges += 1
	challenge := fmt.Sprintf("%064x", f.challenges)
	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id": f.proposalID,
		"root":        f.eligibilityRoot,
		"depth":       eligibility.DefaultTreeDepth,
		"challenge":   challenge,
	})
}

// claimCredential 预留签发权：**不签名**，也不收公钥。
//
// 响应里带出签发方公钥 —— 客户端要拿它把凭证消息盲化。
func (f *fakeAPI) claimCredential(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&body)

	f.mu.Lock()
	f.claims += 1
	userID := fmt.Sprintf("u-%d", f.claims)
	f.claimBodies = append(f.claimBodies, body)
	f.mu.Unlock()

	// 开了资格校验就必须带资格材料，且证明必须验得过、去重键不能重复。
	if f.eligibilityVerifier != nil {
		if err := f.checkEligibility(body); err != nil {
			writeJSON(w, http.StatusForbidden, ginError("INVALID_ELIGIBILITY_PROOF", err.Error()))
			return
		}
	}

	issuerKey, err := f.issuer.IssuerPublicKey(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ginError("INTERNAL_ERROR", err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"proposal_id":        f.proposalID,
		"user_id":            userID,
		"issuer_key_version": issuerKey.Version,
		"variant":            issuerKey.Variant,
		"modulus":            base64.StdEncoding.EncodeToString(issuerKey.Modulus),
		"exponent":           issuerKey.Exponent,
	})
}

// checkEligibility 校验领取请求里的资格材料。
//
// 三件事：证明要验得过；去重键不能重复（一人一票）；材料要齐。
func (f *fakeAPI) checkEligibility(body map[string]any) error {
	encodedProof, _ := body["eligibility_proof"].(string)
	qualificationID, _ := body["qualification_id"].(string)
	challenge, _ := body["challenge"].(string)
	if encodedProof == "" || qualificationID == "" || challenge == "" {
		return errors.New("eligibility material is incomplete")
	}
	proof, err := base64.StdEncoding.DecodeString(encodedProof)
	if err != nil {
		return fmt.Errorf("decode eligibility proof: %w", err)
	}
	statement, err := eligibility.BuildStatement(
		f.eligibilityRoot, f.proposalID, challenge, qualificationID,
	)
	if err != nil {
		return fmt.Errorf("build statement: %w", err)
	}
	if err := f.eligibilityVerifier.Verify(proof, statement); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eligibilityBodies = append(f.eligibilityBodies, body)
	if f.qualificationIDs[qualificationID] {
		return errors.New("qualification already used")
	}
	f.qualificationIDs[qualificationID] = true
	return nil
}

// blindSignCredential 对**盲化后**的消息做盲签名。
func (f *fakeAPI) blindSignCredential(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID           string `json:"user_id"`
		BlindedMessage   string `json:"blinded_message"`
		IssuerKeyVersion int    `json:"issuer_key_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}
	blinded, err := base64.StdEncoding.DecodeString(payload.BlindedMessage)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", "blinded_message must be base64"))
		return
	}
	signature, err := f.issuer.BlindSign(r.Context(), payload.IssuerKeyVersion, blinded)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ginError("INTERNAL_ERROR", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"blind_signature": base64.StdEncoding.EncodeToString(signature),
	})
}

// attestCredential 校验客户端解盲出来的最终签名，通过后落库。
func (f *fakeAPI) attestCredential(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID     string `json:"user_id"`
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", err.Error()))
		return
	}
	userPubKey, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", "user_pub_key must be base64"))
		return
	}
	signature, err := base64.StdEncoding.DecodeString(payload.Signature)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ginError("INVALID_REQUEST", "signature must be base64"))
		return
	}
	if _, err := f.issuer.VerifyCredential(payload.UserID, userPubKey, signature); err != nil {
		writeJSON(w, http.StatusForbidden, ginError("INVALID_CREDENTIAL", err.Error()))
		return
	}

	f.mu.Lock()
	f.credentials = append(f.credentials, payload.UserID)
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           fmt.Sprintf("c-%s", payload.UserID),
		"proposal_id":  f.proposalID,
		"user_id":      payload.UserID,
		"user_pub_key": payload.UserPubKey,
		"signature":    payload.Signature,
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
			"options":   []string{"candidate-A", "candidate-B"},
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
		Nullifier     string `json:"nullifier"`
		UserID        string `json:"user_id"`
		EncryptedVote string `json:"encrypted_vote"`
		Proof         string `json:"proof"`
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
	// 配了验证器就真验签：服务端按请求里的密文与 nullifier 自拼公开输入。
	if f.verifier != nil {
		if err := f.verifyVote(payload.EncryptedVote, payload.Proof, payload.Nullifier); err != nil {
			writeJSON(w, http.StatusBadRequest, ginError("INVALID_PROOF", err.Error()))
			return
		}
	}
	f.nullifiers[payload.Nullifier] = true
	f.votedUsers[payload.UserID] = true
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ok"})
}

// verifyVote 用与服务端相同的入口拼公开输入并验签。
func (f *fakeAPI) verifyVote(encodedCiphertext string, encodedProof string, nullifierHex string) error {
	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return fmt.Errorf("decode encrypted_vote: %w", err)
	}
	proof, err := base64.StdEncoding.DecodeString(encodedProof)
	if err != nil {
		return fmt.Errorf("decode proof: %w", err)
	}
	nullifier, err := hex.DecodeString(nullifierHex)
	if err != nil {
		return fmt.Errorf("decode nullifier: %w", err)
	}
	uncompressed, err := base64.StdEncoding.DecodeString(f.publicKey)
	if err != nil {
		return fmt.Errorf("decode election public key: %w", err)
	}
	compressed, err := ballot.CompressPoint(uncompressed)
	if err != nil {
		return fmt.Errorf("compress election public key: %w", err)
	}
	inputs, err := ballot.BuildPublicInputs(entity.BallotStatement{
		ProposalID:        f.proposalID,
		Nullifier:         nullifier,
		ElectionPublicKey: compressed,
		Ciphertext:        ciphertext,
		NumOptions:        2,
	})
	if err != nil {
		return fmt.Errorf("build public inputs: %w", err)
	}
	return f.verifier.Verify(proof, inputs)
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

	// 领取请求**不许带公钥**：服务端一旦知道公钥就能算出消息原文，盲化当场失效。
	// 这条断言同时钉住协议版本 —— v0.1.0 的一步式领取是把公钥直接发过去的。
	for index, body := range api.claimBodies {
		assert.NotContains(t, body, "user_pub_key", "第 %d 次领取的请求体带了公钥", index+1)
	}
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
	// 吞吐是**派生数字**：这一轮如果比计时器的分辨率还快，就测不准 —— 那时必须
	// 标出来，而不是报一个假的 0 或一个假的高吞吐。
	assert.GreaterOrEqual(t, result.ThroughputPerS, 0.0)
	if result.TimingResolutionLimited {
		assert.Equal(t, 0.0, result.ThroughputPerS, "测不准时吞吐应当是 0，并单独标记")
	} else {
		assert.Greater(t, result.ThroughputPerS, 0.0)
	}
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

// TestRun_RealProof_VerifiesAgainstServer 覆盖「真证明」路径。
//
// 它是 T27 的核心证据：压测工具出的证明**必须能被服务端那套公开输入验过** ——
// 不是工具自己说了算。假服务端配了真验证器，任何一个公开输入对不上都会 400。
func TestRun_RealProof_VerifiesAgainstServer(t *testing.T) {
	dir := t.TempDir()
	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	provingKeyPath := filepath.Join(dir, "proving.key")
	verifyingKeyPath := filepath.Join(dir, "verifying.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o600))

	api := newFakeAPI(t)
	verifier, err := ballot.NewVerifier(verifyingKeyPath)
	require.NoError(t, err)
	api.verifier = verifier
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", testAdminToken,
		"-votes", "4",
		"-concurrency", "2",
		"-open-delay", "0",
		"-duplicates", "3",
		"-proving-key", provingKeyPath,
		"-json",
	})
	output := read()
	restore()

	assert.Equal(t, 0, code, "真证明路径应当成功退出，输出：\n%s", output)

	// stdout 必须是**纯 JSON**：gnark 的日志（编译电路、出证明）走 stderr，
	// 混进 stdout 的话 `-json > report.json` 出来的文件根本解析不了。
	var result report
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	assert.True(t, result.RealProof)
	// 4 张票 + 3 个防重投复核 = 7 份证明：复核那几份各自有自己的密文
	//（密文进公开输入），但**共用同一个 nullifier**。
	assert.Equal(t, 7, result.ProofsGenerated)
	assert.Equal(t, map[string]int{"201": 4}, result.StatusCounts)
	assert.NotEmpty(t, result.ProofGeneration, "客户端出证明的耗时要单独报出来")
	assert.Contains(t, proofModeLabel(result), "真证明")

	api.mu.Lock()
	defer api.mu.Unlock()
	assert.Equal(t, 7, api.voteRequests)
	assert.Len(t, api.nullifiers, 5, "4 张票 + 1 个共用的 nullifier")
}

// TestRun_EligibilityPath 覆盖「带资格证明领取」路径。
//
// 这是能在 **release 模式**（强制一人一票）的服务上跑压测的前提：
// 没有资格材料，领取直接被 ELIGIBILITY_REQUIRED 拒掉。
func TestRun_EligibilityPath(t *testing.T) {
	dir := t.TempDir()
	eligProvingKey, eligVerifyingKey, err := eligibility.GenerateKeys(eligibility.DefaultTreeDepth)
	require.NoError(t, err)
	eligProvingKeyPath := filepath.Join(dir, "eligibility-proving.key")
	require.NoError(t, os.WriteFile(eligProvingKeyPath, eligProvingKey, 0o600))
	verifier, err := eligibility.NewVerifierFromBytes(eligVerifyingKey)
	require.NoError(t, err)

	root, memberDir := writeEligibilityList(t, dir, 6)

	api := newFakeAPI(t)
	api.eligibilityVerifier = verifier
	api.eligibilityRoot = root
	server := newFakeServer(t, api)

	read, restore := captureStdout(t)
	code := run([]string{
		"-base-url", server.URL,
		"-admin-token", testAdminToken,
		"-votes", "3",
		"-concurrency", "2",
		"-open-delay", "0",
		"-duplicates", "2",
		"-eligibility-dir", memberDir,
		"-eligibility-proving-key", eligProvingKeyPath,
	})
	output := read()
	restore()

	assert.Equal(t, 0, code, "资格路径应当成功退出，输出：\n%s", output)

	api.mu.Lock()
	defer api.mu.Unlock()
	assert.Equal(t, 5, api.claims, "票数 + 防重投复核，每个成员各领一次")
	assert.Len(t, api.qualificationIDs, 5, "5 个不同的资格去重键")
	for index, body := range api.eligibilityBodies {
		assert.NotEmpty(t, body["eligibility_proof"], "第 %d 次领取带了资格证明", index+1)
		assert.NotEmpty(t, body["qualification_id"], "第 %d 次领取带了过去重键", index+1)
		assert.NotEmpty(t, body["challenge"], "第 %d 次领取带了挑战", index+1)
	}
}

// TestRun_EligibilityRequiresProvingKey 钉住「资格目录与资格 key 必须成对」。
//
// 只给一个就是「配错」—— 那时要么报错，要么更糟：悄悄走了没校验的路径。
func TestRun_EligibilityRequiresProvingKey(t *testing.T) {
	read, restore := captureStdout(t)
	code := run([]string{
		"-admin-token", testAdminToken,
		"-votes", "1",
		"-concurrency", "1",
		"-eligibility-dir", t.TempDir(),
	})
	read()
	restore()

	assert.Equal(t, 2, code, "缺资格 proving key 属于用法错误")
}

// writeEligibilityList 造一份资格名单：返回树根与成员包目录。
//
// 它照着 `cmd/eligibility-list` 的做法走：成员包先写，root 最后 ——
// 压测要的就是「一份能用的名单」，不是名单工具本身的行为。
func writeEligibilityList(t *testing.T, dir string, members int) (string, string) {
	t.Helper()
	memberDir := filepath.Join(dir, "eligibility")
	require.NoError(t, os.MkdirAll(memberDir, 0o700))

	secrets := make([][]byte, members)
	leaves := make([]fr.Element, members)
	for index := range members {
		secret := make([]byte, eligibility.FieldSize)
		_, err := rand.Read(secret)
		require.NoError(t, err)
		canonical, err := eligibility.CanonicalSecret(secret)
		require.NoError(t, err)
		secrets[index] = canonical
		leaf, err := eligibility.DeriveLeaf(canonical)
		require.NoError(t, err)
		leaves[index] = leaf
	}
	root, paths, err := eligibility.BuildTree(leaves, eligibility.DefaultTreeDepth)
	require.NoError(t, err)
	rootHex := eligibility.EncodeField(root)

	for index := range members {
		path := make([]string, len(paths[index]))
		for j := range paths[index] {
			path[j] = eligibility.EncodeField(paths[index][j])
		}
		pack := eligibility.MemberPackage{
			MemberID: fmt.Sprintf("member-%02d", index),
			Secret:   hex.EncodeToString(secrets[index]),
			Leaf:     eligibility.EncodeField(leaves[index]),
			Index:    uint64(index),
			Path:     path,
			Root:     rootHex,
			Depth:    eligibility.DefaultTreeDepth,
		}
		raw, err := json.Marshal(pack)
		require.NoError(t, err)
		name := filepath.Join(memberDir, fmt.Sprintf("eligibility-member-%02d.json", index))
		require.NoError(t, os.WriteFile(name, raw, 0o600))
	}
	return rootHex, memberDir
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
	assert.False(t, result.TimingResolutionLimited, "耗时测得到就不是「低于分辨率」")
}

// TestBuildReport_TimingBelowResolution 钉住「这一轮比计时器还快」时的报告形态。
//
// 计时器有分辨率（本机约 1ms）：耗时更短的阶段会测出 0。这时报告**不能**写
// 「0 req/s」（会被误读成没跑），也不该编一个高吞吐出来 —— 而是单独标出来。
func TestBuildReport_TimingBelowResolution(t *testing.T) {
	votePhase := phaseResult{
		outcomes: []outcome{
			{status: http.StatusCreated},
			{status: http.StatusCreated},
		},
		elapsed: 0,
	}
	result := buildReport(&options{concurrency: 2}, "p1", votePhase, phaseResult{}, 0)

	assert.True(t, result.TimingResolutionLimited)
	assert.Equal(t, 0.0, result.ThroughputPerS)
	assert.Equal(t, 2, result.Votes, "请求数照样是准的")
}
