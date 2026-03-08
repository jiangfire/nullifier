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
