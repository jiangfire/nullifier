package crypto

import (
	"context"

	"anonvote/internal/domain/entity"
)

type CredentialIssuer interface {
	Issue(ctx context.Context, userID string, userPubKey []byte) (*entity.Credential, error)
}
