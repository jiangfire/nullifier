package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err := r.Create(ctx, vote); err != nil {
		if isUniqueConstraintError(err) {
			return repository.ErrDuplicateVote
		}
		return err
	}
	return nil
}

func (r *VoteRepository) SaveWithParticipation(
	ctx context.Context,
	vote *entity.Vote,
	userHash string,
	votedAt time.Time,
) error {
	if vote == nil {
		return errors.New("vote is nil")
	}
	if vote.ProposalID == "" {
		return errors.New("proposal id is required")
	}
	if userHash == "" {
		return errors.New("user hash is required")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		record := &entity.VoteParticipation{
			ID:         uuid.NewString(),
			ProposalID: vote.ProposalID,
			UserHash:   userHash,
			VotedAt:    votedAt.UTC(),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "proposal_id"},
				{Name: "user_hash"},
			},
			DoNothing: true,
		}).Create(record)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return repository.ErrDuplicateParticipation
		}

		if err := tx.Create(vote).Error; err != nil {
			if isUniqueConstraintError(err) {
				return repository.ErrDuplicateVote
			}
			return err
		}
		return nil
	})
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

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique violation")
}
