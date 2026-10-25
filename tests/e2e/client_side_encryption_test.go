package e2e

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestAPI_ClientSideEncryption_E2E 是能力 A（选票内容对服务端不可知）的端到端证据。
//
// 选票由**客户端**加密：客户端只拿到公开的计票公钥，在本地把选项下标加密成
// 密文，服务端收到的只有密文 —— 明文选项从未上过网。计票仍要能还原出正确的
// 选项，证明密文与提案的计票密钥是配套的。
//
// 浏览器端用的是 web/src/lib/ballot-cipher.ts，那份实现与 Go 侧逐字节一致
// （见 internal/infrastructure/crypto/ballot/kat_interop_test.go）。
func TestAPI_ClientSideEncryption_E2E(t *testing.T) {
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

	// 1) 创建提案
	resp := postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, map[string]any{
		"title":     "Client Side Election",
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

	// 2) 签发凭证（真两段式盲签名）
	credentialResp := issueCredential(
		t, ts, adminToken, proposalResp.ID, "u-1", []byte("user-pub-key"),
	)

	// 3) 初始化门限解密会话
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup",
		adminToken, map[string]any{"threshold": 2, "trustees": 3})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var setupResp struct {
		PublicKey string   `json:"public_key"`
		Shares    []string `json:"shares"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setupResp))
	resp.Body.Close()
	require.Len(t, setupResp.Shares, 3)

	// 4) 客户端取计票公钥（公开值），在本地加密选项下标。
	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/election-key")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var keyResp struct {
		PublicKey string `json:"public_key"`
		Encoding  string `json:"encoding"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&keyResp))
	resp.Body.Close()
	require.Equal(t, "uncompressed", keyResp.Encoding)
	electionKey, err := base64.StdEncoding.DecodeString(keyResp.PublicKey)
	require.NoError(t, err)
	require.Len(t, electionKey, 64)

	// 客户端选的选项是 candidate-B（下标 1）。
	ciphertext, _, err := ballot.EncryptBallotWithUncompressedKey(electionKey, 1)
	require.NoError(t, err)

	// 5) 提交选票：请求里只有密文，**没有明文选项**。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	resp = postJSON(t, ts.URL+"/api/v1/votes", map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString([]byte("dev-proof")),
		"nullifier":            strings.Repeat("a1", 32),
		"user_id":              "u-1",
		"user_pub_key":         credentialResp.UserPubKey,
		"credential_signature": credentialResp.Signature,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 6) 落库记录里没有明文选项：只有密文。
	stored, err := voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, ciphertext, stored[0].Cipher)

	// 7) 计票：门限私钥能还原出客户端选的那个选项。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"status":  "closed",
			"ends_at": time.Now().UTC().Add(-1 * time.Second),
		}).Error)

	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/results/publish",
		adminToken, map[string]any{"shares": setupResp.Shares[:2]})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var publishResp struct {
		TotalVotes int            `json:"total_votes"`
		Tally      map[string]int `json:"tally"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&publishResp))
	resp.Body.Close()
	require.Equal(t, 1, publishResp.TotalVotes)
	require.Equal(t, 1, publishResp.Tally["candidate-B"])
	require.Equal(t, 0, publishResp.Tally["candidate-A"])
}
