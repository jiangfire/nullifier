package usecase

import (
	"context"
	"fmt"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type CreateProposalRequest struct {
	Title       string
	Description string
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

	proposal := &entity.Proposal{
		ID:          uuid.NewString(),
		Title:       req.Title,
		Description: req.Description,
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
