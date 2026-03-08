package usecase

import (
	"context"
	"testing"
	"time"

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
	uc := NewParticipationUseCase(repo, zap.NewNop())

	repo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()

	hasVoted, err := uc.HasVoted(context.Background(), "p-1", "u-1")
	assert.NoError(t, err)
	assert.True(t, hasVoted)
}

func TestParticipationUseCase_HasVoted_InvalidRequest(t *testing.T) {
	repo := new(participationRepositoryMock)
	uc := NewParticipationUseCase(repo, zap.NewNop())

	hasVoted, err := uc.HasVoted(context.Background(), "", "u-1")
	assert.False(t, hasVoted)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}
