package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func (m *proofVerifierMock) Verify(proof *entity.Proof, inputs entity.PublicInputs) error {
	args := m.Called(proof, inputs)
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
	}, nil).Once()
	sessionRepo.On("GetByProposal", mock.Anything, proposalID).Return(&entity.DecryptionSession{
		ProposalID: proposalID,
		Threshold:  1,
		Trustees:   1,
		SharesJSON: `["c2hhcmUtMQ=="]`,
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-123",
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
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-123").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-dup",
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-dup").Return(true, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrDoubleVoting)
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-x",
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(errors.New("invalid")).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)

	err := uc.Submit(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid proof")
}

func TestVotingUseCase_Submit_WithPublicWitness(t *testing.T) {
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-123",
		PublicWitness:       []byte("witness-binary"),
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.MatchedBy(func(inputs entity.PublicInputs) bool {
		val, ok := inputs["public_witness"]
		return ok && string(val) == "witness-binary"
	})).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-123").Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(nil).Once()
	mockParticipationRepo.On("MarkVoted", mock.Anything, "prop-001", hashUserID("u-1"), mock.AnythingOfType("time.Time")).Return(nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.NoError(t, err)
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-1",
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
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-1").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-atomic",
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
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-atomic").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-atomic-part",
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
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-atomic-part").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-atomic-vote",
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
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-atomic-vote").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-dup-save",
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	expectVotingProposalAndSession(mockProposalRepo, mockSessionRepo, req.ProposalID)
	expectVotingCredential(mockCredentialRepo, "prop-001", "u-1", []byte("pub"), []byte("sig"))
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-dup-save").Return(false, nil).Once()
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-001",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-1",
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-closed",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-closed",
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
		zap.NewNop(),
	)

	req := &SubmitVoteRequest{
		ProposalID:          "prop-no-session",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "null-no-session",
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
