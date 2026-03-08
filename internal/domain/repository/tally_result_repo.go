package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type TallyResultRepository interface {
	Save(ctx context.Context, result *entity.TallyResult) error
	GetByProposal(ctx context.Context, proposalID string) (*entity.TallyResult, error)
}
