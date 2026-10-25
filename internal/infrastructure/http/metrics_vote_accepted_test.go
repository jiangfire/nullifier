package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anonvote/internal/config"
	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/usecase"
	"anonvote/internal/usecase/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// acceptingIssuer 是一个「什么都验得过」的签发方替身。
//
// 投票路径只用到它的 VerifyCredential：真签发方要拿 RSA 私钥验签，
// 而这条用例要验的是**指标计数**，不是签名算法。
type acceptingIssuer struct{}

func (acceptingIssuer) BlindSign(context.Context, int, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (acceptingIssuer) IssuerPublicKey(context.Context) (*domaincrypto.IssuerPublicKey, error) {
	return nil, errors.New("not used")
}

func (acceptingIssuer) VerifyCredential(string, []byte, []byte) (int, error) {
	return 1, nil
}

// metricsUserHash 复刻 usecase 里的用户哈希口径：SHA-256 的小写 hex。
func metricsUserHash(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:])
}

// TestMetricsEndpoint_CountsAcceptedVote 验证**成功入库**的票会进指标。
//
// 被拒的票有单独的用例（TestMetricsEndpoint_CountsVoteRejections），
// 缺了这条就没人保证「票真的收下了」时计数器会动 —— 运营看板上的
// 「已投票数」全靠它。
func TestMetricsEndpoint_CountsAcceptedVote(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	voteRepo := mocks.NewMockVoteRepository(ctrl)
	participationRepo := mocks.NewMockParticipationRepository(ctrl)
	credentialRepo := mocks.NewMockCredentialRepository(ctrl)
	proposalRepo := mocks.NewMockProposalRepository(ctrl)
	sessionRepo := mocks.NewMockDecryptionSessionRepository(ctrl)
	verifier := mocks.NewMockProofVerifier(ctrl)
	encrypter := mocks.NewMockEncrypter(ctrl)

	const proposalID = "proposal-1"
	const userID = "u-1"
	userPubKey := []byte("pub")
	credentialSignature := []byte("sig")
	cipher := []byte("cipher")
	nullifier := strings.Repeat("ab", entity.NullifierByteSize)
	userHash := metricsUserHash(userID)

	proposalRepo.EXPECT().GetByID(gomock.Any(), proposalID).Return(&entity.Proposal{
		ID:       proposalID,
		Status:   "open",
		StartsAt: time.Now().UTC().Add(-1 * time.Hour),
		EndsAt:   time.Now().UTC().Add(1 * time.Hour),
		Options:  []string{"yes", "no"},
	}, nil)
	sessionRepo.EXPECT().GetByProposal(gomock.Any(), proposalID).Return(&entity.DecryptionSession{
		ProposalID: proposalID,
		Threshold:  1,
		Trustees:   1,
		SharesJSON: `["c2hhcmUtMQ=="]`,
		PublicKey:  []byte("0123456789abcdef0123456789abcdef"),
	}, nil)
	verifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(nil)
	encrypter.EXPECT().ValidateCiphertext(cipher).Return(nil)
	credentialRepo.EXPECT().GetByProposalAndUserID(gomock.Any(), proposalID, userID).Return(&entity.Credential{
		ProposalID: proposalID,
		UserID:     userID,
		UserPubKey: userPubKey,
		Signature:  credentialSignature,
	}, nil)
	voteRepo.EXPECT().ExistsByNullifier(gomock.Any(), nullifier).Return(false, nil)
	participationRepo.EXPECT().HasVoted(gomock.Any(), proposalID, userHash).Return(false, nil)
	voteRepo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	participationRepo.EXPECT().MarkVoted(gomock.Any(), proposalID, userHash, gomock.Any()).Return(nil)

	handler := NewHandlerWithTally(
		usecase.NewVotingUseCase(
			voteRepo,
			participationRepo,
			credentialRepo,
			proposalRepo,
			sessionRepo,
			verifier,
			encrypter,
			acceptingIssuer{},
			zap.NewNop(),
		),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		zap.NewNop(),
	)
	engine := NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, jsonRequest(t, http.MethodPost, "/api/v1/proposals/"+proposalID+"/votes", map[string]any{
		"encrypted_vote":       base64.StdEncoding.EncodeToString(cipher),
		"proof":                base64.StdEncoding.EncodeToString([]byte("proof")),
		"nullifier":            nullifier,
		"user_id":              userID,
		"user_pub_key":         base64.StdEncoding.EncodeToString(userPubKey),
		"credential_signature": base64.StdEncoding.EncodeToString(credentialSignature),
	}))
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Contains(t, rec.Body.String(), "anonvote_votes_accepted_total 1")
}
