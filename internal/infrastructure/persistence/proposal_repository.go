package persistence

import (
	"context"
	"errors"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type ProposalRepository struct {
	*BaseRepository[entity.Proposal]
	db *gorm.DB
}

var _ repository.ProposalRepository = (*ProposalRepository)(nil)

func NewProposalRepository(db *gorm.DB) *ProposalRepository {
	return &ProposalRepository{
		BaseRepository: NewBaseRepository[entity.Proposal](db),
		db:             db,
	}
}

func (r *ProposalRepository) Save(ctx context.Context, proposal *entity.Proposal) error {
	return r.Create(ctx, proposal)
}

func (r *ProposalRepository) GetByID(ctx context.Context, id string) (*entity.Proposal, error) {
	return r.BaseRepository.GetByID(ctx, id)
}

func (r *ProposalRepository) ListOpen(ctx context.Context) ([]*entity.Proposal, error) {
	var proposals []*entity.Proposal
	err := r.db.WithContext(ctx).
		Where("status = ?", "open").
		Order("created_at asc").
		Find(&proposals).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []*entity.Proposal{}, nil
	}
	if err != nil {
		return nil, err
	}
	return proposals, nil
}
