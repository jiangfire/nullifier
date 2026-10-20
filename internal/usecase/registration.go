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

const (
	// maxCredentialPoolSize 限制单次备池数量，避免一个请求打爆数据库。
	maxCredentialPoolSize = 10000
	// claimMaxAttempts 是领取时的重试次数（被别人抢先领走时换槽位重试）。
	claimMaxAttempts = 5
)

type RegisterUseCase struct {
	repo         repository.CredentialRepository
	proposalRepo repository.ProposalRepository
	issuer       domaincrypto.CredentialIssuer
	claimRepo    repository.CredentialClaimRepository
	logger       *zap.Logger
}

func NewRegisterUseCase(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	logger *zap.Logger,
) *RegisterUseCase {
	return NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, nil, logger)
}

// NewRegisterUseCaseWithPool 额外接入凭证池仓储，用于匿名领取。
func NewRegisterUseCaseWithPool(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	claimRepo repository.CredentialClaimRepository,
	logger *zap.Logger,
) *RegisterUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RegisterUseCase{
		repo:         repo,
		proposalRepo: proposalRepo,
		issuer:       issuer,
		claimRepo:    claimRepo,
		logger:       logger,
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

// FillCredentialPool 为提案备好 count 个待领取的凭证槽位。
//
// 每个槽位带一个随机 user_id —— 它是投票人的**假名**，不绑定任何真人。
// 运营方知道池里有哪些 user_id，但不知道谁领走了哪一个。
func (uc *RegisterUseCase) FillCredentialPool(
	ctx context.Context,
	proposalID string,
	count int,
) (int, error) {
	if uc.claimRepo == nil {
		return 0, errors.New("credential claim repository is not configured")
	}
	if uc.proposalRepo == nil {
		return 0, errors.New("proposal repository is not configured")
	}
	if proposalID == "" {
		return 0, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if count <= 0 {
		return 0, fmt.Errorf("%w: count must be positive", ErrInvalidRequest)
	}
	if count > maxCredentialPoolSize {
		return 0, fmt.Errorf(
			"%w: count must not exceed %d", ErrInvalidRequest, maxCredentialPoolSize,
		)
	}
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return 0, fmt.Errorf("load proposal: %w", err)
	}

	now := time.Now().UTC()
	claims := make([]*entity.CredentialClaim, 0, count)
	for i := 0; i < count; i++ {
		claims = append(claims, &entity.CredentialClaim{
			ID:         uuid.NewString(),
			ProposalID: proposalID,
			UserID:     uuid.NewString(),
			UserPubKey: []byte{},
			Signature:  []byte{},
			CreatedAt:  now,
		})
	}
	if err := uc.claimRepo.Save(ctx, claims); err != nil {
		return 0, fmt.Errorf("save credential pool: %w", err)
	}

	uc.logger.Info("credential pool filled",
		zap.String("proposal_id", proposalID),
		zap.Int("count", count),
	)
	return count, nil
}

// ClaimCredential 匿名领取一个凭证。
//
// 请求里**不含任何身份信息**：服务端随机给出一个未领取的槽位，用领取者
// 自己带来的公钥签发凭证。因此服务端与运营方都无法把凭证和投票人对应起来。
//
// 代价要说清楚：服务端因此**无法强制一人一票** —— 任何人都能反复领取并投票。
// 「一人一票」要靠资格证明（零知识证明「我符合资格」而不透露身份）来保证，
// 那是 v0.2.0 证明链阶段的任务。
func (uc *RegisterUseCase) ClaimCredential(
	ctx context.Context,
	proposalID string,
	userPubKey []byte,
) (*entity.Credential, error) {
	if uc.claimRepo == nil {
		return nil, errors.New("credential claim repository is not configured")
	}
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
	if len(userPubKey) == 0 {
		return nil, fmt.Errorf("%w: user_pub_key is required", ErrInvalidRequest)
	}
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	// 领取是「抢一个槽位」：可能被别人抢先，因此重试几次。
	for attempt := 0; attempt < claimMaxAttempts; attempt++ {
		claim, err := uc.claimRepo.TakeAvailable(ctx, proposalID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, fmt.Errorf("%w: no credential available", ErrCredentialPoolEmpty)
			}
			return nil, fmt.Errorf("take available credential: %w", err)
		}

		issued, err := uc.issuer.Issue(ctx, claim.UserID, userPubKey)
		if err != nil {
			return nil, fmt.Errorf("issue credential: %w", err)
		}
		if issued == nil {
			return nil, errors.New("issue credential: empty result")
		}

		claimedAt := time.Now().UTC()
		ok, err := uc.claimRepo.MarkClaimed(
			ctx,
			claim.ID,
			userPubKey,
			issued.Signature,
			claimedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("mark credential claimed: %w", err)
		}
		if !ok {
			// 槽位被别人领走了，换一个再试。
			continue
		}

		credential := &entity.Credential{
			ID:               uuid.NewString(),
			ProposalID:       proposalID,
			UserID:           claim.UserID,
			UserPubKey:       append([]byte(nil), userPubKey...),
			Signature:        append([]byte(nil), issued.Signature...),
			IssuerKeyVersion: issued.IssuerKeyVersion,
			IssuedAt:         claimedAt,
		}
		if credential.IssuerKeyVersion <= 0 {
			credential.IssuerKeyVersion = 1
		}
		if err := uc.repo.Save(ctx, credential); err != nil {
			return nil, fmt.Errorf("save credential: %w", err)
		}

		// 只记数量，不记是哪个 user_id 被领走了 —— 领取的关联性由领取方自己掌握。
		uc.logger.Info("credential claimed",
			zap.String("proposal_id", proposalID),
		)
		return credential, nil
	}

	return nil, fmt.Errorf("%w: no credential available", ErrCredentialPoolEmpty)
}
