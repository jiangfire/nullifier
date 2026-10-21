package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

type cli struct {
	baseURL    string
	adminToken string
	httpClient *http.Client
}

type apiError struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

type credentialFile struct {
	Version          int    `json:"version"`
	ProposalID       string `json:"proposal_id"`
	UserID           string `json:"user_id"`
	UserPubKey       string `json:"user_pub_key"`
	Signature        string `json:"signature"`
	IssuerKeyVersion any    `json:"issuer_key_version"`
	IssuedAt         string `json:"issued_at"`
	ExportedAt       string `json:"exported_at,omitempty"`
}

type ballotFile struct {
	ProposalID     string `json:"proposal_id,omitempty"`
	SelectedOption string `json:"selected_option,omitempty"`
	EncryptedVote  string `json:"encrypted_vote"`
	Proof          string `json:"proof"`
	Nullifier      string `json:"nullifier"`
	// NullifierSecret 是 nullifier 秘密：nullifier 由它单向推导出来。
	// 它只留在本地（选票文件里），prover 需要它才能满足电路里的
	// nullifier 约束。**不要把它提交给服务端。**
	NullifierSecret string `json:"nullifier_secret,omitempty"`
}

// version 由构建时的 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	baseURL := envOrDefault("NULLIFIER_BASE_URL", "http://localhost:8080")
	adminToken := strings.TrimSpace(os.Getenv("NULLIFIER_ADMIN_TOKEN"))

	root := flag.NewFlagSet("nullifier-cli", flag.ContinueOnError)
	root.SetOutput(io.Discard)
	root.StringVar(&baseURL, "base-url", baseURL, "API base URL")
	root.StringVar(&adminToken, "admin-token", adminToken, "admin bearer token")
	if err := root.Parse(args); err != nil {
		printRootUsage()
		fmt.Fprintf(os.Stderr, "\n参数错误: %v\n", err)
		return 2
	}

	rest := root.Args()
	if len(rest) == 0 {
		printRootUsage()
		return 2
	}

	c := &cli{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminToken: strings.TrimSpace(adminToken),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}

	var err error
	switch rest[0] {
	case "health":
		err = c.cmdHealth(rest[1:])
	case "proposals-open":
		err = c.cmdOpenProposals(rest[1:])
	case "proposal-create":
		err = c.cmdCreateProposal(rest[1:])
	case "credential-issue":
		err = c.cmdIssueCredential(rest[1:])
	case "credential-claim":
		err = c.cmdClaimCredential(rest[1:])
	case "credential-show":
		err = c.cmdShowCredential(rest[1:])
	case "ballot-draft":
		err = c.cmdBallotDraft(rest[1:])
	case "ballot-encrypt":
		err = c.cmdBallotEncrypt(rest[1:])
	case "vote-submit":
		err = c.cmdVoteSubmit(rest[1:])
	case "vote-status":
		err = c.cmdVoteStatus(rest[1:])
	case "credentials-list":
		err = c.cmdCredentialStatuses(rest[1:])
	case "decrypt-setup":
		err = c.cmdDecryptSetup(rest[1:])
	case "result-publish":
		err = c.cmdPublishResult(rest[1:])
	case "result-get":
		err = c.cmdGetResult(rest[1:])
	case "anchors-list":
		err = c.cmdListAnchors(rest[1:])
	case "help", "-h", "--help":
		printRootUsage()
		return 0
	case "version":
		fmt.Println(version)
		return 0
	default:
		printRootUsage()
		fmt.Fprintf(os.Stderr, "\n未知命令: %s\n", rest[0])
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
	return 0
}

func (c *cli) cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var out map[string]any
	if err := c.getJSON("/healthz", nil, false, &out); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdOpenProposals(args []string) error {
	fs := flag.NewFlagSet("proposals-open", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var out map[string]any
	if err := c.getJSON("/api/v1/proposals/open", nil, false, &out); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdCreateProposal(args []string) error {
	fs := flag.NewFlagSet("proposal-create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	title := fs.String("title", "", "proposal title")
	description := fs.String("description", "", "proposal description")
	options := fs.String("options", "", "comma separated options")
	start := fs.String("start", "", "RFC3339 starts_at")
	end := fs.String("end", "", "RFC3339 ends_at")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" || strings.TrimSpace(*start) == "" || strings.TrimSpace(*end) == "" {
		return errors.New("必须提供 -title -start -end")
	}
	desc := strings.TrimSpace(*description)
	body := map[string]any{
		"title":       strings.TrimSpace(*title),
		"description": desc,
		"options":     splitCSV(*options),
		"starts_at":   strings.TrimSpace(*start),
		"ends_at":     strings.TrimSpace(*end),
	}
	var out map[string]any
	if err := c.postJSON("/api/v1/proposals", body, true, &out); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdIssueCredential(args []string) error {
	fs := flag.NewFlagSet("credential-issue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	userID := fs.String("user", "", "user id")
	pubKey := fs.String("pubkey", "", "base64 public key; default random")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" || strings.TrimSpace(*userID) == "" {
		return errors.New("必须提供 -proposal -user")
	}
	userPubKey := strings.TrimSpace(*pubKey)
	if userPubKey == "" {
		generated, err := randomBase64(32)
		if err != nil {
			return err
		}
		userPubKey = generated
	}
	body := map[string]any{
		"proposal_id":  strings.TrimSpace(*proposalID),
		"user_id":      strings.TrimSpace(*userID),
		"user_pub_key": userPubKey,
	}
	var out credentialFile
	if err := c.postJSON("/api/v1/credentials", body, true, &out); err != nil {
		return err
	}
	out.Version = 1
	out.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	if *outFile != "" {
		if err := writeJSON(*outFile, out); err != nil {
			return err
		}
		fmt.Printf("凭证已写入 %s\n", *outFile)
		return nil
	}
	printJSON(out)
	return nil
}

// cmdClaimCredential 匿名领取一张凭证。
//
// 领取请求里只有自己生成的公钥，没有任何身份信息：服务端随机给出一个凭证，
// 因此服务端与运营方都无法把凭证和投票人对应起来。这条路径是公开的
// （不需要 admin token），与门户的「领取凭证」按钮是同一个接口。
func (c *cli) cmdClaimCredential(args []string) error {
	fs := flag.NewFlagSet("credential-claim", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	pubKey := fs.String("pubkey", "", "base64 public key; default random")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	proposal := strings.TrimSpace(*proposalID)
	if proposal == "" {
		return errors.New("必须提供 -proposal")
	}
	userPubKey := strings.TrimSpace(*pubKey)
	if userPubKey == "" {
		generated, err := randomBase64(32)
		if err != nil {
			return err
		}
		userPubKey = generated
	}

	body := map[string]any{"user_pub_key": userPubKey}
	var out credentialFile
	if err := c.postJSON(
		fmt.Sprintf("/api/v1/proposals/%s/credentials/claim", url.PathEscape(proposal)),
		body,
		false,
		&out,
	); err != nil {
		return err
	}
	out.Version = 1
	out.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	if *outFile != "" {
		if err := writeJSON(*outFile, out); err != nil {
			return err
		}
		fmt.Printf("凭证已写入 %s\n", *outFile)
		return nil
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdShowCredential(args []string) error {
	fs := flag.NewFlagSet("credential-show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "credential json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("必须提供 -file")
	}
	var credential credentialFile
	if err := readJSON(*file, &credential); err != nil {
		return err
	}
	printJSON(credential)
	return nil
}

func (c *cli) cmdBallotDraft(args []string) error {
	fs := flag.NewFlagSet("ballot-draft", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	option := fs.String("option", "", "vote option")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" || strings.TrimSpace(*option) == "" {
		return errors.New("必须提供 -proposal -option")
	}
	var raw map[string]any
	if err := c.postJSON(
		fmt.Sprintf("/api/v1/proposals/%s/ballot-draft", url.PathEscape(strings.TrimSpace(*proposalID))),
		map[string]any{"option": strings.TrimSpace(*option)},
		false,
		&raw,
	); err != nil {
		return err
	}
	ballot := ballotFile{
		ProposalID:     stringify(raw["proposal_id"]),
		SelectedOption: stringify(raw["option"]),
		EncryptedVote:  stringify(raw["encrypted_vote"]),
		Proof:          stringify(raw["proof"]),
		Nullifier:      stringify(raw["nullifier"]),
	}
	if *outFile != "" {
		if err := writeJSON(*outFile, ballot); err != nil {
			return err
		}
		fmt.Printf("选票草稿已写入 %s\n", *outFile)
		return nil
	}
	printJSON(ballot)
	return nil
}

// devProofBase64 是开发模式的占位 proof。
//
// 服务端在 crypto.vk_path 为空时只要求 proof 非空。release 模式必须用
// 本地 prover 产出的真实证明（用 -proof 传入）。
const devProofBase64 = "ZGV2LXByb29m"

// cmdBallotEncrypt 在本地加密选票。
//
// 计票公钥是公开的，客户端拿到它就能自己把选项加密成密文：**服务端全程看不到
// 明文选项**。这是能力 A（选票内容对服务端不可知）的客户端一半。
//
// proof 由本地 prover 提供（-proof）；不提供时用开发占位值，只能在开发模式下
// 通过验签。nullifier 在本地生成，并与提案 ID 绑定（电路约束要求两者一致），
// 秘密只留在选票文件里。
func (c *cli) cmdBallotEncrypt(args []string) error {
	fs := flag.NewFlagSet("ballot-encrypt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	option := fs.String("option", "", "vote option")
	proof := fs.String("proof", "", "base64 proof from a local prover (default: dev placeholder)")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	proposal := strings.TrimSpace(*proposalID)
	candidate := strings.TrimSpace(*option)
	if proposal == "" || candidate == "" {
		return errors.New("必须提供 -proposal -option")
	}

	// 提案的选项顺序决定选项下标 —— 下标是密文与电路共用的口径。
	options, err := c.proposalOptions(proposal)
	if err != nil {
		return err
	}
	index := -1
	for i, item := range options {
		if item == candidate {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("提案 %s 没有这个选项: %s", proposal, candidate)
	}

	var key struct {
		PublicKey string `json:"public_key"`
	}
	if err := c.getJSON(
		fmt.Sprintf("/api/v1/proposals/%s/election-key", url.PathEscape(proposal)),
		nil,
		false,
		&key,
	); err != nil {
		return err
	}
	publicKey, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		return fmt.Errorf("decode election public key: %w", err)
	}

	ciphertext, _, err := ballot.EncryptBallotWithUncompressedKey(publicKey, uint64(index))
	if err != nil {
		return fmt.Errorf("encrypt ballot: %w", err)
	}

	// nullifier 与提案绑定：电路约束是 nullifier = MiMC(secret, proposal_id)，
	// 秘密留在本地，prover 需要它。
	secret, err := ballot.GenerateNullifierSecret()
	if err != nil {
		return fmt.Errorf("generate nullifier secret: %w", err)
	}
	var secretElement fr.Element
	secretElement.SetBytes(secret)
	nullifier := ballot.DeriveNullifier(secretElement, ballot.DeriveProposalField(proposal))

	proofValue := strings.TrimSpace(*proof)
	if proofValue == "" {
		proofValue = devProofBase64
	}

	out := ballotFile{
		ProposalID:      proposal,
		SelectedOption:  candidate,
		EncryptedVote:   base64.StdEncoding.EncodeToString(ciphertext),
		Proof:           proofValue,
		Nullifier:       ballot.EncodeScalarHex(nullifier),
		NullifierSecret: base64.StdEncoding.EncodeToString(secret),
	}
	if *outFile != "" {
		if err := writeJSON(*outFile, out); err != nil {
			return err
		}
		fmt.Printf("本地加密的选票已写入 %s\n", *outFile)
		return nil
	}
	printJSON(out)
	return nil
}

// proposalOptions 查出提案的选项列表，顺序即选项下标。
func (c *cli) proposalOptions(proposalID string) ([]string, error) {
	var list struct {
		Items []struct {
			ID      string   `json:"id"`
			Options []string `json:"options"`
		} `json:"items"`
	}
	if err := c.getJSON("/api/v1/proposals", nil, false, &list); err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.ID == proposalID {
			return item.Options, nil
		}
	}
	return nil, fmt.Errorf("找不到提案: %s", proposalID)
}

func (c *cli) cmdVoteSubmit(args []string) error {
	fs := flag.NewFlagSet("vote-submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	credentialPath := fs.String("credential-file", "", "credential json file")
	ballotPath := fs.String("ballot-file", "", "ballot json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *credentialPath == "" || *ballotPath == "" {
		return errors.New("必须提供 -credential-file -ballot-file")
	}
	var credential credentialFile
	if err := readJSON(*credentialPath, &credential); err != nil {
		return err
	}
	var ballot ballotFile
	if err := readJSON(*ballotPath, &ballot); err != nil {
		return err
	}
	proposalID := strings.TrimSpace(firstNonEmpty(ballot.ProposalID, credential.ProposalID))
	if proposalID == "" {
		return errors.New("凭证和选票里都没有 proposal_id")
	}
	body := map[string]any{
		"proposal_id":          proposalID,
		"encrypted_vote":       ballot.EncryptedVote,
		"proof":                ballot.Proof,
		"nullifier":            ballot.Nullifier,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	}
	// 不发送 selected_option：服务端不消费这个字段，而明文选项上服务端
	// 会抵消「选票是加密的」这个属性。
	var out map[string]any
	if err := c.postJSON("/api/v1/votes", body, false, &out); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdVoteStatus(args []string) error {
	fs := flag.NewFlagSet("vote-status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	credentialPath := fs.String("credential-file", "", "credential json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" || strings.TrimSpace(*credentialPath) == "" {
		return errors.New("必须提供 -proposal -credential-file")
	}
	var credential credentialFile
	if err := readJSON(*credentialPath, &credential); err != nil {
		return err
	}
	var out map[string]any
	if err := c.postJSON(
		fmt.Sprintf("/api/v1/proposals/%s/vote-status", url.PathEscape(strings.TrimSpace(*proposalID))),
		map[string]any{
			"user_id":              credential.UserID,
			"user_pub_key":         credential.UserPubKey,
			"credential_signature": credential.Signature,
		},
		false,
		&out,
	); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdCredentialStatuses(args []string) error {
	fs := flag.NewFlagSet("credentials-list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	var out map[string]any
	if err := c.getJSON(
		fmt.Sprintf("/api/v1/proposals/%s/credentials", url.PathEscape(strings.TrimSpace(*proposalID))),
		nil,
		true,
		&out,
	); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdDecryptSetup(args []string) error {
	fs := flag.NewFlagSet("decrypt-setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	threshold := fs.Int("threshold", 2, "threshold")
	trustees := fs.Int("trustees", 3, "trustees")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	if strings.TrimSpace(*outFile) == "" {
		// 门限份额只返回一次，服务端不保存明文；必须落到文件里，不能打到 stdout。
		return errors.New("必须提供 -out：门限解密份额只会返回一次，需要保存到文件")
	}
	var out map[string]any
	if err := c.postJSON(
		fmt.Sprintf("/api/v1/proposals/%s/decryption/setup", url.PathEscape(strings.TrimSpace(*proposalID))),
		map[string]any{"threshold": *threshold, "trustees": *trustees},
		true,
		&out,
	); err != nil {
		return err
	}
	if err := writeJSON(*outFile, out); err != nil {
		return err
	}
	fmt.Printf("解密会话已写入 %s\n", *outFile)
	return nil
}

func (c *cli) cmdPublishResult(args []string) error {
	fs := flag.NewFlagSet("result-publish", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	shares := fs.String("shares", "", "comma separated shares")
	sharesFile := fs.String("shares-file", "", "json/text file containing shares")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	values, err := loadShares(*shares, *sharesFile)
	if err != nil {
		return err
	}
	var out map[string]any
	if err := c.postJSON(
		fmt.Sprintf("/api/v1/proposals/%s/results/publish", url.PathEscape(strings.TrimSpace(*proposalID))),
		map[string]any{"shares": values},
		true,
		&out,
	); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdGetResult(args []string) error {
	fs := flag.NewFlagSet("result-get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	var out map[string]any
	if err := c.getJSON(
		fmt.Sprintf("/api/v1/proposals/%s/results", url.PathEscape(strings.TrimSpace(*proposalID))),
		nil,
		false,
		&out,
	); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) cmdListAnchors(args []string) error {
	fs := flag.NewFlagSet("anchors-list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	var out map[string]any
	if err := c.getJSON(
		fmt.Sprintf("/api/v1/proposals/%s/audit-anchors", url.PathEscape(strings.TrimSpace(*proposalID))),
		nil,
		false,
		&out,
	); err != nil {
		return err
	}
	printJSON(out)
	return nil
}

func (c *cli) getJSON(path string, query map[string]string, admin bool, out any) error {
	u, err := c.buildURL(path, query)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, admin, out)
}

func (c *cli) postJSON(path string, body any, admin bool, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	u, err := c.buildURL(path, nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, admin, out)
}

func (c *cli) do(req *http.Request, admin bool, out any) error {
	if admin {
		if c.adminToken == "" {
			return errors.New("该命令需要 admin token，请传 -admin-token 或设置 NULLIFIER_ADMIN_TOKEN")
		}
		req.Header.Set("Authorization", "Bearer "+c.adminToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			if apiErr.Code != "" {
				return fmt.Errorf("%s (%s)", apiErr.Error, apiErr.Code)
			}
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *cli) buildURL(path string, query map[string]string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	// path 的各段已经被 url.PathEscape 转义过一次。base.Path 要放解码后的形式，
	// base.RawPath 放转义后的形式；否则 url.URL.String() 会再转义一次，
	// 含空格或 "/" 的 id 会变成 %2520 / %252F。
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return "", err
	}
	basePrefix := strings.TrimRight(base.Path, "/")
	baseEscapedPrefix := strings.TrimRight(base.EscapedPath(), "/")
	base.Path = basePrefix + decoded
	base.RawPath = baseEscapedPrefix + path
	values := base.Query()
	for k, v := range query {
		if strings.TrimSpace(v) != "" {
			values.Set(k, v)
		}
	}
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func printJSON(v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(raw))
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func randomBase64(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func loadShares(raw string, file string) ([]string, error) {
	if strings.TrimSpace(raw) != "" {
		values := splitCSV(raw)
		if len(values) == 0 {
			return nil, errors.New("shares 为空")
		}
		return values, nil
	}
	if strings.TrimSpace(file) == "" {
		return nil, errors.New("必须提供 -shares 或 -shares-file")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	values := strings.FieldsFunc(string(data), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("shares 文件为空")
	}
	return out, nil
}

func printRootUsage() {
	fmt.Println(`用法:
  go run ./cmd/cli [全局参数] <命令> [命令参数]

全局参数:
  -base-url       API 地址，默认 http://localhost:8080
  -admin-token    管理员 Bearer Token

命令:
  health
  version
  proposals-open
  proposal-create   -title -description -options -start -end
  credential-issue  -proposal -user [-pubkey] [-out]  (需要 admin token)
  credential-claim  -proposal [-pubkey] [-out]         (匿名领取:凭证池随机给一张)
  credential-show   -file
  ballot-draft      -proposal -option [-out]
  ballot-encrypt    -proposal -option [-proof] [-out]   (本地加密:服务端看不到明文选项)
  vote-submit       -credential-file -ballot-file
  vote-status       -proposal -credential-file
  credentials-list  -proposal
  decrypt-setup     -proposal (-out file) [-threshold 2] [-trustees 3]
  result-publish    -proposal (-shares a,b | -shares-file file)
  result-get        -proposal
  anchors-list      -proposal

示例:
  go run ./cmd/cli -admin-token dev-admin-token proposal-create -title "Board" -options "A,B" -start 2026-12-01T10:00:00Z -end 2026-12-01T12:00:00Z
  go run ./cmd/cli -admin-token dev-admin-token credential-issue -proposal p-1 -user u-1001 -out cred-u1001.json
  go run ./cmd/cli credential-claim -proposal p-1 -out cred.json
  go run ./cmd/cli ballot-draft -proposal p-1 -option A -out ballot.json
  go run ./cmd/cli ballot-encrypt -proposal p-1 -option A -out ballot.json
  go run ./cmd/cli vote-submit -credential-file cred-u1001.json -ballot-file ballot.json
  go run ./cmd/cli -admin-token dev-admin-token decrypt-setup -proposal p-1 -out shares.json
  go run ./cmd/cli -admin-token dev-admin-token credentials-list -proposal p-1`)
}
