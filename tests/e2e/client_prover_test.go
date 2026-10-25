package e2e

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestAPI_ClientSideProof_E2E 是「客户端自己出证明」的端到端证据。
//
// 服务端配了**真的 verifying key**（不是开发模式的「proof 非空即通过」），
// 证明由客户端在本地用 proving key 生成 —— 服务端全程没有参与出证明，
// 也从未看到明文选项。
//
// 这条链路把能力 A（选票内容对服务端不可知）、能力 B（选票可验证）与
// 能力 C（proof 与提案 / nullifier / 密文绑定）合在一起验证：客户端拼出的
// 公开输入必须与服务端自拼的那一份**逐字段相同**，否则验签必然失败。
//
// 服务端以**非 debug 模式**启动：ballot-draft（服务端代加密）此时是 404，
// 因此这张票只可能来自客户端。
func TestAPI_ClientSideProof_E2E(t *testing.T) {
	const adminToken = "secret"

	// 电路密钥：proving key 在客户端侧，verifying key 部署到服务端。
	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	provingKeyPath := filepath.Join(keyDir, "proving.key")
	verifyingKeyPath := filepath.Join(keyDir, "verifying.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o644))

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

	// 真的验签：vk_path 指向 verifying key，服务端不再放过任何非空 proof。
	proofVerifier, err := crypto.NewGnarkProofVerifier(verifyingKeyPath)
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
		encrypter, crypto.NewBallotNullifierGenerator(), merkleBuilder, logger,
	)

	// 非 debug 模式：服务端代加密的 ballot-draft 不可达。
	handler := httpapi.NewHandlerWithTallyAndMode(
		votingUC, registerUC, proposalUC, participationUC, tallyUC, false, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)
	ts := httptest.NewServer(server.Engine())
	defer ts.Close()

	// 1) 创建提案（两个选项）。
	resp := postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, map[string]any{
		"title":     "Client Side Proof Election",
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

	// 2) 签发凭证（真两段式盲签名）。
	credential := issueCredential(
		t, ts, adminToken, proposalResp.ID, "u-1", []byte("user-pub-key"),
	)

	// 3) 初始化门限解密会话。
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
	uncompressedKey, err := base64.StdEncoding.DecodeString(keyResp.PublicKey)
	require.NoError(t, err)

	// 客户端选 candidate-B（下标 1）。
	const optionIndex = 1
	ciphertext, nonce, err := ballot.EncryptBallotWithUncompressedKey(uncompressedKey, optionIndex)
	require.NoError(t, err)

	// 5) 客户端在本地出证明：拼公开输入 → 用 proving key 出 Groth16 证明。
	secretBytes, err := ballot.GenerateNullifierSecret()
	require.NoError(t, err)
	var secretElement fr.Element
	secretElement.SetBytes(secretBytes)
	nullifier := ballot.DeriveNullifier(secretElement, ballot.DeriveProposalField(proposalResp.ID))
	nullifierHex := ballot.EncodeScalarHex(nullifier)

	nullifierBytes, err := hex.DecodeString(nullifierHex)
	require.NoError(t, err)
	compressedKey, err := ballot.CompressPoint(uncompressedKey)
	require.NoError(t, err)

	inputs, err := ballot.BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalResp.ID,
		Nullifier:         nullifierBytes,
		ElectionPublicKey: compressedKey,
		Ciphertext:        ciphertext,
		NumOptions:        2,
	})
	require.NoError(t, err)

	prover, err := ballot.NewProver(provingKeyPath)
	require.NoError(t, err)
	proof, err := prover.Prove(inputs, optionIndex, nonce, secretElement)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	// 6) 先投一份**被改过**的证明：服务端必须真的在验签（400，不是 5xx，
	// 也不能把票落库）。
	tamperedProof := append([]byte(nil), proof...)
	tamperedProof[0] ^= 0x01
	resp = postJSON(t, ts.URL+"/api/v1/votes", map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(tamperedProof),
		"nullifier":            nullifierHex,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
	stored, err := voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Empty(t, stored, "验签失败的票不能落库")

	// 7) 提交真正的选票：请求里只有密文、nullifier 与 proof，**没有明文选项**。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	resp = postJSON(t, ts.URL+"/api/v1/votes", map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(proof),
		"nullifier":            nullifierHex,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 8) 落库记录里只有密文：服务端无从还原选项。
	stored, err = voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, ciphertext, stored[0].Cipher)

	// 9) 计票：门限私钥还原出客户端选的那个选项 —— 密文与计票密钥是配套的。
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
