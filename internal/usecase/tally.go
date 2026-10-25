package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"
	"anonvote/internal/shares"
	"anonvote/internal/tally"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ThresholdCrypto interface {
	GenerateKeyPair() ([]byte, []byte, error)
	SplitPrivateKey(privateKey []byte, threshold, trustees int) ([][]byte, error)
	RecoverPrivateKey(shares [][]byte, threshold, trustees int) ([]byte, error)
	// Decrypt 用计票私钥还原选项下标。numOptions 是提案的选项数，
	// 解密方据此在选项集合里比对。
	Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error)
	// UncompressPublicKey 把计票公钥转成未压缩形式（X 与 Y 各 32 字节大端）。
	// 客户端用它本地加密选票 —— 公钥是公开值，客户端拿到它就能在不告诉
	// 服务端「选了哪个」的前提下完成加密。
	UncompressPublicKey(publicKey []byte) ([]byte, error)
}

type MerkleRootBuilder interface {
	BuildRoot(leaves [][]byte) ([]byte, error)
}

type ThresholdSessionSetup struct {
	ProposalID string   `json:"proposal_id"`
	Threshold  int      `json:"threshold"`
	Trustees   int      `json:"trustees"`
	PublicKey  string   `json:"public_key"`
	Shares     []string `json:"shares"`
}

type TallyUseCase struct {
	voteRepo      repository.VoteRepository
	proposalRepo  repository.ProposalRepository
	sessionRepo   repository.DecryptionSessionRepository
	resultRepo    repository.TallyResultRepository
	anchorRepo    repository.AuditAnchorRepository
	crypto        ThresholdCrypto
	merkleBuilder MerkleRootBuilder
	logger        *zap.Logger
}

func NewTallyUseCase(
	voteRepo repository.VoteRepository,
	proposalRepo repository.ProposalRepository,
	sessionRepo repository.DecryptionSessionRepository,
	resultRepo repository.TallyResultRepository,
	anchorRepo repository.AuditAnchorRepository,
	crypto ThresholdCrypto,
	merkleBuilder MerkleRootBuilder,
	logger *zap.Logger,
) *TallyUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TallyUseCase{
		voteRepo:      voteRepo,
		proposalRepo:  proposalRepo,
		sessionRepo:   sessionRepo,
		resultRepo:    resultRepo,
		anchorRepo:    anchorRepo,
		crypto:        crypto,
		merkleBuilder: merkleBuilder,
		logger:        logger,
	}
}

func (uc *TallyUseCase) SetupThresholdSession(
	ctx context.Context,
	proposalID string,
	threshold int,
	trustees int,
) (*ThresholdSessionSetup, error) {
	if err := uc.validateDeps(); err != nil {
		return nil, err
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if threshold <= 0 {
		return nil, fmt.Errorf("%w: threshold must be positive", ErrInvalidRequest)
	}
	if trustees < threshold {
		return nil, fmt.Errorf("%w: trustees must be >= threshold", ErrInvalidRequest)
	}

	proposal, err := uc.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if isProposalOpenForVoting(proposal, time.Now().UTC()) {
		return nil, fmt.Errorf(
			"%w: proposal decryption must be prepared before voting opens (proposal_id=%s status=%s now=%s starts_at=%s ends_at=%s)",
			ErrInvalidRequest,
			proposalID,
			strings.TrimSpace(proposal.Status),
			time.Now().UTC().Format(time.RFC3339),
			proposal.StartsAt.UTC().Format(time.RFC3339),
			proposal.EndsAt.UTC().Format(time.RFC3339),
		)
	}

	if _, err := uc.sessionRepo.GetByProposal(ctx, proposalID); err == nil {
		return nil, fmt.Errorf("%w: proposal %s", ErrSessionAlreadyExists, proposalID)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("check existing decryption session: %w", err)
	}

	privateKey, publicKey, err := uc.crypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate decryption key pair: %w", err)
	}
	splitShares, err := uc.crypto.SplitPrivateKey(privateKey, threshold, trustees)
	if err != nil {
		return nil, fmt.Errorf("split private key: %w", err)
	}

	encodedShares := shares.EncodeList(splitShares)
	sharesJSON, err := json.Marshal(shares.HashList(encodedShares))
	if err != nil {
		return nil, fmt.Errorf("marshal shares: %w", err)
	}

	now := time.Now().UTC()
	session := &entity.DecryptionSession{
		ID:         uuid.NewString(),
		ProposalID: proposalID,
		Threshold:  threshold,
		Trustees:   trustees,
		PublicKey:  append([]byte(nil), publicKey...),
		SharesJSON: string(sharesJSON),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := uc.sessionRepo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save decryption session: %w", err)
	}

	uc.logger.Info("threshold session prepared",
		zap.String("proposal_id", proposalID),
		zap.Int("threshold", threshold),
		zap.Int("trustees", trustees),
	)
	return &ThresholdSessionSetup{
		ProposalID: proposalID,
		Threshold:  threshold,
		Trustees:   trustees,
		PublicKey:  base64.StdEncoding.EncodeToString(publicKey),
		Shares:     encodedShares,
	}, nil
}

func (uc *TallyUseCase) PublishResult(
	ctx context.Context,
	proposalID string,
	providedShares []string,
) (*entity.TallyResult, error) {
	if err := uc.validateDeps(); err != nil {
		return nil, err
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	if len(providedShares) == 0 {
		return nil, fmt.Errorf("%w: shares are required", ErrInvalidRequest)
	}

	session, err := uc.sessionRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load decryption session: %w", err)
	}
	proposal, err := uc.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if !isProposalClosed(proposal, time.Now().UTC()) {
		return nil, fmt.Errorf("%w: proposal must be closed before tally", ErrInvalidRequest)
	}
	if _, err := uc.resultRepo.GetByProposal(ctx, proposalID); err == nil {
		return nil, fmt.Errorf("%w: proposal %s", ErrResultAlreadyPublished, proposalID)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("check existing tally result: %w", err)
	}
	if len(providedShares) < session.Threshold {
		return nil, fmt.Errorf("%w: insufficient shares", ErrInvalidRequest)
	}

	registeredShares, err := shares.ParseRegistered(session.SharesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode session shares: %w", err)
	}
	if err := shares.EnsureBelongToSession(registeredShares, providedShares); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	shareBytes, err := shares.DecodeList(providedShares)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid share format: %v", ErrInvalidRequest, err)
	}
	privateKey, err := uc.crypto.RecoverPrivateKey(shareBytes, session.Threshold, session.Trustees)
	if err != nil {
		return nil, fmt.Errorf("recover private key: %w", err)
	}

	votes, err := uc.voteRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load votes: %w", err)
	}

	// 计票算法只有一份：审计复算工具（cmd/audit-recompute）调的是同一个
	// tally.Compute —— 两边各写一份算法，漂移了复算结论就没有意义。
	ballots := make([]tally.Ballot, 0, len(votes))
	for _, vote := range votes {
		ballots = append(ballots, tally.Ballot{
			ID:        vote.ID,
			Nullifier: vote.Nullifier,
			Cipher:    vote.Cipher,
		})
	}
	computed, err := tally.Compute(tally.Input{
		ProposalID: proposalID,
		Options:    proposal.Options,
		Ballots:    ballots,
		PrivateKey: privateKey,
	}, uc.crypto, uc.merkleBuilder)
	if err != nil {
		var invalidOption *tally.InvalidOptionError
		if errors.As(err, &invalidOption) {
			return nil, fmt.Errorf(
				"%w: option index %d in vote %s is not defined for proposal",
				ErrInvalidRequest,
				invalidOption.Index,
				invalidOption.BallotID,
			)
		}
		return nil, err
	}

	now := time.Now().UTC()
	result := &entity.TallyResult{
		ID:          uuid.NewString(),
		ProposalID:  proposalID,
		TotalVotes:  computed.TotalVotes,
		TallyJSON:   computed.TallyJSON,
		MerkleRoot:  append([]byte(nil), computed.MerkleRoot...),
		AnchorHash:  computed.AnchorHash,
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tally:       cloneTallyMap(computed.Tally),
	}
	if err := uc.resultRepo.Save(ctx, result); err != nil {
		return nil, fmt.Errorf("save tally result: %w", err)
	}

	anchor := &entity.AuditAnchor{
		ID:         uuid.NewString(),
		ProposalID: proposalID,
		MerkleRoot: append([]byte(nil), computed.MerkleRoot...),
		AnchorHash: computed.AnchorHash,
		CreatedAt:  now,
	}
	if err := uc.anchorRepo.Save(ctx, anchor); err != nil {
		return nil, fmt.Errorf("save audit anchor: %w", err)
	}

	uc.logger.Info("tally result published",
		zap.String("proposal_id", proposalID),
		zap.Int("total_votes", computed.TotalVotes),
	)
	return result, nil
}

func (uc *TallyUseCase) GetResult(ctx context.Context, proposalID string) (*entity.TallyResult, error) {
	if uc.resultRepo == nil {
		return nil, errors.New("tally result repository is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}

	result, err := uc.resultRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("get tally result: %w", err)
	}
	if result.Tally == nil {
		parsed, err := tally.UnmarshalCanonicalTally(result.TallyJSON)
		if err != nil {
			return nil, fmt.Errorf("decode tally result: %w", err)
		}
		result.Tally = parsed
	}
	return result, nil
}

func (uc *TallyUseCase) ListAuditAnchors(ctx context.Context, proposalID string) ([]*entity.AuditAnchor, error) {
	if uc.anchorRepo == nil {
		return nil, errors.New("audit anchor repository is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	return uc.anchorRepo.ListByProposal(ctx, proposalID)
}

// ElectionPublicKey 返回提案的计票公钥（未压缩形式）。
//
// 公钥是公开值：客户端拿到它就能在本地把选项加密，服务端全程看不到明文
// 选项 —— 这是能力 A（选票内容对服务端不可知）的前提。
func (uc *TallyUseCase) ElectionPublicKey(ctx context.Context, proposalID string) ([]byte, error) {
	if uc.sessionRepo == nil {
		return nil, errors.New("decryption session repository is not configured")
	}
	if uc.crypto == nil {
		return nil, errors.New("threshold crypto is not configured")
	}
	if strings.TrimSpace(proposalID) == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	session, err := uc.sessionRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load decryption session: %w", err)
	}
	key, err := uc.crypto.UncompressPublicKey(session.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("uncompress election public key: %w", err)
	}
	return key, nil
}

func (uc *TallyUseCase) validateDeps() error {
	switch {
	case uc.voteRepo == nil:
		return errors.New("vote repository is not configured")
	case uc.proposalRepo == nil:
		return errors.New("proposal repository is not configured")
	case uc.sessionRepo == nil:
		return errors.New("decryption session repository is not configured")
	case uc.resultRepo == nil:
		return errors.New("tally result repository is not configured")
	case uc.anchorRepo == nil:
		return errors.New("audit anchor repository is not configured")
	case uc.crypto == nil:
		return errors.New("threshold crypto is not configured")
	case uc.merkleBuilder == nil:
		return errors.New("merkle root builder is not configured")
	default:
		return nil
	}
}

func isProposalClosed(proposal *entity.Proposal, now time.Time) bool {
	if proposal == nil {
		return false
	}
	if proposal.Status == "closed" || proposal.Status == "tallied" {
		return true
	}
	return !proposal.EndsAt.IsZero() && proposal.EndsAt.Before(now)
}

func cloneTallyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
