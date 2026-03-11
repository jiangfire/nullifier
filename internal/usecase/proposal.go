package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"anonvote/internal/domain/entity"
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
	repo   repository.ProposalRepository
	logger *zap.Logger
}

func NewProposalUseCase(repo repository.ProposalRepository, logger *zap.Logger) *ProposalUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProposalUseCase{
		repo:   repo,
		logger: logger,
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

	proposal := &entity.Proposal{
		ID:          uuid.NewString(),
		Title:       req.Title,
		Description: req.Description,
		OptionsJSON: string(optionsJSON),
		Options:     options,
		Status:      "open",
		StartsAt:    req.StartsAt.UTC(),
		EndsAt:      req.EndsAt.UTC(),
		CreatedAt:   time.Now().UTC(),
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
