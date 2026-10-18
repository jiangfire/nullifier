package persistence

import (
	"context"
	"errors"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ParticipationRepository struct {
	db *gorm.DB
}

var _ repository.ParticipationRepository = (*ParticipationRepository)(nil)

func NewParticipationRepository(db *gorm.DB) *ParticipationRepository {
	return &ParticipationRepository{db: db}
}

func (r *ParticipationRepository) MarkVoted(
	ctx context.Context,
	proposalID string,
	userHash string,
	votedAt time.Time,
) error {
	now := time.Now().UTC()
	record := &entity.VoteParticipation{
		ID:         uuid.NewString(),
		ProposalID: proposalID,
		UserHash:   userHash,
		VotedAt:    votedAt.UTC(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "proposal_id"},
				{Name: "user_hash"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"voted_at":   record.VotedAt,
				"updated_at": now,
			}),
		}).
		Create(record).Error
}

func (r *ParticipationRepository) HasVoted(ctx context.Context, proposalID string, userHash string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&entity.VoteParticipation{}).
		Where("proposal_id = ? AND user_hash = ?", proposalID, userHash).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ParticipationRepository) ListVotedUserHashes(
	ctx context.Context,
	proposalID string,
) (map[string]struct{}, error) {
	voted := make(map[string]struct{})
	var rows []entity.VoteParticipation
	err := r.db.WithContext(ctx).
		Model(&entity.VoteParticipation{}).
		Where("proposal_id = ?", proposalID).
		Find(&rows).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return voted, nil
	}
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		voted[row.UserHash] = struct{}{}
	}
	return voted, nil
}
