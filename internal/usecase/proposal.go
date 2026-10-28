package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"anonvote/internal/domain/entity"
	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type CreateProposalRequest struct {
	Title       string
	Description string
	Options     []string
	StartsAt    time.Time
	EndsAt      time.Time
	// EligibilityScope 是要绑定的资格名单域（可选）。
	//
	// 绑定不存在的名单域在**创建时**就被拒绝（400）—— 不留到第一个投票人来
	// 领取时才失败。服务端没开启资格校验时禁止绑定：一个永远解析不出名单的
	// 绑定只会把人引进死胡同。
	EligibilityScope string
}

func (r *CreateProposalRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: request is nil", ErrInvalidRequest)
	}
	if r.Title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidRequest)
	}
	if len(r.Options) < 2 {
		return fmt.Errorf("%w: at least 2 options are required", ErrInvalidRequest)
	}
	seen := make(map[string]struct{}, len(r.Options))
	for _, option := range r.Options {
		normalized := strings.TrimSpace(option)
		if normalized == "" {
			return fmt.Errorf("%w: option cannot be empty", ErrInvalidRequest)
		}
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("%w: duplicate option %q", ErrInvalidRequest, normalized)
		}
		seen[normalized] = struct{}{}
	}
	if r.StartsAt.IsZero() {
		return fmt.Errorf("%w: starts_at is required", ErrInvalidRequest)
	}
	if r.EndsAt.IsZero() {
		return fmt.Errorf("%w: ends_at is required", ErrInvalidRequest)
	}
	if !r.EndsAt.After(r.StartsAt) {
		return fmt.Errorf("%w: ends_at must be after starts_at", ErrInvalidRequest)
	}
	return nil
}

type ProposalUseCase struct {
	repo        repository.ProposalRepository
	eligibility domaincrypto.EligibilityService
	logger      *zap.Logger
}

func NewProposalUseCase(
	repo repository.ProposalRepository,
	eligibility domaincrypto.EligibilityService,
	logger *zap.Logger,
) *ProposalUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProposalUseCase{
		repo:        repo,
		eligibility: eligibility,
		logger:      logger,
	}
}

func (uc *ProposalUseCase) Create(ctx context.Context, req *CreateProposalRequest) (*entity.Proposal, error) {
	if uc.repo == nil {
		return nil, fmt.Errorf("proposal repository is not configured")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	options := normalizeProposalOptions(req.Options)
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("marshal proposal options: %w", err)
	}

	// 名单域绑定：存在性在创建时就核过，领取链路（挑战 / 证明校验）据此解析。
	scope := strings.TrimSpace(req.EligibilityScope)
	if scope != "" {
		if uc.eligibility == nil || !uc.eligibility.Enabled() {
			return nil, fmt.Errorf(
				"%w: the server does not verify eligibility proofs; eligibility_scope cannot be set",
				ErrInvalidRequest,
			)
		}
		if !uc.eligibility.ScopeKnown(scope) {
			return nil, fmt.Errorf("%w: unknown eligibility scope %q", ErrInvalidRequest, scope)
		}
	}

	proposal := &entity.Proposal{
		ID:               uuid.NewString(),
		Title:            req.Title,
		Description:      req.Description,
		OptionsJSON:      string(optionsJSON),
		Options:          options,
		Status:           "open",
		StartsAt:         req.StartsAt.UTC(),
		EndsAt:           req.EndsAt.UTC(),
		CreatedAt:        time.Now().UTC(),
		EligibilityScope: scope,
	}
	if err := uc.repo.Save(ctx, proposal); err != nil {
		return nil, fmt.Errorf("save proposal: %w", err)
	}

	uc.logger.Info("proposal created",
		zap.String("proposal_id", proposal.ID),
		zap.String("title", proposal.Title),
	)
	return proposal, nil
}

func (uc *ProposalUseCase) ListOpen(ctx context.Context) ([]*entity.Proposal, error) {
	if uc.repo == nil {
		return nil, fmt.Errorf("proposal repository is not configured")
	}
	proposals, err := uc.repo.ListOpen(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open proposals: %w", err)
	}
	return proposals, nil
}

func (uc *ProposalUseCase) List(ctx context.Context) ([]*entity.Proposal, error) {
	if uc.repo == nil {
		return nil, fmt.Errorf("proposal repository is not configured")
	}
	proposals, err := uc.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	return proposals, nil
}

func normalizeProposalOptions(options []string) []string {
	out := make([]string, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		normalized := strings.TrimSpace(option)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}
