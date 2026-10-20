package persistence

import (
	"context"
	"errors"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type CredentialClaimRepository struct {
	db *gorm.DB
}

var _ repository.CredentialClaimRepository = (*CredentialClaimRepository)(nil)

func NewCredentialClaimRepository(db *gorm.DB) *CredentialClaimRepository {
	return &CredentialClaimRepository{db: db}
}

func (r *CredentialClaimRepository) Save(ctx context.Context, claims []*entity.CredentialClaim) error {
	if len(claims) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&claims).Error
}

func (r *CredentialClaimRepository) TakeAvailable(
	ctx context.Context,
	proposalID string,
) (*entity.CredentialClaim, error) {
	var claim entity.CredentialClaim
	err := r.db.WithContext(ctx).
		Where("proposal_id = ? AND claimed_at IS NULL", proposalID).
		Order("created_at ASC, id ASC").
		First(&claim).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// MarkClaimed 用条件更新保证同一个槽位只会被领取一次：
// 只有 claimed_at 仍为空的行会被更新，affected rows 为 0 说明被别人抢先领走了。
func (r *CredentialClaimRepository) MarkClaimed(
	ctx context.Context,
	claimID string,
	userPubKey []byte,
	signature []byte,
	claimedAt time.Time,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&entity.CredentialClaim{}).
		Where("id = ? AND claimed_at IS NULL", claimID).
		Updates(map[string]any{
			"user_pub_key": userPubKey,
			"signature":    signature,
			"claimed_at":   claimedAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *CredentialClaimRepository) CountAvailable(ctx context.Context, proposalID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&entity.CredentialClaim{}).
		Where("proposal_id = ? AND claimed_at IS NULL", proposalID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
