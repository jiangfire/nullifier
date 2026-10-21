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
	if _, _, err := decodeNullifier(r.Nullifier); err != nil {
		return fmt.Errorf("%w: invalid nullifier: %v", ErrInvalidRequest, err)
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

// PublicInputs 拼装选票证明的公开输入。
//
// 公开输入全部来自服务端自己能拿到的材料：请求里的密文与 nullifier、
// 提案的选项数、计票会话的公钥。客户端不提交公开输入，因此一份 proof
// 只能用于它被生成的那份密文。
func (r *SubmitVoteRequest) PublicInputs(
	proposal *entity.Proposal,
	session *entity.DecryptionSession,
) (entity.BallotStatement, error) {
	if proposal == nil {
		return entity.BallotStatement{}, fmt.Errorf("%w: proposal is required", ErrInvalidRequest)
	}
	if session == nil {
		return entity.BallotStatement{}, fmt.Errorf("%w: decryption session is required", ErrInvalidRequest)
	}
	nullifier, _, err := decodeNullifier(r.Nullifier)
	if err != nil {
		return entity.BallotStatement{}, fmt.Errorf("%w: invalid nullifier: %v", ErrInvalidRequest, err)
	}
	return entity.BallotStatement{
		ProposalID:        r.ProposalID,
		Nullifier:         nullifier,
		ElectionPublicKey: append([]byte(nil), session.PublicKey...),
		Ciphertext:        append([]byte(nil), r.EncryptedVote...),
		NumOptions:        len(proposal.Options),
	}, nil
}

// decodeNullifier 解析客户端提交的 nullifier：十六进制编码的域元素。
//
// 除了字节，还返回**规范形式**（64 位小写 hex）。客户端可以用大写或带
// 空白的形式提交同一个 nullifier；如果原样落库，同一个 nullifier 就能
// 占掉多行，绕过 `votes.nullifier` 的唯一索引 —— 防重投就失效了。
//
// 长度与规范性由验证器再查一遍，这里只做格式检查，避免把明显非法的输入
// 送进密码学层。
func decodeNullifier(value string) ([]byte, string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, "", errors.New("nullifier is empty")
	}
	raw, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, "", fmt.Errorf("decode nullifier: %w", err)
	}
	if len(raw) != entity.NullifierByteSize {
		return nil, "", fmt.Errorf("nullifier must be %d bytes, got %d", entity.NullifierByteSize, len(raw))
	}
	return raw, hex.EncodeToString(raw), nil
}

type VotingUseCase struct {
	repo              repository.VoteRepository
	participationRepo repository.ParticipationRepository
	credentialRepo    repository.CredentialRepository
	proposalRepo      repository.ProposalRepository
	sessionRepo       repository.DecryptionSessionRepository
	verifier          domaincrypto.ProofVerifier
	encrypter         domaincrypto.Encrypter
	issuer            domaincrypto.CredentialIssuer
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
	issuer domaincrypto.CredentialIssuer,
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
		issuer:            issuer,
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
	session, err := uc.sessionRepo.GetByProposal(ctx, req.ProposalID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: decryption session is not ready", ErrInvalidRequest)
		}
		return fmt.Errorf("load decryption session: %w", err)
	}

	statement, err := req.PublicInputs(proposal, session)
	if err != nil {
		return err
	}
	if err := uc.verifier.Verify(req.Proof, statement); err != nil {
		// 证明校验失败是客户端输入问题，必须映射到 400 而不是 500。
		return fmt.Errorf("%w: invalid proof: %v", ErrInvalidRequest, err)
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
	// 凭证签名必须由签发方的公钥验签通过。只比对「库里存的签名」与
	// 「请求里带的签名」是否逐字节相同是不够的：任何人拿一张合法凭证
	// 换个 user_id 重放，都会通过那一比对。
	if uc.issuer == nil {
		return errors.New("credential issuer is not configured")
	}
	if err := uc.issuer.VerifyCredential(req.UserID, req.UserPubKey, req.CredentialSignature); err != nil {
		return fmt.Errorf("%w: credential signature verification failed", ErrInvalidCredential)
	}

	userHash := hashUserID(req.UserID)

	// 查重与落库都用**规范形式**。客户端可以用大写或带空白的形式提交同一个
	// nullifier：如果原样落库，同一个 nullifier 就能占掉多行，
	// `votes.nullifier` 的唯一索引拦不住 —— 防重投就形同虚设。
	_, canonicalNullifier, err := decodeNullifier(req.Nullifier)
	if err != nil {
		return fmt.Errorf("%w: invalid nullifier: %v", ErrInvalidRequest, err)
	}
	exists, err := uc.repo.ExistsByNullifier(ctx, canonicalNullifier)
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
		Nullifier:  canonicalNullifier,
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
