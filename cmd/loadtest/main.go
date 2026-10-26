// Command loadtest 对运行中的 nullifier 服务做一次并发投票压测，并在压测过程中
// 复核「并发下的防重投」。
//
// 路线图 §4.3 的第三个盲区是「并发投票下的防重投与性能容量」：
// 防重投那一半由 tests/e2e 的回归测试覆盖，这个工具补的是**容量**那一半 ——
// 一份可复现的脚本 + 一份可读的报告。
//
// 用法：
//
//	nullifier-loadtest -admin-token dev-admin-token -votes 200 -concurrency 20
//
// 工具只走 HTTP API，不碰数据库：
//  1. 建一个窗口即将打开的提案（解密会话必须在窗口打开**之前**初始化）
//  2. 初始化门限解密会话、往凭证池投槽位、匿名领取凭证
//  3. 等窗口打开后并发提交选票（每个请求一个不同的凭证与 nullifier）
//  4. 再用**同一个 nullifier** 并发提交若干次，验证只有一张票能入库
//  5. 输出吞吐、延迟分位、状态分布与防重投结论
//
// 领取凭证走的是**真两段式盲签名**（claim 预留 → 本地盲化 + 服务端盲签名 →
// 交回解盲后的签名由服务端验签落库），与门户、CLI 完全同一条链路 —— 压测用的
// 就是真实协议，不会因为「服务端给个假凭证」而把并发写路径测歪。
//
// ⚠️ **默认模式没有 prover**（门户与 CLI 都有）：它只能跑在**开发模式**
// （服务端未加载 crypto.vk_path，只要求 proof 非空）的服务上。给
// `-proving-key` 之后会走**真证明**路径：本地加密、派生 nullifier、出 Groth16
// 证明，再并发提交 —— 这时服务端会真验签，测出来的才是真实投票路径的容量。
//
// ⚠️ **默认模式也不带资格证明**，所以它只能在**未开启资格校验**
// （crypto.eligibility_root 未配置）的服务上可用。给 `-eligibility-dir` 之后会
// 用名单目录里的成员包逐个领凭证（一人一票路径），这时可以跑 release 模式的服务。
//
// 两把 key 的口径与门户、CLI 完全一致：proving key **只在客户端**，不进服务端。
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	gnarkLogger "github.com/consensys/gnark/logger"
)

// devProofBase64 是 "dev-proof" 的 base64 编码。开发模式下服务端只要求
// proof 非空，不校验内容。
const devProofBase64 = "ZGV2LXByb29m"

type options struct {
	baseURL     string
	adminToken  string
	proposalID  string
	voteCount   int
	concurrency int
	duplicates  int
	openDelay   time.Duration
	asJSON      bool

	// provingKeyPath 是**选票** proving key（只在客户端）。
	// 给了就出真证明；不给则发占位 proof（只够跑开发模式的服务）。
	provingKeyPath string
	// eligibilityDir 是资格名单目录（`cmd/eligibility-list` 的产物）。
	// 给了就逐个用成员包领凭证（一人一票路径），可以跑 release 模式的服务。
	eligibilityDir string
	// eligibilityKeyPath 是**资格** proving key（只在客户端）。
	eligibilityKeyPath string
	// proveWorkers 是并行出证明的 worker 数（默认 CPU 核数）。
	proveWorkers int
	// optionIndex 是投票的选项下标（从 0 开始）。
	optionIndex uint64
}

// ballotMaterial 是一张准备好的选票：密文 + nullifier + 证明。
//
// 真证明路径下三者**一一对应**：证明绑在这份密文与这个 nullifier 上，
// 换任何一个都验不过（服务端按请求里的密文与 nullifier 自拼公开输入）。
type ballotMaterial struct {
	ciphertext string // base64
	nullifier  string // 规范 hex
	proof      string // base64
}

type credential struct {
	userID    string
	pubKey    string
	signature string
}

type outcome struct {
	status  int
	code    string
	latency time.Duration
}

type phaseResult struct {
	outcomes []outcome
	elapsed  time.Duration
}

type report struct {
	ProposalID      string         `json:"proposal_id"`
	Votes           int            `json:"votes"`
	Concurrency     int            `json:"concurrency"`
	Duration        string         `json:"duration"`
	ThroughputPerS  float64        `json:"throughput_per_second"`
	LatencyP50      string         `json:"latency_p50"`
	LatencyP90      string         `json:"latency_p90"`
	LatencyP99      string         `json:"latency_p99"`
	LatencyMax      string         `json:"latency_max"`
	StatusCounts    map[string]int `json:"status_counts"`
	DuplicateCount  int            `json:"duplicate_count"`
	DuplicateOK     int            `json:"duplicate_accepted"`
	DuplicateCodes  []string       `json:"duplicate_codes"`
	VotedCredential int            `json:"voted_credentials"`
	ExpectedVoted   int            `json:"expected_voted_credentials"`
	// TimingResolutionLimited 表示这一轮比计时器的分辨率还快，耗时与吞吐都测不准。
	//
	// 真实压测（几百个请求）不会触发；但拿它测个位数请求时会出现 —— 那时报告
	// 里写「0 req/s」会被误读成「没跑」，所以单独标出来而不是假装测到了 0。
	TimingResolutionLimited bool `json:"timing_resolution_limited"`

	// RealProof 表示这一轮走的是**真证明**路径（服务端会真验签）。
	// false 表示发的是占位 proof —— 那时测的是「不验签时服务端能吞多快」，
	// 与真实投票路径不是一回事，读报告时不能混为一谈。
	RealProof bool `json:"real_proof"`
	// Eligibility 表示这一轮走的是**资格证明**领取（一人一票路径）。
	Eligibility bool `json:"eligibility"`
	// ProofGeneration 是**客户端**出证明的总耗时。
	//
	// 它**不计入**下面的吞吐与延迟：吞吐测的是服务端能验多少张票，
	// 出证明是客户端的活（浏览器里更慢）。分开放是因为两者混在一起
	// 会让人以为「服务端只能每秒处理这么少」。
	ProofGeneration string `json:"proof_generation"`
	// ProofsGenerated 是这一轮真实生成的证明数。
	ProofsGenerated int `json:"proofs_generated"`
	// ProveWorkers 是并行出证明的 worker 数。
	ProveWorkers int `json:"prove_workers"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// newHTTPClient 造一个压测专用的 HTTP 客户端。
//
// **必须显式配连接池**：Go 默认 Transport 的 MaxIdleConnsPerHost 只有 2，
// 并发度一高，每个 worker 都要现开一条 TCP 连接（用完就关）。那量出来的是
// **客户端的连接churn**，不是服务端的容量 —— 高并发下客户端自己成了瓶颈，
// 数字会随并发度升高反而**变差**（吞吐降下来、延迟飙上去），看起来像服务端
// 扛不住。真实部署里客户端在别的机器上，不存在这个瓶颈。
//
// 空闲连接数按并发度给（上限 256），并全部走 keep-alive。
func newHTTPClient(concurrency int) *http.Client {
	if concurrency <= 0 {
		concurrency = 1
	}
	idle := concurrency
	if idle > 256 {
		idle = 256
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          idle,
		MaxIdleConnsPerHost:   idle,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}
}

func run(args []string) int {
	// gnark 的默认 logger 往 **stdout** 写日志（编译电路、出证明的 INFO 行）。
	// 那些日志混在报告前面，`-json` 的输出就不是合法 JSON 了 —— 报告要能被
	// 机器读，就不能往 stdout 掺别的东西。挪到 stderr：stdout 只留报告。
	gnarkLogger.SetOutput(os.Stderr)

	opts := options{}
	fs := flag.NewFlagSet("nullifier-loadtest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.baseURL, "base-url", envOrDefault("NULLIFIER_BASE_URL", "http://localhost:8080"), "API base URL")
	fs.StringVar(&opts.adminToken, "admin-token", os.Getenv("NULLIFIER_ADMIN_TOKEN"), "admin bearer token")
	fs.StringVar(&opts.proposalID, "proposal", "", "复用已有提案（不给就新建一个）")
	fs.IntVar(&opts.voteCount, "votes", 200, "并发提交的选票数")
	fs.IntVar(&opts.concurrency, "concurrency", 20, "并发度")
	fs.IntVar(&opts.duplicates, "duplicates", 5, "用同一个 nullifier 并发提交的次数（防重投复核）")
	fs.DurationVar(&opts.openDelay, "open-delay", 5*time.Second, "新建提案时距窗口打开的延迟")
	fs.BoolVar(&opts.asJSON, "json", false, "以 JSON 输出报告")
	fs.StringVar(
		&opts.provingKeyPath, "proving-key", "",
		"选票 proving key（只在客户端）。给了就出真证明，服务端会真验签",
	)
	fs.StringVar(
		&opts.eligibilityDir, "eligibility-dir", "",
		"资格名单目录（cmd/eligibility-list 的产物）。给了就带资格证明领凭证",
	)
	fs.StringVar(
		&opts.eligibilityKeyPath, "eligibility-proving-key", "",
		"资格 proving key（只在客户端，配合 -eligibility-dir）",
	)
	fs.IntVar(&opts.proveWorkers, "prove-workers", 0, "并行出证明的 worker 数（默认 CPU 核数）")
	fs.Uint64Var(&opts.optionIndex, "option", 0, "投票的选项下标（从 0 开始）")
	if err := fs.Parse(args); err != nil {
		// 参数错误要说清楚是**哪个**参数 —— 只打印用法的话，调用方只能自己
		// 一个个试（`-open-delay 5` 与 `-open-delay 5s` 的差别就是这么来的）。
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		usage()
		return 2
	}
	if opts.voteCount <= 0 || opts.concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "votes 与 concurrency 必须为正数")
		return 2
	}
	if opts.duplicates < 2 {
		opts.duplicates = 2
	}
	if strings.TrimSpace(opts.adminToken) == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -admin-token（建提案、初始化解密会话、投凭证池都要管理令牌）")
		return 2
	}
	if strings.TrimSpace(opts.eligibilityDir) != "" && strings.TrimSpace(opts.eligibilityKeyPath) == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -eligibility-proving-key（资格 proving key，只在客户端）")
		return 2
	}
	if strings.TrimSpace(opts.eligibilityKeyPath) != "" && strings.TrimSpace(opts.eligibilityDir) == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -eligibility-dir（资格名单目录）")
		return 2
	}
	if opts.proveWorkers <= 0 {
		opts.proveWorkers = runtime.NumCPU()
	}

	client := newHTTPClient(opts.concurrency)
	proposalID, err := prepareProposal(client, &opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备提案失败: %v\n", err)
		return 1
	}

	numOptions, err := fetchNumOptions(client, &opts, proposalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "取提案选项数失败: %v\n", err)
		return 1
	}

	// 门限解密会话与凭证池必须在投票窗口打开**之前**准备好（服务端的硬性约束）。
	// 所以这两步排在出证明之前 —— 出证明很慢（真证明路径下每张票约 0.1 秒），
	// 放在前面会把窗口拖过去，服务端直接拒绝初始化。
	if err := setupDecryptionAndPool(client, &opts, proposalID); err != nil {
		fmt.Fprintf(os.Stderr, "准备解密会话与凭证池失败: %v\n", err)
		return 1
	}

	credentials, err := prepareCredentials(client, &opts, proposalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备凭证失败: %v\n", err)
		return 1
	}

	electionKey, err := fetchElectionKey(client, &opts, proposalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "取计票公钥失败: %v\n", err)
		return 1
	}

	// 选票材料：真证明路径下每张票一份密文 + nullifier + 证明。
	materials, generation, err := prepareBallots(
		&opts, proposalID, electionKey, numOptions,
		opts.voteCount+opts.duplicates, opts.duplicates,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备选票失败: %v\n", err)
		return 1
	}

	// 窗口还没打开时投票会被拒 —— 明确等到窗口开始。
	if err := waitForWindow(client, &opts, proposalID); err != nil {
		fmt.Fprintf(os.Stderr, "等待投票窗口失败: %v\n", err)
		return 1
	}

	// 第一轮：每个请求一个不同的凭证与 nullifier。
	votePhase := runVotes(
		client, &opts, proposalID, credentials[:opts.voteCount], materials[:opts.voteCount],
	)
	// 第二轮：同一个 nullifier、不同凭证 —— 唯一索引必须只放行一张。
	duplicatePhase := runVotes(
		client, &opts, proposalID,
		credentials[opts.voteCount:opts.voteCount+opts.duplicates],
		materials[opts.voteCount:],
	)

	voted, err := countVotedCredentials(client, &opts, proposalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "核对凭证投票状态失败: %v\n", err)
		return 1
	}

	result := buildReport(&opts, proposalID, votePhase, duplicatePhase, voted)
	result.RealProof = strings.TrimSpace(opts.provingKeyPath) != ""
	result.Eligibility = strings.TrimSpace(opts.eligibilityDir) != ""
	result.ProofGeneration = generation.elapsed.String()
	result.ProofsGenerated = generation.count
	result.ProveWorkers = opts.proveWorkers
	if opts.asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "输出报告失败: %v\n", err)
			return 1
		}
	} else {
		printReport(result)
	}

	// 防重投没守住就是失败 —— 压测工具的结论必须能当门禁用。
	if result.DuplicateOK != 1 {
		fmt.Fprintf(
			os.Stderr,
			"防重投复核失败: %d 个同 nullifier 的并发请求里放行了 %d 个（应恰好 1 个）\n",
			result.DuplicateCount, result.DuplicateOK,
		)
		return 1
	}
	// 落库数与放行数对不上，说明有票没进 participation 表（或者有票重复计了）。
	if result.VotedCredential != result.ExpectedVoted {
		fmt.Fprintf(
			os.Stderr,
			"落库核对失败: 预期 %d 个凭证已投票，实际 %d 个\n",
			result.ExpectedVoted, result.VotedCredential,
		)
		return 1
	}
	return 0
}

// prepareProposal 新建提案或复用已有提案，返回提案 ID。
//
// 新建时窗口设在「稍后打开」：门限解密会话必须在投票窗口打开之前初始化，
// 这是服务端的硬性约束。
func prepareProposal(client *http.Client, opts *options) (string, error) {
	if strings.TrimSpace(opts.proposalID) != "" {
		return strings.TrimSpace(opts.proposalID), nil
	}
	startsAt := time.Now().UTC().Add(opts.openDelay)
	payload := map[string]any{
		"title":     "Load Test",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": startsAt.Format(time.RFC3339),
		"ends_at":   startsAt.Add(time.Hour).Format(time.RFC3339),
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := postJSON(client, opts, "/api/v1/proposals", payload, true, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("创建提案的响应里没有 id")
	}
	return created.ID, nil
}

// setupDecryptionAndPool 初始化门限解密会话并往凭证池投槽位。
//
// 这两步**必须在投票窗口打开之前**做完：服务端拒绝给「投票开放中」的提案
// 初始化解密会话（那时票已经投出去了，解不开）。所以它们排在出证明之前。
func setupDecryptionAndPool(client *http.Client, opts *options, proposalID string) error {
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/decryption/setup",
		map[string]any{"threshold": 2, "trustees": 3},
		true, nil,
	); err != nil {
		return fmt.Errorf("初始化解密会话: %w", err)
	}
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/credentials/pool",
		map[string]any{"count": opts.voteCount + opts.duplicates},
		true, nil,
	); err != nil {
		return fmt.Errorf("投凭证池: %w", err)
	}
	return nil
}

// prepareCredentials 匿名领取凭证（每个请求一张）。
//
// 给了 `-eligibility-dir` 时走**资格证明**领取（一人一票路径）：每个成员包
// 领一张 —— 这样才能在**开启资格校验**的服务上跑（release 模式必开）。
// 没给时是纯匿名领取，只能跑未开启资格校验的服务。
func prepareCredentials(client *http.Client, opts *options, proposalID string) ([]credential, error) {
	need := opts.voteCount + opts.duplicates

	if strings.TrimSpace(opts.eligibilityDir) != "" {
		members, err := loadMemberPackages(opts.eligibilityDir)
		if err != nil {
			return nil, err
		}
		if len(members) < need {
			return nil, fmt.Errorf(
				"资格名单只有 %d 个成员，压测需要 %d 个（票数 + 防重投复核）",
				len(members), need,
			)
		}
		return claimCredentialsWithEligibility(client, opts, proposalID, members[:need])
	}

	credentials := make([]credential, 0, need)
	for i := 0; i < need; i += 1 {
		pubKey, err := randomBase64(32)
		if err != nil {
			return nil, err
		}
		cred, err := claimCredential(client, opts, proposalID, pubKey, nil)
		if err != nil {
			return nil, fmt.Errorf("领取凭证: %w", err)
		}
		credentials = append(credentials, cred)
	}
	return credentials, nil
}

// loadMemberPackages 读资格名单目录里的成员包。
//
// 文件名形如 `eligibility-<成员>.json`（`cmd/eligibility-list` 的产物）。
// 按文件名排序 —— 顺序稳定，重复跑压测时「哪个成员领哪张票」也可复现。
func loadMemberPackages(dir string) ([]eligibility.MemberPackage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取资格名单目录: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "eligibility-") {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("资格名单目录 %s 里没有成员包（eligibility-*.json）", dir)
	}
	packages := make([]eligibility.MemberPackage, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("读取成员包 %s: %w", name, err)
		}
		var pack eligibility.MemberPackage
		if err := json.Unmarshal(raw, &pack); err != nil {
			return nil, fmt.Errorf("解析成员包 %s: %w", name, err)
		}
		packages = append(packages, pack)
	}
	return packages, nil
}

// eligibilityChallengeResponse 是领取挑战端点的响应。
type eligibilityChallengeResponse struct {
	ProposalID string `json:"proposal_id"`
	Root       string `json:"root"`
	Depth      int    `json:"depth"`
	Challenge  string `json:"challenge"`
}

// claimCredentialsWithEligibility 用成员包逐个领凭证（一人一票路径）。
//
// 三步：① 取一次领取挑战（**root 以服务端为准**）→ ② 本地出资格证明 →
// ③ 带着资格材料去领（资格不过**不消耗**池槽位）。
//
// 领取是**并行**的：每个成员都要出一次 Groth16 证明（资格电路约 6860 条约束），
// 串行下来光是出证明就要几分钟 —— 那是客户端的成本，不该拖住压测本身。
func claimCredentialsWithEligibility(
	client *http.Client,
	opts *options,
	proposalID string,
	members []eligibility.MemberPackage,
) ([]credential, error) {
	prover, err := eligibility.NewProver(opts.eligibilityKeyPath)
	if err != nil {
		return nil, fmt.Errorf("加载资格 proving key 失败: %w", err)
	}

	credentials := make([]credential, len(members))
	errs := make([]error, len(members))
	runParallel(len(members), opts.proveWorkers, func(index int) {
		pubKey, keyErr := randomBase64(32)
		if keyErr != nil {
			errs[index] = keyErr
			return
		}
		material, materialErr := buildEligibilityMaterial(
			client, opts, proposalID, prover, members[index],
		)
		if materialErr != nil {
			errs[index] = materialErr
			return
		}
		cred, claimErr := claimCredential(client, opts, proposalID, pubKey, material)
		if claimErr != nil {
			errs[index] = claimErr
			return
		}
		credentials[index] = cred
	})
	for index, claimErr := range errs {
		if claimErr != nil {
			return nil, fmt.Errorf("领取凭证（第 %d 个成员）: %w", index+1, claimErr)
		}
	}
	return credentials, nil
}

// buildEligibilityMaterial 备好领取请求里的资格材料。
//
// 与 CLI 的同名步骤完全一致：挑战向服务端要（root 一律以服务端为准），
// 证明在本地出 —— proving key 只在客户端，服务端拿到的只有证明本身。
func buildEligibilityMaterial(
	client *http.Client,
	opts *options,
	proposalID string,
	prover *eligibility.Prover,
	pack eligibility.MemberPackage,
) (map[string]any, error) {
	var challenge eligibilityChallengeResponse
	path := "/api/v1/proposals/" + url.PathEscape(proposalID) + "/credentials/eligibility-challenge"
	if err := postJSON(client, opts, path, map[string]any{}, false, &challenge); err != nil {
		return nil, fmt.Errorf("获取领取挑战失败: %w", err)
	}
	result, err := prover.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: pack,
		ProposalID:    proposalID,
		Challenge:     challenge.Challenge,
		Root:          challenge.Root,
	})
	if err != nil {
		return nil, fmt.Errorf("生成资格证明失败: %w", err)
	}
	return map[string]any{
		"eligibility_proof": base64.StdEncoding.EncodeToString(result.Proof),
		"qualification_id":  result.QualificationID,
		"challenge":         challenge.Challenge,
	}, nil
}

// runParallel 把 [0, total) 的下标分给 workers 个 goroutine。
//
// 压测里的并行度有两条线：**提交**的并发度（`-concurrency`）测的是服务端能吞
// 多少请求；**出证明**的并行度（`-prove-workers`）是客户端自己的资源分配。
// 两者分开 —— 用提交并发度去出证明，会让客户端先把自己压死，测出来的就不是
// 服务端的能力了。
func runParallel(total int, workers int, work func(index int)) {
	if total <= 0 {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > total {
		workers = total
	}
	queue := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i += 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				work(index)
			}
		}()
	}
	for i := range total {
		queue <- i
	}
	close(queue)
	wg.Wait()
}

// credentialReservation 是「预留签发权」的响应。
//
// 它带出签发方公钥：客户端要拿它把凭证消息盲化。公钥本来就是公开值。
type credentialReservation struct {
	ProposalID       string `json:"proposal_id"`
	UserID           string `json:"user_id"`
	IssuerKeyVersion int    `json:"issuer_key_version"`
	Variant          string `json:"variant"`
	Modulus          string `json:"modulus"`
	Exponent         int    `json:"exponent"`
}

// issuerPublicKey 把响应里的公钥字段还原成领域类型。
func (r *credentialReservation) issuerPublicKey() (*domaincrypto.IssuerPublicKey, error) {
	modulus, err := base64.StdEncoding.DecodeString(r.Modulus)
	if err != nil {
		return nil, fmt.Errorf("issuer modulus must be base64: %w", err)
	}
	return &domaincrypto.IssuerPublicKey{
		Version:  r.IssuerKeyVersion,
		Modulus:  modulus,
		Exponent: r.Exponent,
		Variant:  r.Variant,
	}, nil
}

// blindSignatureResponse 是盲签名的响应。
type blindSignatureResponse struct {
	BlindSignature string `json:"blind_signature"`
}

// claimMaxAttempts 是匿名领取的重试次数。
//
// 预留**不占位**：服务端取槽位时不改状态，并发领取会抢到同一个槽位。抢输的那个
// 换一个槽位重来 —— 但重试要重新盲化（新的 user_id 意味着新的消息）。
const claimMaxAttempts = 5

// claimCredential 匿名领取一张凭证，走完**真两段式盲签名**。
//
// v0.2.0 的领取是三步：
// ① 预留签发权（请求体里**没有**公钥）→ ② 本地盲化 + 服务端盲签名 → ③ 交回
// 解盲后的签名，由服务端验签落库。
//
// 客户端从头到尾没把公钥发给服务端 —— 服务端一旦知道公钥就能算出消息原文，
// 盲化当场失效。
//
// extra 是可选的资格材料（证明 + 去重键 + 挑战）：没有资格校验的服务不需要。
func claimCredential(
	client *http.Client,
	opts *options,
	proposalID string,
	userPubKeyBase64 string,
	extra map[string]any,
) (credential, error) {
	userPubKey, err := base64.StdEncoding.DecodeString(userPubKeyBase64)
	if err != nil {
		return credential{}, fmt.Errorf("user pub key must be base64: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < claimMaxAttempts; attempt++ {
		cred, err := claimCredentialOnce(client, opts, proposalID, userPubKey, extra)
		if err == nil {
			return cred, nil
		}
		if !isSlotTaken(err) {
			return credential{}, err
		}
		lastErr = err
	}
	return credential{}, lastErr
}

// claimCredentialOnce 走一遍领取流程（不重试）。
func claimCredentialOnce(
	client *http.Client,
	opts *options,
	proposalID string,
	userPubKey []byte,
	extra map[string]any,
) (credential, error) {
	escaped := url.PathEscape(proposalID)

	// ① 预留签发权。请求体里没有公钥：服务端只给 user_id 与签发方公钥。
	//    资格材料（如果开了资格校验）也在这一步带上 —— 资格不过不消耗槽位。
	body := make(map[string]any, len(extra))
	for key, value := range extra {
		body[key] = value
	}
	var reservation credentialReservation
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+escaped+"/credentials/claim",
		body,
		false,
		&reservation,
	); err != nil {
		return credential{}, err
	}
	issuerKey, err := reservation.issuerPublicKey()
	if err != nil {
		return credential{}, err
	}

	// ② 本地盲化凭证消息。
	blindClient, err := cryptoinfra.NewCredentialBlindClient(issuerKey)
	if err != nil {
		return credential{}, err
	}
	blinded, err := blindClient.Blind(reservation.UserID, userPubKey)
	if err != nil {
		return credential{}, err
	}

	// ③ 把盲化消息送去盲签名。
	var blindResponse blindSignatureResponse
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+escaped+"/credentials/blind-sign",
		map[string]any{
			"user_id":            reservation.UserID,
			"blinded_message":    base64.StdEncoding.EncodeToString(blinded.BlindedMessage),
			"issuer_key_version": reservation.IssuerKeyVersion,
		},
		false,
		&blindResponse,
	); err != nil {
		return credential{}, err
	}
	blindSignature, err := base64.StdEncoding.DecodeString(blindResponse.BlindSignature)
	if err != nil {
		return credential{}, fmt.Errorf("blind signature must be base64: %w", err)
	}

	// ④ 本地解盲，得到最终签名。
	signature, err := blindClient.Finalize(blinded, blindSignature)
	if err != nil {
		return credential{}, err
	}

	// ⑤ 交回最终签名，服务端验签后落库 —— 公钥到这一步才第一次发出去。
	var attested struct {
		UserID     string `json:"user_id"`
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+escaped+"/credentials/attest",
		map[string]any{
			"user_id":      reservation.UserID,
			"user_pub_key": base64.StdEncoding.EncodeToString(userPubKey),
			"signature":    base64.StdEncoding.EncodeToString(signature),
		},
		false,
		&attested,
	); err != nil {
		return credential{}, err
	}
	return credential{
		userID:    attested.UserID,
		pubKey:    attested.UserPubKey,
		signature: attested.Signature,
	}, nil
}

func fetchElectionKey(client *http.Client, opts *options, proposalID string) ([]byte, error) {
	var key struct {
		PublicKey string `json:"public_key"`
	}
	if err := getJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/election-key",
		&key,
	); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(key.PublicKey)
}

// fetchNumOptions 取提案的选项数。
//
// 它进公开输入：客户端与服务端必须用**同一个**数，否则证明验不过。
// 复用已有提案时尤其重要 —— 那个提案不一定是压测工具自己建的那个。
func fetchNumOptions(client *http.Client, opts *options, proposalID string) (int, error) {
	var listed struct {
		Items []struct {
			ID      string   `json:"id"`
			Options []string `json:"options"`
		} `json:"items"`
	}
	if err := getJSON(client, opts, "/api/v1/proposals", &listed); err != nil {
		return 0, err
	}
	for _, item := range listed.Items {
		if item.ID != proposalID {
			continue
		}
		if len(item.Options) == 0 {
			return 0, errors.New("提案没有选项")
		}
		return len(item.Options), nil
	}
	return 0, fmt.Errorf("提案 %s 不在提案列表里", proposalID)
}

// proofGeneration 记录**客户端**出证明的耗时与数量。
type proofGeneration struct {
	elapsed time.Duration
	count   int
}

// prepareBallots 备好这一轮要提交的选票材料。
//
// 默认（没有 `-proving-key`）：所有请求共用一份密文，nullifier 每个请求随机
// 生成，proof 是占位值 —— 只能跑**开发模式**（服务端不验签）的服务。
//
// 有 `-proving-key`：每张票一份密文 + 派生好的 nullifier + **真证明**。
// 服务端会按请求里的密文与 nullifier 自拼公开输入并真验签 —— 这时测出来的
// 才是真实投票路径的容量。
//
// 最后 `duplicates` 份材料**共用一个 nullifier**（同一个秘密派生）：防重投复核
// 要的就是「同一个 nullifier 被并发提交多次，只有一次能成」。
//
// 出证明是**客户端**的活（浏览器里更慢），所以单独计时、单独汇报：混进
// 服务端的吞吐里会让人以为「服务端只能每秒处理这么少」。
func prepareBallots(
	opts *options,
	proposalID string,
	electionKey []byte,
	numOptions int,
	count int,
	duplicates int,
) ([]ballotMaterial, proofGeneration, error) {
	if strings.TrimSpace(opts.provingKeyPath) == "" {
		ciphertext, _, err := ballot.EncryptBallotWithUncompressedKey(electionKey, opts.optionIndex)
		if err != nil {
			return nil, proofGeneration{}, fmt.Errorf("本地加密选票失败: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(ciphertext)
		materials := make([]ballotMaterial, count)
		// 防重投那一组共用一个 nullifier。
		shared, err := randomNullifier()
		if err != nil {
			return nil, proofGeneration{}, err
		}
		for index := range materials {
			nullifier := shared
			if index < count-duplicates {
				generated, err := randomNullifier()
				if err != nil {
					return nil, proofGeneration{}, err
				}
				nullifier = generated
			}
			materials[index] = ballotMaterial{
				ciphertext: encoded,
				nullifier:  nullifier,
				proof:      devProofBase64,
			}
		}
		return materials, proofGeneration{}, nil
	}

	prover, err := ballot.NewProver(opts.provingKeyPath)
	if err != nil {
		return nil, proofGeneration{}, fmt.Errorf("加载选票 proving key 失败: %w", err)
	}
	// 服务端拼公开输入时用的是压缩公钥：客户端要转成同一形式，
	// 两边算出来的公开输入才逐字段相同。
	compressedKey, err := ballot.CompressPoint(electionKey)
	if err != nil {
		return nil, proofGeneration{}, fmt.Errorf("压缩计票公钥失败: %w", err)
	}
	proposalField := ballot.DeriveProposalField(proposalID)

	// 防重投那一组共用一个 nullifier 秘密。
	sharedSecret, err := ballot.GenerateNullifierSecret()
	if err != nil {
		return nil, proofGeneration{}, err
	}
	var sharedElement fr.Element
	sharedElement.SetBytes(sharedSecret)

	materials := make([]ballotMaterial, count)
	errs := make([]error, count)
	started := time.Now()
	runParallel(count, opts.proveWorkers, func(index int) {
		secretElement := sharedElement
		if index < count-duplicates {
			secret, secretErr := ballot.GenerateNullifierSecret()
			if secretErr != nil {
				errs[index] = secretErr
				return
			}
			secretElement.SetBytes(secret)
		}
		material, materialErr := proveBallot(
			opts, proposalID, proposalField, compressedKey, electionKey,
			numOptions, secretElement, prover,
		)
		if materialErr != nil {
			errs[index] = materialErr
			return
		}
		materials[index] = material
	})
	elapsed := time.Since(started)
	for index, proveErr := range errs {
		if proveErr != nil {
			return nil, proofGeneration{}, fmt.Errorf("出证明（第 %d 张）: %w", index+1, proveErr)
		}
	}
	return materials, proofGeneration{elapsed: elapsed, count: count}, nil
}

// proveBallot 为一张选票出证明：加密 → 派生 nullifier → 拼公开输入 → 出证明。
//
// 公开输入与服务端用**同一套**拼装逻辑（ballot.BuildPublicInputs）——
// 两边算出来的公开输入逐字段相同，证明才验得过。
func proveBallot(
	opts *options,
	proposalID string,
	proposalField fr.Element,
	compressedKey []byte,
	electionKey []byte,
	numOptions int,
	secretElement fr.Element,
	prover *ballot.Prover,
) (ballotMaterial, error) {
	ciphertext, nonce, err := ballot.EncryptBallotWithUncompressedKey(
		electionKey, opts.optionIndex,
	)
	if err != nil {
		return ballotMaterial{}, fmt.Errorf("本地加密选票失败: %w", err)
	}
	nullifier := ballot.DeriveNullifier(secretElement, proposalField)
	nullifierBytes := nullifier.Bytes()
	inputs, err := ballot.BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         nullifierBytes[:],
		ElectionPublicKey: compressedKey,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	})
	if err != nil {
		return ballotMaterial{}, fmt.Errorf("拼公开输入失败: %w", err)
	}
	proof, err := prover.Prove(inputs, opts.optionIndex, nonce, secretElement)
	if err != nil {
		return ballotMaterial{}, fmt.Errorf("出证明失败: %w", err)
	}
	return ballotMaterial{
		ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		nullifier:  ballot.EncodeScalarHex(nullifier),
		proof:      base64.StdEncoding.EncodeToString(proof),
	}, nil
}

// waitForWindow 等到投票窗口打开。
//
// 服务端没有「按 ID 取单个提案」的公开端点，用提案列表（公开只读）定位。
func waitForWindow(client *http.Client, opts *options, proposalID string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var listed struct {
			Items []struct {
				ID       string    `json:"id"`
				StartsAt time.Time `json:"starts_at"`
				EndsAt   time.Time `json:"ends_at"`
			} `json:"items"`
		}
		if err := getJSON(client, opts, "/api/v1/proposals", &listed); err != nil {
			return err
		}
		for _, item := range listed.Items {
			if item.ID != proposalID {
				continue
			}
			if time.Now().UTC().After(item.StartsAt) {
				return nil
			}
			if time.Now().After(deadline) {
				return errors.New("等待投票窗口超时")
			}
			time.Sleep(500 * time.Millisecond)
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("提案 %s 不在提案列表里", proposalID)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// runVotes 并发提交选票。
//
// credentials 与 materials 一一对应：第 i 个请求用第 i 张凭证与第 i 份材料。
// 防重投复核那一轮的材料**共用同一个 nullifier** —— 唯一索引必须只放行一张。
func runVotes(
	client *http.Client,
	opts *options,
	proposalID string,
	credentials []credential,
	materials []ballotMaterial,
) phaseResult {
	outcomes := make([]outcome, len(credentials))
	queue := make(chan int)
	var wg sync.WaitGroup

	workers := opts.concurrency
	if workers > len(credentials) {
		workers = len(credentials)
	}
	for i := 0; i < workers; i += 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				outcomes[index] = submitVote(
					client, opts, proposalID, credentials[index], materials[index],
				)
			}
		}()
	}

	started := time.Now()
	for i := range credentials {
		queue <- i
	}
	close(queue)
	wg.Wait()
	return phaseResult{outcomes: outcomes, elapsed: time.Since(started)}
}

func submitVote(
	client *http.Client,
	opts *options,
	proposalID string,
	cred credential,
	material ballotMaterial,
) outcome {
	payload := map[string]any{
		"encrypted_vote":       material.ciphertext,
		"proof":                material.proof,
		"nullifier":            material.nullifier,
		"user_id":              cred.userID,
		"user_pub_key":         cred.pubKey,
		"credential_signature": cred.signature,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return outcome{status: 0, code: "marshal: " + err.Error()}
	}
	// 提案 ID 走路径，不走请求体（见 docs/api-contract-v1.md）。
	votePath := "/api/v1/proposals/" + url.PathEscape(proposalID) + "/votes"
	request, err := http.NewRequest(
		http.MethodPost, strings.TrimRight(opts.baseURL, "/")+votePath, bytes.NewReader(body),
	)
	if err != nil {
		return outcome{status: 0, code: "request: " + err.Error()}
	}
	request.Header.Set("Content-Type", "application/json")

	start := time.Now()
	response, err := client.Do(request)
	latency := time.Since(start)
	if err != nil {
		return outcome{status: 0, code: "do: " + err.Error(), latency: latency}
	}
	defer func() { _ = response.Body.Close() }()

	var payloadError struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(response.Body).Decode(&payloadError)
	return outcome{status: response.StatusCode, code: payloadError.Code, latency: latency}
}

// countVotedCredentials 用管理面接口核对「已投票的凭证数」。
func countVotedCredentials(client *http.Client, opts *options, proposalID string) (int, error) {
	var listed struct {
		Items []struct {
			HasVoted bool `json:"has_voted"`
		} `json:"items"`
	}
	if err := getJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/credentials",
		&listed,
	); err != nil {
		return 0, err
	}
	count := 0
	for _, item := range listed.Items {
		if item.HasVoted {
			count++
		}
	}
	return count, nil
}

func buildReport(
	opts *options,
	proposalID string,
	votePhase phaseResult,
	duplicatePhase phaseResult,
	voted int,
) report {
	statusCounts := make(map[string]int)
	latencies := make([]time.Duration, 0, len(votePhase.outcomes))
	for _, result := range votePhase.outcomes {
		statusCounts[statusLabel(result)]++
		latencies = append(latencies, result.latency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	duplicateAccepted := 0
	duplicateCodes := make([]string, 0, len(duplicatePhase.outcomes))
	for _, result := range duplicatePhase.outcomes {
		duplicateCodes = append(duplicateCodes, statusLabel(result))
		if result.status == http.StatusCreated {
			duplicateAccepted++
		}
	}

	throughput := 0.0
	timingLimited := votePhase.elapsed <= 0
	if !timingLimited {
		throughput = float64(len(votePhase.outcomes)) / votePhase.elapsed.Seconds()
	}

	return report{
		ProposalID:      proposalID,
		Votes:           len(votePhase.outcomes),
		Concurrency:     opts.concurrency,
		Duration:        votePhase.elapsed.String(),
		ThroughputPerS:  throughput,
		LatencyP50:      percentile(latencies, 0.50).String(),
		LatencyP90:      percentile(latencies, 0.90).String(),
		LatencyP99:      percentile(latencies, 0.99).String(),
		LatencyMax:      percentile(latencies, 1).String(),
		StatusCounts:    statusCounts,
		DuplicateCount:  len(duplicatePhase.outcomes),
		DuplicateOK:     duplicateAccepted,
		DuplicateCodes:  duplicateCodes,
		VotedCredential: voted,
		// 第一轮每张票一个凭证，第二轮恰好再放行一张 → 期望的已投票凭证数。
		ExpectedVoted:           len(votePhase.outcomes) + duplicateAccepted,
		TimingResolutionLimited: timingLimited,
	}
}

func statusLabel(result outcome) string {
	if result.status == 0 {
		return "error:" + result.code
	}
	if result.code == "" {
		return fmt.Sprintf("%d", result.status)
	}
	return fmt.Sprintf("%d/%s", result.status, result.code)
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(quantile * float64(len(sorted)-1))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func printReport(result report) {
	fmt.Printf("=== 压测报告 ===\n")
	fmt.Printf("提案:        %s\n", result.ProposalID)
	fmt.Printf("请求:        %d 张票，并发度 %d\n", result.Votes, result.Concurrency)
	fmt.Printf("证明模式:    %s\n", proofModeLabel(result))
	fmt.Printf("总耗时:      %s\n", result.Duration)
	if result.TimingResolutionLimited {
		fmt.Printf("吞吐:        低于计时分辨率（这一轮太快，耗时与吞吐都测不准）\n")
	} else {
		fmt.Printf("吞吐:        %.1f req/s\n", result.ThroughputPerS)
	}
	fmt.Printf(
		"延迟:        p50=%s p90=%s p99=%s max=%s\n",
		result.LatencyP50, result.LatencyP90, result.LatencyP99, result.LatencyMax,
	)
	fmt.Printf("状态分布:    %s\n", formatCounts(result.StatusCounts))
	fmt.Printf(
		"防重投复核:  %d 个同 nullifier 的并发请求 → 放行 %d 个 %s\n",
		result.DuplicateCount, result.DuplicateOK, formatCounts(countSlice(result.DuplicateCodes)),
	)
	fmt.Printf(
		"落库核对:    %d 个凭证已投票（预期 %d 个）\n",
		result.VotedCredential, result.ExpectedVoted,
	)
	if result.RealProof {
		fmt.Printf(
			"客户端出证明: %s（%d 份，%d 并行）—— 不计入上面的吞吐与延迟\n",
			result.ProofGeneration, result.ProofsGenerated, result.ProveWorkers,
		)
	}
}

// proofModeLabel 说清这一轮测的是哪条路径。
//
// 「占位证明」与「真证明」的数字**不能混着比**：前者服务端不验签，量的是
// 「不验签时能吞多快」；后者才是真实投票路径的成本。
func proofModeLabel(result report) string {
	if !result.RealProof {
		return "占位证明（服务端不验签，只能跑开发模式）"
	}
	if result.Eligibility {
		return "真证明 + 资格证明领取（release 模式形态）"
	}
	return "真证明（服务端真验签）"
}

func countSlice(labels []string) map[string]int {
	counts := make(map[string]int)
	for _, label := range labels {
		counts[label]++
	}
	return counts
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s×%d", key, counts[key]))
	}
	return strings.Join(parts, " ")
}

func postJSON(
	client *http.Client,
	opts *options,
	path string,
	body any,
	admin bool,
	out any,
) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(
		http.MethodPost, strings.TrimRight(opts.baseURL, "/")+path, bytes.NewReader(raw),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if admin {
		request.Header.Set("Authorization", "Bearer "+opts.adminToken)
	}
	return doRequest(client, request, out)
}

func getJSON(client *http.Client, opts *options, path string, out any) error {
	request, err := http.NewRequest(
		http.MethodGet, strings.TrimRight(opts.baseURL, "/")+path, nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+opts.adminToken)
	return doRequest(client, request, out)
}

// httpError 是带状态码与错误码的失败。
//
// 状态码是必需的：领取凭证遇到「槽位被别人抢走」时客户端要换个槽位重来，
// 判断依据就是 409 + CREDENTIAL_EXISTS。
type httpError struct {
	Status  int
	Code    string
	Message string
}

func (e *httpError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

// isSlotTaken 判断错误是不是「槽位被别人抢走」—— 只有这种情况值得重试。
//
// 409 有两种：槽位被抢（换个槽位重来就行）与凭证池已空（重试也没用）。
func isSlotTaken(err error) bool {
	var httpErr *httpError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.Status == http.StatusConflict && httpErr.Code == "CREDENTIAL_EXISTS"
}

func doRequest(client *http.Client, request *http.Request, out any) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		if failure.Code == "" {
			failure.Code = http.StatusText(response.StatusCode)
		}
		return &httpError{
			Status:  response.StatusCode,
			Code:    failure.Code,
			Message: strings.TrimSpace(failure.Error),
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// randomNullifier 生成一个规范编码的 nullifier（64 位小写 hex）。
func randomNullifier() (string, error) {
	secret, err := ballot.GenerateNullifierSecret()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(secret), nil
}

func randomBase64(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer), nil
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func usage() {
	fmt.Println(`用法:
  nullifier-loadtest [参数]

参数:
  -base-url       API 地址，默认 http://localhost:8080
  -admin-token    管理 Bearer Token（建提案/初始化解密会话/投凭证池都要）
  -proposal       复用已有提案（不给就新建一个）
  -votes          并发提交的选票数，默认 200
  -concurrency    并发度，默认 20
  -duplicates     用同一个 nullifier 并发提交的次数（防重投复核），默认 5
  -open-delay     新建提案时距窗口打开的延迟，默认 5s
  -json           以 JSON 输出报告

  -proving-key          选票 proving key（只在客户端）。给了就出真证明，
                        服务端会真验签 —— 只能跑配了 verifying key 的服务
  -eligibility-dir      资格名单目录（cmd/eligibility-list 的产物）。
                        给了就带资格证明领凭证，可跑 release 模式的服务
  -eligibility-proving-key
                        资格 proving key（只在客户端，配合 -eligibility-dir）
  -prove-workers        并行出证明的 worker 数，默认 CPU 核数
  -option               投票的选项下标（从 0 开始），默认 0

示例（开发模式，量「不验签时能吞多快」）:
  nullifier-loadtest -admin-token dev-admin-token -votes 200 -concurrency 20

示例（真实投票路径：真证明 + 资格证明 + release 模式）:
  nullifier-loadtest -admin-token "$TOKEN" \
    -proving-key keys/proving.key \
    -eligibility-dir elig -eligibility-proving-key keys/eligibility-proving.key \
    -votes 50 -concurrency 10`)
}
