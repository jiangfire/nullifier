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
	if err := r.Create(ctx, credential); err != nil {
		// 同一提案下的 user_id 唯一：并发 attest 时靠它兜底。
		if isUniqueConstraintError(err) {
			return repository.ErrDuplicateCredential
		}
		return err
	}
	return nil
}

func (r *CredentialRepository) GetByProposalAndUserID(ctx context.Context, proposalID string, userID string) (*entity.Credential, error) {
	var credential entity.Credential
	err := r.db.WithContext(ctx).
		Where("proposal_id = ? AND user_id = ?", proposalID, userID).
		First(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (r *CredentialRepository) ListByProposal(ctx context.Context, proposalID string) ([]*entity.Credential, error) {
	var credentials []*entity.Credential
	if err := r.db.WithContext(ctx).
		Where("proposal_id = ?", proposalID).
		Order("issued_at ASC").
		Find(&credentials).Error; err != nil {
		return nil, err
	}
	return credentials, nil
}
