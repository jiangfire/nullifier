package integration

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"
	"anonvote/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
)

// TestEligibilityUseRepository_Postgres 在 PostgreSQL 上验证「一人一票」的强制点。
//
// 这条在 SQLite 上验过（`internal/infrastructure/persistence`），但**生产库是
// PostgreSQL** —— 而去重靠的是 AutoMigrate 建出来的**复合唯一索引**
// `(proposal_id, qualification_id)`。SQLite 与 PG 的 DDL 行为不同（索引名、类型、
// 并发插入时的冲突报错都不一样），而这张表一旦没建出那个复合索引，生产上就会
// 变成「同一个人能领两次票」，而且**没有任何报错**。
//
// 因此这里用行为证明索引存在：同键第二次写必须失败；换提案必须仍然能写
// （说明约束是复合的，不是单列）。
func TestEligibilityUseRepository_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewEligibilityUseRepository(db)
	now := time.Now().UTC()

	require.NoError(t, repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-pg-1",
		ProposalID:      "p-pg-1",
		QualificationID: "qual-pg-a",
		UsedAt:          now,
	}))

	// 同一提案下的同一份资格第二次用 → 唯一索引必须拦下。
	// 这正是服务端回 409 `ELIGIBILITY_ALREADY_USED` 的落地点。
	err := repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-pg-2",
		ProposalID:      "p-pg-1",
		QualificationID: "qual-pg-a",
		UsedAt:          now,
	})
	require.ErrorIs(t, err, repository.ErrDuplicateEligibilityUse)

	// 同一个去重键换一个提案 → 仍然能写：约束必须是**复合**的。
	require.NoError(t, repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-pg-3",
		ProposalID:      "p-pg-2",
		QualificationID: "qual-pg-a",
		UsedAt:          now,
	}))

	// 同一个提案下的**另一份**资格 → 也必须能写：约束不能是单列
	// `proposal_id`（那会把整个提案的资格全堵死）。
	require.NoError(t, repo.RecordUse(ctx, &entity.EligibilityUse{
		ID:              "use-pg-4",
		ProposalID:      "p-pg-1",
		QualificationID: "qual-pg-b",
		UsedAt:          now,
	}))

	count, err := repo.CountUses(ctx, "p-pg-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	count, err = repo.CountUses(ctx, "p-pg-2")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

// TestEligibilityUseRepository_Postgres_ConcurrentSameQualification 并发场景：
// 两个人同时领同一份资格（同一提案），必须**恰好一个**成功，另一个撞唯一约束。
//
// 这是「一人一票」在真实并发下的样子 —— 单线程验过不算数。
func TestEligibilityUseRepository_Postgres_ConcurrentSameQualification(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewEligibilityUseRepository(db)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		use := &entity.EligibilityUse{
			ID:              fmt.Sprintf("use-pg-concurrent-%d", i+1),
			ProposalID:      "p-pg-concurrent",
			QualificationID: "qual-pg-concurrent",
			UsedAt:          time.Now().UTC(),
		}
		wg.Add(1)
		go func(u *entity.EligibilityUse) {
			defer wg.Done()
			<-start
			errs <- repo.RecordUse(ctx, u)
		}(use)
	}

	close(start)
	wg.Wait()
	close(errs)

	success := 0
	duplicate := 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, repository.ErrDuplicateEligibilityUse):
			duplicate++
		default:
			require.NoError(t, err)
		}
	}

	require.Equal(t, 1, success)
	require.Equal(t, 1, duplicate)

	count, err := repo.CountUses(ctx, "p-pg-concurrent")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}
