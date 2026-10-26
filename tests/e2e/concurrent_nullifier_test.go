package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"anonvote/internal/infrastructure/crypto/ballot"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestAPI_ConcurrentSameNullifier_E2E 覆盖路线图 §4.3 的第三个盲区：
// **并发投票下的防重投**。
//
// 用例里 N 个请求**同时**用同一个 nullifier 提交，但各自带不同的凭证
// （不同的 user_id）—— 因此参与状态的唯一索引拦不住它们，唯一能拦住的是
// `votes.nullifier` 上的唯一索引。
//
// 关键性质：usecase 里是「先查后写」，两个请求完全可能都通过查重；真正的
// 防线是数据库唯一索引。这条用例断言的就是：**最终只有一张票入库**，其余
// 全部被拒（409 DOUBLE_VOTING）。
func TestAPI_ConcurrentSameNullifier_E2E(t *testing.T) {
	const (
		adminToken  = "secret"
		concurrency = 8
	)

	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver: "sqlite",
		// 文件库 + WAL：共享缓存的内存库在并发写时会直接抛 "table is locked"
		//（共享缓存用表级锁，busy_timeout 不管用）。WAL 允许一个写者 + 并发读者，
		// 写写冲突由 busy_timeout 等待，_txlock=immediate 让事务一开始就拿写锁，
		// 避免「先读后写」的升级死锁。
		Database: "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "concurrent.db")) +
			"?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(concurrency)
	sqlDB.SetMaxIdleConns(concurrency)
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
		voteRepo, participationRepo, credentialRepo, proposalRepo, sessionRepo,
		proofVerifier, encrypter, issuer, logger,
	)
	registerUC := usecase.NewRegisterUseCase(credentialRepo, proposalRepo, issuer, logger)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		encrypter, merkleBuilder, logger,
	)

	handler := httpapi.NewHandlerWithTally(
		votingUC, registerUC, proposalUC, participationUC, tallyUC, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)
	ts := httptest.NewServer(server.Engine())
	defer ts.Close()

	// 1) 创建提案（先不开窗，setup 要求未开始）。
	resp := postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, map[string]any{
		"title":     "Concurrent Nullifier Test",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposalResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposalResp))
	resp.Body.Close()

	// 2) 初始化门限解密会话。
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup",
		adminToken, map[string]any{"threshold": 2, "trustees": 3})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 3) 签发 N 张**不同**的凭证：参与状态的唯一索引拦不住它们，
	//    唯一能拦住重复投票的是 nullifier 的唯一索引。
	credentials := make([]struct {
		userID    string
		pubKey    string
		signature string
	}, 0, concurrency)
	for i := 0; i < concurrency; i++ {
		userID := fmt.Sprintf("u-concurrent-%d", i)
		credential := issueCredential(
			t, ts, adminToken, proposalResp.ID, userID, []byte("pub-"+userID),
		)
		credentials = append(credentials, struct {
			userID    string
			pubKey    string
			signature string
		}{userID: userID, pubKey: credential.UserPubKey, signature: credential.Signature})
	}

	// 4) 取公开的计票公钥，在本地加密一份合法密文（每个请求都用同一份密文：
	//    这里要验的是防重投，不是密文的多样性）。
	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/election-key")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var keyResp struct {
		PublicKey string `json:"public_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&keyResp))
	resp.Body.Close()
	electionKey, err := base64.StdEncoding.DecodeString(keyResp.PublicKey)
	require.NoError(t, err)
	ciphertext, _, err := ballot.EncryptBallotWithUncompressedKey(electionKey, 0)
	require.NoError(t, err)

	// 5) 把投票窗口挪到现在。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	// 6) N 个请求**同时**提交，同一个 nullifier，不同的凭证。
	nullifier := strings.Repeat("b2", 32)
	type outcome struct {
		status int
		code   string
		detail string
	}
	outcomes := make([]outcome, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			credential := credentials[index]
			body, marshalErr := json.Marshal(map[string]any{
				"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
				"proof":                base64.StdEncoding.EncodeToString([]byte("dev-proof")),
				"nullifier":            nullifier,
				"user_id":              credential.userID,
				"user_pub_key":         credential.pubKey,
				"credential_signature": credential.signature,
			})
			if marshalErr != nil {
				outcomes[index] = outcome{status: 0, code: "marshal:" + marshalErr.Error()}
				return
			}
			<-start
			req, reqErr := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/votes", bytes.NewReader(body))
			if reqErr != nil {
				outcomes[index] = outcome{status: 0, code: "request:" + reqErr.Error()}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, doErr := http.DefaultClient.Do(req)
			if doErr != nil {
				outcomes[index] = outcome{status: 0, code: "do:" + doErr.Error()}
				return
			}
			defer func() { _ = res.Body.Close() }()
			var payload struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			_ = json.NewDecoder(res.Body).Decode(&payload)
			outcomes[index] = outcome{status: res.StatusCode, code: payload.Code, detail: payload.Error}
		}(i)
	}
	close(start)
	wg.Wait()

	// 7) 断言：恰好一张票入库，其余全部被拒。
	created, conflicts := 0, 0
	for _, result := range outcomes {
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			require.Equal(t, "DOUBLE_VOTING", result.code)
			conflicts++
		default:
			t.Fatalf("意外的响应: status=%d code=%s detail=%s", result.status, result.code, result.detail)
		}
	}
	require.Equal(t, 1, created, "同一个 nullifier 只能有一张票入库")
	require.Equal(t, concurrency-1, conflicts, "其余请求必须被唯一索引拦下")

	stored, err := voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1, "票表里只能有一行")
}
