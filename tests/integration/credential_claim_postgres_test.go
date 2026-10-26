package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/persistence"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// TestCredentialClaimRepository_ConcurrentReservationsPostgres 在**真 PostgreSQL**
// 上钉住「并发领取凭证槽位」的正确性。
//
// 这条用例守着的是一个真实事故：服务端原本用「挑一个候选 → 条件更新 → 失败
// 就重试」来占住槽位。备池的槽位是批量插入的，同一提案的所有槽位 created_at
// 完全相同，排序落回随机的 UUID id —— 于是所有并发领取者都盯上**同一行**，
// 条件更新在行锁上排队失败后重试又挑回同一行。重试次数一耗尽，池子里明明还
// 有几十上百个空槽位，却报「池空」：真实投票窗口里几百人同时领取时，大半
// 人会拿到「没有可领取的凭证」。
//
// PostgreSQL 的解法是行锁 + SKIP LOCKED：被别人锁住的行直接跳过，每个并发
// 领取者立刻拿到各自��槽位。这条用例断言的就是这件事：**每一次预留都必须
// 成功，且拿到互不相同的槽位**。
func TestCredentialClaimRepository_ConcurrentReservationsPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewCredentialClaimRepository(db)
	proposalID := "proposal-reserve-" + uuid.NewString()

	// 槽位数量与并发领取者数量都按真实投票窗口的量级来。
	const slotCount = 205
	const workers = 100

	claims := make([]*entity.CredentialClaim, 0, slotCount)
	for i := 0; i < slotCount; i++ {
		claims = append(claims, &entity.CredentialClaim{
			ID:         uuid.NewString(),
			ProposalID: proposalID,
			UserID:     uuid.NewString(),
			UserPubKey: []byte{},
			Signature:  []byte{},
			CreatedAt:  time.Now().UTC(),
		})
	}
	require.NoError(t, repo.Save(ctx, claims))

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		reserved  = map[string]bool{}
		failures  int
		errSample error
	)

	// 起跑栅栏：所有领取者**同时**发起请求。
	//
	// 这不是为了好看 —— 真实投票窗口就是这种形态：几百个人在同一个时刻
	// 点下「领取」。少了栅栏，各goroutine 的 SELECT 会错开，撞车概率
	// 大幅下降，这条用例就抓不到那个缺陷了。
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, err := repo.ReserveAvailable(ctx, proposalID, time.Minute)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				if errSample == nil {
					errSample = err
				}
				return
			}
			reserved[claim.ID] = true
		}()
	}
	close(start)
	wg.Wait()

	require.NoError(t, errSample, "并发预留不应当失败")
	require.Zero(t, failures, "每一次预留都应当拿到一个槽位")
	require.Len(t, reserved, workers, "并发预留必须拿到互不相同的槽位")
}

// TestCredentialClaimRepository_ReservationExpiryPostgres 验证过期预留可以被
// 重新拿走 —— 只预留不领取的客户端不能把池子占死。
func TestCredentialClaimRepository_ReservationExpiryPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewCredentialClaimRepository(db)
	proposalID := "proposal-expiry-" + uuid.NewString()

	require.NoError(t, repo.Save(ctx, []*entity.CredentialClaim{{
		ID:         uuid.NewString(),
		ProposalID: proposalID,
		UserID:     uuid.NewString(),
		UserPubKey: []byte{},
		Signature:  []byte{},
		CreatedAt:  time.Now().UTC(),
	}}))

	// TTL 为负 → 任何预留都算过期。
	first, err := repo.ReserveAvailable(ctx, proposalID, -time.Second)
	require.NoError(t, err)

	second, err := repo.ReserveAvailable(ctx, proposalID, -time.Second)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "过期预留应当可以被重新拿走")
}

// TestCredentialClaimRepository_SkipsLockedSlotPostgres 验证预留**不会**排在
// 别人的行锁后面等。
//
// 这是 PostgreSQL 专属路径存在的理由：没有 SKIP LOCKED 时，预留的实现是
// 「挑一个候选 → 条件更新」。候选被别人锁住（正在被别人领）时，条件更新
// 会在行锁上排队等到对方提交 —— 一旦对方那边慢，所有并发领取者都堵在
// 同一行上，最后重试次数耗尽，明明池子里还有空槽位却报「池空」。
//
// 有了 SKIP LOCKED，被锁住的行直接跳过：每个领取者立刻拿到自己的槽位。
//
// 这条用例是确定性的：自己拿一个事务把「下一个会被挑中的槽位」锁住，
// 然后断言预留能在限定时间内拿到**另一个**槽位。
func TestCredentialClaimRepository_SkipsLockedSlotPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewCredentialClaimRepository(db)
	proposalID := "proposal-locked-" + uuid.NewString()

	const slotCount = 8
	claims := make([]*entity.CredentialClaim, 0, slotCount)
	for i := 0; i < slotCount; i++ {
		claims = append(claims, &entity.CredentialClaim{
			ID:         uuid.NewString(),
			ProposalID: proposalID,
			UserID:     uuid.NewString(),
			UserPubKey: []byte{},
			Signature:  []byte{},
			CreatedAt:  time.Now().UTC(),
		})
	}
	require.NoError(t, repo.Save(ctx, claims))

	// 用独立的事务锁住「排序上第一个」的那个槽位 —— 那正是预留会先挑中的。
	locker := db.WithContext(ctx).Begin()
	require.NoError(t, locker.Error)
	defer func() { _ = locker.Rollback().Error }()

	var first entity.CredentialClaim
	require.NoError(t, locker.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("proposal_id = ?", proposalID).
		Order("created_at ASC, id ASC").
		First(&first).Error)

	// 预留必须在限定时间内完成：排在别人的行锁后面等 = 失败。
	reserveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	claim, err := repo.ReserveAvailable(reserveCtx, proposalID, time.Minute)
	require.NoError(t, err, "被锁住的槽位应当被跳过，而不是排队等锁")
	require.NotEqual(t, first.ID, claim.ID, "不该拿到被别人锁住的那个槽位")
}
