package persistence

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

func newClaimRepo(t *testing.T) *CredentialClaimRepository {
	t.Helper()
	return newClaimRepoWithDSN(t, ":memory:")
}

func newClaimRepoWithDSN(t *testing.T, dsn string) *CredentialClaimRepository {
	t.Helper()
	db, err := NewDB(&DBConfig{Driver: "sqlite", Database: dsn})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewCredentialClaimRepository(db)
}

func seedClaim(t *testing.T, repo *CredentialClaimRepository, proposalID string, count int) {
	t.Helper()
	claims := make([]*entity.CredentialClaim, 0, count)
	for i := 0; i < count; i++ {
		claims = append(claims, &entity.CredentialClaim{
			ID:         "slot-" + proposalID + "-" + string(rune('a'+i)),
			ProposalID: proposalID,
			UserID:     "pseudonym-" + string(rune('a'+i)),
			UserPubKey: []byte{},
			Signature:  []byte{},
			CreatedAt:  time.Now().UTC(),
		})
	}
	require.NoError(t, repo.Save(context.Background(), claims))
}

func TestCredentialClaimRepository_CountAvailable(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 2)

	count, err := repo.CountAvailable(ctx, "p-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestCredentialClaimRepository_MarkClaimedOnlyOnce(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 1)

	claim, err := repo.TakeAvailable(ctx, "p-1")
	require.NoError(t, err)

	ok, err := repo.MarkClaimed(ctx, claim.ID, []byte("pub"), []byte("sig"), time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)

	// 同一个槽位不能再被领取。
	ok, err = repo.MarkClaimed(ctx, claim.ID, []byte("other"), []byte("sig"), time.Now().UTC())
	require.NoError(t, err)
	require.False(t, ok)

	count, err := repo.CountAvailable(ctx, "p-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestCredentialClaimRepository_TakeAvailableSkipsClaimed(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 2)

	first, err := repo.TakeAvailable(ctx, "p-1")
	require.NoError(t, err)
	ok, err := repo.MarkClaimed(ctx, first.ID, []byte("pub"), []byte("sig"), time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)

	second, err := repo.TakeAvailable(ctx, "p-1")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

// 并发领取时，同一个槽位不能被两个人同时拿走。
//
// 用文件库而不是 :memory: —— SQLite 的内存库是「每个连接一个」，
// 并发写会落到不同的库上。
func TestCredentialClaimRepository_ConcurrentClaimsGetDistinctSlots(t *testing.T) {
	repo := newClaimRepoWithDSN(t, filepath.Join(t.TempDir(), "claims.db"))
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 8)

	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := map[string]bool{}
	failures := 0

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 与用例层一致：槽位被抢走就换一个重试。
			for attempt := 0; attempt < 50; attempt++ {
				claim, err := repo.TakeAvailable(ctx, "p-1")
				if err != nil {
					continue
				}
				ok, err := repo.MarkClaimed(
					ctx, claim.ID, []byte("pub"), []byte("sig"), time.Now().UTC(),
				)
				if err != nil || !ok {
					continue
				}
				mu.Lock()
				claimed[claim.ID] = true
				mu.Unlock()
				return
			}
			mu.Lock()
			failures++
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.Zero(t, failures, "每次领取都应当最终拿到一个槽位")
	require.Len(t, claimed, 8, "8 个槽位应当被 8 次领取各拿一个")
}

func TestCredentialClaimRepository_TakeAvailableNotFound(t *testing.T) {
	repo := newClaimRepo(t)

	_, err := repo.TakeAvailable(context.Background(), "p-missing")
	require.Error(t, err)
}
