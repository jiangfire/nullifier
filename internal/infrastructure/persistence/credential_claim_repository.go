package persistence

import (
	"context"
	"errors"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CredentialClaimRepository struct {
	db *gorm.DB
}

var _ repository.CredentialClaimRepository = (*CredentialClaimRepository)(nil)

// claimSaveBatchSize 是单条 INSERT 语句里最多写入的行数。
//
// SQLite 对一条语句的绑定参数数量有上限（旧版 999，新版 32766）。
// credential_claims 有 7 列，一次性写 4681 行以上就会报
// "too many SQL variables" —— 而备池接口允许一次投到 10000 个槽位，
// 那条路径必然失败。100 行一批 = 700 个参数，两种上限都够得着。
//
// 另外，多行 INSERT 做主键去重时会建一个临时索引（见 db.go 里 temp_store
// 那段说明），批越小它越小，所以也不把批次开大。
const claimSaveBatchSize = 100

func NewCredentialClaimRepository(db *gorm.DB) *CredentialClaimRepository {
	return &CredentialClaimRepository{db: db}
}

func (r *CredentialClaimRepository) Save(ctx context.Context, claims []*entity.CredentialClaim) error {
	if len(claims) == 0 {
		return nil
	}
	// CreateInBatches 把切片拆成多条 INSERT，但仍然包在同一个事务里 ——
	// 备池要么整体成功，要么整体回滚，不会留下半个池子。
	return r.db.WithContext(ctx).CreateInBatches(&claims, claimSaveBatchSize).Error
}

// reserveAttempts 是「挑候选 → 条件更新」这条路的重试次数上限。
//
// 每次重试都是「重新挑一个候选槽位 + 条件更新」：并发时可能有多个领取者
// 挑到同一个候选，后到的那次条件更新会失败（槽位已被预留），换下一个即可。
//
// 次数要**大于并发领取者数量**：否则会出现「池子里还有几十上百个空槽位，
// 却因为重试耗尽而报池空」。备池的槽位是批量插入的，同一提案的所有槽位
// created_at 完全相同，排序落回随机的 UUID id —— 所有并发者都会盯上同一行。
const reserveAttempts = 64

// reserveRetryDelay 是重试前的等待。
//
// 只在「候选被抢走」或「空槽位正被别人锁着」这两种**瞬时**冲突上等待；
// 真正池空时不等待，直接返回。
const reserveRetryDelay = 2 * time.Millisecond

// errSlotTaken 表示挑中的候选槽位在落手前被别人拿走了。
var errSlotTaken = errors.New("credential slot was taken by another claimer")

// ReserveAvailable 原子地预留一个空闲槽位。
//
// **为什么必须原子**：不预留的话，所有并发领取者都会拿到「第一个空闲槽位」
// —— 领取流程要走三步（预留 → 盲签名 → 落库），这段时间里槽位在别人眼里
// 依然是空的，于是所有人抢同一个槽位，最后只有一个人成功，其余全部要
// 重新盲化再来一遍。真实投票窗口里几百人同时领取，那会是一场灾难。
//
// 实现分两种方言（见 reserveWithSkipLocked / reserveWithRetry）：PostgreSQL
// 用行锁一次挑中并占住，SQLite 用「挑候选 + 条件更新」重试。两条路的判据
// 完全一致（未被领取、且没有新鲜预留），所以同一个槽位不会被两个人拿走。
//
// 预留会过期（ttl）：只预留不领取的客户端不能永久占住池子。
func (r *CredentialClaimRepository) ReserveAvailable(
	ctx context.Context,
	proposalID string,
	ttl time.Duration,
) (*entity.CredentialClaim, error) {
	now := time.Now().UTC()
	expiry := now.Add(-ttl)
	if r.db.Dialector.Name() == "postgres" {
		return r.reserveWithSkipLocked(ctx, proposalID, now, expiry)
	}
	return r.reserveWithRetry(ctx, proposalID, now, expiry)
}

// reserveWithSkipLocked 用 PostgreSQL 的行锁完成「挑一个空槽位并占住它」。
//
// **为什么 PostgreSQL 要单独一条路**：SELECT ... FOR UPDATE 会给选中的行
// 加锁，别人再选同一行只能等。加了 SKIP LOCKED 之后，被别人锁住的行会被
// 直接跳过 —— 于是每个并发领取者都立刻拿到**各自**的槽位，一次成功。
//
// 没有 SKIP LOCKED 时会活锁：所有并发者都盯上同一行（批量插入的槽位
// created_at 完全相同，排序落回随机的 UUID id），条件更新在行锁上排队失败
// 后重试又挑回同一行 —— 重试次数耗尽，池子里明明还有空槽位，却报「池空」。
//
// 「挑 + 占」必须在**同一个事务**里：行锁只在事务内有效，事务一提交就释放。
func (r *CredentialClaimRepository) reserveWithSkipLocked(
	ctx context.Context,
	proposalID string,
	now time.Time,
	expiry time.Time,
) (*entity.CredentialClaim, error) {
	for attempt := 0; attempt < reserveAttempts; attempt++ {
		var candidate entity.CredentialClaim
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where(
				"proposal_id = ? AND claimed_at IS NULL AND (reserved_at IS NULL OR reserved_at < ?)",
				proposalID, expiry,
			).Order("created_at ASC, id ASC").
				Clauses(clause.Locking{
					Strength: clause.LockingStrengthUpdate,
					Options:  clause.LockingOptionsSkipLocked,
				}).
				First(&candidate).Error; err != nil {
				return err
			}
			result := tx.Model(&entity.CredentialClaim{}).
				Where(
					"id = ? AND claimed_at IS NULL AND (reserved_at IS NULL OR reserved_at < ?)",
					candidate.ID, expiry,
				).
				Update("reserved_at", now)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				// 判据与挑选时一致，所以这里只可能是并发写者抢走了 —— 重来。
				return errSlotTaken
			}
			candidate.ReservedAt = &now
			return nil
		})
		switch {
		case err == nil:
			return &candidate, nil
		case errors.Is(err, errSlotTaken):
			// 候选被抢走：换一个再来。
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 没有可选的行 —— 可能是池子真空了，也可能是所有空槽位都被
			// 别人的事务锁着（他们正在领）。后者要等一下再试，不能报池空。
			free, countErr := r.countFreeSlots(ctx, proposalID, expiry)
			if countErr != nil {
				return nil, countErr
			}
			if free == 0 {
				return nil, repository.ErrNotFound
			}
		default:
			return nil, err
		}
		time.Sleep(reserveRetryDelay)
	}
	return nil, repository.ErrNotFound
}

// reserveWithRetry 是没有行级锁的方言（SQLite）的实现：挑一个候选、条件更新、
// 被抢走就换一个再来。
//
// SQLite 的写是整库串行的，条件更新的判据保证同一个槽位不会被两个人拿走；
// 并发领取者会在写锁上排队，一个一个拿到不同的槽位。
func (r *CredentialClaimRepository) reserveWithRetry(
	ctx context.Context,
	proposalID string,
	now time.Time,
	expiry time.Time,
) (*entity.CredentialClaim, error) {
	for attempt := 0; attempt < reserveAttempts; attempt++ {
		var candidate entity.CredentialClaim
		err := r.db.WithContext(ctx).
			Where(
				"proposal_id = ? AND claimed_at IS NULL AND (reserved_at IS NULL OR reserved_at < ?)",
				proposalID, expiry,
			).
			Order("created_at ASC, id ASC").
			First(&candidate).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repository.ErrNotFound
		}
		if err != nil {
			return nil, err
		}

		result := r.db.WithContext(ctx).
			Model(&entity.CredentialClaim{}).
			Where(
				"id = ? AND claimed_at IS NULL AND (reserved_at IS NULL OR reserved_at < ?)",
				candidate.ID, expiry,
			).
			Update("reserved_at", now)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			candidate.ReservedAt = &now
			return &candidate, nil
		}
		// 候选被别人抢走了：换一个再来。
	}
	return nil, repository.ErrNotFound
}

// countFreeSlots 数一数池子里还有几个「未被领取、也没有新鲜预留」的槽位。
//
// 判据里含「没有新鲜预留」：正在被别人领的槽位虽然没被领取，但也不该再分出去。
func (r *CredentialClaimRepository) countFreeSlots(
	ctx context.Context,
	proposalID string,
	expiry time.Time,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&entity.CredentialClaim{}).
		Where(
			"proposal_id = ? AND claimed_at IS NULL AND (reserved_at IS NULL OR reserved_at < ?)",
			proposalID, expiry,
		).
		Count(&count).Error
	return count, err
}

// ReleaseReservation 释放一次预留。
//
// 领取流程后续失败（比如同一份资格第二次来领）时调用：凭证还没签发，
// 槽位不该被白白消耗。
func (r *CredentialClaimRepository) ReleaseReservation(ctx context.Context, claimID string) error {
	return r.db.WithContext(ctx).
		Model(&entity.CredentialClaim{}).
		Where("id = ? AND claimed_at IS NULL", claimID).
		Update("reserved_at", nil).Error
}

// GetAvailableByUserID 按 user_id 查一个尚未领取的槽位。
//
// 真两段式盲签名用它把盲签名绑定到具体槽位：公开端点只能为池里还空着的槽位
// 盲签 —— 否则签发方就成了「任意消息」的签名预言机。
func (r *CredentialClaimRepository) GetAvailableByUserID(
	ctx context.Context,
	proposalID string,
	userID string,
) (*entity.CredentialClaim, error) {
	var claim entity.CredentialClaim
	err := r.db.WithContext(ctx).
		Where("proposal_id = ? AND user_id = ? AND claimed_at IS NULL", proposalID, userID).
		Order("created_at ASC, id ASC").
		First(&claim).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// GetReservedByUserID 按 user_id 查一个**正在预留中**（且未过期）的槽位。
//
// 公开的盲签名端点用它：预留过期的槽位可能已经被别人重新预留，替它签名
// 等于替别人签名。
func (r *CredentialClaimRepository) GetReservedByUserID(
	ctx context.Context,
	proposalID string,
	userID string,
	ttl time.Duration,
) (*entity.CredentialClaim, error) {
	var claim entity.CredentialClaim
	err := r.db.WithContext(ctx).
		Where(
			"proposal_id = ? AND user_id = ? AND claimed_at IS NULL"+
				" AND reserved_at IS NOT NULL AND reserved_at > ?",
			proposalID, userID, time.Now().UTC().Add(-ttl),
		).
		Order("created_at ASC, id ASC").
		First(&claim).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// MarkClaimed 用条件更新保证同一个槽位只会被领取一次：
// 只有 claimed_at 仍为空的行会被更新，affected rows 为 0 说明被别人抢先领走了。
func (r *CredentialClaimRepository) MarkClaimed(
	ctx context.Context,
	claimID string,
	userPubKey []byte,
	signature []byte,
	claimedAt time.Time,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&entity.CredentialClaim{}).
		Where("id = ? AND claimed_at IS NULL", claimID).
		Updates(map[string]any{
			"user_pub_key": userPubKey,
			"signature":    signature,
			"claimed_at":   claimedAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *CredentialClaimRepository) CountAvailable(ctx context.Context, proposalID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&entity.CredentialClaim{}).
		Where("proposal_id = ? AND claimed_at IS NULL", proposalID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
