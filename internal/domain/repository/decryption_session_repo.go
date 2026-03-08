package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type DecryptionSessionRepository interface {
	Save(ctx context.Context, session *entity.DecryptionSession) error
	GetByProposal(ctx context.Context, proposalID string) (*entity.DecryptionSession, error)
}
