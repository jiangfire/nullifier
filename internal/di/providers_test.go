package di

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviders_WiringSmoke(t *testing.T) {
	cfg := &config.Config{
		ServerAddress:               ":0",
		GinMode:                     gin.TestMode,
		VerifyingKeyPath:            "",
		CredentialIssuerKeyPath:     filepath.Join(t.TempDir(), "issuer_keys.json"),
		CredentialIssuerRotateAfter: 24 * time.Hour,
		DB: &persistence.DBConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
	}

	logger, err := NewLogger()
	require.NoError(t, err)

	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NotNil(t, db)

	voteRepo := NewVoteRepository(db)
	credentialRepo := NewCredentialRepository(db)
	proposalRepo := NewProposalRepository(db)
	participationRepo := NewParticipationRepository(db)
	sessionRepo := NewDecryptionSessionRepository(db)
	resultRepo := NewTallyResultRepository(db)
	anchorRepo := NewAuditAnchorRepository(db)
	require.NotNil(t, voteRepo)
	require.NotNil(t, credentialRepo)
	require.NotNil(t, proposalRepo)
	require.NotNil(t, participationRepo)
	require.NotNil(t, sessionRepo)
	require.NotNil(t, resultRepo)
	require.NotNil(t, anchorRepo)

	proofVerifier, err := NewProofVerifier(cfg)
	require.NoError(t, err)
	require.NotNil(t, proofVerifier)

	encrypter := NewEncrypter()
	thresholdCrypto := NewThresholdCrypto()
	merkleBuilder := NewMerkleRootBuilder()
	credentialIssuer := NewCredentialIssuer(cfg)
	require.NotNil(t, encrypter)
	require.NotNil(t, thresholdCrypto)
	require.NotNil(t, merkleBuilder)
	require.NotNil(t, credentialIssuer)

	votingUC := NewVotingUseCase(voteRepo, participationRepo, credentialRepo, proofVerifier, encrypter, logger)
	registerUC := NewRegisterUseCase(credentialRepo, credentialIssuer, logger)
	proposalUC := NewProposalUseCase(proposalRepo, logger)
	participationUC := NewParticipationUseCase(participationRepo, logger)
	tallyUC := NewTallyUseCase(voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo, thresholdCrypto, merkleBuilder, logger)
	require.NotNil(t, votingUC)
	require.NotNil(t, registerUC)
	require.NotNil(t, proposalUC)
	require.NotNil(t, participationUC)
	require.NotNil(t, tallyUC)

	handler := NewHTTPHandler(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
	server := NewHTTPServer(cfg, handler, logger)
	require.NotNil(t, server)
	require.NotNil(t, server.Engine())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.Engine().ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
}

func TestNewProofVerifier_InvalidPath(t *testing.T) {
	cfg := &config.Config{
		VerifyingKeyPath: filepath.Join(t.TempDir(), "not-exist.vk"),
	}
	_, err := NewProofVerifier(cfg)
	require.Error(t, err)
}

func TestNewDB_InvalidConfig(t *testing.T) {
	cfg := &config.Config{
		DB: &persistence.DBConfig{
			Driver: "unsupported",
		},
	}
	_, err := NewDB(cfg)
	require.Error(t, err)
}

func TestInitializeServer(t *testing.T) {
	cfg := &config.Config{
		ServerAddress:               ":0",
		GinMode:                     gin.TestMode,
		VerifyingKeyPath:            "",
		CredentialIssuerKeyPath:     filepath.Join(t.TempDir(), "issuer_keys.json"),
		CredentialIssuerRotateAfter: 24 * time.Hour,
		DB: &persistence.DBConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
	}
	server, err := InitializeServer(cfg)
	require.NoError(t, err)
	require.NotNil(t, server)

	// 确认迁移后新实体表可插入（间接验证 DI 新增仓储链路可用）
	db, err := NewDB(cfg)
	require.NoError(t, err)
	err = db.Create(&entity.AuditAnchor{
		ID:         "a1",
		ProposalID: "p1",
		MerkleRoot: []byte("root"),
		AnchorHash: "hash",
		CreatedAt:  time.Now().UTC(),
	}).Error
	require.NoError(t, err)
}
