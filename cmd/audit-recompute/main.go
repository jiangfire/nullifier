// Command audit-recompute 对已发布的计票结果做**离线复算**。
//
// 用途：投票结束后，运营方（或第三方审计方）拿计票份额重新跑一遍计票，
// 与库里已发布的结果逐字节比对 —— 不一致就说明发布出去的结果被改过，
// 或者票库被动过手脚。
//
// 用法：
//
//	nullifier-audit-recompute -proposal p-1 -shares-file shares.json
//
// 关键性质：
//
//  1. **与服务端共用同一份计票算法**（internal/tally）。审计工具自己再写一份
//     算法，两边一旦漂移，复算结论就没有意义 —— 所以算法只有这一份。
//  2. **只读票库**。连接是 SQLite 的 mode=ro / PostgreSQL 的
//     default_transaction_read_only=on —— 不是「我们保证不写」，是「写不了」。
//  3. **不碰服务端**。它直接读库，不依赖投票服务在跑，也不需要 admin token。
//
// 退出码（可以拿来做门禁）：
//
//	0  复算结论与已发布结论逐字节一致
//	1  审计未通过：复算结论与已发布结论不一致
//	2  用法错误或运行失败（库打不开、份额不够、还原私钥失败……）
//
// ⚠️ **份额是秘密材料**：门限份数凑齐就能还原全部选票的明文选项。
// 份额文件用完请按秘密材料处理，不要进版本库、不要再发群里。
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/shares"
	"anonvote/internal/tally"

	"gorm.io/gorm"
)

const (
	// exitOK 表示复算结论与已发布结论一致。
	exitOK = 0
	// exitMismatch 表示审计未通过：复算结论与已发布结论不一致。
	exitMismatch = 1
	// exitRunFailure 表示用法错误或运行失败。
	exitRunFailure = 2
)

type options struct {
	proposalID string
	sharesRaw  string
	sharesFile string
	driver     string
	dbPath     string
	asJSON     bool
}

// report 是一次复算的完整结论。
type report struct {
	ProposalID string
	Published  tally.Published
	Recomputed tally.Result
	Mismatches []tally.Mismatch
	// Failure 是「复算根本跑不完」的原因（空 = 跑完了）。
	//
	// 库里的票解不开、选项下标越界 —— 这类是**审计未通过**，不是工具跑不动：
	// 用合法份额还原出的私钥解不开一张票，只可能是票本身被改过。
	Failure string
	// AnchorIssue 是「锚点表与结果表自相矛盾」的说明（空 = 一致）。
	//
	// 它不是复算与库内的比对，而是库内两张表之间的比对 —— 两张表是同一份
	// 结论的两个落点，它们自己对不上，结论本身就不可信。
	AnchorIssue string
}

// Consistent 表示审计是否通过。
func (r *report) Consistent() bool {
	return r.Failure == "" && len(r.Mismatches) == 0 && r.AnchorIssue == ""
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "用法错误：", err)
		printUsage()
		return exitRunFailure
	}

	result, err := recompute(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "审计复算失败：", err)
		return exitRunFailure
	}

	if err := printReport(result, opts.asJSON); err != nil {
		fmt.Fprintln(os.Stderr, "输出结论失败：", err)
		return exitRunFailure
	}
	if !result.Consistent() {
		return exitMismatch
	}
	return exitOK
}

func parseOptions(args []string) (options, error) {
	opts := options{}

	fs := flag.NewFlagSet("nullifier-audit-recompute", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "提案 ID")
	sharesRaw := fs.String("shares", "", "逗号分隔的计票份额")
	sharesFile := fs.String("shares-file", "", "份额文件（JSON 数组 / 会话对象 / 每行一个）")
	driver := fs.String("driver", "", "sqlite | postgres（默认读服务端配置）")
	dbPath := fs.String("db", "", "SQLite 库路径（覆盖配置里的 db.path）")
	asJSON := fs.Bool("json", false, "输出机器可读的 JSON 结论")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}

	opts.proposalID = strings.TrimSpace(*proposalID)
	opts.sharesRaw = *sharesRaw
	opts.sharesFile = *sharesFile
	opts.driver = strings.TrimSpace(*driver)
	opts.dbPath = strings.TrimSpace(*dbPath)
	opts.asJSON = *asJSON

	if opts.proposalID == "" {
		return opts, errors.New("必须提供 -proposal")
	}
	if opts.sharesRaw == "" && opts.sharesFile == "" {
		return opts, errors.New("必须提供 -shares 或 -shares-file")
	}
	return opts, nil
}

// recompute 跑一遍复算。
func recompute(opts options) (*report, error) {
	ctx := context.Background()

	dbCfg, err := resolveDBConfig(opts)
	if err != nil {
		return nil, err
	}
	db, err := persistence.NewDB(dbCfg)
	if err != nil {
		return nil, fmt.Errorf("打开数据库：%w", err)
	}
	defer closeDB(db)

	proposalRepo := persistence.NewProposalRepository(db)
	voteRepo := persistence.NewVoteRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	proposal, err := proposalRepo.GetByID(ctx, opts.proposalID)
	if err != nil {
		return nil, fmt.Errorf("读取提案 %s：%w", opts.proposalID, err)
	}
	session, err := sessionRepo.GetByProposal(ctx, opts.proposalID)
	if err != nil {
		return nil, fmt.Errorf("读取解密会话：%w", err)
	}
	published, err := resultRepo.GetByProposal(ctx, opts.proposalID)
	if err != nil {
		return nil, fmt.Errorf("读取已发布的计票结果（提案还没有发布过结果）：%w", err)
	}
	votes, err := voteRepo.GetByProposal(ctx, opts.proposalID)
	if err != nil {
		return nil, fmt.Errorf("读取选票：%w", err)
	}

	privateKey, err := recoverPrivateKey(session, opts)
	if err != nil {
		return nil, err
	}

	ballots := make([]tally.Ballot, 0, len(votes))
	for _, vote := range votes {
		ballots = append(ballots, tally.Ballot{
			ID:        vote.ID,
			Nullifier: vote.Nullifier,
			Cipher:    vote.Cipher,
		})
	}

	crypto := cryptoinfra.NewBallotThresholdCrypto()
	recomputed, computeErr := tally.Compute(tally.Input{
		ProposalID: opts.proposalID,
		Options:    proposal.Options,
		Ballots:    ballots,
		PrivateKey: privateKey,
	}, crypto, cryptoinfra.NewGnarkMerkleRootBuilder())
	if computeErr != nil {
		// 票解不开 / 选项越界 = 库里的票坏了 —— 审计未通过，不是工具跑不动。
		var invalidOption *tally.InvalidOptionError
		var decryptErr *tally.DecryptError
		if errors.As(computeErr, &invalidOption) || errors.As(computeErr, &decryptErr) {
			return &report{
				ProposalID: opts.proposalID,
				Failure:    computeErr.Error(),
			}, nil
		}
		return nil, fmt.Errorf("重算计票：%w", computeErr)
	}

	publishedView := tally.Published{
		TotalVotes: published.TotalVotes,
		TallyJSON:  published.TallyJSON,
		MerkleRoot: published.MerkleRoot,
		AnchorHash: published.AnchorHash,
	}

	result := &report{
		ProposalID: opts.proposalID,
		Published:  publishedView,
		Recomputed: *recomputed,
		Mismatches: tally.Compare(publishedView, *recomputed),
	}

	anchors, err := anchorRepo.ListByProposal(ctx, opts.proposalID)
	if err != nil {
		return nil, fmt.Errorf("读取审计锚点：%w", err)
	}
	result.AnchorIssue = anchorConflict(anchors, publishedView)

	return result, nil
}

// recoverPrivateKey 用提交的份额还原计票私钥。
//
// 校验口径与服务端**完全一致**（同一份 internal/shares 代码）：份额必须属于
// 本会话，且同一份份额不能重复计入门限份数 —— 否则审计走的就不是服务端
// 走的那条路了。
func recoverPrivateKey(session *entity.DecryptionSession, opts options) ([]byte, error) {
	provided, err := shares.Load(opts.sharesRaw, opts.sharesFile)
	if err != nil {
		return nil, err
	}
	if len(provided) < session.Threshold {
		return nil, fmt.Errorf("份额不足：门限是 %d，提供了 %d 份", session.Threshold, len(provided))
	}

	registered, err := shares.ParseRegistered(session.SharesJSON)
	if err != nil {
		return nil, fmt.Errorf("解析会话登记的份额：%w", err)
	}
	if err := shares.EnsureBelongToSession(registered, provided); err != nil {
		return nil, fmt.Errorf("份额校验失败：%w", err)
	}

	shareBytes, err := shares.DecodeList(provided)
	if err != nil {
		return nil, fmt.Errorf("份额格式错误：%w", err)
	}

	crypto := cryptoinfra.NewBallotThresholdCrypto()
	privateKey, err := crypto.RecoverPrivateKey(shareBytes, session.Threshold, session.Trustees)
	if err != nil {
		return nil, fmt.Errorf("还原计票私钥：%w", err)
	}
	return privateKey, nil
}

// anchorConflict 检查锚点表与结果表是否自相矛盾。空串表示一致。
func anchorConflict(anchors []*entity.AuditAnchor, published tally.Published) string {
	for _, anchor := range anchors {
		if !bytes.Equal(anchor.MerkleRoot, published.MerkleRoot) || anchor.AnchorHash != published.AnchorHash {
			return fmt.Sprintf(
				"锚点表（merkle=%s anchor=%s）与结果表（merkle=%s anchor=%s）不一致",
				hex.EncodeToString(anchor.MerkleRoot),
				anchor.AnchorHash,
				hex.EncodeToString(published.MerkleRoot),
				published.AnchorHash,
			)
		}
	}
	return ""
}

// resolveDBConfig 决定复算读哪个库。
//
// 默认读服务端同一份配置（CONFIG_FILE / configs/config.yaml / 环境变量）——
// 审计的对象必须是服务端用的那个库，不是某个随手指的路径。
func resolveDBConfig(opts options) (*persistence.DBConfig, error) {
	cfg, err := config.LoadForMigration()
	if err != nil {
		return nil, fmt.Errorf("读取配置：%w", err)
	}

	dbCfg := cfg.DB
	if dbCfg == nil {
		dbCfg = persistence.GetDBConfigFromEnv()
	}
	if opts.driver != "" {
		dbCfg.Driver = opts.driver
	}
	if opts.dbPath != "" {
		if !strings.EqualFold(dbCfg.Driver, "sqlite") {
			return nil, errors.New("-db 只能用于 sqlite；PostgreSQL 请用 DB_HOST / DB_NAME 等环境变量")
		}
		dbCfg.Database = opts.dbPath
	}

	// 复算工具只读票库：结论要能被信任，就不能有改数据的可能。
	dbCfg.ReadOnly = true
	return dbCfg, nil
}

func closeDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

// jsonReport 是 -json 模式的输出结构。
type jsonReport struct {
	ProposalID  string           `json:"proposal_id"`
	Consistent  bool             `json:"consistent"`
	TotalVotes  int              `json:"total_votes"`
	TallyJSON   string           `json:"tally_json"`
	MerkleRoot  string           `json:"merkle_root"`
	AnchorHash  string           `json:"anchor_hash"`
	Mismatches  []tally.Mismatch `json:"mismatches"`
	Failure     string           `json:"failure,omitempty"`
	AnchorIssue string           `json:"anchor_issue,omitempty"`
}

func printReport(result *report, asJSON bool) error {
	if asJSON {
		payload := jsonReport{
			ProposalID:  result.ProposalID,
			Consistent:  result.Consistent(),
			TotalVotes:  result.Recomputed.TotalVotes,
			TallyJSON:   result.Recomputed.TallyJSON,
			MerkleRoot:  hex.EncodeToString(result.Recomputed.MerkleRoot),
			AnchorHash:  result.Recomputed.AnchorHash,
			Mismatches:  result.Mismatches,
			Failure:     result.Failure,
			AnchorIssue: result.AnchorIssue,
		}
		if payload.Mismatches == nil {
			payload.Mismatches = []tally.Mismatch{}
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	}

	fmt.Printf("审计复算：提案 %s\n", result.ProposalID)
	if result.Failure != "" {
		fmt.Println("结论：审计未通过")
		fmt.Printf("  - 复算跑不完：%s\n", result.Failure)
		return nil
	}

	fmt.Printf("  票数       复算 %d，已发布 %d\n",
		result.Recomputed.TotalVotes, result.Published.TotalVotes)
	fmt.Printf("  计票       %s\n", result.Recomputed.TallyJSON)
	fmt.Printf("  merkle 根  %s\n", hex.EncodeToString(result.Recomputed.MerkleRoot))
	fmt.Printf("  锚点哈希   %s\n", result.Recomputed.AnchorHash)

	if result.Consistent() {
		fmt.Println("结论：审计通过 —— 复算结论与已发布结论逐字节一致")
		return nil
	}

	fmt.Println("结论：审计未通过")
	for _, mismatch := range result.Mismatches {
		fmt.Printf("  - %s：已发布 %s，复算 %s\n",
			mismatch.Field, mismatch.Published, mismatch.Recomputed)
	}
	if result.AnchorIssue != "" {
		fmt.Printf("  - %s\n", result.AnchorIssue)
	}
	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `用法:
  nullifier-audit-recompute -proposal <提案ID> (-shares <a,b> | -shares-file <文件>) [选项]

选项:
  -proposal     提案 ID（必填）
  -shares       逗号分隔的计票份额
  -shares-file  份额文件（JSON 数组 / 会话对象 / 每行一个）
  -driver       sqlite | postgres（默认读服务端配置）
  -db           SQLite 库路径（覆盖配置里的 db.path）
  -json         输出机器可读的 JSON 结论

退出码:
  0  复算结论与已发布结论一致
  1  审计未通过（结论不一致）
  2  用法错误或运行失败`)
}
