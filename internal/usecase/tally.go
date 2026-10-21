package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ThresholdCrypto interface {
	GenerateKeyPair() ([]byte, []byte, error)
	SplitPrivateKey(privateKey []byte, threshold, trustees int) ([][]byte, error)
	RecoverPrivateKey(shares [][]byte, threshold, trustees int) ([]byte, error)
	// Encrypt 用计票公钥加密选项下标。
	Encrypt(publicKey []byte, optionIndex int) ([]byte, error)
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
	nullifiers    domaincrypto.NullifierGenerator
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
	nullifiers domaincrypto.NullifierGenerator,
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
		nullifiers:    nullifiers,
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
	shares, err := uc.crypto.SplitPrivateKey(privateKey, threshold, trustees)
	if err != nil {
		return nil, fmt.Errorf("split private key: %w", err)
	}

	encodedShares := encodeBase64List(shares)
	sharesJSON, err := json.Marshal(hashShareList(encodedShares))
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

	registeredShares, err := parseSharesJSON(session.SharesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode session shares: %w", err)
	}
	if err := ensureSharesBelongToSession(registeredShares, providedShares); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	shareBytes, err := decodeBase64List(providedShares)
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

	tally := make(map[string]int)
	leaves := make([][]byte, 0, len(votes))
	for _, vote := range votes {
		index, err := uc.crypto.Decrypt(privateKey, vote.Cipher, len(proposal.Options))
		if err != nil {
			return nil, fmt.Errorf("decrypt vote %s: %w", vote.ID, err)
		}
		if index < 0 || index >= len(proposal.Options) {
			return nil, fmt.Errorf("%w: option index %d in vote %s is not defined for proposal", ErrInvalidRequest, index, vote.ID)
		}
		candidate := proposal.Options[index]
		tally[candidate]++
		leaves = append(leaves, buildVoteLeaf(vote))
	}
	if len(leaves) == 0 {
		leaves = append(leaves, []byte("empty-vote-set"))
	}

	merkleRoot, err := uc.merkleBuilder.BuildRoot(leaves)
	if err != nil {
		return nil, fmt.Errorf("build merkle root: %w", err)
	}
	tallyJSON, err := marshalCanonicalTally(tally)
	if err != nil {
		return nil, fmt.Errorf("marshal tally: %w", err)
	}

	hashInput := strings.Join([]string{
		proposalID,
		base64.StdEncoding.EncodeToString(merkleRoot),
		tallyJSON,
	}, "|")
	sum := sha256.Sum256([]byte(hashInput))
	anchorHash := hex.EncodeToString(sum[:])

	now := time.Now().UTC()
	result := &entity.TallyResult{
		ID:          uuid.NewString(),
		ProposalID:  proposalID,
		TotalVotes:  len(votes),
		TallyJSON:   tallyJSON,
		MerkleRoot:  append([]byte(nil), merkleRoot...),
		AnchorHash:  anchorHash,
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tally:       cloneTallyMap(tally),
	}
	if err := uc.resultRepo.Save(ctx, result); err != nil {
		return nil, fmt.Errorf("save tally result: %w", err)
	}

	anchor := &entity.AuditAnchor{
		ID:         uuid.NewString(),
		ProposalID: proposalID,
		MerkleRoot: append([]byte(nil), merkleRoot...),
		AnchorHash: anchorHash,
		CreatedAt:  now,
	}
	if err := uc.anchorRepo.Save(ctx, anchor); err != nil {
		return nil, fmt.Errorf("save audit anchor: %w", err)
	}

	uc.logger.Info("tally result published",
		zap.String("proposal_id", proposalID),
		zap.Int("total_votes", len(votes)),
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
		parsed, err := unmarshalTally(result.TallyJSON)
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

func (uc *TallyUseCase) EncryptVoteOption(
	ctx context.Context,
	proposalID string,
	option string,
) ([]byte, error) {
	if uc.sessionRepo == nil {
		return nil, errors.New("decryption session repository is not configured")
	}
	if uc.crypto == nil {
		return nil, errors.New("threshold crypto is not configured")
	}
	if proposalID == "" {
		return nil, fmt.Errorf("%w: proposal_id is required", ErrInvalidRequest)
	}
	candidate := strings.TrimSpace(option)
	if candidate == "" {
		return nil, fmt.Errorf("%w: option is required", ErrInvalidRequest)
	}
	if uc.proposalRepo == nil {
		return nil, errors.New("proposal repository is not configured")
	}
	proposal, err := uc.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if !proposalAllowsOption(proposal, candidate) {
		return nil, fmt.Errorf("%w: option %q is not defined for proposal", ErrInvalidRequest, candidate)
	}
	index := proposalOptionIndex(proposal, candidate)

	session, err := uc.sessionRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load decryption session: %w", err)
	}
	cipher, err := uc.crypto.Encrypt(session.PublicKey, index)
	if err != nil {
		return nil, fmt.Errorf("encrypt vote option: %w", err)
	}
	return cipher, nil
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

// BallotDraft 是服务端为开发联调准备的选票草稿。
//
// 这条路径只在 debug 模式下可达：服务端此时仍然知道选了哪个选项，
// 因此它不是匿名的，客户端 prover 落地后会被删除。
type BallotDraft struct {
	// Option 是被选中的选项文本。
	Option string
	// EncryptedVote 是 v3 选票密文。
	EncryptedVote []byte
	// Nullifier 是规范编码的 nullifier。
	Nullifier string
}

// PrepareBallotDraft 准备一份开发联调用的选票草稿。
func (uc *TallyUseCase) PrepareBallotDraft(
	ctx context.Context,
	proposalID string,
	option string,
) (*BallotDraft, error) {
	if uc.nullifiers == nil {
		return nil, errors.New("nullifier generator is not configured")
	}
	cipher, err := uc.EncryptVoteOption(ctx, proposalID, option)
	if err != nil {
		return nil, err
	}
	nullifier, err := uc.nullifiers.GenerateNullifier()
	if err != nil {
		return nil, fmt.Errorf("generate nullifier: %w", err)
	}
	return &BallotDraft{
		Option:        strings.TrimSpace(option),
		EncryptedVote: cipher,
		Nullifier:     nullifier,
	}, nil
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

func encodeBase64List(values [][]byte) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, base64.StdEncoding.EncodeToString(v))
	}
	return out
}

func decodeBase64List(values []string) ([][]byte, error) {
	out := make([][]byte, 0, len(values))
	for _, item := range values {
		data, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, nil
}

func parseSharesJSON(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("empty session shares")
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no shares in session")
	}
	return out, nil
}

func ensureSharesBelongToSession(registered []string, provided []string) error {
	allowed := make(map[string]struct{}, len(registered))
	for _, item := range registered {
		allowed[item] = struct{}{}
	}
	seen := make(map[string]struct{}, len(provided))
	for _, item := range provided {
		digest := hashShare(item)
		if _, ok := allowed[digest]; !ok {
			return fmt.Errorf("share does not belong to session")
		}
		// Repeating one share must not count towards the threshold.
		if _, ok := seen[digest]; ok {
			return fmt.Errorf("duplicate share submitted")
		}
		seen[digest] = struct{}{}
	}
	return nil
}

func hashShare(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hashShareList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, hashShare(value))
	}
	return out
}

func buildVoteLeaf(vote *entity.Vote) []byte {
	leaf := make([]byte, 0, len(vote.Nullifier)+1+len(vote.Cipher))
	leaf = append(leaf, []byte(vote.Nullifier)...)
	leaf = append(leaf, '|')
	leaf = append(leaf, vote.Cipher...)
	return leaf
}

type tallyEntry struct {
	Candidate string `json:"candidate"`
	Count     int    `json:"count"`
}

func marshalCanonicalTally(tally map[string]int) (string, error) {
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := make([]tallyEntry, 0, len(keys))
	for _, key := range keys {
		items = append(items, tallyEntry{
			Candidate: key,
			Count:     tally[key],
		})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalTally(raw string) (map[string]int, error) {
	if raw == "" {
		return map[string]int{}, nil
	}
	var items []tallyEntry
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(items))
	for _, item := range items {
		out[item.Candidate] = item.Count
	}
	return out, nil
}

func cloneTallyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func proposalAllowsOption(proposal *entity.Proposal, candidate string) bool {
	if proposal == nil {
		return false
	}
	normalized := strings.TrimSpace(candidate)
	if normalized == "" {
		return false
	}
	for _, option := range proposal.Options {
		if strings.TrimSpace(option) == normalized {
			return true
		}
	}
	return false
}

// proposalOptionIndex 返回选项在提案选项列表里的下标。
//
// 选票密文加密的是下标而不是选项文本：下标是电路里可做范围检查的小整数，
// 选项文本可能很长且不唯一。
func proposalOptionIndex(proposal *entity.Proposal, candidate string) int {
	if proposal == nil {
		return -1
	}
	normalized := strings.TrimSpace(candidate)
	for index, option := range proposal.Options {
		if strings.TrimSpace(option) == normalized {
			return index
		}
	}
	return -1
}
