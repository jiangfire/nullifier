package repository

import (
	"context"

	"anonvote/internal/domain/entity"
)

type CredentialRepository interface {
	Save(ctx context.Context, credential *entity.Credential) error
	GetByUserID(ctx context.Context, userID string) (*entity.Credential, error)
}
