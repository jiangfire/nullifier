package persistence

import (
	"context"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type VoteRepository struct {
	*BaseRepository[entity.Vote]
	db *gorm.DB
}

var _ repository.VoteRepository = (*VoteRepository)(nil)

func NewVoteRepository(db *gorm.DB) *VoteRepository {
	return &VoteRepository{
		BaseRepository: NewBaseRepository[entity.Vote](db),
		db:             db,
	}
}

func (r *VoteRepository) Save(ctx context.Context, vote *entity.Vote) error {
	return r.Create(ctx, vote)
}

func (r *VoteRepository) ExistsByNullifier(ctx context.Context, nullifier string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&entity.Vote{}).
		Where("nullifier = ?", nullifier).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *VoteRepository) GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error) {
	var votes []*entity.Vote
	err := r.db.WithContext(ctx).
		Where("proposal_id = ?", proposalID).
		Order("created_at asc").
		Find(&votes).Error
	if err != nil {
		return nil, err
	}
	return votes, nil
}
