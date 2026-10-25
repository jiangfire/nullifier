package persistence

import (
	"context"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type EligibilityUseRepository struct {
	db *gorm.DB
}

var _ repository.EligibilityUseRepository = (*EligibilityUseRepository)(nil)

func NewEligibilityUseRepository(db *gorm.DB) *EligibilityUseRepository {
	return &EligibilityUseRepository{db: db}
}

// RecordUse 记录一次资格使用。
//
// 唯一索引 `(proposal_id, qualification_id)` 冲突时返回
// repository.ErrDuplicateEligibilityUse —— 那正是「一人一票」被触发的时刻。
func (r *EligibilityUseRepository) RecordUse(ctx context.Context, use *entity.EligibilityUse) error {
	if err := r.db.WithContext(ctx).Create(use).Error; err != nil {
		if isUniqueConstraintError(err) {
			return repository.ErrDuplicateEligibilityUse
		}
		return err
	}
	return nil
}

func (r *EligibilityUseRepository) CountUses(ctx context.Context, proposalID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&entity.EligibilityUse{}).
		Where("proposal_id = ?", proposalID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
