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
// ⚠️ 客户端还没有 prover，这个工具只能跑在**开发模式**（服务端未加载
// crypto.vk_path，只要求 proof 非空）的服务上。release 模式下所有请求都会
// 因验签失败被拒 —— 报告里的状态分布会如实反映这一点。
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
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"anonvote/internal/infrastructure/crypto/ballot"
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
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
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
	if err := fs.Parse(args); err != nil {
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

	client := &http.Client{Timeout: 30 * time.Second}
	proposalID, err := prepareProposal(client, &opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备提案失败: %v\n", err)
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
	ciphertext, _, err := ballot.EncryptBallotWithUncompressedKey(electionKey, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "本地加密选票失败: %v\n", err)
		return 1
	}
	encodedCiphertext := base64.StdEncoding.EncodeToString(ciphertext)

	// 窗口还没打开时投票会被拒 —— 明确等到窗口开始。
	if err := waitForWindow(client, &opts, proposalID); err != nil {
		fmt.Fprintf(os.Stderr, "等待投票窗口失败: %v\n", err)
		return 1
	}

	// 第一轮：每个请求一个不同的凭证与 nullifier。
	votePhase := runVotes(
		client, &opts, proposalID, encodedCiphertext, credentials[:opts.voteCount], "",
	)
	// 第二轮：同一个 nullifier、不同凭证 —— 唯一索引必须只放行一张。
	duplicateNullifier, err := randomNullifier()
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成 nullifier 失败: %v\n", err)
		return 1
	}
	duplicatePhase := runVotes(
		client, &opts, proposalID, encodedCiphertext,
		credentials[opts.voteCount:opts.voteCount+opts.duplicates],
		duplicateNullifier,
	)

	voted, err := countVotedCredentials(client, &opts, proposalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "核对凭证投票状态失败: %v\n", err)
		return 1
	}

	result := buildReport(&opts, proposalID, votePhase, duplicatePhase, voted)
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

// prepareCredentials 初始化解密会话、往池里投槽位，然后匿名领取凭证。
func prepareCredentials(client *http.Client, opts *options, proposalID string) ([]credential, error) {
	// 初始化门限解密会话：必须在投票窗口打开之前。
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/decryption/setup",
		map[string]any{"threshold": 2, "trustees": 3},
		true, nil,
	); err != nil {
		return nil, fmt.Errorf("初始化解密会话: %w", err)
	}

	need := opts.voteCount + opts.duplicates
	if err := postJSON(
		client, opts,
		"/api/v1/proposals/"+url.PathEscape(proposalID)+"/credentials/pool",
		map[string]any{"count": need},
		true, nil,
	); err != nil {
		return nil, fmt.Errorf("投凭证池: %w", err)
	}

	credentials := make([]credential, 0, need)
	for i := 0; i < need; i += 1 {
		pubKey, err := randomBase64(32)
		if err != nil {
			return nil, err
		}
		var claimed struct {
			UserID     string `json:"user_id"`
			UserPubKey string `json:"user_pub_key"`
			Signature  string `json:"signature"`
		}
		if err := postJSON(
			client, opts,
			"/api/v1/proposals/"+url.PathEscape(proposalID)+"/credentials/claim",
			map[string]any{"user_pub_key": pubKey},
			false, &claimed,
		); err != nil {
			return nil, fmt.Errorf("领取凭证: %w", err)
		}
		credentials = append(credentials, credential{
			userID:    claimed.UserID,
			pubKey:    claimed.UserPubKey,
			signature: claimed.Signature,
		})
	}
	return credentials, nil
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

// runVotes 并发提交选票。fixedNullifier 非空时所有请求都用它（防重投复核）。
func runVotes(
	client *http.Client,
	opts *options,
	proposalID string,
	encodedCiphertext string,
	credentials []credential,
	fixedNullifier string,
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
				outcomes[index] = submitVote(client, opts, proposalID, encodedCiphertext, credentials[index], fixedNullifier)
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
	encodedCiphertext string,
	cred credential,
	fixedNullifier string,
) outcome {
	nullifier := fixedNullifier
	if nullifier == "" {
		generated, err := randomNullifier()
		if err != nil {
			return outcome{status: 0, code: "nullifier: " + err.Error()}
		}
		nullifier = generated
	}
	payload := map[string]any{
		"proposal_id":          proposalID,
		"encrypted_vote":       encodedCiphertext,
		"proof":                devProofBase64,
		"nullifier":            nullifier,
		"user_id":              cred.userID,
		"user_pub_key":         cred.pubKey,
		"credential_signature": cred.signature,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return outcome{status: 0, code: "marshal: " + err.Error()}
	}
	request, err := http.NewRequest(
		http.MethodPost, strings.TrimRight(opts.baseURL, "/")+"/api/v1/votes", bytes.NewReader(body),
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
	if votePhase.elapsed > 0 {
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
		ExpectedVoted: len(votePhase.outcomes) + duplicateAccepted,
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
	fmt.Printf("总耗时:      %s\n", result.Duration)
	fmt.Printf("吞吐:        %.1f req/s\n", result.ThroughputPerS)
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
		return fmt.Errorf("%s (%s)", failure.Code, strings.TrimSpace(failure.Error))
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
  -json           以 JSON 输出报告`)
}
