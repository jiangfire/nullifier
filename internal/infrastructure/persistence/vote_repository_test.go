package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoteRepository_SaveAndQuery(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	vote := &entity.Vote{
		ID:         "v-1",
		ProposalID: "p-1",
		Nullifier:  "n-1",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.Save(context.Background(), vote))

	exists, err := repo.ExistsByNullifier(context.Background(), "n-1")
	require.NoError(t, err)
	assert.True(t, exists)

	votes, err := repo.GetByProposal(context.Background(), "p-1")
	require.NoError(t, err)
	require.Len(t, votes, 1)
	assert.Equal(t, "v-1", votes[0].ID)
}
