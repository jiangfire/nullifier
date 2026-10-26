package persistence

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/google/uuid"
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

	claim, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
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

// TestCredentialClaimRepository_ReserveAvailableSkipsReserved 验证预留过的槽位
// 不会被下一次预留拿走 —— 并发领取的人必须各拿各的。
func TestCredentialClaimRepository_ReserveAvailableSkipsReserved(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 2)

	first, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first.ReservedAt)

	second, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

// TestCredentialClaimRepository_ReserveAvailableReclaimsExpired 验证过期的预留
// 可以被重新拿走 —— 只预留不领取的客户端不能永久占住池子。
func TestCredentialClaimRepository_ReserveAvailableReclaimsExpired(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 1)

	// TTL 为负 → 任何预留都算过期。
	first, err := repo.ReserveAvailable(ctx, "p-1", -time.Second)
	require.NoError(t, err)

	second, err := repo.ReserveAvailable(ctx, "p-1", -time.Second)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "过期预留应当可以被重新拿走")
}

// TestCredentialClaimRepository_ReleaseReservationFreesSlot 验证释放预留后
// 槽位回到空闲状态（资格被拒时不能白白消耗池子）。
func TestCredentialClaimRepository_ReleaseReservationFreesSlot(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 1)

	claim, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.ReleaseReservation(ctx, claim.ID))

	// 释放之后，另一个领取者能拿到同一个槽位。
	again, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)
	require.Equal(t, claim.ID, again.ID)
}

// TestCredentialClaimRepository_GetReservedByUserID 验证「正在预留中」的判定：
// 新鲜的预留查得到，过期的查不到，没预留的也查不到。
func TestCredentialClaimRepository_GetReservedByUserID(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 2)

	claim, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)

	found, err := repo.GetReservedByUserID(ctx, "p-1", claim.UserID, time.Minute)
	require.NoError(t, err)
	require.Equal(t, claim.ID, found.ID)

	// 过期的预留不算「正在领」。
	_, err = repo.GetReservedByUserID(ctx, "p-1", claim.UserID, -time.Second)
	require.Error(t, err)

	// 没有预留过的槽位也不算。
	other, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.ReleaseReservation(ctx, other.ID))
	_, err = repo.GetReservedByUserID(ctx, "p-1", other.UserID, time.Minute)
	require.Error(t, err)
}

// 并发领取时，同一个槽位不能被两个人同时拿走。
//
// 用文件库而不是 :memory: —— SQLite 的内存库是「每个连接一个」，
// 并发写会落到不同的库上。
//
// 这条用例钉住的是**预留的原子性**：N 个并发预留必须拿到 N 个**不同**的槽位，
// 且**不需要重试**。预留不是原子的（旧实现只 SELECT 不改状态）时，所有人
// 会拿到同一个槽位，只能靠「撞车 → 重试」兜着 —— 真实投票窗口里几百人同时
// 领取时，那是一场灾难（每一轮只有一个人能成）。
func TestCredentialClaimRepository_ConcurrentReservationsGetDistinctSlots(t *testing.T) {
	repo := newClaimRepoWithDSN(t, filepath.Join(t.TempDir(), "claims.db"))
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 8)

	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := map[string]bool{}
	failures := 0

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := repo.ReserveAvailable(ctx, "p-1", time.Minute)
			if err != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			mu.Lock()
			reserved[claim.ID] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.Zero(t, failures, "每次预留都应当拿到一个槽位")
	require.Len(t, reserved, 8, "8 次并发预留应当拿到 8 个不同的槽位")
}

func TestCredentialClaimRepository_ReserveAvailableNotFound(t *testing.T) {
	repo := newClaimRepo(t)

	_, err := repo.ReserveAvailable(context.Background(), "p-missing", time.Minute)
	require.Error(t, err)
}

// TestCredentialClaimRepository_ConcurrentReservationsScaleToPool 把并发预留
// 拉到真实投票窗口的规模：几百个槽位、几十个并发领取者。
//
// 8 槽位 8 并发的小规模用例**测不出**真实缺陷：池子里槽位够多时，
// 「挑候选 → 条件更新失败 → 重试」的循环会因为所有并发者都盯上同一行
// （同一批插入的 created_at 完全相同，排序落回随机的 UUID id）而反复空转，
// 重试次数耗尽就误报「池空」。
//
// 这条用例钉住的是：**每一次预留都必须成功，且拿到互不相同的槽位**。
func TestCredentialClaimRepository_ConcurrentReservationsScaleToPool(t *testing.T) {
	repo := newClaimRepoWithDSN(t, filepath.Join(t.TempDir(), "scale-claims.db"))
	ctx := context.Background()

	const slotCount = 205
	const workers = 32
	seedClaim(t, repo, "p-scale", slotCount)

	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := map[string]bool{}
	failures := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := repo.ReserveAvailable(ctx, "p-scale", time.Minute)
			if err != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			mu.Lock()
			reserved[claim.ID] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.Zero(t, failures, "每次预留都应当成功 —— 池子够大就不该报池空")
	require.Len(t, reserved, workers, "并发预留必须拿到互不相同的槽位")
}

// 备池一次投几千个槽位是允许的（上限 10000），所以写入必须分批 ——
// 一次性 INSERT 会撞上 SQLite 的绑定参数上限，报 "too many SQL variables"。
//
// 这条用例必须用**文件库**：内存库的临时表天生就在内存里（temp_store 相当于
// MEMORY），复现不出「多行 INSERT 的临时索引溢写临时文件失败」那条路径
// （见 db.go 里 temp_store 那段说明）。
//
// id / user_id 用 UUID 长度（36 字符）：临时索引的键越长，越早撑破内存去落
// 临时文件 —— 真实备池用的就是 UUID（usecase/registration.go）。
func TestCredentialClaimRepository_SaveLargeBatch(t *testing.T) {
	repo := newClaimRepoWithDSN(t, filepath.Join(t.TempDir(), "big-claims.db"))
	ctx := context.Background()

	const count = 5000
	claims := make([]*entity.CredentialClaim, 0, count)
	for i := 0; i < count; i++ {
		claims = append(claims, &entity.CredentialClaim{
			ID:         uuid.NewString(),
			ProposalID: "p-big",
			UserID:     uuid.NewString(),
			UserPubKey: []byte{},
			Signature:  []byte{},
			CreatedAt:  time.Now().UTC(),
		})
	}
	require.NoError(t, repo.Save(ctx, claims))

	available, err := repo.CountAvailable(ctx, "p-big")
	require.NoError(t, err)
	require.Equal(t, int64(count), available)
}
