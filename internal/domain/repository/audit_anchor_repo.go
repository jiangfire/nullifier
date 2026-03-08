package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type AuditAnchorRepository interface {
	Save(ctx context.Context, anchor *entity.AuditAnchor) error
	ListByProposal(ctx context.Context, proposalID string) ([]*entity.AuditAnchor, error)
}
