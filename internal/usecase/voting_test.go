package usecase

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type voteRepositoryMock struct {
	mock.Mock
}

func (m *voteRepositoryMock) Save(ctx context.Context, vote *entity.Vote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

func (m *voteRepositoryMock) ExistsByNullifier(ctx context.Context, nullifier string) (bool, error) {
	args := m.Called(ctx, nullifier)
	return args.Bool(0), args.Error(1)
}

func (m *voteRepositoryMock) GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error) {
	args := m.Called(ctx, proposalID)
	return args.Get(0).([]*entity.Vote), args.Error(1)
}

var _ repository.VoteRepository = (*voteRepositoryMock)(nil)

type voteRepositoryAtomicMock struct {
	mock.Mock
}

func (m *voteRepositoryAtomicMock) Save(ctx context.Context, vote *entity.Vote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

func (m *voteRepositoryAtomicMock) ExistsByNullifier(ctx context.Context, nullifier string) (bool, error) {
	args := m.Called(ctx, nullifier)
	return args.Bool(0), args.Error(1)
}

func (m *voteRepositoryAtomicMock) GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error) {
	args := m.Called(ctx, proposalID)
	return args.Get(0).([]*entity.Vote), args.Error(1)
}

func (m *voteRepositoryAtomicMock) SaveWithParticipation(
	ctx context.Context,
	vote *entity.Vote,
	userHash string,
	votedAt time.Time,
) error {
	args := m.Called(ctx, vote, userHash, votedAt)
	return args.Error(0)
}

var _ repository.VoteRepository = (*voteRepositoryAtomicMock)(nil)

type proofVerifierMock struct {
	mock.Mock
}

func (m *proofVerifierMock) Verify(proof *entity.Proof, statement entity.BallotStatement) error {
	args := m.Called(proof, statement)
	return args.Error(0)
}

var _ domaincrypto.ProofVerifier = (*proofVerifierMock)(nil)

type encrypterMock struct {
	mock.Mock
}

func (m *encrypterMock) ValidateCiphertext(cipher []byte) error {
	args := m.Called(cipher)
	return args.Error(0)
}

var _ domaincrypto.Encrypter = (*encrypterMock)(nil)

// acceptingCredentialIssuer 是总是验签通过的假签发方。
type acceptingCredentialIssuer struct{}

func (acceptingCredentialIssuer) BlindSign(
	ctx context.Context,
	keyVersion int,
	blindedMessage []byte,
) ([]byte, error) {
	return blindedMessage, nil
}

func (acceptingCredentialIssuer) IssuerPublicKey(ctx context.Context) (*domaincrypto.IssuerPublicKey, error) {
	return &domaincrypto.IssuerPublicKey{
		Version:  1,
		Modulus:  []byte("modulus"),
		Exponent: 65537,
		Variant:  "SHA384PSSDeterministic",
	}, nil
}

func (acceptingCredentialIssuer) VerifyCredential(
	userID string,
	userPubKey []byte,
	signature []byte,
) (int, error) {
	return 1, nil
}

// rejectingCredentialIssuer 是验签总是失败的假签发方，用来覆盖伪造签名。
type rejectingCredentialIssuer struct{}

func (rejectingCredentialIssuer) BlindSign(
	ctx context.Context,
	keyVersion int,
	blindedMessage []byte,
) ([]byte, error) {
	return blindedMessage, nil
}

func (rejectingCredentialIssuer) IssuerPublicKey(ctx context.Context) (*domaincrypto.IssuerPublicKey, error) {
	return &domaincrypto.IssuerPublicKey{
		Version:  1,
		Modulus:  []byte("modulus"),
		Exponent: 65537,
		Variant:  "SHA384PSSDeterministic",
	}, nil
}

func (rejectingCredentialIssuer) VerifyCredential(
	userID string,
	userPubKey []byte,
	signature []byte,
) (int, error) {
	return 0, errors.New("signature verification failed")
}

type decryptionSessionRepositoryMock struct {
	mock.Mock
}

func (m *decryptionSessionRepositoryMock) Save(ctx context.Context, session *entity.DecryptionSession) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *decryptionSessionRepositoryMock) GetByProposal(
	ctx context.Context,
	proposalID string,
) (*entity.DecryptionSession, error) {
	args := m.Called(ctx, proposalID)
	session, _ := args.Get(0).(*entity.DecryptionSession)
	return session, args.Error(1)
}

var _ repository.DecryptionSessionRepository = (*decryptionSessionRepositoryMock)(nil)

// votingProposalOptions 是投票用例共享的提案选项，决定公开输入里的选项数。
var votingProposalOptions = []string{"yes", "no", "abstain"}

// votingSessionPublicKey 是投票用例共享的计票会话公钥（32 字节压缩点）。
var votingSessionPublicKey = []byte("0123456789abcdef0123456789abcdef")

// testNullifier 构造一个合法的 nullifier：64 位十六进制字符串（32 字节域元素）。
func testNullifier(fill string) string {
	return strings.Repeat(fill, 2*entity.NullifierByteSize/len(fill))
}

func expectVotingProposalAndSession(
	proposalRepo *proposalRepositoryMock,
	sessionRepo *decryptionSessionRepositoryMock,
	proposalID string,
) {
	proposalRepo.On("GetByID", mock.Anything, proposalID).Return(&entity.Proposal{
		ID:       proposalID,
		Status:   "open",
		StartsAt: time.Now().UTC().Add(-1 * time.Hour),
		EndsAt:   time.Now().UTC().Add(1 * time.Hour),
		Options:  votingProposalOptions,
	}, nil).Once()
	sessionRepo.On("GetByProposal", mock.Anything, proposalID).Return(&entity.DecryptionSession{
		ProposalID: proposalID,
		Threshold:  1,
		Trustees:   1,
		SharesJSON: `["c2hhcmUtMQ=="]`,
		PublicKey:  votingSessionPublicKey,
	}, nil).Once()
}

// expectVotingCredential registers the credential lookup a valid vote requires.
func expectVotingCredential(
	credentialRepo *credentialRepositoryMock,
	proposalID string,
	userID string,
	userPubKey []byte,
	signature []byte,
) {
	credentialRepo.On("GetByProposalAndUserID", mock.Anything, proposalID, userID).Return(&entity.Credential{
		ProposalID: proposalID,
		UserID:     userID,
		UserPubKey: userPubKey,
		Signature:  signature,
	}, nil).Once()
}

func TestVotingUseCase_Submit(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("ab")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(nil).Once()
	mockParticipationRepo.On("MarkVoted", mock.Anything, "prop-001", hashUserID("u-1"), mock.AnythingOfType("time.Time")).Return(nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockVerifier.AssertExpectations(t)
	mockEncrypter.AssertExpectations(t)
}

func TestVotingUseCase_Submit_DoubleVoting(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("cd")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(true, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrDoubleVoting)
}

// TestVotingUseCase_Submit_NormalizesNullifier 断言 nullifier 以规范形式查重与落库。
//
// 客户端可以用大写或带空白的形式提交同一个 nullifier。如果原样落库，
// 同一个 nullifier 就能占掉多行，绕过 votes.nullifier 的唯一索引 ——
// 防重投就形同虚设（生产模式同样可绕：验签看的是解码后的字节）。
func TestVotingUseCase_Submit_NormalizesNullifier(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	canonical := testNullifier("cd")
	// 客户端提交的变体：大写 + 首尾空白 —— 解码后是同一个 nullifier。
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "  " + strings.ToUpper(canonical) + "  ",
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	// 查重必须用规范形式 —— 否则同一 nullifier 的另一种写法查不到已有记录。
	mockRepo.On("ExistsByNullifier", mock.Anything, canonical).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.MatchedBy(func(v *entity.Vote) bool {
		return v.Nullifier == canonical
	})).Return(nil).Once()
	mockParticipationRepo.On("MarkVoted", mock.Anything, "prop-001", hashUserID("u-1"), mock.AnythingOfType("time.Time")).Return(nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockVerifier.AssertExpectations(t)
	mockEncrypter.AssertExpectations(t)
}

func TestVotingUseCase_Submit_InvalidProof(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("ef")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(errors.New("invalid")).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)

	err := uc.Submit(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid proof")
	// 证明校验失败是客户端输入问题，必须映射到 400 而不是 500。
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

// TestVotingUseCase_Submit_ServerAssemblesStatement 验证公开输入由服务端自行拼装：
// 密文、nullifier、提案选项数与计票会话公钥都来自服务端能拿到的材料，
// 客户端不再自由提交公开输入。
func TestVotingUseCase_Submit_ServerAssemblesStatement(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("12")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	expectedNullifier, err := hex.DecodeString(nullifier)
	require.NoError(t, err)

	mockVerifier.On("Verify", req.Proof, mock.MatchedBy(func(statement entity.BallotStatement) bool {
		return statement.ProposalID == req.ProposalID &&
			bytes.Equal(statement.Nullifier, expectedNullifier) &&
			bytes.Equal(statement.ElectionPublicKey, votingSessionPublicKey) &&
			bytes.Equal(statement.Ciphertext, req.EncryptedVote) &&
			statement.NumOptions == len(votingProposalOptions)
	})).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(nil).Once()
	mockParticipationRepo.On("MarkVoted", mock.Anything, "prop-001", hashUserID("u-1"), mock.AnythingOfType("time.Time")).Return(nil).Once()

	err = uc.Submit(context.Background(), req)
	assert.NoError(t, err)
	mockVerifier.AssertExpectations(t)
}

// TestVotingUseCase_Submit_InvalidNullifier 验证 nullifier 必须是 64 位十六进制
// 字符串（32 字节域元素）：格式非法时直接 400，不进入密码学校验。
func TestVotingUseCase_Submit_InvalidNullifier(t *testing.T) {
	cases := []struct {
		name      string
		nullifier string
	}{
		{name: "not hex", nullifier: "null-123"},
		{name: "too short", nullifier: strings.Repeat("ab", 16)},
		{name: "too long", nullifier: strings.Repeat("ab", 33)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo := new(voteRepositoryMock)
			mockParticipationRepo := new(participationRepositoryMock)
			mockCredentialRepo := new(credentialRepositoryMock)
			mockProposalRepo := new(proposalRepositoryMock)
			mockSessionRepo := new(decryptionSessionRepositoryMock)
			mockVerifier := new(proofVerifierMock)
			mockEncrypter := new(encrypterMock)
			uc := NewVotingUseCase(
				mockRepo,
				mockParticipationRepo,
				mockCredentialRepo,
				mockProposalRepo,
				mockSessionRepo,
				mockVerifier,
				mockEncrypter,
				acceptingCredentialIssuer{},
				zap.NewNop(),
			)

			req := &SubmitVoteRequest{
				ProposalID:          "prop-001",
				EncryptedVote:       []byte("cipher"),
				Proof:               &entity.Proof{Data: []byte("proof")},
				Nullifier:           tc.nullifier,
				UserID:              "u-1",
				UserPubKey:          []byte("pub"),
				CredentialSignature: []byte("sig"),
			}

			expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)

			err := uc.Submit(context.Background(), req)
			assert.ErrorIs(t, err, ErrInvalidRequest)
			// 非法 nullifier 必须在进入密码学校验之前被拦下。
			mockVerifier.AssertNotCalled(t, "Verify", mock.Anything, mock.Anything)
			mockEncrypter.AssertNotCalled(t, "ValidateCiphertext", mock.Anything)
		})
	}
}

func TestVotingUseCase_Submit_UserAlreadyVoted(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("34")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(true, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrUserAlreadyVoted)
}

func TestVotingUseCase_Submit_UsesAtomicSaveWhenSupported(t *testing.T) {
	mockRepo := new(voteRepositoryAtomicMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("56")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	userHash := hashUserID("u-1")
	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", userHash).Return(false, nil).Once()
	mockRepo.On("SaveWithParticipation",
		mock.Anything,
		mock.AnythingOfType("*entity.Vote"),
		userHash,
		mock.AnythingOfType("time.Time"),
	).Return(nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.NoError(t, err)
	mockParticipationRepo.AssertNotCalled(t, "MarkVoted", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
}

func TestVotingUseCase_Submit_AtomicSaveDuplicateParticipation(t *testing.T) {
	mockRepo := new(voteRepositoryAtomicMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("78")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	userHash := hashUserID("u-1")
	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", userHash).Return(false, nil).Once()
	mockRepo.On("SaveWithParticipation",
		mock.Anything,
		mock.AnythingOfType("*entity.Vote"),
		userHash,
		mock.AnythingOfType("time.Time"),
	).Return(repository.ErrDuplicateParticipation).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrUserAlreadyVoted)
}

func TestVotingUseCase_Submit_AtomicSaveDuplicateVote(t *testing.T) {
	mockRepo := new(voteRepositoryAtomicMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("9a")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	userHash := hashUserID("u-1")
	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", userHash).Return(false, nil).Once()
	mockRepo.On("SaveWithParticipation",
		mock.Anything,
		mock.AnythingOfType("*entity.Vote"),
		userHash,
		mock.AnythingOfType("time.Time"),
	).Return(repository.ErrDuplicateVote).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrDoubleVoting)
}

func TestVotingUseCase_Submit_SaveDuplicateVote(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("bc")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, nullifier).Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(repository.ErrDuplicateVote).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrDoubleVoting)
}

func TestVotingUseCase_Submit_InvalidCredential(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	nullifier := testNullifier("de")
	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           nullifier,
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig-x"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	mockCredentialRepo.On("GetByProposalAndUserID", mock.Anything, "prop-001", "u-1").Return(&entity.Credential{
		ProposalID: "prop-001",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig-y"),
	}, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredential)
}

func TestVotingUseCase_Submit_RejectClosedProposal(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-closed",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           testNullifier("f0"),
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockProposalRepo.On("GetByID", mock.Anything, "prop-closed").Return(&entity.Proposal{
		ID:       "prop-closed",
		Status:   "closed",
		StartsAt: time.Now().UTC().Add(-2 * time.Hour),
		EndsAt:   time.Now().UTC().Add(-1 * time.Hour),
	}, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestVotingUseCase_Submit_RejectWhenSessionMissing(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockProposalRepo := new(proposalRepositoryMock)
	mockSessionRepo := new(decryptionSessionRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(
		mockRepo,
		mockParticipationRepo,
		mockCredentialRepo,
		mockProposalRepo,
		mockSessionRepo,
		mockVerifier,
		mockEncrypter,
		acceptingCredentialIssuer{},
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-no-session",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           testNullifier("1f"),
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockProposalRepo.On("GetByID", mock.Anything, "prop-no-session").Return(&entity.Proposal{
		ID:       "prop-no-session",
		Status:   "open",
		StartsAt: time.Now().UTC().Add(-1 * time.Hour),
		EndsAt:   time.Now().UTC().Add(1 * time.Hour),
	}, nil).Once()
	mockSessionRepo.On("GetByProposal", mock.Anything, "prop-no-session").Return((*entity.DecryptionSession)(nil), repository.ErrNotFound).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}
