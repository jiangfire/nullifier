package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecryptionSessionRepository_SaveAndGetByProposal(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewDecryptionSessionRepository(db)
	now := time.Now().UTC()
	session := &entity.DecryptionSession{
		ID:         "sess-1",
		ProposalID: "proposal-1",
		Threshold:  2,
		Trustees:   3,
		PublicKey:  []byte("pub"),
		SharesJSON: `["s1","s2","s3"]`,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, repo.Save(context.Background(), session))

	got, err := repo.GetByProposal(context.Background(), "proposal-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.ID)
	assert.Equal(t, 2, got.Threshold)
}

func TestTallyResultRepository_SaveAndGetByProposal(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewTallyResultRepository(db)
	now := time.Now().UTC()
	result := &entity.TallyResult{
		ID:          "result-1",
		ProposalID:  "proposal-1",
		TotalVotes:  3,
		MerkleRoot:  []byte("root"),
		AnchorHash:  "anchor",
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tally: map[string]int{
			"candidate-A": 2,
			"candidate-B": 1,
		},
	}
	require.NoError(t, repo.Save(context.Background(), result))

	got, err := repo.GetByProposal(context.Background(), "proposal-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3, got.TotalVotes)
	assert.Equal(t, 2, got.Tally["candidate-A"])
	assert.Equal(t, 1, got.Tally["candidate-B"])
}

func TestAuditAnchorRepository_SaveAndListByProposal(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewAuditAnchorRepository(db)
	now := time.Now().UTC()
	require.NoError(t, repo.Save(context.Background(), &entity.AuditAnchor{
		ID:         "a1",
		ProposalID: "proposal-1",
		MerkleRoot: []byte("root-1"),
		AnchorHash: "hash-1",
		CreatedAt:  now,
	}))
	require.NoError(t, repo.Save(context.Background(), &entity.AuditAnchor{
		ID:         "a2",
		ProposalID: "proposal-1",
		MerkleRoot: []byte("root-2"),
		AnchorHash: "hash-2",
		CreatedAt:  now.Add(time.Second),
	}))

	anchors, err := repo.ListByProposal(context.Background(), "proposal-1")
	require.NoError(t, err)
	require.Len(t, anchors, 2)
	assert.Equal(t, "a1", anchors[0].ID)
	assert.Equal(t, "a2", anchors[1].ID)
}

func TestDecryptionSessionRepository_GetByProposal_NotFound(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewDecryptionSessionRepository(db)
	_, err = repo.GetByProposal(context.Background(), "missing")
	require.ErrorIs(t, err, repository.ErrNotFound)
}
