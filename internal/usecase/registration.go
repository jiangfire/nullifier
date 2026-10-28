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

// EligibilityMaterial 是客户端提交的一份资格材料。
//
// 三件材料缺一不可：证明、去重键、挑战。去重键与挑战都是**公开输入**，
// 服务端据此自拼 statement —— 客户端无从篡改。
type EligibilityMaterial struct {
	// Proof 是 Groth16 资格证明。
	Proof []byte
	// QualificationID 是去重键（十六进制的域元素）。
	QualificationID string
	// Challenge 是本次领取会话的挑战（十六进制的域元素）。
	Challenge string
}

// EligibilityChallenge 是服务端签发的一次领取挑战。
type EligibilityChallenge struct {
	// ProposalID 是提案 ID。
	ProposalID string
	// Root 是资格名单的树根 —— 客户端用它出证明（以服务端为准）。
	Root string
	// Depth 是名单树的深度。
	Depth int
	// Challenge 是本次会话的一次性挑战。
	Challenge string
}

type RegisterUseCase struct {
	repo         repository.CredentialRepository
	proposalRepo repository.ProposalRepository
	issuer       domaincrypto.CredentialIssuer
	claimRepo    repository.CredentialClaimRepository
	// eligibilityRepo 记录资格的使用情况 —— 一人一票的强制点。
	eligibilityRepo repository.EligibilityUseRepository
	// eligibility 校验资格证明。为 nil 或未开启时，服务端不强制一人一票。
	eligibility domaincrypto.EligibilityService
	logger      *zap.Logger
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
	return newRegisterUseCase(repo, proposalRepo, issuer, claimRepo, nil, nil, logger)
}

// NewRegisterUseCaseWithEligibility 额外接入资格校验：凭证池仓储 + 资格使用
// 仓储 + 资格校验器。
//
// 这是生产形态：没有它，服务端不强制「一人一票」。
func NewRegisterUseCaseWithEligibility(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	claimRepo repository.CredentialClaimRepository,
	eligibilityRepo repository.EligibilityUseRepository,
	eligibility domaincrypto.EligibilityService,
	logger *zap.Logger,
) *RegisterUseCase {
	return newRegisterUseCase(repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, logger)
}

func newRegisterUseCase(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	claimRepo repository.CredentialClaimRepository,
	eligibilityRepo repository.EligibilityUseRepository,
	eligibility domaincrypto.EligibilityService,
	logger *zap.Logger,
) *RegisterUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RegisterUseCase{
		repo:            repo,
		proposalRepo:    proposalRepo,
		issuer:          issuer,
		claimRepo:       claimRepo,
		eligibilityRepo: eligibilityRepo,
		eligibility:     eligibility,
		logger:          logger,
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

// IssueEligibilityChallenge 签发一次领取会话的挑战。
//
// 服务端**不记账**：挑战的价值在于把证明钉在本次会话上，重放由去重键
// （唯一索引）挡住 —— 见 docs/t22-eligibility-design.md §3.5。
// root 与树深按**提案绑定的名单域**解析（未绑定时取默认名单）—— 一台
// 服务端并存多套名单时，每个提案只认自己那套名单。
func (uc *RegisterUseCase) IssueEligibilityChallenge(
	ctx context.Context,
	proposalID string,
) (*EligibilityChallenge, error) {
	if uc.eligibility == nil || !uc.eligibility.Enabled() {
		return nil, fmt.Errorf("%w: the server does not verify eligibility proofs", ErrEligibilityNotEnabled)
	}
	if uc.proposalRepo == nil {
		return nil, errors.New("proposal repository is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	proposal, err := uc.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	root, depth, err := uc.eligibility.RosterFor(proposal.EligibilityScope)
	if err != nil {
		return nil, fmt.Errorf("load eligibility roster: %w", err)
	}
	challenge, err := uc.eligibility.NewChallenge()
	if err != nil {
		return nil, fmt.Errorf("generate eligibility challenge: %w", err)
	}
	return &EligibilityChallenge{
		ProposalID: proposalID,
		Root:       root,
		Depth:      depth,
		Challenge:  challenge,
	}, nil
}

// verifyEligibility 校验一份资格材料。
//
// root 用提案绑定名单域的那一个；statement 的五个公开输入全部由服务端拼装，
// 客户端只能提供「证明 + 去重键 + 挑战」三样。
func (uc *RegisterUseCase) verifyEligibility(
	scope string,
	proposalID string,
	material *EligibilityMaterial,
) error {
	if material == nil || len(material.Proof) == 0 ||
		material.QualificationID == "" || material.Challenge == "" {
		return fmt.Errorf("%w: proof, qualification_id and challenge are required", ErrEligibilityRequired)
	}
	root, _, err := uc.eligibility.RosterFor(scope)
	if err != nil {
		return fmt.Errorf("load eligibility root: %w", err)
	}
	statement := domaincrypto.EligibilityStatement{
		Root:            root,
		ProposalID:      proposalID,
		Challenge:       material.Challenge,
		QualificationID: material.QualificationID,
	}
	if err := uc.eligibility.VerifyFor(scope, material.Proof, statement); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEligibilityProof, err)
	}
	return nil
}

// CredentialReservationTTL 是槽位预留的有效期。
//
// 预留是「占位」：领取流程要走三步（预留 → 盲签名 → 落库），这段时间里
// 槽位不能被别人拿走 —— 否则并发领取的人会抢到同一个槽位（服务端不预留
// 的话，所有人都会拿到「第一个空闲槽位」，最后只有一个人成功，其余全部
// 要重新盲化再来一遍；真实投票窗口里几百人同时领取，那会是一场灾难）。
//
// 但预留不能永久有效：只预留不领取的客户端（掉线、崩溃、恶意）不能把池子
// 永久占死。十分钟足够走完三步（正常是毫秒级），也足够覆盖「浏览器在
// Worker 里初始化 prover」这种慢路径。
const CredentialReservationTTL = 10 * time.Minute

// ReserveCredential 匿名预留一个凭证池槽位。
//
// 预留请求里**不含任何身份信息**：服务端随机给出一个未领取的槽位，运营方因此
// 不知道谁领了哪一个。这一步**不签名** —— 签名在客户端盲化之后才发生。
//
// 开启了资格校验时，material 必填：服务端先验资格证明、再记一次「资格已用过」
// （唯一索引），然后才给槽位。**资格不过不消耗槽位**。
func (uc *RegisterUseCase) ReserveCredential(
	ctx context.Context,
	proposalID string,
	material *EligibilityMaterial,
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
	proposal, err := uc.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	// 资格校验放在取槽位之前：不合格的人拿不到槽位，池子也不会被浪费。
	// 名单域按提案绑定解析（未绑定时取默认名单）。
	eligibilityEnabled := uc.eligibility != nil && uc.eligibility.Enabled()
	switch {
	case eligibilityEnabled:
		if err := uc.verifyEligibility(proposal.EligibilityScope, proposalID, material); err != nil {
			return nil, err
		}
	case material != nil:
		// 服务端没有开启资格校验，就没法验这份证明：不能让它「看起来被验过」。
		return nil, fmt.Errorf(
			"%w: the server does not verify eligibility proofs", ErrEligibilityNotEnabled,
		)
	}

	claim, err := uc.claimRepo.ReserveAvailable(ctx, proposalID, CredentialReservationTTL)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: no credential available", ErrCredentialPoolEmpty)
		}
		return nil, fmt.Errorf("reserve credential slot: %w", err)
	}

	// 资格使用记录放在预留之后：唯一约束冲突时（同一份资格第二次来领），
	// 槽位还没被标记领取 —— 把预留释放掉就不会被白白消耗。
	if eligibilityEnabled {
		// 少了去重仓储就等于「一人一票」没生效 —— 宁可报错，也不能悄悄放行。
		if uc.eligibilityRepo == nil {
			return nil, errors.New("eligibility use repository is not configured")
		}
		use := &entity.EligibilityUse{
			ID:              uuid.NewString(),
			ProposalID:      proposalID,
			QualificationID: material.QualificationID,
			UsedAt:          time.Now().UTC(),
		}
		if err := uc.eligibilityRepo.RecordUse(ctx, use); err != nil {
			_ = uc.claimRepo.ReleaseReservation(ctx, claim.ID)
			if errors.Is(err, repository.ErrDuplicateEligibilityUse) {
				return nil, fmt.Errorf("%w: this eligibility has already been used", ErrEligibilityAlreadyUsed)
			}
			return nil, fmt.Errorf("record eligibility use: %w", err)
		}
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
		// 公开端点只能为**有人正在领**的槽位盲签：否则签发方会变成「任意
		// 消息」的签名预言机。
		//
		// 槽位不在预留中只有两种可能：被别人领走了（并发撞车），或者预留过期
		// 了（客户端太慢，槽位可能已被别人重新预留）。两种都意味着「换一个
		// 槽位重来」—— 跟 attest 阶段的冲突是**同一个**信号：409
		// CREDENTIAL_EXISTS，客户端据此重试。
		if _, err := uc.claimRepo.GetReservedByUserID(
			ctx, proposalID, userID, CredentialReservationTTL,
		); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				// 槽位已经不在「可用」集合里了 —— 别人把它领走了。
				//
				// 预留**是占位**（占位会让「只预留不 attest」的客户端变成耗尽
				// 池子的 DoS，所以预留会过期），但并发领取时两个客户端仍可能
				// 拿到同一个 user_id。**这跟 attest 阶段的冲突是同一个信号**：
				// 「换一个槽位重来」—— 客户端（门户 / CLI / 压测工具）都认
				// 409 CREDENTIAL_EXISTS 并重试。
				//
				// 之前这里回的是 400 INVALID_REQUEST：客户端的判据是
				// 「409 + CREDENTIAL_EXISTS 才重试」，认不出 400，于是直接放弃
				// —— 并发领取的投票人会拿到一句看不懂的「请求不合法」。
				return nil, ErrCredentialAlreadyIssued
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
