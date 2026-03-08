package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RegisterUseCase struct {
	repo   repository.CredentialRepository
	issuer domaincrypto.CredentialIssuer
	logger *zap.Logger
}

func NewRegisterUseCase(
	repo repository.CredentialRepository,
	issuer domaincrypto.CredentialIssuer,
	logger *zap.Logger,
) *RegisterUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RegisterUseCase{
		repo:   repo,
		issuer: issuer,
		logger: logger,
	}
}

func (uc *RegisterUseCase) IssueCredential(
	ctx context.Context,
	userID string,
	userPubKey []byte,
) (*entity.Credential, error) {
	if uc.repo == nil {
		return nil, errors.New("credential repository is not configured")
	}
	if uc.issuer == nil {
		return nil, errors.New("credential issuer is not configured")
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}
	if len(userPubKey) == 0 {
		return nil, fmt.Errorf("%w: user_pub_key is required", ErrInvalidRequest)
	}

	existing, err := uc.repo.GetByUserID(ctx, userID)
	if err == nil && existing != nil {
		return nil, ErrCredentialAlreadyIssued
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("check existing credential: %w", err)
	}

	credential, err := uc.issuer.Issue(ctx, userID, userPubKey)
	if err != nil {
		return nil, fmt.Errorf("issue credential: %w", err)
	}
	if credential == nil {
		return nil, errors.New("issue credential: empty result")
	}

	if credential.ID == "" {
		credential.ID = uuid.NewString()
	}
	if credential.UserID == "" {
		credential.UserID = userID
	}
	if len(credential.UserPubKey) == 0 {
		credential.UserPubKey = append([]byte(nil), userPubKey...)
	}
	if credential.IssuedAt.IsZero() {
		credential.IssuedAt = time.Now().UTC()
	}
	if credential.IssuerKeyVersion <= 0 {
		credential.IssuerKeyVersion = 1
	}

	if err := uc.repo.Save(ctx, credential); err != nil {
		return nil, fmt.Errorf("save credential: %w", err)
	}

	uc.logger.Info("credential issued",
		zap.String("credential_id", credential.ID),
		zap.String("user_id", credential.UserID),
	)
	return credential, nil
}
