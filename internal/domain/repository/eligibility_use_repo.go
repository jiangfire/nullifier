package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

// EligibilityUseRepository 记录资格的使用情况。
type EligibilityUseRepository interface {
	// RecordUse 记录一次资格使用。
	//
	// 同一个 (proposal_id, qualification_id) 第二次使用时返回
	// ErrDuplicateEligibilityUse —— 调用方据此拒绝领取（409）。
	RecordUse(ctx context.Context, use *entity.EligibilityUse) error
	// CountUses 返回该提案已经用掉的资格数。
	CountUses(ctx context.Context, proposalID string) (int64, error)
}
