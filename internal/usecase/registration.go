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
)

// BlindSignScope 规定盲签名能覆盖哪些 user_id。
type BlindSignScope int

const (
	// BlindSignScopePoolSlot 只允许为「池里还空着的槽位」盲签 —— 公开端点用。
	//
	// 少了这条限制，公开的盲签名端点会把签发方变成「任意消息」的签名预言机：
	// 任何人都能拿它签任意消息。绑定到具体槽位后，能签的就只有池里的假名。
	BlindSignScopePoolSlot BlindSignScope = iota
	// BlindSignScopeAnyUser 允许为任意 user_id 盲签 —— 管理端用。
	//
	// 运营方本来就有服务端的完整权限，这条不给他新增任何能力。
	BlindSignScopeAnyUser
)

// CredentialReservation 是一次「预留签发权」的结果。
//
// 预留**不签名**：客户端拿到 user_id 与签发方公钥后，在本地把凭证消息盲化，
// 再把盲化消息送来盲签。服务端从头到尾没见过被签内容。
type CredentialReservation struct {
	// ProposalID 是提案 ID。
	ProposalID string
	// UserID 是这次预留拿到的凭证假名。
	UserID string
	// IssuerKey 是签发方公钥：客户端盲化消息时需要它。
	IssuerKey *domaincrypto.IssuerPublicKey
}

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

// ReserveCredentialForUser 管理端为一个指定的 user_id 预留签发权。
//
// 它**不签名**：管理端客户端拿到 user_id 与签发方公钥后，在本地把凭证消息
// 盲化，再走「盲签名 → attest」两步。服务端因此在签名阶段只见盲化消息。
func (uc *RegisterUseCase) ReserveCredentialForUser(
	ctx context.Context,
	proposalID string,
	userID string,
) (*CredentialReservation, error) {
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

	issuerKey, err := uc.issuer.IssuerPublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("load issuer public key: %w", err)
	}
	return &CredentialReservation{
		ProposalID: proposalID,
		UserID:     userID,
		IssuerKey:  issuerKey,
	}, nil
}

// ReserveCredential 匿名预留一个凭证池槽位。
//
// 预留请求里**不含任何身份信息**：服务端随机给出一个未领取的槽位，运营方因此
// 不知道谁领了哪一个。这一步**不签名** —— 签名在客户端盲化之后才发生。
func (uc *RegisterUseCase) ReserveCredential(
	ctx context.Context,
	proposalID string,
) (*CredentialReservation, error) {
	if uc.claimRepo == nil {
		return nil, errors.New("credential claim repository is not configured")
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
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	claim, err := uc.claimRepo.TakeAvailable(ctx, proposalID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: no credential available", ErrCredentialPoolEmpty)
		}
		return nil, fmt.Errorf("take available credential: %w", err)
	}

	issuerKey, err := uc.issuer.IssuerPublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("load issuer public key: %w", err)
	}
	return &CredentialReservation{
		ProposalID: proposalID,
		UserID:     claim.UserID,
		IssuerKey:  issuerKey,
	}, nil
}

// BlindSignCredential 对**盲化后**的消息做盲签名。
//
// 这是签发方在真两段式盲签名里唯一的签名动作：服务端从头到尾只见盲化消息。
func (uc *RegisterUseCase) BlindSignCredential(
	ctx context.Context,
	proposalID string,
	userID string,
	blindedMessage []byte,
	keyVersion int,
	scope BlindSignScope,
) ([]byte, error) {
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
	if len(blindedMessage) == 0 {
		return nil, fmt.Errorf("%w: blinded_message is required", ErrInvalidRequest)
	}
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if scope == BlindSignScopePoolSlot {
		if uc.claimRepo == nil {
			return nil, errors.New("credential claim repository is not configured")
		}
		// 公开端点只能为池里还空着的槽位盲签：否则签发方会变成「任意消息」的
		// 签名预言机。
		if _, err := uc.claimRepo.GetAvailableByUserID(ctx, proposalID, userID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, fmt.Errorf(
					"%w: no credential slot available for this user_id", ErrInvalidRequest,
				)
			}
			return nil, fmt.Errorf("look up credential slot: %w", err)
		}
	}
	blindSignature, err := uc.issuer.BlindSign(ctx, keyVersion, blindedMessage)
	if err != nil {
		return nil, fmt.Errorf("blind sign credential: %w", err)
	}
	return blindSignature, nil
}

// AttestCredential 校验客户端解盲出来的最终签名，通过后落库。
//
// 验签绑定的消息是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)`：客户端拿 A 的盲
// 签名去签 B，服务端这边必然验不过 —— 交叉攻击就是被这一步挡住的。
func (uc *RegisterUseCase) AttestCredential(
	ctx context.Context,
	proposalID string,
	userID string,
	userPubKey []byte,
	signature []byte,
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
	if len(signature) == 0 {
		return nil, fmt.Errorf("%w: signature is required", ErrInvalidRequest)
	}
	if _, err := uc.proposalRepo.GetByID(ctx, proposalID); err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	// 先验签：签名不对的凭证一律不落库。
	keyVersion, err := uc.issuer.VerifyCredential(userID, userPubKey, signature)
	if err != nil {
		return nil, fmt.Errorf("%w: credential signature verification failed", ErrInvalidCredential)
	}

	existing, err := uc.repo.GetByProposalAndUserID(ctx, proposalID, userID)
	if err == nil && existing != nil {
		return nil, ErrCredentialAlreadyIssued
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("check existing credential: %w", err)
	}

	claimedAt := time.Now().UTC()
	// 池里的槽位要原子地标记为已领取：槽位被别人抢走时返回冲突，客户端换一个
	// 槽位重来。管理端路径的 user_id 不在池里，没有槽位要标记。
	if uc.claimRepo != nil {
		claim, err := uc.claimRepo.GetAvailableByUserID(ctx, proposalID, userID)
		switch {
		case err == nil:
			ok, err := uc.claimRepo.MarkClaimed(ctx, claim.ID, userPubKey, signature, claimedAt)
			if err != nil {
				return nil, fmt.Errorf("mark credential claimed: %w", err)
			}
			if !ok {
				return nil, ErrCredentialAlreadyIssued
			}
		case errors.Is(err, repository.ErrNotFound):
		default:
			return nil, fmt.Errorf("look up credential slot: %w", err)
		}
	}

	credential := &entity.Credential{
		ID:               uuid.NewString(),
		ProposalID:       proposalID,
		UserID:           userID,
		UserPubKey:       append([]byte(nil), userPubKey...),
		Signature:        append([]byte(nil), signature...),
		IssuerKeyVersion: keyVersion,
		IssuedAt:         claimedAt,
	}
	if err := uc.repo.Save(ctx, credential); err != nil {
		if errors.Is(err, repository.ErrDuplicateCredential) {
			return nil, ErrCredentialAlreadyIssued
		}
		return nil, fmt.Errorf("save credential: %w", err)
	}

	uc.logger.Info("credential attested",
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
