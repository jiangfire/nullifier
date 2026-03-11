package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	if proposal != nil && len(proposal.Options) > 0 && proposal.OptionsJSON == "" {
		proposal.Options = normalizeStoredProposalOptions(proposal.Options)
		raw, err := json.Marshal(proposal.Options)
		if err != nil {
			return err
		}
		proposal.OptionsJSON = string(raw)
	}
	return r.Create(ctx, proposal)
}

func (r *ProposalRepository) GetByID(ctx context.Context, id string) (*entity.Proposal, error) {
	proposal, err := r.BaseRepository.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return hydrateProposal(proposal)
}

func (r *ProposalRepository) ListOpen(ctx context.Context) ([]*entity.Proposal, error) {
	var proposals []*entity.Proposal
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("status = ? AND starts_at <= ? AND ends_at > ?", "open", now, now).
		Order("created_at asc").
		Find(&proposals).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []*entity.Proposal{}, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range proposals {
		if _, err := hydrateProposal(proposals[i]); err != nil {
			return nil, err
		}
	}
	return proposals, nil
}

func hydrateProposal(proposal *entity.Proposal) (*entity.Proposal, error) {
	if proposal == nil {
		return nil, nil
	}
	if proposal.OptionsJSON == "" {
		proposal.Options = []string{}
		return proposal, nil
	}
	var options []string
	if err := json.Unmarshal([]byte(proposal.OptionsJSON), &options); err != nil {
		return nil, err
	}
	proposal.Options = normalizeStoredProposalOptions(options)
	return proposal, nil
}

func normalizeStoredProposalOptions(options []string) []string {
	out := make([]string, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		normalized := strings.TrimSpace(option)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}
