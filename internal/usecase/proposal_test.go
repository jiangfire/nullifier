package usecase

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type proposalRepositoryMock struct {
	mock.Mock
}

func (m *proposalRepositoryMock) Save(ctx context.Context, proposal *entity.Proposal) error {
	args := m.Called(ctx, proposal)
	return args.Error(0)
}

func (m *proposalRepositoryMock) GetByID(ctx context.Context, id string) (*entity.Proposal, error) {
	args := m.Called(ctx, id)
	proposal, _ := args.Get(0).(*entity.Proposal)
	return proposal, args.Error(1)
}

func (m *proposalRepositoryMock) ListOpen(ctx context.Context) ([]*entity.Proposal, error) {
	args := m.Called(ctx)
	proposals, _ := args.Get(0).([]*entity.Proposal)
	return proposals, args.Error(1)
}

var _ repository.ProposalRepository = (*proposalRepositoryMock)(nil)

func TestProposalUseCase_Create(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, zap.NewNop())

	req := &CreateProposalRequest{
		Title:       "Board Election",
		Description: "Choose one board member",
		StartsAt:    time.Now().UTC().Add(1 * time.Hour),
		EndsAt:      time.Now().UTC().Add(2 * time.Hour),
	}

	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Proposal")).Return(nil).Once()

	proposal, err := uc.Create(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)
	assert.NotEmpty(t, proposal.ID)
	assert.Equal(t, "open", proposal.Status)
}

func TestProposalUseCase_Create_InvalidTimeRange(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, zap.NewNop())

	req := &CreateProposalRequest{
		Title:       "Board Election",
		Description: "Choose one board member",
		StartsAt:    time.Now().UTC().Add(2 * time.Hour),
		EndsAt:      time.Now().UTC().Add(1 * time.Hour),
	}

	proposal, err := uc.Create(context.Background(), req)
	assert.Nil(t, proposal)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestProposalUseCase_ListOpen(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, zap.NewNop())

	repo.On("ListOpen", mock.Anything).Return([]*entity.Proposal{
		{ID: "p1", Title: "A", Status: "open"},
		{ID: "p2", Title: "B", Status: "open"},
	}, nil).Once()

	proposals, err := uc.ListOpen(context.Background())
	assert.NoError(t, err)
	assert.Len(t, proposals, 2)
}
