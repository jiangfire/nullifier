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
	"github.com/stretchr/testify/require"
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

func (m *proposalRepositoryMock) ListAll(ctx context.Context) ([]*entity.Proposal, error) {
	args := m.Called(ctx)
	proposals, _ := args.Get(0).([]*entity.Proposal)
	return proposals, args.Error(1)
}

var _ repository.ProposalRepository = (*proposalRepositoryMock)(nil)

func TestProposalUseCase_Create(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	req := &CreateProposalRequest{
		Title:       "Board Election",
		Description: "Choose one board member",
		Options:     []string{"A", "B"},
		StartsAt:    time.Now().UTC().Add(1 * time.Hour),
		EndsAt:      time.Now().UTC().Add(2 * time.Hour),
	}

	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Proposal")).Return(nil).Once()

	proposal, err := uc.Create(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)
	assert.NotEmpty(t, proposal.ID)
	assert.Equal(t, "open", proposal.Status)
	assert.Equal(t, []string{"A", "B"}, proposal.Options)
}

func TestProposalUseCase_Create_InvalidTimeRange(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	req := &CreateProposalRequest{
		Title:       "Board Election",
		Description: "Choose one board member",
		Options:     []string{"A", "B"},
		StartsAt:    time.Now().UTC().Add(2 * time.Hour),
		EndsAt:      time.Now().UTC().Add(1 * time.Hour),
	}

	proposal, err := uc.Create(context.Background(), req)
	assert.Nil(t, proposal)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestProposalUseCase_ListOpen(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	repo.On("ListOpen", mock.Anything).Return([]*entity.Proposal{
		{ID: "p1", Title: "A", Status: "open"},
		{ID: "p2", Title: "B", Status: "open"},
	}, nil).Once()

	proposals, err := uc.ListOpen(context.Background())
	assert.NoError(t, err)
	assert.Len(t, proposals, 2)
}

func TestProposalUseCase_List(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	repo.On("ListAll", mock.Anything).Return([]*entity.Proposal{
		{ID: "p2", Title: "B", Status: "closed"},
		{ID: "p1", Title: "A", Status: "open"},
	}, nil).Once()

	proposals, err := uc.List(context.Background())
	assert.NoError(t, err)
	assert.Len(t, proposals, 2)
	assert.Equal(t, "p2", proposals[0].ID)
}

func TestProposalUseCase_List_Error(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	repo.On("ListAll", mock.Anything).Return(([]*entity.Proposal)(nil), errors.New("db down")).Once()

	proposals, err := uc.List(context.Background())
	assert.Nil(t, proposals)
	assert.Error(t, err)
}

// TestProposalUseCase_Create_EligibilityScope 覆盖名单域绑定：存在的名单域照常
// 落库，不存在的当场 400（不留到第一个投票人来领取时才失败）。
func TestProposalUseCase_Create_EligibilityScope(t *testing.T) {
	newUsecase := func(enabled bool) (*ProposalUseCase, *proposalRepositoryMock, *eligibilityServiceMock) {
		repo := new(proposalRepositoryMock)
		eligibility := new(eligibilityServiceMock)
		eligibility.On("Enabled").Return(enabled).Maybe()
		return NewProposalUseCase(repo, eligibility, zap.NewNop()), repo, eligibility
	}
	baseReq := func(scope string) *CreateProposalRequest {
		return &CreateProposalRequest{
			Title:            "Board Election",
			Options:          []string{"A", "B"},
			StartsAt:         time.Now().UTC().Add(1 * time.Hour),
			EndsAt:           time.Now().UTC().Add(2 * time.Hour),
			EligibilityScope: scope,
		}
	}

	// 绑定存在的名单域 → 落库。
	uc, repo, eligibility := newUsecase(true)
	eligibility.On("ScopeKnown", "acme-2026").Return(true).Once()
	repo.On("Save", mock.Anything, mock.MatchedBy(
		func(proposal *entity.Proposal) bool { return proposal.EligibilityScope == "acme-2026" },
	)).Return(nil).Once()
	proposal, err := uc.Create(context.Background(), baseReq("acme-2026"))
	require.NoError(t, err)
	assert.Equal(t, "acme-2026", proposal.EligibilityScope)

	// 绑定不存在的名单域 → 拒绝，且不落库。
	uc, repo, eligibility = newUsecase(true)
	eligibility.On("ScopeKnown", "nope").Return(false).Once()
	_, err = uc.Create(context.Background(), baseReq("nope"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown eligibility scope")
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)

	// 资格校验没开启时禁止绑定：一个永远解析不出名单的绑定只会把人引进死胡同。
	uc, repo, _ = newUsecase(false)
	_, err = uc.Create(context.Background(), baseReq("acme-2026"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not verify eligibility proofs")
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)

	// 空 scope 不做校验（未开启资格校验的服务也能创建普通提案）。
	uc, repo, _ = newUsecase(false)
	repo.On("Save", mock.Anything, mock.MatchedBy(
		func(proposal *entity.Proposal) bool { return proposal.EligibilityScope == "" },
	)).Return(nil).Once()
	_, err = uc.Create(context.Background(), baseReq(""))
	require.NoError(t, err)
}
