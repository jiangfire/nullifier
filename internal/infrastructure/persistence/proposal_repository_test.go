package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

func TestProposalRepository_ListOpen_OnlyReturnsActiveOpenProposals(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewProposalRepository(db)
	now := time.Now().UTC()
	proposals := []*entity.Proposal{
		{
			ID:        "p-active-1",
			Title:     "active-1",
			Status:    "open",
			StartsAt:  now.Add(-2 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-3 * time.Hour),
		},
		{
			ID:        "p-active-2",
			Title:     "active-2",
			Status:    "open",
			StartsAt:  now.Add(-1 * time.Hour),
			EndsAt:    now.Add(1 * time.Hour),
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "p-expired",
			Title:     "expired",
			Status:    "open",
			StartsAt:  now.Add(-3 * time.Hour),
			EndsAt:    now.Add(-1 * time.Minute),
			CreatedAt: now.Add(-4 * time.Hour),
		},
		{
			ID:        "p-future",
			Title:     "future",
			Status:    "open",
			StartsAt:  now.Add(1 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "p-closed",
			Title:     "closed",
			Status:    "closed",
			StartsAt:  now.Add(-2 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-30 * time.Minute),
		},
	}
	for _, proposal := range proposals {
		require.NoError(t, repo.Save(context.Background(), proposal))
	}

	got, err := repo.ListOpen(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "p-active-1", got[0].ID)
	require.Equal(t, "p-active-2", got[1].ID)
}
