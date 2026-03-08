package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
)

type TallyResultRepository struct {
	*BaseRepository[entity.TallyResult]
	db *gorm.DB
}

var _ repository.TallyResultRepository = (*TallyResultRepository)(nil)

func NewTallyResultRepository(db *gorm.DB) *TallyResultRepository {
	return &TallyResultRepository{
		BaseRepository: NewBaseRepository[entity.TallyResult](db),
		db:             db,
	}
}

func (r *TallyResultRepository) Save(ctx context.Context, result *entity.TallyResult) error {
	if result.TallyJSON == "" && result.Tally != nil {
		raw, err := marshalTallyJSON(result.Tally)
		if err != nil {
			return err
		}
		result.TallyJSON = raw
	}
	return r.db.WithContext(ctx).Save(result).Error
}

func (r *TallyResultRepository) GetByProposal(ctx context.Context, proposalID string) (*entity.TallyResult, error) {
	var result entity.TallyResult
	err := r.db.WithContext(ctx).
		Where("proposal_id = ?", proposalID).
		First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if result.TallyJSON != "" {
		tally, err := unmarshalTallyJSON(result.TallyJSON)
		if err != nil {
			return nil, err
		}
		result.Tally = tally
	}
	return &result, nil
}

type tallyEntry struct {
	Candidate string `json:"candidate"`
	Count     int    `json:"count"`
}

func marshalTallyJSON(tally map[string]int) (string, error) {
	keys := make([]string, 0, len(tally))
	for key := range tally {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]tallyEntry, 0, len(keys))
	for _, key := range keys {
		items = append(items, tallyEntry{
			Candidate: key,
			Count:     tally[key],
		})
	}

	raw, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalTallyJSON(raw string) (map[string]int, error) {
	var items []tallyEntry
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(items))
	for _, item := range items {
		out[item.Candidate] = item.Count
	}
	return out, nil
}
