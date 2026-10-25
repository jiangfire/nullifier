package persistence

import (
	"context"
	"errors"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"gorm.io/gorm"
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

func (r *CredentialClaimRepository) TakeAvailable(
	ctx context.Context,
	proposalID string,
) (*entity.CredentialClaim, error) {
	var claim entity.CredentialClaim
	err := r.db.WithContext(ctx).
		Where("proposal_id = ? AND claimed_at IS NULL", proposalID).
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
