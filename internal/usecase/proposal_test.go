package usecase

import (
	"context"
	"errors"
	"strings"
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

// 空 scope + 资格已开启 + 默认名单解析不出 → 创建时就拒绝（F9）：
// 不能等第一个投票人打挑战时才 500。
func TestProposalUseCase_Create_EmptyScopeRequiresDefaultRoster(t *testing.T) {
	newUsecase := func(rosterErr error) (*ProposalUseCase, *proposalRepositoryMock) {
		repo := new(proposalRepositoryMock)
		eligibility := new(eligibilityServiceMock)
		eligibility.On("Enabled").Return(true).Maybe()
		eligibility.On("RosterFor", "").Return("", 0, rosterErr)
		return NewProposalUseCase(repo, eligibility, zap.NewNop()), repo
	}
	req := func() *CreateProposalRequest {
		return &CreateProposalRequest{
			Title:    "Board Election",
			Options:  []string{"A", "B"},
			StartsAt: time.Now().UTC().Add(1 * time.Hour),
			EndsAt:   time.Now().UTC().Add(2 * time.Hour),
		}
	}

	uc, repo := newUsecase(errors.New("no default roster"))
	_, err := uc.Create(context.Background(), req())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default eligibility roster")
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)

	// 默认名单解析得出 → 空 scope 正常落库。
	uc, repo = newUsecase(nil)
	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Proposal")).Return(nil).Once()
	proposal, err := uc.Create(context.Background(), req())
	require.NoError(t, err)
	assert.Equal(t, "", proposal.EligibilityScope)
}

// 创建时窗口已在投票中 → **允许**：解密会话在关窗后仍可建立（守卫只拦
// 「开放中」），危害只是这个窗口作废而非提案永久不可用；多名单 e2e 的
// 领取链路测试也依赖创建开窗提案。这条用例把该行为钉住，防止将来被
// 「防呆」改动悄悄破坏。
func TestProposalUseCase_Create_AllowsAlreadyOpenWindow(t *testing.T) {
	repo := new(proposalRepositoryMock)
	uc := NewProposalUseCase(repo, nil, zap.NewNop())

	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Proposal")).Return(nil).Once()
	proposal, err := uc.Create(context.Background(), &CreateProposalRequest{
		Title:    "Board Election",
		Options:  []string{"A", "B"},
		StartsAt: time.Now().UTC().Add(-1 * time.Hour),
		EndsAt:   time.Now().UTC().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	assert.NotNil(t, proposal)
}

// eligibility_scope 超过 64 字符 → 创建时拒绝（N5）：列是 size:64，
// PostgreSQL 上超长值会炸 500，SQLite 不纠察 —— 统一在用例层拦。
func TestProposalUseCase_Create_RejectsTooLongScope(t *testing.T) {
	repo := new(proposalRepositoryMock)
	eligibility := new(eligibilityServiceMock)
	eligibility.On("Enabled").Return(true).Maybe()
	// ScopeKnown 不设期望：长度检查必须发生在名单存在性检查**之前**。
	uc := NewProposalUseCase(repo, eligibility, zap.NewNop())

	_, err := uc.Create(context.Background(), &CreateProposalRequest{
		Title:            "Board Election",
		Options:          []string{"A", "B"},
		StartsAt:         time.Now().UTC().Add(1 * time.Hour),
		EndsAt:           time.Now().UTC().Add(2 * time.Hour),
		EligibilityScope: strings.Repeat("a", 65),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
	assert.Contains(t, err.Error(), "at most 64 characters")
	eligibility.AssertNotCalled(t, "ScopeKnown", mock.Anything)
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
}
