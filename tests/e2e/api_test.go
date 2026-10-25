package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type voteCircuit struct {
	X frontend.Variable
	Y frontend.Variable `gnark:",public"`
}

func (c *voteCircuit) Define(api frontend.API) error {
	x3 := api.Mul(c.X, c.X, c.X)
	api.AssertIsEqual(c.Y, api.Add(x3, c.X, 5))
	return nil
}

func TestAPI_VoteFlow_E2E(t *testing.T) {
	const adminToken = "secret"
	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	proofVerifier, err := crypto.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := crypto.NewGnarkMerkleRootBuilder()

	logger := zap.NewNop()
	votingUC := usecase.NewVotingUseCase(
		voteRepo,
		participationRepo,
		credentialRepo,
		proposalRepo,
		sessionRepo,
		proofVerifier,
		encrypter,
		issuer,
		logger,
	)
	registerUC := usecase.NewRegisterUseCase(credentialRepo, proposalRepo, issuer, logger)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		encrypter,
		merkleBuilder,
		logger,
	)

	handler := httpapi.NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)

	ts := httptest.NewServer(server.Engine())
	defer ts.Close()

	// 1) 签发凭证
	var resp *http.Response
	// 2) 创建提案
	startAt := time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339)
	endAt := time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339)
	proposalReq := map[string]any{
		"title":       "Board Election",
		"description": "Vote for one candidate",
		"options":     []string{"candidate-A", "candidate-B"},
		"starts_at":   startAt,
		"ends_at":     endAt,
	}
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, proposalReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposalResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposalResp))
	resp.Body.Close()
	require.NotEmpty(t, proposalResp.ID)

	credentialResp := issueCredential(
		t, ts, adminToken, proposalResp.ID, "u-1", []byte("user-pub-key"),
	)
	require.NotEmpty(t, credentialResp.UserPubKey)
	require.NotEmpty(t, credentialResp.Signature)

	// 3) 初始化阈值解密会话
	setupReq := map[string]any{
		"threshold": 2,
		"trustees":  3,
	}
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup", adminToken, setupReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var setupResp struct {
		PublicKey string   `json:"public_key"`
		Shares    []string `json:"shares"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setupResp))
	resp.Body.Close()
	require.Len(t, setupResp.Shares, 3)

	// 4) 生成有效 proof + 密文并投票（密文使用会话公钥）
	publicKey, err := base64.StdEncoding.DecodeString(setupResp.PublicKey)
	require.NoError(t, err)

	ciphertext, err := encrypter.Encrypt(publicKey, 0)
	require.NoError(t, err)

	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	proofBytes := mustBuildProofBytes(t, 3, 35)
	voteReq := map[string]any{
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(proofBytes),
		"nullifier":            strings.Repeat("a1", 32),
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	}

	// 4.1) 投票前状态为 false
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/vote-status", map[string]any{
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var statusBefore struct {
		HasVoted bool `json:"has_voted"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&statusBefore))
	resp.Body.Close()
	require.False(t, statusBefore.HasVoted)

	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/votes", voteReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 5) 同一用户再次投票（不同 nullifier）必须冲突
	voteReq["nullifier"] = strings.Repeat("b2", 32)
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/votes", voteReq)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	// 5.1) 冒充他人凭证必须被拒绝
	forgedVoteReq := map[string]any{
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(proofBytes),
		"nullifier":            strings.Repeat("c3", 32),
		"user_id":              "u-2",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	}
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/votes", forgedVoteReq)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// 6) 投票后状态为 true
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/vote-status", map[string]any{
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var statusAfter struct {
		HasVoted bool `json:"has_voted"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&statusAfter))
	resp.Body.Close()
	require.True(t, statusAfter.HasVoted)

	// 7) 查询开放提案
	resp, err = http.Get(ts.URL + "/api/v1/proposals/open")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 8) 发布计票结果
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"status":  "closed",
			"ends_at": time.Now().UTC().Add(-1 * time.Second),
		}).Error)

	publishReq := map[string]any{
		"shares": setupResp.Shares[:2],
	}
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/results/publish", adminToken, publishReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var publishResp struct {
		TotalVotes int            `json:"total_votes"`
		Tally      map[string]int `json:"tally"`
		AnchorHash string         `json:"anchor_hash"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&publishResp))
	resp.Body.Close()
	require.Equal(t, 1, publishResp.TotalVotes)
	require.Equal(t, 1, publishResp.Tally["candidate-A"])
	require.NotEmpty(t, publishResp.AnchorHash)

	// 9) 查询计票结果
	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/results")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var resultResp struct {
		TotalVotes int            `json:"total_votes"`
		Tally      map[string]int `json:"tally"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&resultResp))
	resp.Body.Close()
	require.Equal(t, 1, resultResp.TotalVotes)
	require.Equal(t, 1, resultResp.Tally["candidate-A"])

	// 10) 导出审计锚点
	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/audit-anchors")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var anchorsResp struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&anchorsResp))
	resp.Body.Close()
	require.NotEmpty(t, anchorsResp.Items)
}

func TestAPI_VoteFlow_ConcurrentSameUser_E2E(t *testing.T) {
	const adminToken = "secret"
	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver: "sqlite",
		// 文件库 + WAL：共享缓存的内存库在并发写时会直接抛 "table is locked"
		//（共享缓存用表级锁，busy_timeout 不管用）。两个并发投票会因此双双
		// 失败，断言「恰好一张票入库」变成 flaky。WAL 允许一个写者 + 并发读者，
		// 写写冲突由 busy_timeout 等待，_txlock=immediate 让事务一开始就拿写锁，
		// 避免「先读后写」的升级死锁。
		Database: "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "concurrent-user.db")) +
			"?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	// 句柄不关的话，Windows 上临时目录清理会因文件被占用而失败。
	t.Cleanup(func() { _ = sqlDB.Close() })

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	proofVerifier, err := crypto.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := crypto.NewGnarkMerkleRootBuilder()

	logger := zap.NewNop()
	votingUC := usecase.NewVotingUseCase(
		voteRepo,
		participationRepo,
		credentialRepo,
		proposalRepo,
		sessionRepo,
		proofVerifier,
		encrypter,
		issuer,
		logger,
	)
	registerUC := usecase.NewRegisterUseCase(credentialRepo, proposalRepo, issuer, logger)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		encrypter,
		merkleBuilder,
		logger,
	)

	handler := httpapi.NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)

	ts := httptest.NewServer(server.Engine())
	defer ts.Close()

	var resp *http.Response
	proposalReq := map[string]any{
		"title":       "Concurrent Vote Test",
		"description": "Same user concurrent vote requests",
		"options":     []string{"candidate-concurrent", "candidate-other"},
		"starts_at":   time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":     time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	}
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, proposalReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposalResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposalResp))
	_ = resp.Body.Close()

	credentialResp := issueCredential(
		t, ts, adminToken, proposalResp.ID, "u-concurrent", []byte("user-pub-key-concurrent"),
	)

	setupReq := map[string]any{
		"threshold": 2,
		"trustees":  3,
	}
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup", adminToken, setupReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var setupResp struct {
		PublicKey string `json:"public_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setupResp))
	_ = resp.Body.Close()

	publicKey, err := base64.StdEncoding.DecodeString(setupResp.PublicKey)
	require.NoError(t, err)
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)
	ciphertext, err := encrypter.Encrypt(publicKey, 0)
	require.NoError(t, err)
	proofBytes := mustBuildProofBytes(t, 3, 35)

	type submitResult struct {
		status int
		body   string
		err    error
	}

	start := make(chan struct{})
	results := make(chan submitResult, 2)
	nullifiers := []string{strings.Repeat("d4", 32), strings.Repeat("e5", 32)}
	var wg sync.WaitGroup
	for _, nullifier := range nullifiers {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			<-start

			reqBody := map[string]any{
				"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
				"proof":                base64.StdEncoding.EncodeToString(proofBytes),
				"nullifier":            n,
				"user_id":              "u-concurrent",
				"user_pub_key":         credentialResp.UserPubKey,
				"credential_signature": credentialResp.Signature,
			}
			raw, err := json.Marshal(reqBody)
			if err != nil {
				results <- submitResult{err: err}
				return
			}

			resp, err := http.Post(ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/votes", "application/json", bytes.NewReader(raw))
			if err != nil {
				results <- submitResult{err: err}
				return
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			results <- submitResult{
				status: resp.StatusCode,
				body:   string(bodyBytes),
			}
		}(nullifier)
	}

	close(start)
	wg.Wait()
	close(results)

	created := 0
	conflict := 0
	for result := range results {
		require.NoError(t, result.err)
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected submit status=%d body=%s", result.status, result.body)
		}
	}
	require.Equal(t, 1, created)
	require.Equal(t, 1, conflict)

	votes, err := voteRepo.GetByProposal(context.Background(), proposalResp.ID)
	require.NoError(t, err)
	require.Len(t, votes, 1)

	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/vote-status", map[string]any{
		"user_id":              "u-concurrent",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var statusResp struct {
		HasVoted bool `json:"has_voted"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&statusResp))
	_ = resp.Body.Close()
	require.True(t, statusResp.HasVoted)
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()

	data, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	return resp
}

func postAdminJSON(t *testing.T, url string, token string, body any) *http.Response {
	t.Helper()

	data, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func mustBuildProofBytes(t *testing.T, x int, y int) []byte {
	t.Helper()

	var circuit voteCircuit
	ccs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &circuit)
	require.NoError(t, err)

	pk, _, err := groth16.Setup(ccs)
	require.NoError(t, err)

	assignment := voteCircuit{X: x, Y: y}
	witness, err := frontend.NewWitness(&assignment, ecc.BN254)
	require.NoError(t, err)

	proof, err := groth16.Prove(ccs, pk, witness)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = proof.WriteTo(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}
