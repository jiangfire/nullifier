package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SubmitVoteRequest struct {
	ProposalID          string
	EncryptedVote       []byte
	Proof               *entity.Proof
	Nullifier           string
	PublicWitness       []byte
	UserID              string
	UserPubKey          []byte
	CredentialSignature []byte
}

func (r *SubmitVoteRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: request is nil", ErrInvalidRequest)
	}
	if r.ProposalID == "" {
		return fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if r.Nullifier == "" {
		return fmt.Errorf("%w: nullifier is required", ErrInvalidRequest)
	}
	if len(r.EncryptedVote) == 0 {
		return fmt.Errorf("%w: encrypted_vote is required", ErrInvalidRequest)
	}
	if r.Proof == nil || len(r.Proof.Data) == 0 {
		return fmt.Errorf("%w: proof is required", ErrInvalidRequest)
	}
	if r.UserID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidRequest)
	}
	if len(r.UserPubKey) == 0 {
		return fmt.Errorf("%w: user_pub_key is required", ErrInvalidRequest)
	}
	if len(r.CredentialSignature) == 0 {
		return fmt.Errorf("%w: credential_signature is required", ErrInvalidRequest)
	}
	return nil
}

func (r *SubmitVoteRequest) PublicInputs() entity.PublicInputs {
	inputs := entity.PublicInputs{
		"proposal_id": []byte(r.ProposalID),
		"nullifier":   []byte(r.Nullifier),
	}
	if len(r.PublicWitness) > 0 {
		inputs["public_witness"] = append([]byte(nil), r.PublicWitness...)
	}
	return inputs
}

type VotingUseCase struct {
	repo              repository.VoteRepository
	participationRepo repository.ParticipationRepository
	credentialRepo    repository.CredentialRepository
	proposalRepo      repository.ProposalRepository
	sessionRepo       repository.DecryptionSessionRepository
	verifier          domaincrypto.ProofVerifier
	encrypter         domaincrypto.Encrypter
	logger            *zap.Logger
}

func NewVotingUseCase(
	repo repository.VoteRepository,
	participationRepo repository.ParticipationRepository,
	credentialRepo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	sessionRepo repository.DecryptionSessionRepository,
	verifier domaincrypto.ProofVerifier,
	encrypter domaincrypto.Encrypter,
	logger *zap.Logger,
) *VotingUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &VotingUseCase{
		repo:              repo,
		participationRepo: participationRepo,
		credentialRepo:    credentialRepo,
		proposalRepo:      proposalRepo,
		sessionRepo:       sessionRepo,
		verifier:          verifier,
		encrypter:         encrypter,
		logger:            logger,
	}
}

type atomicVoteParticipationSaver interface {
	SaveWithParticipation(ctx context.Context, vote *entity.Vote, userHash string, votedAt time.Time) error
}

func (uc *VotingUseCase) Submit(ctx context.Context, req *SubmitVoteRequest) error {
	if uc.repo == nil {
		return errors.New("vote repository is not configured")
	}
	if uc.proposalRepo == nil {
		return errors.New("proposal repository is not configured")
	}
	if uc.sessionRepo == nil {
		return errors.New("decryption session repository is not configured")
	}
	if uc.credentialRepo == nil {
		return errors.New("credential repository is not configured")
	}
	if uc.participationRepo == nil {
		return errors.New("participation repository is not configured")
	}
	if uc.verifier == nil {
		return errors.New("proof verifier is not configured")
	}
	if err := req.Validate(); err != nil {
		return err
	}
	proposal, err := uc.proposalRepo.GetByID(ctx, req.ProposalID)
	if err != nil {
		return fmt.Errorf("load proposal: %w", err)
	}
	if !isProposalOpenForVoting(proposal, time.Now().UTC()) {
		return fmt.Errorf("%w: proposal is not open for voting", ErrInvalidRequest)
	}
	_, err = uc.sessionRepo.GetByProposal(ctx, req.ProposalID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: decryption session is not ready", ErrInvalidRequest)
		}
		return fmt.Errorf("load decryption session: %w", err)
	}

	if err := uc.verifier.Verify(req.Proof, req.PublicInputs()); err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	if uc.encrypter != nil {
		if err := uc.encrypter.ValidateCiphertext(req.EncryptedVote); err != nil {
			return fmt.Errorf("%w: invalid encrypted vote: %v", ErrInvalidRequest, err)
		}
	}
	credential, err := uc.credentialRepo.GetByProposalAndUserID(ctx, req.ProposalID, req.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidCredential
		}
		return fmt.Errorf("load credential: %w", err)
	}
	if !bytes.Equal(credential.UserPubKey, req.UserPubKey) {
		return ErrInvalidCredential
	}
	if !bytes.Equal(credential.Signature, req.CredentialSignature) {
		return ErrInvalidCredential
	}

	userHash := hashUserID(req.UserID)

	exists, err := uc.repo.ExistsByNullifier(ctx, req.Nullifier)
	if err != nil {
		return fmt.Errorf("check nullifier: %w", err)
	}
	if exists {
		return ErrDoubleVoting
	}
	hasVoted, err := uc.participationRepo.HasVoted(ctx, req.ProposalID, userHash)
	if err != nil {
		return fmt.Errorf("check user participation: %w", err)
	}
	if hasVoted {
		return ErrUserAlreadyVoted
	}

	vote := &entity.Vote{
		ID:         uuid.NewString(),
		ProposalID: req.ProposalID,
		Nullifier:  req.Nullifier,
		Cipher:     req.EncryptedVote,
		CreatedAt:  time.Now().UTC(),
	}

	if txSaver, ok := uc.repo.(atomicVoteParticipationSaver); ok {
		if err := txSaver.SaveWithParticipation(ctx, vote, userHash, vote.CreatedAt); err != nil {
			if errors.Is(err, repository.ErrDuplicateParticipation) {
				return ErrUserAlreadyVoted
			}
			if errors.Is(err, repository.ErrDuplicateVote) {
				return ErrDoubleVoting
			}
			return fmt.Errorf("save vote with participation: %w", err)
		}
	} else {
		if err := uc.repo.Save(ctx, vote); err != nil {
			if errors.Is(err, repository.ErrDuplicateVote) {
				return ErrDoubleVoting
			}
			return fmt.Errorf("save vote: %w", err)
		}
		if err := uc.participationRepo.MarkVoted(
			ctx,
			req.ProposalID,
			userHash,
			vote.CreatedAt,
		); err != nil {
			return fmt.Errorf("mark user voted: %w", err)
		}
	}

	uc.logger.Info("vote submitted",
		zap.String("proposal_id", req.ProposalID),
		zap.String("vote_id", vote.ID),
	)
	return nil
}

func hashUserID(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:])
}

func isProposalOpenForVoting(proposal *entity.Proposal, now time.Time) bool {
	if proposal == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(proposal.Status))
	if status != "" && status != "open" {
		return false
	}
	if !proposal.StartsAt.IsZero() && now.Before(proposal.StartsAt) {
		return false
	}
	if !proposal.EndsAt.IsZero() && !now.Before(proposal.EndsAt) {
		return false
	}
	return true
}
