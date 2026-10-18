package repository

import (
	"context"
	"time"
)

type ParticipationRepository interface {
	MarkVoted(ctx context.Context, proposalID string, userHash string, votedAt time.Time) error
	HasVoted(ctx context.Context, proposalID string, userHash string) (bool, error)
	// ListVotedUserHashes returns every user hash that has voted for the proposal,
	// so callers can resolve many credentials in a single query instead of one per credential.
	ListVotedUserHashes(ctx context.Context, proposalID string) (map[string]struct{}, error)
}
