package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParticipationRepository_MarkAndHasVoted(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewParticipationRepository(db)
	err = repo.MarkVoted(context.Background(), "proposal-1", "user-hash-1", time.Now().UTC())
	require.NoError(t, err)

	hasVoted, err := repo.HasVoted(context.Background(), "proposal-1", "user-hash-1")
	require.NoError(t, err)
	assert.True(t, hasVoted)

	hasVoted, err = repo.HasVoted(context.Background(), "proposal-1", "user-hash-2")
	require.NoError(t, err)
	assert.False(t, hasVoted)
}

func TestParticipationRepository_ListVotedUserHashes(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewParticipationRepository(db)
	now := time.Now().UTC()
	require.NoError(t, repo.MarkVoted(context.Background(), "proposal-1", "hash-1", now))
	require.NoError(t, repo.MarkVoted(context.Background(), "proposal-1", "hash-2", now))
	require.NoError(t, repo.MarkVoted(context.Background(), "proposal-2", "hash-3", now))

	voted, err := repo.ListVotedUserHashes(context.Background(), "proposal-1")
	require.NoError(t, err)
	assert.Len(t, voted, 2)
	assert.Contains(t, voted, "hash-1")
	assert.Contains(t, voted, "hash-2")
	assert.NotContains(t, voted, "hash-3", "must stay scoped to the requested proposal")

	empty, err := repo.ListVotedUserHashes(context.Background(), "proposal-missing")
	require.NoError(t, err)
	assert.Empty(t, empty)
}
