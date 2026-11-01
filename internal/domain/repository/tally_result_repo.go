package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type TallyResultRepository interface {
	Save(ctx context.Context, result *entity.TallyResult) error
	// SaveWithAnchor 把计票结果与审计锚点写进**同一事务**：发布时锚点写失败
	// 必须连结果一起回滚 —— 结果一旦落库，唯一索引挡住一切重试，锚点缺了
	// 就永远补不上。
	SaveWithAnchor(ctx context.Context, result *entity.TallyResult, anchor *entity.AuditAnchor) error
	GetByProposal(ctx context.Context, proposalID string) (*entity.TallyResult, error)
}
