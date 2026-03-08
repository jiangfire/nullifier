package usecase

import (
	"context"
	"errors"
	"testing"

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

func TestVotingUseCase_Submit(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

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
	mockCredentialRepo.On("GetByUserID", mock.Anything, "u-1").Return(&entity.Credential{
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
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

	req := &SubmitVoteRequest{
		ProposalID:    "prop-001",
		EncryptedVote: []byte("cipher"),
		Proof:         &entity.Proof{Data: []byte("proof")},
		Nullifier:     "null-dup",
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-dup").Return(true, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrDoubleVoting)
}

func TestVotingUseCase_Submit_InvalidProof(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

	req := &SubmitVoteRequest{
		ProposalID:    "prop-001",
		EncryptedVote: []byte("cipher"),
		Proof:         &entity.Proof{Data: []byte("proof")},
		Nullifier:     "null-x",
	}

	mockVerifier.On("Verify", req.Proof, mock.Anything).Return(errors.New("invalid")).Once()

	err := uc.Submit(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid proof")
}

func TestVotingUseCase_Submit_WithPublicWitness(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

	req := &SubmitVoteRequest{
		ProposalID:    "prop-001",
		EncryptedVote: []byte("cipher"),
		Proof:         &entity.Proof{Data: []byte("proof")},
		Nullifier:     "null-123",
		PublicWitness: []byte("witness-binary"),
	}

	mockVerifier.On("Verify", req.Proof, mock.MatchedBy(func(inputs entity.PublicInputs) bool {
		val, ok := inputs["public_witness"]
		return ok && string(val) == "witness-binary"
	})).Return(nil).Once()
	mockEncrypter.On("ValidateCiphertext", req.EncryptedVote).Return(nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-123").Return(false, nil).Once()
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.NoError(t, err)
}

func TestVotingUseCase_Submit_UserAlreadyVoted(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

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
	mockCredentialRepo.On("GetByUserID", mock.Anything, "u-1").Return(&entity.Credential{
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	mockRepo.On("ExistsByNullifier", mock.Anything, "null-1").Return(false, nil).Once()
	mockParticipationRepo.On("HasVoted", mock.Anything, "prop-001", hashUserID("u-1")).Return(true, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrUserAlreadyVoted)
}

func TestVotingUseCase_Submit_InvalidCredential(t *testing.T) {
	mockRepo := new(voteRepositoryMock)
	mockParticipationRepo := new(participationRepositoryMock)
	mockCredentialRepo := new(credentialRepositoryMock)
	mockVerifier := new(proofVerifierMock)
	mockEncrypter := new(encrypterMock)
	uc := NewVotingUseCase(mockRepo, mockParticipationRepo, mockCredentialRepo, mockVerifier, mockEncrypter, zap.NewNop())

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
	mockCredentialRepo.On("GetByUserID", mock.Anything, "u-1").Return(&entity.Credential{
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig-y"),
	}, nil).Once()

	err := uc.Submit(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredential)
}
