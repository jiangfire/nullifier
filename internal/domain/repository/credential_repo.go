package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type CredentialRepository interface {
	Save(ctx context.Context, credential *entity.Credential) error
	GetByProposalAndUserID(ctx context.Context, proposalID string, userID string) (*entity.Credential, error)
	ListByProposal(ctx context.Context, proposalID string) ([]*entity.Credential, error)
}
