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

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ThresholdCrypto interface {
	GenerateKeyPair() ([]byte, []byte, error)
	SplitPrivateKey(privateKey []byte, threshold, trustees int) ([][]byte, error)
	RecoverPrivateKey(shares [][]byte, threshold, trustees int) ([]byte, error)
	Encrypt(publicKey []byte, plaintext []byte) ([]byte, error)
	Decrypt(privateKey []byte, ciphertext []byte) ([]byte, error)
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
	if !isProposalClosed(proposal, time.Now().UTC()) {
		return nil, fmt.Errorf("%w: proposal must be closed before tally", ErrInvalidRequest)
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
	sharesJSON, err := json.Marshal(encodedShares)
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
		plain, err := uc.crypto.Decrypt(privateKey, vote.Cipher)
		if err != nil {
			return nil, fmt.Errorf("decrypt vote %s: %w", vote.ID, err)
		}
		candidate := strings.TrimSpace(string(plain))
		if candidate == "" {
			return nil, fmt.Errorf("%w: empty candidate in vote %s", ErrInvalidRequest, vote.ID)
		}
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

	session, err := uc.sessionRepo.GetByProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load decryption session: %w", err)
	}
	cipher, err := uc.crypto.Encrypt(session.PublicKey, []byte(candidate))
	if err != nil {
		return nil, fmt.Errorf("encrypt vote option: %w", err)
	}
	return cipher, nil
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
	for _, item := range provided {
		if _, ok := allowed[item]; !ok {
			return fmt.Errorf("share does not belong to session")
		}
	}
	return nil
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
