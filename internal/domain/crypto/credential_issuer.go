package crypto

import (
	"context"

	"anonvote/internal/domain/entity"
)

type CredentialIssuer interface {
	Issue(ctx context.Context, userID string, userPubKey []byte) (*entity.Credential, error)
	// VerifyCredential 用签发方的公钥校验凭证签名。
	//
	// 投票路径必须在落库前调用它：只比对「库里存的签名」与「请求里带的
	// 签名」是否逐字节相同，任何人都能拿一张合法凭证换个 user_id 重放。
	VerifyCredential(userID string, userPubKey []byte, signature []byte) error
}
