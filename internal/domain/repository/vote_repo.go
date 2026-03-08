package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type VoteRepository interface {
	Save(ctx context.Context, vote *entity.Vote) error
	ExistsByNullifier(ctx context.Context, nullifier string) (bool, error)
	GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error)
}
