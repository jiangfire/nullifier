package usecase

import (
	"context"
	"fmt"

	"anonvote/internal/domain/repository"

	"go.uber.org/zap"
)

type ParticipationUseCase struct {
	repo   repository.ParticipationRepository
	logger *zap.Logger
}

func NewParticipationUseCase(repo repository.ParticipationRepository, logger *zap.Logger) *ParticipationUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ParticipationUseCase{
		repo:   repo,
		logger: logger,
	}
}

func (uc *ParticipationUseCase) HasVoted(ctx context.Context, proposalID string, userID string) (bool, error) {
	if uc.repo == nil {
		return false, fmt.Errorf("participation repository is not configured")
	}
	if proposalID == "" {
		return false, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if userID == "" {
		return false, fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}

	hasVoted, err := uc.repo.HasVoted(ctx, proposalID, hashUserID(userID))
	if err != nil {
		return false, fmt.Errorf("check participation: %w", err)
	}
	return hasVoted, nil
}
