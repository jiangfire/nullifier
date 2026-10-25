package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// fixture 是一个已经计票完的提案：库里同时有票、有会话、有已发布的结果与锚点。
type fixture struct {
	dbPath string
	shares []string
}

// setupFixture 造一个真实的投票场景：真密码学加密的选票、真门限会话、
// 真发布流程产出的结果与锚点。
//
// 这里刻意**不 mock** 任何密码学 —— 审计工具的意义就是「用真算法复算」，
// mock 过的复算证明不了任何事。
func setupFixture(t *testing.T) fixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "audit.db")
	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: dbPath})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestDB(t, db) })
	require.NoError(t, persistence.AutoMigrate(db))

	ctx := context.Background()
	proposalRepo := persistence.NewProposalRepository(db)
	voteRepo := persistence.NewVoteRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	crypto := cryptoinfra.NewBallotThresholdCrypto()
	uc := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		crypto, cryptoinfra.NewGnarkMerkleRootBuilder(), zap.NewNop(),
	)

	require.NoError(t, proposalRepo.Save(ctx, &entity.Proposal{
		ID:        "p1",
		Title:     "复算用提案",
		Options:   []string{"甲", "乙"},
		Status:    "closed",
		StartsAt:  time.Now().UTC().Add(-2 * time.Hour),
		EndsAt:    time.Now().UTC().Add(-time.Hour),
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}))

	setup, err := uc.SetupThresholdSession(ctx, "p1", 2, 3)
	require.NoError(t, err)

	// setup.PublicKey 是压缩形式的计票公钥（服务端拼公开输入时也用它）。
	publicKey, err := base64.StdEncoding.DecodeString(setup.PublicKey)
	require.NoError(t, err)

	for i, option := range []int{0, 1, 0, 1, 0} {
		cipher, err := crypto.Encrypt(publicKey, option)
		require.NoError(t, err)
		require.NoError(t, voteRepo.Save(ctx, &entity.Vote{
			ID:         fmt.Sprintf("v%d", i+1),
			ProposalID: "p1",
			Nullifier:  fmt.Sprintf("%064x", i+1),
			Cipher:     cipher,
			CreatedAt:  time.Now().UTC(),
		}))
	}

	_, err = uc.PublishResult(ctx, "p1", setup.Shares[:2])
	require.NoError(t, err)

	return fixture{dbPath: dbPath, shares: setup.Shares}
}

func (f fixture) options(t *testing.T) options {
	t.Helper()
	return options{
		proposalID: "p1",
		sharesRaw:  strings.Join(f.shares[:2], ","),
		dbPath:     f.dbPath,
	}
}

func TestRecompute_MatchesPublishedResult(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	result, err := recompute(fx.options(t))
	require.NoError(t, err)
	require.True(t, result.Consistent(), "复算结论应与已发布结论一致：%+v", result.Mismatches)
	require.Empty(t, result.Mismatches)
	require.Equal(t, 5, result.Recomputed.TotalVotes)
	require.Equal(t, 5, result.Published.TotalVotes)
	require.Equal(t, `[{"candidate":"乙","count":2},{"candidate":"甲","count":3}]`, result.Recomputed.TallyJSON)
	require.Equal(t, result.Recomputed.AnchorHash, result.Published.AnchorHash)
}

func TestRecompute_DetectsTamperedTallyResult(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	// 偷偷改掉已发布的计票结果 —— 审计必须发现。
	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: fx.dbPath})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestDB(t, db) })
	require.NoError(t, db.Exec(
		"UPDATE tally_results SET tally_json = ? WHERE proposal_id = ?",
		`[{"candidate":"甲","count":5}]`, "p1",
	).Error)

	result, err := recompute(fx.options(t))
	require.NoError(t, err)
	require.False(t, result.Consistent())
	require.NotEmpty(t, result.Mismatches)

	fields := make([]string, 0, len(result.Mismatches))
	for _, mismatch := range result.Mismatches {
		fields = append(fields, mismatch.Field)
	}
	require.Contains(t, fields, "tally_json")
}

func TestRecompute_DetectsTamperedVote(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	// 改掉一张票的密文 —— merkle 根与锚点哈希都要跟着对不上。
	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: fx.dbPath})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestDB(t, db) })
	require.NoError(t, db.Exec(
		"UPDATE votes SET cipher = ? WHERE id = ?",
		[]byte{0x03, 0x04, 0x05}, "v1",
	).Error)

	result, err := recompute(fx.options(t))
	require.NoError(t, err)
	require.False(t, result.Consistent())
	require.NotEmpty(t, result.Failure, "票被改坏属于审计未通过，不是工具跑不动")
}

func TestRecompute_RejectsShareFromAnotherSession(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	opts := fx.options(t)
	// 份数够门限，但其中一份不属于本会话 —— 必须被拒。
	opts.sharesRaw = fx.shares[0] + ",not-a-real-share"

	_, err := recompute(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "份额校验失败")
}

func TestRecompute_RejectsInsufficientShares(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	opts := fx.options(t)
	opts.sharesRaw = fx.shares[0]

	_, err := recompute(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "份额不足")
}

func TestRecompute_RejectsUnknownProposal(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	opts := fx.options(t)
	opts.proposalID = "missing"

	_, err := recompute(opts)
	require.Error(t, err)
}

func TestRun_ExitCodes(t *testing.T) {
	t.Parallel()

	fx := setupFixture(t)

	// 一致 → 0
	require.Equal(t, exitOK, run([]string{
		"-proposal", "p1",
		"-shares", strings.Join(fx.shares[:2], ","),
		"-db", fx.dbPath,
	}))

	// 不一致 → 1
	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: fx.dbPath})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestDB(t, db) })
	require.NoError(t, db.Exec(
		"UPDATE tally_results SET anchor_hash = ? WHERE proposal_id = ?",
		"deadbeef", "p1",
	).Error)
	require.Equal(t, exitMismatch, run([]string{
		"-proposal", "p1",
		"-shares", strings.Join(fx.shares[:2], ","),
		"-db", fx.dbPath,
	}))

	// 用法错误 → 2
	require.Equal(t, exitRunFailure, run([]string{"-shares", "a,b"}))
}

func TestParseOptions_RequiresProposalAndShares(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{"-shares", "a,b"})
	require.Error(t, err)

	_, err = parseOptions([]string{"-proposal", "p1"})
	require.Error(t, err)

	opts, err := parseOptions([]string{"-proposal", " p1 ", "-shares", "a,b", "-json"})
	require.NoError(t, err)
	require.Equal(t, "p1", opts.proposalID)
	require.True(t, opts.asJSON)
}
