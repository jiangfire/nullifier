package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestVoteRepository_SaveWithParticipation(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	vote := &entity.Vote{
		ID:         "v-atomic-1",
		ProposalID: "p-atomic-1",
		Nullifier:  "n-atomic-1",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.SaveWithParticipation(context.Background(), vote, "user-hash-atomic", vote.CreatedAt))

	exists, err := repo.ExistsByNullifier(context.Background(), "n-atomic-1")
	require.NoError(t, err)
	require.True(t, exists)

	participationRepo := NewParticipationRepository(db)
	hasVoted, err := participationRepo.HasVoted(context.Background(), "p-atomic-1", "user-hash-atomic")
	require.NoError(t, err)
	require.True(t, hasVoted)
}

func TestVoteRepository_SaveWithParticipation_DuplicateParticipation(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	first := &entity.Vote{
		ID:         "v-dupe-part-1",
		ProposalID: "p-dupe-part",
		Nullifier:  "n-dupe-part-1",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	second := &entity.Vote{
		ID:         "v-dupe-part-2",
		ProposalID: "p-dupe-part",
		Nullifier:  "n-dupe-part-2",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}

	err = repo.SaveWithParticipation(context.Background(), first, "user-hash-dupe", first.CreatedAt)
	require.NoError(t, err)
	err = repo.SaveWithParticipation(context.Background(), second, "user-hash-dupe", second.CreatedAt)
	require.ErrorIs(t, err, repository.ErrDuplicateParticipation)

	exists, err := repo.ExistsByNullifier(context.Background(), "n-dupe-part-2")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestVoteRepository_Save_DuplicateNullifier(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	first := &entity.Vote{
		ID:         "v-dupe-null-1",
		ProposalID: "p-dupe-null",
		Nullifier:  "n-dupe-null",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	second := &entity.Vote{
		ID:         "v-dupe-null-2",
		ProposalID: "p-dupe-null",
		Nullifier:  "n-dupe-null",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.Save(context.Background(), first))
	require.ErrorIs(t, repo.Save(context.Background(), second), repository.ErrDuplicateVote)
}

func TestVoteRepository_SaveWithParticipation_DuplicateNullifierRollsBackParticipation(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	participationRepo := NewParticipationRepository(db)
	now := time.Now().UTC()

	first := &entity.Vote{
		ID:         "v-dupe-null-part-1",
		ProposalID: "p-dupe-null-part",
		Nullifier:  "n-dupe-null-part",
		Cipher:     []byte("cipher"),
		CreatedAt:  now,
	}
	second := &entity.Vote{
		ID:         "v-dupe-null-part-2",
		ProposalID: "p-dupe-null-part",
		Nullifier:  "n-dupe-null-part",
		Cipher:     []byte("cipher"),
		CreatedAt:  now,
	}

	require.NoError(t, repo.SaveWithParticipation(context.Background(), first, "user-hash-a", now))
	err = repo.SaveWithParticipation(context.Background(), second, "user-hash-b", now)
	require.ErrorIs(t, err, repository.ErrDuplicateVote)

	hasVoted, err := participationRepo.HasVoted(context.Background(), "p-dupe-null-part", "user-hash-b")
	require.NoError(t, err)
	require.False(t, hasVoted)
}

func TestVoteRepository_SaveWithParticipation_RollsBackOnParticipationError(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	injectedErr := errors.New("injected participation insert failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(
		"fail_vote_participation_insert",
		func(tx *gorm.DB) {
			if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "vote_participations" {
				tx.AddError(injectedErr)
			}
		},
	))
	t.Cleanup(func() {
		db.Callback().Create().Remove("fail_vote_participation_insert")
	})

	repo := NewVoteRepository(db)
	vote := &entity.Vote{
		ID:         "v-atomic-rollback",
		ProposalID: "p-atomic-rollback",
		Nullifier:  "n-atomic-rollback",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}

	err = repo.SaveWithParticipation(context.Background(), vote, "user-hash-rollback", vote.CreatedAt)
	require.Error(t, err)
	require.ErrorContains(t, err, injectedErr.Error())

	exists, err := repo.ExistsByNullifier(context.Background(), "n-atomic-rollback")
	require.NoError(t, err)
	require.False(t, exists)
}

// created_at 并列时必须按 id 稳定排序（F4）：merkle 叶子顺序决定计票锚点，
// 读票顺序必须完全确定 —— 否则服务端发布与审计复算在并列票上可能算出
// 不同的根，合法选举被误报为审计不一致。
func TestVoteRepository_GetByProposal_TieBrokenByID(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewVoteRepository(db)
	sameTime := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	// 插入顺序故意与 id 顺序不同：并列的 created_at 不许按插入顺序漂移。
	for _, id := range []string{"v-3", "v-1", "v-2"} {
		require.NoError(t, repo.Save(context.Background(), &entity.Vote{
			ID:         id,
			ProposalID: "p-tie",
			Nullifier:  "n-" + id,
			Cipher:     []byte("cipher"),
			CreatedAt:  sameTime,
		}))
	}

	first, err := repo.GetByProposal(context.Background(), "p-tie")
	require.NoError(t, err)
	second, err := repo.GetByProposal(context.Background(), "p-tie")
	require.NoError(t, err)

	require.Len(t, first, 3)
	require.Len(t, second, 3)
	got := []string{first[0].ID, first[1].ID, first[2].ID}
	assert.Equal(t, []string{"v-1", "v-2", "v-3"}, got)
	assert.Equal(t, got, []string{second[0].ID, second[1].ID, second[2].ID})
}
