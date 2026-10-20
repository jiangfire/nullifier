package repository

import (
	"context"
	"time"

	"anonvote/internal/domain/entity"
)

// CredentialClaimRepository 读写凭证池。
type CredentialClaimRepository interface {
	// Save 批量写入待领取的槽位，返回实际写入的数量。
	Save(ctx context.Context, claims []*entity.CredentialClaim) error
	// TakeAvailable 取一个尚未领取的槽位，**不修改状态**。
	// 领取流程需要先拿到 user_id 去签名，再原子地标记已领取。
	TakeAvailable(ctx context.Context, proposalID string) (*entity.CredentialClaim, error)
	// MarkClaimed 原子地把槽位标记为已领取并写入领取者的公钥与签名。
	// 槽位已被别人领走时返回 false，调用方应重试。
	MarkClaimed(
		ctx context.Context,
		claimID string,
		userPubKey []byte,
		signature []byte,
		claimedAt time.Time,
	) (bool, error)
	// CountAvailable 返回该提案还剩多少未领取的槽位。
	CountAvailable(ctx context.Context, proposalID string) (int64, error)
}
