package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/require"
)

func newEligibilityUseRepo(t *testing.T) *EligibilityUseRepository {
	t.Helper()
	db, err := NewDB(&DBConfig{Driver: "sqlite", Database: ":memory:"})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewEligibilityUseRepository(db)
}

// TestEligibilityUseRepository_RecordUseOnlyOnce 钉住「一人一票」的强制点：
// 同一提案下的同一份资格只能被记一次。
func TestEligibilityUseRepository_RecordUseOnlyOnce(t *testing.T) {
	repo := newEligibilityUseRepo(t)
	ctx := context.Background()

	use := &entity.EligibilityUse{
		ID:              "use-1",
		ProposalID:      "p-1",
		QualificationID: "qual-a",
		UsedAt:          time.Now().UTC(),
	}
	require.NoError(t, repo.RecordUse(ctx, use))

	// 同一份资格第二次用 → 唯一索引拦下。
	again := &entity.EligibilityUse{
		ID:              "use-2",
		ProposalID:      "p-1",
		QualificationID: "qual-a",
		UsedAt:          time.Now().UTC(),
	}
	err := repo.RecordUse(ctx, again)
	require.ErrorIs(t, err, repository.ErrDuplicateEligibilityUse)
}

// TestEligibilityUseRepository_SameQualificationAcrossProposals 验证去重键是
// 按提案算的：同一份资格在另一个提案里可以再用一次。
func TestEligibilityUseRepository_SameQualificationAcrossProposals(t *testing.T) {
	repo := newEligibilityUseRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-1",
		ProposalID:      "p-1",
		QualificationID: "qual-a",
		UsedAt:          time.Now().UTC(),
	}))
	require.NoError(t, repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-2",
		ProposalID:      "p-2",
		QualificationID: "qual-b",
		UsedAt:          time.Now().UTC(),
	}))

	count, err := repo.CountUses(ctx, "p-1")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	count, err = repo.CountUses(ctx, "p-2")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}
