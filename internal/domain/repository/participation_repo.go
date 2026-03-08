package repository

import (
	"context"
	"time"
)

type ParticipationRepository interface {
	MarkVoted(ctx context.Context, proposalID string, userHash string, votedAt time.Time) error
	HasVoted(ctx context.Context, proposalID string, userHash string) (bool, error)
}
