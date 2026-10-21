package crypto

import "anonvote/internal/domain/entity"

type ProofVerifier interface {
	// Verify 校验选票证明是否与公开输入一致。
	//
	// 公开输入由调用方根据请求字段、提案与计票会话自行拼装，验证器不再
	// 接受客户端自由提交的公开输入。
	Verify(proof *entity.Proof, statement entity.BallotStatement) error
}
