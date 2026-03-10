package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type participationRepositoryMock struct {
	mock.Mock
}

func (m *participationRepositoryMock) MarkVoted(
	ctx context.Context,
	proposalID string,
	userHash string,
	votedAt time.Time,
) error {
	args := m.Called(ctx, proposalID, userHash, votedAt)
	return args.Error(0)
}

func (m *participationRepositoryMock) HasVoted(
	ctx context.Context,
	proposalID string,
	userHash string,
) (bool, error) {
	args := m.Called(ctx, proposalID, userHash)
	return args.Bool(0), args.Error(1)
}

var _ repository.ParticipationRepository = (*participationRepositoryMock)(nil)

func TestParticipationUseCase_HasVoted(t *testing.T) {
	repo := new(participationRepositoryMock)
	uc := NewParticipationUseCase(repo, nil, zap.NewNop())

	repo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()

	hasVoted, err := uc.HasVoted(context.Background(), "p-1", "u-1")
	assert.NoError(t, err)
	assert.True(t, hasVoted)
}

func TestParticipationUseCase_HasVoted_InvalidRequest(t *testing.T) {
	repo := new(participationRepositoryMock)
	uc := NewParticipationUseCase(repo, nil, zap.NewNop())

	hasVoted, err := uc.HasVoted(context.Background(), "", "u-1")
	assert.False(t, hasVoted)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestParticipationUseCase_ListCredentialStatuses(t *testing.T) {
	participationRepo := new(participationRepositoryMock)
	credentialRepo := new(credentialRepositoryMock)
	uc := NewParticipationUseCase(participationRepo, credentialRepo, zap.NewNop())

	credentialRepo.On("ListByProposal", mock.Anything, "p-1").Return([]*entity.Credential{
		{
			ID:               "c-1",
			ProposalID:       "p-1",
			UserID:           "u-1",
			UserPubKey:       []byte("pub-1"),
			IssuerKeyVersion: 1,
			IssuedAt:         time.Now().UTC(),
		},
		{
			ID:               "c-2",
			ProposalID:       "p-1",
			UserID:           "u-2",
			UserPubKey:       []byte("pub-2"),
			IssuerKeyVersion: 1,
			IssuedAt:         time.Now().UTC(),
		},
	}, nil).Once()
	participationRepo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()
	participationRepo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-2")).Return(false, nil).Once()

	items, err := uc.ListCredentialStatuses(context.Background(), "p-1")
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.True(t, items[0].HasVoted)
	assert.False(t, items[1].HasVoted)
}

func TestParticipationUseCase_ListCredentialStatuses_ListError(t *testing.T) {
	participationRepo := new(participationRepositoryMock)
	credentialRepo := new(credentialRepositoryMock)
	uc := NewParticipationUseCase(participationRepo, credentialRepo, zap.NewNop())

	credentialRepo.On("ListByProposal", mock.Anything, "p-1").Return(([]*entity.Credential)(nil), errors.New("db down")).Once()

	items, err := uc.ListCredentialStatuses(context.Background(), "p-1")
	assert.Nil(t, items)
	assert.Error(t, err)
}
