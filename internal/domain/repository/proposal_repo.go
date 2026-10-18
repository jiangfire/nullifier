package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type ProposalRepository interface {
	Save(ctx context.Context, proposal *entity.Proposal) error
	GetByID(ctx context.Context, id string) (*entity.Proposal, error)
	ListOpen(ctx context.Context) ([]*entity.Proposal, error)
	// ListAll returns every proposal regardless of status or time window, newest first.
	ListAll(ctx context.Context) ([]*entity.Proposal, error)
}
