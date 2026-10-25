package testutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
)

// 凭证盲签名的**客户端侧**测试助手。
//
// 它走真两段式盲签名的完整流程：预留槽位 → 本地盲化 → 盲签名 → 本地解盲 →
// attest。服务端全程只见盲化消息 —— 这正是路线图 T17 要验证的那件事。
//
// 中间步骤也单独暴露出来：交叉攻击测试要拿「A 的盲签名」去签 B。

// Credential 是 attest 之后拿到的凭证。
type Credential struct {
	ID               string `json:"id"`
	ProposalID       string `json:"proposal_id"`
	UserID           string `json:"user_id"`
	UserPubKey       string `json:"user_pub_key"`
	Signature        string `json:"signature"`
	IssuerKeyVersion int    `json:"issuer_key_version"`
	IssuedAt         string `json:"issued_at"`
}

// Reservation 是「预留签发权」的响应。
type Reservation struct {
	ProposalID       string `json:"proposal_id"`
	UserID           string `json:"user_id"`
	IssuerKeyVersion int    `json:"issuer_key_version"`
	Variant          string `json:"variant"`
	Modulus          string `json:"modulus"`
	Exponent         int    `json:"exponent"`
}

// BlindedCredential 是客户端本地盲化的结果。
type BlindedCredential struct {
	// BlindedMessage 是交给签发方的盲化消息。
	BlindedMessage []byte
	// client 与 state 是解盲要用的状态（不导出给服务端）。
	client *cryptoinfra.CredentialBlindClient
	state  *cryptoinfra.BlindedCredential
}

// APIError 是服务端返回的错误。
type APIError struct {
	// Status 是 HTTP 状态码。
	Status int
	// Code 是业务错误码（如 `CREDENTIAL_EXISTS`）。
	Code string
	// Message 是错误文案。
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (%s)", e.Code, e.Message)
}

// IsConflict 判断错误是不是「槽位被别人抢走」—— 只有这种情况值得重试。
//
// 409 有两种：槽位被抢（换个槽位重来就行）与凭证池已空（重试也没用）。
func IsConflict(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusConflict && apiErr.Code == "CREDENTIAL_EXISTS"
}

// claimMaxAttempts 是匿名领取的重试次数。
//
// 预留**不占位**：服务端取槽位时不改状态，所以并发领取会抢到同一个槽位。
// 抢输的那个换一个槽位重来 —— 但重试要重新盲化（新的 user_id 意味着新的
// 消息），所以重试只能在客户端做。
const claimMaxAttempts = 5

// BlindClient 是走 HTTP API 的盲签名客户端。
type BlindClient struct {
	// BaseURL 是 API 根地址。
	BaseURL string
	// AdminToken 是管理令牌：备池与管理端签发要用。
	AdminToken string

	http *http.Client
}

// NewBlindClient 构造一个客户端。
func NewBlindClient(baseURL string) *BlindClient {
	return &BlindClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Reserve 预留一个凭证池槽位（公开端点，匿名）。
func (c *BlindClient) Reserve(proposalID string) (*Reservation, error) {
	var reservation Reservation
	path := fmt.Sprintf("/api/v1/proposals/%s/credentials/claim", url.PathEscape(proposalID))
	if err := c.post(path, map[string]any{}, false, &reservation); err != nil {
		return nil, err
	}
	return &reservation, nil
}

// ReserveForUser 管理端为一个指定的 user_id 预留签发权。
func (c *BlindClient) ReserveForUser(proposalID string, userID string) (*Reservation, error) {
	var reservation Reservation
	if err := c.post(
		"/api/v1/credentials",
		map[string]any{"proposal_id": proposalID, "user_id": userID},
		true,
		&reservation,
	); err != nil {
		return nil, err
	}
	return &reservation, nil
}

// Blind 本地盲化凭证消息。
func (c *BlindClient) Blind(
	reservation *Reservation,
	userPubKey []byte,
) (*BlindedCredential, error) {
	issuerKey, err := issuerKeyOf(reservation)
	if err != nil {
		return nil, err
	}
	client, err := cryptoinfra.NewCredentialBlindClient(issuerKey)
	if err != nil {
		return nil, err
	}
	state, err := client.Blind(reservation.UserID, userPubKey)
	if err != nil {
		return nil, err
	}
	return &BlindedCredential{
		BlindedMessage: state.BlindedMessage,
		client:         client,
		state:          state,
	}, nil
}

// BlindSignPool 走公开端的盲签名（只覆盖池里还空着的槽位）。
func (c *BlindClient) BlindSignPool(
	proposalID string,
	reservation *Reservation,
	blinded []byte,
) ([]byte, error) {
	var payload struct {
		BlindSignature string `json:"blind_signature"`
	}
	path := fmt.Sprintf(
		"/api/v1/proposals/%s/credentials/blind-sign", url.PathEscape(proposalID),
	)
	body := map[string]any{
		"user_id":            reservation.UserID,
		"blinded_message":    base64.StdEncoding.EncodeToString(blinded),
		"issuer_key_version": reservation.IssuerKeyVersion,
	}
	if err := c.post(path, body, false, &payload); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(payload.BlindSignature)
}

// BlindSignAdmin 走管理端的盲签名（可以覆盖任意 user_id）。
func (c *BlindClient) BlindSignAdmin(
	proposalID string,
	reservation *Reservation,
	blinded []byte,
) ([]byte, error) {
	var payload struct {
		BlindSignature string `json:"blind_signature"`
	}
	body := map[string]any{
		"proposal_id":        proposalID,
		"user_id":            reservation.UserID,
		"blinded_message":    base64.StdEncoding.EncodeToString(blinded),
		"issuer_key_version": reservation.IssuerKeyVersion,
	}
	if err := c.post("/api/v1/credentials/blind-sign", body, true, &payload); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(payload.BlindSignature)
}

// Finalize 本地解盲，得到最终签名。
func (b *BlindedCredential) Finalize(blindSignature []byte) ([]byte, error) {
	return b.client.Finalize(b.state, blindSignature)
}

// Attest 交回最终签名，服务端验签后落库。
func (c *BlindClient) Attest(
	proposalID string,
	userID string,
	userPubKey []byte,
	signature []byte,
) (*Credential, error) {
	var credential Credential
	path := fmt.Sprintf("/api/v1/proposals/%s/credentials/attest", url.PathEscape(proposalID))
	body := map[string]any{
		"user_id":      userID,
		"user_pub_key": base64.StdEncoding.EncodeToString(userPubKey),
		"signature":    base64.StdEncoding.EncodeToString(signature),
	}
	if err := c.post(path, body, false, &credential); err != nil {
		return nil, err
	}
	return &credential, nil
}

// ClaimCredential 走完匿名领取的完整两段式流程。
//
// 槽位被抢走时会换一个槽位重来（服务端取槽位时不占位，并发领取会撞车）。
func (c *BlindClient) ClaimCredential(
	proposalID string,
	userPubKey []byte,
) (*Credential, error) {
	var lastErr error
	for attempt := 0; attempt < claimMaxAttempts; attempt++ {
		credential, err := c.claimOnce(proposalID, userPubKey)
		if err == nil {
			return credential, nil
		}
		if !IsConflict(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// claimOnce 走一遍匿名领取。
func (c *BlindClient) claimOnce(
	proposalID string,
	userPubKey []byte,
) (*Credential, error) {
	reservation, err := c.Reserve(proposalID)
	if err != nil {
		return nil, err
	}
	blinded, err := c.Blind(reservation, userPubKey)
	if err != nil {
		return nil, err
	}
	blindSignature, err := c.BlindSignPool(proposalID, reservation, blinded.BlindedMessage)
	if err != nil {
		return nil, err
	}
	signature, err := blinded.Finalize(blindSignature)
	if err != nil {
		return nil, err
	}
	return c.Attest(proposalID, reservation.UserID, userPubKey, signature)
}

// IssueCredential 走完管理端签发的完整两段式流程。
func (c *BlindClient) IssueCredential(
	proposalID string,
	userID string,
	userPubKey []byte,
) (*Credential, error) {
	reservation, err := c.ReserveForUser(proposalID, userID)
	if err != nil {
		return nil, err
	}
	blinded, err := c.Blind(reservation, userPubKey)
	if err != nil {
		return nil, err
	}
	blindSignature, err := c.BlindSignAdmin(proposalID, reservation, blinded.BlindedMessage)
	if err != nil {
		return nil, err
	}
	signature, err := blinded.Finalize(blindSignature)
	if err != nil {
		return nil, err
	}
	return c.Attest(proposalID, reservation.UserID, userPubKey, signature)
}

// FillCredentialPool 管理端备池。
func (c *BlindClient) FillCredentialPool(proposalID string, count int) error {
	path := fmt.Sprintf(
		"/api/v1/proposals/%s/credentials/pool", url.PathEscape(proposalID),
	)
	return c.post(path, map[string]any{"count": count}, true, nil)
}

// issuerKeyOf 把预留响应里的公钥字段还原成领域类型。
func issuerKeyOf(reservation *Reservation) (*domaincrypto.IssuerPublicKey, error) {
	modulus, err := base64.StdEncoding.DecodeString(reservation.Modulus)
	if err != nil {
		return nil, fmt.Errorf("issuer modulus must be base64: %w", err)
	}
	return &domaincrypto.IssuerPublicKey{
		Version:  reservation.IssuerKeyVersion,
		Modulus:  modulus,
		Exponent: reservation.Exponent,
		Variant:  reservation.Variant,
	}, nil
}

func (c *BlindClient) post(path string, body any, admin bool, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(
		http.MethodPost, c.BaseURL+path, bytes.NewReader(raw),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if admin {
		request.Header.Set("Authorization", "Bearer "+c.AdminToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(payload, &failure)
		if failure.Code == "" {
			failure.Code = http.StatusText(response.StatusCode)
		}
		return &APIError{
			Status:  response.StatusCode,
			Code:    failure.Code,
			Message: strings.TrimSpace(failure.Error),
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(payload, out)
}
