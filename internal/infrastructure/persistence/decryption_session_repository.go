package persistence

import (
	"context"
	"errors"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type DecryptionSessionRepository struct {
	*BaseRepository[entity.DecryptionSession]
	db *gorm.DB
}

var _ repository.DecryptionSessionRepository = (*DecryptionSessionRepository)(nil)

func NewDecryptionSessionRepository(db *gorm.DB) *DecryptionSessionRepository {
	return &DecryptionSessionRepository{
		BaseRepository: NewBaseRepository[entity.DecryptionSession](db),
		db:             db,
	}
}

func (r *DecryptionSessionRepository) Save(ctx context.Context, session *entity.DecryptionSession) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *DecryptionSessionRepository) GetByProposal(
	ctx context.Context,
	proposalID string,
) (*entity.DecryptionSession, error) {
	var session entity.DecryptionSession
	err := r.db.WithContext(ctx).
		Where("proposal_id = ?", proposalID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}
