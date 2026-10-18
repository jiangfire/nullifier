package usecase

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"go.uber.org/zap"
)

type ParticipationUseCase struct {
	repo           repository.ParticipationRepository
	credentialRepo repository.CredentialRepository
	logger         *zap.Logger
}

type CredentialVoteStatus struct {
	CredentialID     string    `json:"credential_id"`
	ProposalID       string    `json:"proposal_id"`
	UserID           string    `json:"user_id"`
	UserPubKey       []byte    `json:"user_pub_key"`
	IssuerKeyVersion int       `json:"issuer_key_version"`
	IssuedAt         time.Time `json:"issued_at"`
	HasVoted         bool      `json:"has_voted"`
}

func NewParticipationUseCase(
	repo repository.ParticipationRepository,
	credentialRepo repository.CredentialRepository,
	logger *zap.Logger,
) *ParticipationUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ParticipationUseCase{
		repo:           repo,
		credentialRepo: credentialRepo,
		logger:         logger,
	}
}

func (uc *ParticipationUseCase) HasVoted(ctx context.Context, proposalID string, userID string) (bool, error) {
	if uc.repo == nil {
		return false, fmt.Errorf("participation repository is not configured")
	}
	if proposalID == "" {
		return false, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if userID == "" {
		return false, fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}

	hasVoted, err := uc.repo.HasVoted(ctx, proposalID, hashUserID(userID))
	if err != nil {
		return false, fmt.Errorf("check participation: %w", err)
	}
	return hasVoted, nil
}

func (uc *ParticipationUseCase) HasVotedWithCredential(
	ctx context.Context,
	proposalID string,
	userID string,
	userPubKey []byte,
	credentialSignature []byte,
) (bool, error) {
	if uc.credentialRepo == nil {
		return false, fmt.Errorf("credential repository is not configured")
	}
	if proposalID == "" {
		return false, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if userID == "" {
		return false, fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}
	if len(userPubKey) == 0 {
		return false, fmt.Errorf("%w: user_pub_key is required", ErrInvalidRequest)
	}
	if len(credentialSignature) == 0 {
		return false, fmt.Errorf("%w: credential_signature is required", ErrInvalidRequest)
	}

	credential, err := uc.credentialRepo.GetByProposalAndUserID(ctx, proposalID, userID)
	if err != nil {
		return false, fmt.Errorf("load credential: %w", err)
	}
	if !bytes.Equal(credential.UserPubKey, userPubKey) || !bytes.Equal(credential.Signature, credentialSignature) {
		return false, ErrInvalidCredential
	}
	return uc.HasVoted(ctx, proposalID, userID)
}

func (uc *ParticipationUseCase) ListCredentialStatuses(
	ctx context.Context,
	proposalID string,
) ([]CredentialVoteStatus, error) {
	if uc.repo == nil {
		return nil, fmt.Errorf("participation repository is not configured")
	}
	if uc.credentialRepo == nil {
		return nil, fmt.Errorf("credential repository is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}

	credentials, err := uc.credentialRepo.ListByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("list proposal credentials: %w", err)
	}

	votedHashes, err := uc.repo.ListVotedUserHashes(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("list proposal participations: %w", err)
	}

	out := make([]CredentialVoteStatus, 0, len(credentials))
	for _, credential := range credentials {
		if credential == nil {
			continue
		}
		_, hasVoted := votedHashes[hashUserID(credential.UserID)]
		out = append(out, mapCredentialVoteStatus(credential, hasVoted))
	}
	return out, nil
}

func mapCredentialVoteStatus(credential *entity.Credential, hasVoted bool) CredentialVoteStatus {
	return CredentialVoteStatus{
		CredentialID:     credential.ID,
		ProposalID:       credential.ProposalID,
		UserID:           credential.UserID,
		UserPubKey:       append([]byte(nil), credential.UserPubKey...),
		IssuerKeyVersion: credential.IssuerKeyVersion,
		IssuedAt:         credential.IssuedAt,
		HasVoted:         hasVoted,
	}
}
