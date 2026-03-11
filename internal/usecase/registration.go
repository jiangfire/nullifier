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
	repo        repository.CredentialRepository
	proposalRepo repository.ProposalRepository
	issuer      domaincrypto.CredentialIssuer
	logger      *zap.Logger
}

func NewRegisterUseCase(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	logger *zap.Logger,
) *RegisterUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RegisterUseCase{
		repo:        repo,
		proposalRepo: proposalRepo,
		issuer:      issuer,
		logger:      logger,
	}
}

func (uc *RegisterUseCase) IssueCredential(
	ctx context.Context,
	proposalID string,
	userID string,
	userPubKey []byte,
) (*entity.Credential, error) {
	if uc.repo == nil {
		return nil, errors.New("credential repository is not configured")
	}
	if uc.issuer == nil {
		return nil, errors.New("credential issuer is not configured")
	}
	if uc.proposalRepo == nil {
		return nil, errors.New("proposal repository is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}
	if len(userPubKey) == 0 {
		return nil, fmt.Errorf("%w: user_pub_key is required", ErrInvalidRequest)
	}
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	existing, err := uc.repo.GetByProposalAndUserID(ctx, proposalID, userID)
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
	if credential.ProposalID == "" {
		credential.ProposalID = proposalID
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
		zap.String("proposal_id", credential.ProposalID),
		zap.String("user_id", credential.UserID),
	)
	return credential, nil
}
