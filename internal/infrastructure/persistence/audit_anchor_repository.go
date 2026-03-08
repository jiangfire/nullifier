package persistence

import (
	"context"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type AuditAnchorRepository struct {
	*BaseRepository[entity.AuditAnchor]
	db *gorm.DB
}

var _ repository.AuditAnchorRepository = (*AuditAnchorRepository)(nil)

func NewAuditAnchorRepository(db *gorm.DB) *AuditAnchorRepository {
	return &AuditAnchorRepository{
		BaseRepository: NewBaseRepository[entity.AuditAnchor](db),
		db:             db,
	}
}

func (r *AuditAnchorRepository) Save(ctx context.Context, anchor *entity.AuditAnchor) error {
	return r.Create(ctx, anchor)
}

func (r *AuditAnchorRepository) ListByProposal(
	ctx context.Context,
	proposalID string,
) ([]*entity.AuditAnchor, error) {
	var anchors []*entity.AuditAnchor
	err := r.db.WithContext(ctx).
		Where("proposal_id = ?", proposalID).
		Order("created_at asc").
		Find(&anchors).Error
	if err != nil {
		return nil, err
	}
	return anchors, nil
}
