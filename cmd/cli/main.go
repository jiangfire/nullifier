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
	"path/filepath"
	"strings"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/eligibility"
	"anonvote/internal/shares"

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

// httpError 是带 HTTP 状态码的服务端错误。
//
// 状态码是必需的：领取凭证遇到「槽位被别人抢走」时客户端要换个槽位重来，
// 判断依据就是 409 + CREDENTIAL_EXISTS。
type httpError struct {
	Status  int
	Code    string
	Message string
}

func (e *httpError) Error() string {
	// 限流是唯一一种「等一会儿就好」的错误 —— 别让它看起来像服务端坏了。
	if e.Code == "RATE_LIMITED" {
		return fmt.Sprintf("请求过于频繁，服务端要求稍等再试（%s / %s）", e.Message, e.Code)
	}
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
	// Nonce 是加密用的一次性随机数：电路约束 `a = r·G` 需要它。
	// 它只留在本地（选票文件里），prover 需要它才能出证明。
	Nonce string `json:"nonce,omitempty"`
	// OptionIndex 是选项下标（从 0 开始）：加密的是下标，电路的私有见证
	// 也是它。用指针是因为 0 是合法下标 —— omitempty 会把它吞掉。
	OptionIndex *int `json:"option_index,omitempty"`
	// NumOptions 是提案的选项数：它进公开输入。
	NumOptions int `json:"num_options,omitempty"`
	// ElectionPublicKey 是提案的计票公钥（未压缩形式，base64）。
	// 它是公开值，但证明要绑在它上面 —— 出证明时必须用同一把钥匙。
	ElectionPublicKey string `json:"election_public_key,omitempty"`
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
	case "ballot-encrypt":
		err = c.cmdBallotEncrypt(rest[1:])
	case "ballot-prove":
		err = c.cmdBallotProve(rest[1:])
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

// credentialReservation 是「预留签发权」的响应。
type credentialReservation struct {
	ProposalID       string `json:"proposal_id"`
	UserID           string `json:"user_id"`
	IssuerKeyVersion int    `json:"issuer_key_version"`
	Variant          string `json:"variant"`
	Modulus          string `json:"modulus"`
	Exponent         int    `json:"exponent"`
}

// blindSignatureResponse 是盲签名的响应。
type blindSignatureResponse struct {
	BlindSignature string `json:"blind_signature"`
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

// claimMaxAttempts 是匿名领取的重试次数。
//
// 预留**不占位**：服务端取槽位时不改状态，所以并发领取会抢到同一个槽位。抢输的
// 那个换一个槽位重来 —— 但重试要重新盲化（新的 user_id 意味着新的消息），所以
// 重试只能在客户端做。
const claimMaxAttempts = 5

// runBlindSignatureFlow 走完真两段式盲签名的**客户端侧**，并在槽位被抢时重试。
//
// attempts 是最大尝试次数：匿名领取走 claimMaxAttempts（槽位会撞车），管理端
// 签发走 1（user_id 是指定的，重试没有意义 —— 那个 user_id 要么已经领过，要么
// 永远领不到）。
func (c *cli) runBlindSignatureFlow(
	userPubKey []byte,
	attempts int,
	reserve func() (*credentialReservation, error),
	blindSign func(*credentialReservation, []byte) ([]byte, error),
) (*credentialFile, error) {
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		credential, err := c.runBlindSignatureOnce(userPubKey, reserve, blindSign)
		if err == nil {
			return credential, nil
		}
		if !isSlotTaken(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// runBlindSignatureOnce 走一遍真两段式盲签名的客户端侧流程：
//
// ① 预留签发权（拿到 user_id 与签发方公钥）
// ② 本地把凭证消息盲化
// ③ 把盲化消息送去盲签名
// ④ 本地解盲，得到最终签名
// ⑤ 交回最终签名，服务端验签后落库
//
// 预留与盲签名由调用方提供 —— 匿名领取与管理端签发走的是不同的端点，但中间
// 这四步完全一样。
//
// 注意客户端**从头到尾没把公钥发给服务端**（第 ⑤ 步才发）：服务端一旦知道
// 公钥就能算出消息原文，盲化当场失效。
func (c *cli) runBlindSignatureOnce(
	userPubKey []byte,
	reserve func() (*credentialReservation, error),
	blindSign func(*credentialReservation, []byte) ([]byte, error),
) (*credentialFile, error) {
	reservation, err := reserve()
	if err != nil {
		return nil, err
	}
	issuerKey, err := reservation.issuerPublicKey()
	if err != nil {
		return nil, err
	}
	client, err := cryptoinfra.NewCredentialBlindClient(issuerKey)
	if err != nil {
		return nil, err
	}
	blinded, err := client.Blind(reservation.UserID, userPubKey)
	if err != nil {
		return nil, err
	}
	blindSignature, err := blindSign(reservation, blinded.BlindedMessage)
	if err != nil {
		return nil, err
	}
	signature, err := client.Finalize(blinded, blindSignature)
	if err != nil {
		return nil, err
	}

	var credential credentialFile
	attestPath := fmt.Sprintf(
		"/api/v1/proposals/%s/credentials/attest", url.PathEscape(reservation.ProposalID),
	)
	attestBody := map[string]any{
		"user_id":      reservation.UserID,
		"user_pub_key": base64.StdEncoding.EncodeToString(userPubKey),
		"signature":    base64.StdEncoding.EncodeToString(signature),
	}
	if err := c.postJSON(attestPath, attestBody, false, &credential); err != nil {
		return nil, err
	}
	return &credential, nil
}

// resolveUserPubKey 取命令行的 -pubkey，没给就随机生成一个。
//
// 公钥是客户端的一次性密钥：它只在 attest 那一步发给服务端。
func resolveUserPubKey(pubKey string) ([]byte, error) {
	trimmed := strings.TrimSpace(pubKey)
	if trimmed == "" {
		generated, err := randomBase64(32)
		if err != nil {
			return nil, err
		}
		trimmed = generated
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("pubkey must be base64: %w", err)
	}
	return decoded, nil
}

// cmdIssueCredential 管理端为一个指定的 user_id 签发凭证。
//
// 走的是**真两段式盲签名**：管理端客户端在本地盲化凭证消息，服务端只见盲化
// 消息，客户端解盲后再把最终签名交回去验签。
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
	proposal := strings.TrimSpace(*proposalID)
	user := strings.TrimSpace(*userID)
	if proposal == "" || user == "" {
		return errors.New("必须提供 -proposal -user")
	}
	userPubKey, err := resolveUserPubKey(*pubKey)
	if err != nil {
		return err
	}

	credential, err := c.runBlindSignatureFlow(
		userPubKey,
		1,
		func() (*credentialReservation, error) {
			var reservation credentialReservation
			body := map[string]any{"proposal_id": proposal, "user_id": user}
			if err := c.postJSON("/api/v1/credentials", body, true, &reservation); err != nil {
				return nil, err
			}
			return &reservation, nil
		},
		func(reservation *credentialReservation, blinded []byte) ([]byte, error) {
			var response blindSignatureResponse
			body := map[string]any{
				"proposal_id":        reservation.ProposalID,
				"user_id":            reservation.UserID,
				"blinded_message":    base64.StdEncoding.EncodeToString(blinded),
				"issuer_key_version": reservation.IssuerKeyVersion,
			}
			if err := c.postJSON("/api/v1/credentials/blind-sign", body, true, &response); err != nil {
				return nil, err
			}
			return base64.StdEncoding.DecodeString(response.BlindSignature)
		},
	)
	if err != nil {
		return err
	}
	credential.Version = 1
	credential.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	return c.writeCredential(*credential, *outFile)
}

// cmdClaimCredential 匿名领取一张凭证。
//
// 领取请求里没有任何身份信息：服务端随机给出一个池槽位（user_id），运营方
// 因此不知道谁领了哪一个。客户端本地盲化凭证消息 → 服务端盲签名 → 客户端
// 解盲 → 服务端验签落库。
//
// 带 `-eligibility <成员包>` 时多一步「证明我有资格」：先向服务端要一次
// 挑战，用成员包与资格 proving key 在本地出零知识证明，再把证明带进领取
// 请求。服务端据此强制「一人一票」—— 它只知道「某个资格用过」，不知道是谁。
func (c *cli) cmdClaimCredential(args []string) error {
	fs := flag.NewFlagSet("credential-claim", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	proposalID := fs.String("proposal", "", "proposal id")
	pubKey := fs.String("pubkey", "", "base64 public key; default random")
	outFile := fs.String("out", "", "optional output json file")
	eligibilityFile := fs.String("eligibility", "", "member package json (enables one-person-one-vote)")
	provingKeyPath := fs.String("proving-key", "", "eligibility proving key (required with -eligibility)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	proposal := strings.TrimSpace(*proposalID)
	if proposal == "" {
		return errors.New("必须提供 -proposal")
	}
	userPubKey, err := resolveUserPubKey(*pubKey)
	if err != nil {
		return err
	}

	// 资格材料在**领取之前**备好：证明不过就不该消耗池槽位。
	var claimBody map[string]any
	if strings.TrimSpace(*eligibilityFile) != "" {
		claimBody, err = c.buildEligibilityMaterial(proposal, *eligibilityFile, *provingKeyPath)
		if err != nil {
			return err
		}
	}

	credential, err := c.runBlindSignatureFlow(
		userPubKey,
		claimMaxAttempts,
		func() (*credentialReservation, error) {
			var reservation credentialReservation
			path := fmt.Sprintf(
				"/api/v1/proposals/%s/credentials/claim", url.PathEscape(proposal),
			)
			if err := c.postJSON(path, claimBody, false, &reservation); err != nil {
				return nil, err
			}
			return &reservation, nil
		},
		func(reservation *credentialReservation, blinded []byte) ([]byte, error) {
			var response blindSignatureResponse
			path := fmt.Sprintf(
				"/api/v1/proposals/%s/credentials/blind-sign", url.PathEscape(proposal),
			)
			body := map[string]any{
				"user_id":            reservation.UserID,
				"blinded_message":    base64.StdEncoding.EncodeToString(blinded),
				"issuer_key_version": reservation.IssuerKeyVersion,
			}
			if err := c.postJSON(path, body, false, &response); err != nil {
				return nil, err
			}
			return base64.StdEncoding.DecodeString(response.BlindSignature)
		},
	)
	if err != nil {
		return err
	}
	credential.Version = 1
	credential.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	return c.writeCredential(*credential, *outFile)
}

// eligibilityChallengeResponse 是领取挑战端点的响应。
type eligibilityChallengeResponse struct {
	ProposalID string `json:"proposal_id"`
	Root       string `json:"root"`
	Depth      int    `json:"depth"`
	Challenge  string `json:"challenge"`
}

// buildEligibilityMaterial 备好领取请求里的资格材料。
//
// 三步：① 向服务端要一次挑战（root 一律以服务端为准，不信成员包里的）；
// ② 用成员包与资格 proving key 在本地出零知识证明；③ 把证明、去重键与
// 挑战拼成请求体。
//
// proving key 只在客户端：它不进服务端，服务端拿到的只有证明本身。
func (c *cli) buildEligibilityMaterial(
	proposalID string,
	memberPackagePath string,
	provingKeyPath string,
) (map[string]any, error) {
	if strings.TrimSpace(provingKeyPath) == "" {
		return nil, errors.New("必须提供 -proving-key（资格证明的 proving key，只在客户端）")
	}

	var pack eligibility.MemberPackage
	if err := readJSON(memberPackagePath, &pack); err != nil {
		return nil, fmt.Errorf("读取资格包失败: %w", err)
	}

	prover, err := eligibility.NewProver(provingKeyPath)
	if err != nil {
		return nil, fmt.Errorf("加载资格 proving key 失败: %w", err)
	}

	var challenge eligibilityChallengeResponse
	path := fmt.Sprintf(
		"/api/v1/proposals/%s/credentials/eligibility-challenge", url.PathEscape(proposalID),
	)
	if err := c.postJSON(path, map[string]any{}, false, &challenge); err != nil {
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

// writeCredential 把凭证写到文件或标准输出。
func (c *cli) writeCredential(credential credentialFile, outFile string) error {
	if outFile != "" {
		if err := writeJSON(outFile, credential); err != nil {
			return err
		}
		fmt.Printf("凭证已写入 %s\n", outFile)
		return nil
	}
	printJSON(credential)
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

	ciphertext, nonce, err := ballot.EncryptBallotWithUncompressedKey(publicKey, uint64(index))
	if err != nil {
		return fmt.Errorf("encrypt ballot: %w", err)
	}
	nonceBytes := nonce.Bytes()

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
		// 以下四项是本地出证明的见证材料：它们只留在选票文件里，
		// 提交给服务端的只有密文、nullifier 与 proof。
		Nonce:             base64.StdEncoding.EncodeToString(nonceBytes[:]),
		OptionIndex:       &index,
		NumOptions:        len(options),
		ElectionPublicKey: base64.StdEncoding.EncodeToString(publicKey),
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

// cmdBallotProve 在本地为选票生成零知识证明。
//
// 证明完全在客户端生成：服务端只收到「密文 + nullifier + proof」，看不到
// 明文选项，也不需要知道 proving key（它只在客户端侧）。
//
// 公开输入与服务端用**同一套**拼装逻辑（ballot.BuildPublicInputs）：客户端
// 与服务端算出同一份公开输入，证明才验得过。选票文件里的 nonce、选项下标、
// nullifier 秘密与计票公钥就是拼这份输入要用的材料 —— 它们全部只留在本地。
func (c *cli) cmdBallotProve(args []string) error {
	fs := flag.NewFlagSet("ballot-prove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ballotPath := fs.String("ballot-file", "", "ballot json file")
	provingKey := fs.String("proving-key", "", "proving key file (client side only)")
	outFile := fs.String("out", "", "optional output json file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*ballotPath) == "" || strings.TrimSpace(*provingKey) == "" {
		return errors.New("必须提供 -ballot-file -proving-key")
	}
	var ballot ballotFile
	if err := readJSON(*ballotPath, &ballot); err != nil {
		return err
	}
	proof, err := proveBallotFile(ballot, strings.TrimSpace(*provingKey))
	if err != nil {
		return err
	}
	ballot.Proof = base64.StdEncoding.EncodeToString(proof)
	if *outFile != "" {
		if err := writeJSON(*outFile, ballot); err != nil {
			return err
		}
		fmt.Printf("选票证明已写入 %s\n", *outFile)
		return nil
	}
	printJSON(ballot)
	return nil
}

// proveBallotFile 用选票文件里的见证在本地生成证明。
func proveBallotFile(bf ballotFile, provingKeyPath string) ([]byte, error) {
	if strings.TrimSpace(bf.ProposalID) == "" {
		return nil, errors.New("选票文件里没有 proposal_id")
	}
	if bf.OptionIndex == nil {
		return nil, errors.New("选票文件里没有 option_index（用 ballot-encrypt 生成选票）")
	}
	if bf.NumOptions <= 0 {
		return nil, errors.New("选票文件里没有 num_options（用 ballot-encrypt 生成选票）")
	}
	if strings.TrimSpace(bf.ElectionPublicKey) == "" {
		return nil, errors.New("选票文件里没有 election_public_key（用 ballot-encrypt 生成选票）")
	}
	if strings.TrimSpace(bf.Nonce) == "" {
		return nil, errors.New("选票文件里没有 nonce（用 ballot-encrypt 生成选票）")
	}
	if strings.TrimSpace(bf.NullifierSecret) == "" {
		return nil, errors.New("选票文件里没有 nullifier_secret（用 ballot-encrypt 生成选票）")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(bf.EncryptedVote)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted_vote: %w", err)
	}
	nullifier, err := hex.DecodeString(strings.TrimSpace(bf.Nullifier))
	if err != nil {
		return nil, fmt.Errorf("decode nullifier: %w", err)
	}
	secret, err := base64.StdEncoding.DecodeString(bf.NullifierSecret)
	if err != nil {
		return nil, fmt.Errorf("decode nullifier_secret: %w", err)
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(bf.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	uncompressedKey, err := base64.StdEncoding.DecodeString(bf.ElectionPublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode election_public_key: %w", err)
	}
	// 服务端拼公开输入时用的是压缩公钥：客户端要转成同一形式，
	// 两边算出来的公开输入才逐字段相同。
	compressedKey, err := ballot.CompressPoint(uncompressedKey)
	if err != nil {
		return nil, fmt.Errorf("compress election public key: %w", err)
	}

	inputs, err := ballot.BuildPublicInputs(entity.BallotStatement{
		ProposalID:        bf.ProposalID,
		Nullifier:         nullifier,
		ElectionPublicKey: compressedKey,
		Ciphertext:        ciphertext,
		NumOptions:        bf.NumOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("build public inputs: %w", err)
	}

	var secretElement, nonceElement fr.Element
	secretElement.SetBytes(secret)
	nonceElement.SetBytes(nonceBytes)

	prover, err := ballot.NewProver(provingKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load proving key: %w", err)
	}
	proof, err := prover.Prove(inputs, uint64(*bf.OptionIndex), nonceElement, secretElement)
	if err != nil {
		return nil, fmt.Errorf("prove ballot: %w", err)
	}
	return proof, nil
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
		"encrypted_vote":       ballot.EncryptedVote,
		"proof":                ballot.Proof,
		"nullifier":            ballot.Nullifier,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	}
	// 不发送 selected_option：服务端不消费这个字段，而明文选项上服务端
	// 会抵消「选票是加密的」这个属性。
	// 也不发送 proposal_id：它只从路径来（见 docs/api-contract-v1.md）。
	var out map[string]any
	path := fmt.Sprintf(
		"/api/v1/proposals/%s/votes",
		url.PathEscape(proposalID),
	)
	if err := c.postJSON(path, body, false, &out); err != nil {
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
	sharesFlag := fs.String("shares", "", "comma separated shares")
	sharesFileFlag := fs.String("shares-file", "", "json/text file containing shares")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proposalID) == "" {
		return errors.New("必须提供 -proposal")
	}
	values, err := shares.Load(*sharesFlag, *sharesFileFlag)
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
			return &httpError{
				Status:  resp.StatusCode,
				Code:    apiErr.Code,
				Message: apiErr.Error,
			}
		}
		return &httpError{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
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
	return shares.SplitCSV(raw)
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
                     [-eligibility 包.json -proving-key 资格.key]  (带资格证明领取:一人一票)
  credential-show   -file
  ballot-encrypt    -proposal -option [-proof] [-out]   (本地加密:服务端看不到明文选项)
  ballot-prove      -ballot-file -proving-key [-out]     (本地出证明:proving key 只在客户端)
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
  go run ./cmd/cli credential-claim -proposal p-1 -eligibility eligibility-me.json -proving-key keys/eligibility-proving.key -out cred.json
  go run ./cmd/cli ballot-encrypt -proposal p-1 -option A -out ballot.json
  go run ./cmd/cli ballot-prove -ballot-file ballot.json -proving-key keys/proving.key -out ballot.json
  go run ./cmd/cli vote-submit -credential-file cred-u1001.json -ballot-file ballot.json
  go run ./cmd/cli -admin-token dev-admin-token decrypt-setup -proposal p-1 -out shares.json
  go run ./cmd/cli -admin-token dev-admin-token credentials-list -proposal p-1`)
}
