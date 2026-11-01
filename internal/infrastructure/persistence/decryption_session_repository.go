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
	if err := r.db.WithContext(ctx).Save(session).Error; err != nil {
		if isUniqueConstraintError(err) {
			// 并发双建会话撞唯一索引 → 领域错误，用例层映射 409（N4），
			// 不再落到裸方言错误的 500。
			return repository.ErrDuplicateDecryptionSession
		}
		return err
	}
	return nil
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
