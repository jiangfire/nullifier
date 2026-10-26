package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"
	"anonvote/internal/shares"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTallyUseCase_SetupAndPublishResult(t *testing.T) {
	voteRepo := new(inMemoryVoteRepo)
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{}
	sessionRepo := new(inMemoryDecryptionSessionRepo)
	resultRepo := new(inMemoryTallyResultRepo)
	anchorRepo := new(inMemoryAuditAnchorRepo)

	cryptoSvc := new(fakeThresholdCrypto)
	merkleSvc := new(fakeMerkleBuilder)

	uc := NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		cryptoSvc,
		merkleSvc,
		zap.NewNop(),
	)

	now := time.Now().UTC()
	proposal := &entity.Proposal{
		ID:        "proposal-1",
		Title:     "Election",
		Options:   []string{"candidate-A", "candidate-B"},
		Status:    "closed",
		StartsAt:  now.Add(-2 * time.Hour),
		EndsAt:    now.Add(-1 * time.Hour),
		CreatedAt: now.Add(-3 * time.Hour),
	}
	proposalRepo.items[proposal.ID] = proposal

	setup, err := uc.SetupThresholdSession(context.Background(), proposal.ID, 2, 3)
	require.NoError(t, err)
	require.Len(t, setup.Shares, 3)
	require.NotEmpty(t, setup.PublicKey)

	storedSession, err := sessionRepo.GetByProposal(context.Background(), proposal.ID)
	require.NoError(t, err)
	require.Len(t, parseSharesForTest(t, storedSession.SharesJSON), 3)

	pubKeyBytes, err := base64.StdEncoding.DecodeString(setup.PublicKey)
	require.NoError(t, err)

	// 选票密文加密的是选项下标：candidate-A=0，candidate-B=1。
	cipherA, err := cryptoSvc.Encrypt(pubKeyBytes, 0)
	require.NoError(t, err)
	cipherB, err := cryptoSvc.Encrypt(pubKeyBytes, 1)
	require.NoError(t, err)

	voteRepo.items = append(voteRepo.items,
		&entity.Vote{ID: "v1", ProposalID: proposal.ID, Nullifier: "n1", Cipher: cipherA, CreatedAt: time.Now().UTC()},
		&entity.Vote{ID: "v2", ProposalID: proposal.ID, Nullifier: "n2", Cipher: cipherB, CreatedAt: time.Now().UTC()},
		&entity.Vote{ID: "v3", ProposalID: proposal.ID, Nullifier: "n3", Cipher: cipherA, CreatedAt: time.Now().UTC()},
	)

	result, err := uc.PublishResult(context.Background(), proposal.ID, setup.Shares[:2])
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, result.TotalVotes)
	assert.Equal(t, 2, result.Tally["candidate-A"])
	assert.Equal(t, 1, result.Tally["candidate-B"])
	assert.NotEmpty(t, result.MerkleRoot)
	assert.NotEmpty(t, result.AnchorHash)
	require.Len(t, anchorRepo.items[proposal.ID], 1)

	stored, err := uc.GetResult(context.Background(), proposal.ID)
	require.NoError(t, err)
	assert.Equal(t, result.ProposalID, stored.ProposalID)
	assert.Equal(t, result.TotalVotes, stored.TotalVotes)
}

func TestTallyUseCase_RejectsRepeatedSetupAndPublish(t *testing.T) {
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{}
	sessionRepo := new(inMemoryDecryptionSessionRepo)
	resultRepo := new(inMemoryTallyResultRepo)
	anchorRepo := new(inMemoryAuditAnchorRepo)

	proposal := &entity.Proposal{
		ID:        "proposal-1",
		Options:   []string{"candidate-A", "candidate-B"},
		Status:    "closed",
		StartsAt:  time.Now().UTC().Add(-2 * time.Hour),
		EndsAt:    time.Now().UTC().Add(-1 * time.Hour),
		CreatedAt: time.Now().UTC().Add(-3 * time.Hour),
	}
	proposalRepo.items[proposal.ID] = proposal

	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	ctx := context.Background()

	setup, err := uc.SetupThresholdSession(ctx, proposal.ID, 2, 3)
	require.NoError(t, err)

	_, err = uc.SetupThresholdSession(ctx, proposal.ID, 2, 3)
	require.ErrorIs(t, err, ErrSessionAlreadyExists)
	stored, err := sessionRepo.GetByProposal(ctx, proposal.ID)
	require.NoError(t, err)
	require.Len(t, parseSharesForTest(t, stored.SharesJSON), 3)

	_, err = uc.PublishResult(ctx, proposal.ID, setup.Shares[:2])
	require.NoError(t, err)
	require.Len(t, anchorRepo.items[proposal.ID], 1)

	_, err = uc.PublishResult(ctx, proposal.ID, setup.Shares[:2])
	require.ErrorIs(t, err, ErrResultAlreadyPublished)
	require.Len(t, anchorRepo.items[proposal.ID], 1)
}

func TestTallyUseCase_PublishResult_RejectsInsufficientShares(t *testing.T) {
	voteRepo := new(inMemoryVoteRepo)
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{}
	sessionRepo := new(inMemoryDecryptionSessionRepo)
	resultRepo := new(inMemoryTallyResultRepo)
	anchorRepo := new(inMemoryAuditAnchorRepo)
	cryptoSvc := new(fakeThresholdCrypto)
	merkleSvc := new(fakeMerkleBuilder)

	uc := NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		cryptoSvc,
		merkleSvc,
		zap.NewNop(),
	)

	proposal := &entity.Proposal{
		ID:        "proposal-1",
		Options:   []string{"candidate-A", "candidate-B"},
		Status:    "closed",
		StartsAt:  time.Now().UTC().Add(-2 * time.Hour),
		EndsAt:    time.Now().UTC().Add(-1 * time.Hour),
		CreatedAt: time.Now().UTC().Add(-3 * time.Hour),
	}
	proposalRepo.items[proposal.ID] = proposal
	setup, err := uc.SetupThresholdSession(context.Background(), proposal.ID, 2, 3)
	require.NoError(t, err)

	_, err = uc.PublishResult(context.Background(), "proposal-1", setup.Shares[:1])
	require.Error(t, err)
}

func TestTallyUseCase_SetupThresholdSession_RejectsOpenProposal(t *testing.T) {
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{}
	proposalRepo.items["proposal-1"] = &entity.Proposal{
		ID:        "proposal-1",
		Options:   []string{"candidate-A", "candidate-B"},
		Status:    "open",
		StartsAt:  time.Now().UTC().Add(-1 * time.Hour),
		EndsAt:    time.Now().UTC().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}

	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		proposalRepo,
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)

	_, err := uc.SetupThresholdSession(context.Background(), "proposal-1", 2, 3)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_PublishResult_RejectOpenProposal(t *testing.T) {
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{
		"proposal-1": {
			ID:        "proposal-1",
			Options:   []string{"candidate-A", "candidate-B"},
			Status:    "open",
			StartsAt:  time.Now().UTC().Add(-1 * time.Hour),
			EndsAt:    time.Now().UTC().Add(1 * time.Hour),
			CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
	}
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		proposalRepo,
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	require.NoError(t, uc.sessionRepo.Save(context.Background(), &entity.DecryptionSession{
		ID:         "s1",
		ProposalID: "proposal-1",
		Threshold:  2,
		Trustees:   3,
		PublicKey:  []byte("public-key"),
		SharesJSON: `["` + shares.Hash("share-1") + `","` + shares.Hash("share-2") + `","` + shares.Hash("share-3") + `"]`,
	}))

	_, err := uc.PublishResult(context.Background(), "proposal-1", []string{"share-1", "share-2"})
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_ListAuditAnchors(t *testing.T) {
	anchorRepo := new(inMemoryAuditAnchorRepo)
	anchorRepo.items = map[string][]*entity.AuditAnchor{
		"proposal-1": {
			{
				ID:         "a1",
				ProposalID: "proposal-1",
				MerkleRoot: []byte("root"),
				AnchorHash: "hash",
				CreatedAt:  time.Now().UTC(),
			},
		},
	}
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		anchorRepo,
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)

	anchors, err := uc.ListAuditAnchors(context.Background(), "proposal-1")
	require.NoError(t, err)
	require.Len(t, anchors, 1)
	assert.Equal(t, "a1", anchors[0].ID)
}

func TestTallyUseCase_GetResult_DecodeFromJSON(t *testing.T) {
	resultRepo := new(inMemoryTallyResultRepo)
	resultRepo.items = map[string]*entity.TallyResult{
		"proposal-1": {
			ID:         "r1",
			ProposalID: "proposal-1",
			TallyJSON:  `[{"candidate":"candidate-A","count":2}]`,
		},
	}

	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		resultRepo,
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)

	result, err := uc.GetResult(context.Background(), "proposal-1")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Tally["candidate-A"])
}

func TestTallyUseCase_SetupThresholdSession_InvalidRequest(t *testing.T) {
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)

	_, err := uc.SetupThresholdSession(context.Background(), "", 2, 3)
	require.ErrorIs(t, err, ErrInvalidRequest)

	_, err = uc.SetupThresholdSession(context.Background(), "p1", 2, 1)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_PublishResult_InvalidShareFormat(t *testing.T) {
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{
		"proposal-1": {
			ID:        "proposal-1",
			Options:   []string{"candidate-A", "candidate-B"},
			Status:    "closed",
			StartsAt:  time.Now().UTC().Add(-2 * time.Hour),
			EndsAt:    time.Now().UTC().Add(-1 * time.Hour),
			CreatedAt: time.Now().UTC().Add(-3 * time.Hour),
		},
	}
	sessionRepo := new(inMemoryDecryptionSessionRepo)
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		proposalRepo,
		sessionRepo,
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	setup, err := uc.SetupThresholdSession(context.Background(), "proposal-1", 2, 3)
	require.NoError(t, err)

	_, err = uc.PublishResult(context.Background(), "proposal-1", []string{"not-base64", setup.Shares[1]})
	require.ErrorIs(t, err, ErrInvalidRequest)

	// One share repeated twice must be rejected instead of surfacing as a 500.
	_, err = uc.PublishResult(context.Background(), "proposal-1", []string{setup.Shares[0], setup.Shares[0]})
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_validateDeps(t *testing.T) {
	cases := []struct {
		name string
		uc   *TallyUseCase
	}{
		{name: "missing_vote_repo", uc: &TallyUseCase{}},
		{name: "missing_proposal_repo", uc: &TallyUseCase{voteRepo: new(inMemoryVoteRepo)}},
		{name: "missing_session_repo", uc: &TallyUseCase{
			voteRepo:     new(inMemoryVoteRepo),
			proposalRepo: new(inMemoryProposalRepo),
		}},
		{name: "missing_result_repo", uc: &TallyUseCase{
			voteRepo:     new(inMemoryVoteRepo),
			proposalRepo: new(inMemoryProposalRepo),
			sessionRepo:  new(inMemoryDecryptionSessionRepo),
		}},
		{name: "missing_anchor_repo", uc: &TallyUseCase{
			voteRepo:     new(inMemoryVoteRepo),
			proposalRepo: new(inMemoryProposalRepo),
			sessionRepo:  new(inMemoryDecryptionSessionRepo),
			resultRepo:   new(inMemoryTallyResultRepo),
		}},
		{name: "missing_crypto", uc: &TallyUseCase{
			voteRepo:     new(inMemoryVoteRepo),
			proposalRepo: new(inMemoryProposalRepo),
			sessionRepo:  new(inMemoryDecryptionSessionRepo),
			resultRepo:   new(inMemoryTallyResultRepo),
			anchorRepo:   new(inMemoryAuditAnchorRepo),
		}},
		{name: "missing_merkle", uc: &TallyUseCase{
			voteRepo:     new(inMemoryVoteRepo),
			proposalRepo: new(inMemoryProposalRepo),
			sessionRepo:  new(inMemoryDecryptionSessionRepo),
			resultRepo:   new(inMemoryTallyResultRepo),
			anchorRepo:   new(inMemoryAuditAnchorRepo),
			crypto:       new(fakeThresholdCrypto),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.uc.validateDeps()
			require.Error(t, err)
		})
	}
}

func TestTallyUseCase_ListAuditAnchors_Invalid(t *testing.T) {
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		nil,
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	_, err := uc.ListAuditAnchors(context.Background(), "proposal-1")
	require.Error(t, err)

	uc = NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	_, err = uc.ListAuditAnchors(context.Background(), "")
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_GetResult_Invalid(t *testing.T) {
	uc := NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		nil,
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	_, err := uc.GetResult(context.Background(), "proposal-1")
	require.Error(t, err)

	uc = NewTallyUseCase(
		new(inMemoryVoteRepo),
		new(inMemoryProposalRepo),
		new(inMemoryDecryptionSessionRepo),
		new(inMemoryTallyResultRepo),
		new(inMemoryAuditAnchorRepo),
		new(fakeThresholdCrypto),
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)
	_, err = uc.GetResult(context.Background(), "")
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestTallyUseCase_PublishResult_RejectsOutOfRangeOptionIndex(t *testing.T) {
	voteRepo := new(inMemoryVoteRepo)
	proposalRepo := new(inMemoryProposalRepo)
	proposalRepo.items = map[string]*entity.Proposal{}
	sessionRepo := new(inMemoryDecryptionSessionRepo)
	resultRepo := new(inMemoryTallyResultRepo)
	anchorRepo := new(inMemoryAuditAnchorRepo)
	cryptoSvc := new(fakeThresholdCrypto)

	uc := NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		cryptoSvc,
		new(fakeMerkleBuilder),
		zap.NewNop(),
	)

	proposal := &entity.Proposal{
		ID:        "proposal-1",
		Options:   []string{"candidate-A", "candidate-B"},
		Status:    "closed",
		StartsAt:  time.Now().UTC().Add(-2 * time.Hour),
		EndsAt:    time.Now().UTC().Add(-1 * time.Hour),
		CreatedAt: time.Now().UTC().Add(-3 * time.Hour),
	}
	proposalRepo.items[proposal.ID] = proposal

	setup, err := uc.SetupThresholdSession(context.Background(), proposal.ID, 2, 3)
	require.NoError(t, err)

	// 密文里的下标超出提案选项数，必须以 ErrInvalidRequest 拒绝，不能当成密码学故障。
	outOfRange, err := cryptoSvc.Encrypt([]byte("public-key"), 7)
	require.NoError(t, err)
	voteRepo.items = append(voteRepo.items,
		&entity.Vote{
			ID:         "v1",
			ProposalID: proposal.ID,
			Nullifier:  "n1",
			Cipher:     outOfRange,
			CreatedAt:  time.Now().UTC(),
		},
	)

	_, err = uc.PublishResult(context.Background(), proposal.ID, setup.Shares[:2])
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Empty(t, resultRepo.items)
	require.Empty(t, anchorRepo.items[proposal.ID])
}

// ---- in-memory fakes ----

type inMemoryVoteRepo struct {
	items []*entity.Vote
}

func (r *inMemoryVoteRepo) Save(ctx context.Context, vote *entity.Vote) error { return nil }
func (r *inMemoryVoteRepo) ExistsByNullifier(ctx context.Context, nullifier string) (bool, error) {
	return false, nil
}
func (r *inMemoryVoteRepo) GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error) {
	out := make([]*entity.Vote, 0)
	for _, item := range r.items {
		if item.ProposalID == proposalID {
			out = append(out, item)
		}
	}
	return out, nil
}

type inMemoryProposalRepo struct {
	items map[string]*entity.Proposal
}

func (r *inMemoryProposalRepo) Save(ctx context.Context, proposal *entity.Proposal) error { return nil }
func (r *inMemoryProposalRepo) GetByID(ctx context.Context, id string) (*entity.Proposal, error) {
	if r.items == nil {
		r.items = map[string]*entity.Proposal{}
	}
	item, ok := r.items[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return item, nil
}
func (r *inMemoryProposalRepo) ListOpen(ctx context.Context) ([]*entity.Proposal, error) {
	return nil, nil
}

func (r *inMemoryProposalRepo) ListAll(ctx context.Context) ([]*entity.Proposal, error) {
	return nil, nil
}

type inMemoryDecryptionSessionRepo struct {
	items map[string]*entity.DecryptionSession
}

func (r *inMemoryDecryptionSessionRepo) Save(ctx context.Context, session *entity.DecryptionSession) error {
	if r.items == nil {
		r.items = map[string]*entity.DecryptionSession{}
	}
	cloned := *session
	r.items[session.ProposalID] = &cloned
	return nil
}

func (r *inMemoryDecryptionSessionRepo) GetByProposal(
	ctx context.Context,
	proposalID string,
) (*entity.DecryptionSession, error) {
	if r.items == nil {
		r.items = map[string]*entity.DecryptionSession{}
	}
	item, ok := r.items[proposalID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

type inMemoryTallyResultRepo struct {
	items map[string]*entity.TallyResult
}

func (r *inMemoryTallyResultRepo) Save(ctx context.Context, result *entity.TallyResult) error {
	if r.items == nil {
		r.items = map[string]*entity.TallyResult{}
	}
	cloned := *result
	cloned.Tally = cloneTally(result.Tally)
	r.items[result.ProposalID] = &cloned
	return nil
}

func (r *inMemoryTallyResultRepo) GetByProposal(ctx context.Context, proposalID string) (*entity.TallyResult, error) {
	if r.items == nil {
		r.items = map[string]*entity.TallyResult{}
	}
	item, ok := r.items[proposalID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cloned := *item
	if item.Tally != nil {
		cloned.Tally = cloneTally(item.Tally)
	}
	return &cloned, nil
}

type inMemoryAuditAnchorRepo struct {
	items map[string][]*entity.AuditAnchor
}

func (r *inMemoryAuditAnchorRepo) Save(ctx context.Context, anchor *entity.AuditAnchor) error {
	if r.items == nil {
		r.items = map[string][]*entity.AuditAnchor{}
	}
	cloned := *anchor
	r.items[anchor.ProposalID] = append(r.items[anchor.ProposalID], &cloned)
	return nil
}

func (r *inMemoryAuditAnchorRepo) ListByProposal(ctx context.Context, proposalID string) ([]*entity.AuditAnchor, error) {
	if r.items == nil {
		r.items = map[string][]*entity.AuditAnchor{}
	}
	anchors := r.items[proposalID]
	out := make([]*entity.AuditAnchor, 0, len(anchors))
	for _, item := range anchors {
		cloned := *item
		out = append(out, &cloned)
	}
	return out, nil
}

type fakeThresholdCrypto struct {
	recoveredPrivate []byte
}

func (f *fakeThresholdCrypto) GenerateKeyPair() ([]byte, []byte, error) {
	return []byte("private-key"), []byte("public-key"), nil
}

func (f *fakeThresholdCrypto) SplitPrivateKey(
	privateKey []byte,
	threshold int,
	trustees int,
) ([][]byte, error) {
	if threshold < 1 || trustees < threshold {
		return nil, errors.New("invalid threshold parameters")
	}
	out := make([][]byte, 0, trustees)
	for i := 0; i < trustees; i++ {
		out = append(out, []byte(fmt.Sprintf("share-%d", i+1)))
	}
	return out, nil
}

func (f *fakeThresholdCrypto) RecoverPrivateKey(
	shares [][]byte,
	threshold int,
	trustees int,
) ([]byte, error) {
	if len(shares) < threshold {
		return nil, errors.New("insufficient shares")
	}
	f.recoveredPrivate = []byte("private-key")
	return f.recoveredPrivate, nil
}

// Encrypt 加密的是选项下标，不是选项文本。
func (f *fakeThresholdCrypto) Encrypt(publicKey []byte, optionIndex int) ([]byte, error) {
	if len(publicKey) == 0 {
		return nil, errors.New("empty public key")
	}
	if optionIndex < 0 {
		return nil, errors.New("negative option index")
	}
	return []byte("enc:" + strconv.Itoa(optionIndex)), nil
}

// Decrypt 还原选项下标。
//
// 越界判定留给 usecase：解密出超出提案选项数的下标时，usecase 要以
// ErrInvalidRequest 报错，而不是把错误藏进密码学层。
func (f *fakeThresholdCrypto) Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error) {
	if len(privateKey) == 0 {
		return 0, errors.New("empty private key")
	}
	raw, ok := strings.CutPrefix(string(ciphertext), "enc:")
	if !ok {
		return 0, errors.New("invalid ciphertext")
	}
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid ciphertext")
	}
	return index, nil
}

// UncompressPublicKey 把计票公钥转成未压缩形式。
func (f *fakeThresholdCrypto) UncompressPublicKey(publicKey []byte) ([]byte, error) {
	if len(publicKey) == 0 {
		return nil, errors.New("empty public key")
	}
	return append([]byte("uncompressed:"), publicKey...), nil
}

type fakeMerkleBuilder struct{}

func (f *fakeMerkleBuilder) BuildRoot(leaves [][]byte) ([]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("empty leaves")
	}
	return []byte(fmt.Sprintf("root-%d", len(leaves))), nil
}

func cloneTally(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseSharesForTest(t *testing.T, raw string) []string {
	t.Helper()

	values, err := shares.ParseRegistered(raw)
	require.NoError(t, err)
	return values
}
