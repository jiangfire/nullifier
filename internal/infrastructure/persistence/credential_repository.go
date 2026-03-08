package persistence

import (
	"context"
	"errors"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type CredentialRepository struct {
	*BaseRepository[entity.Credential]
	db *gorm.DB
}

var _ repository.CredentialRepository = (*CredentialRepository)(nil)

func NewCredentialRepository(db *gorm.DB) *CredentialRepository {
	return &CredentialRepository{
		BaseRepository: NewBaseRepository[entity.Credential](db),
		db:             db,
	}
}

func (r *CredentialRepository) Save(ctx context.Context, credential *entity.Credential) error {
	return r.Create(ctx, credential)
}

func (r *CredentialRepository) GetByUserID(ctx context.Context, userID string) (*entity.Credential, error) {
	var credential entity.Credential
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &credential, nil
}
