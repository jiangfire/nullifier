package httpapi

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

type Handler struct {
	votingUC        *usecase.VotingUseCase
	registerUC      *usecase.RegisterUseCase
	proposalUC      *usecase.ProposalUseCase
	participationUC *usecase.ParticipationUseCase
	tallyUC         *usecase.TallyUseCase
	validate        *validator.Validate
	debugMode       bool
	logger          *zap.Logger
}

func NewHandler(
	votingUC *usecase.VotingUseCase,
	registerUC *usecase.RegisterUseCase,
	proposalUC *usecase.ProposalUseCase,
	participationUC *usecase.ParticipationUseCase,
	logger *zap.Logger,
) *Handler {
	return NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, nil, logger)
}

func NewHandlerWithTally(
	votingUC *usecase.VotingUseCase,
	registerUC *usecase.RegisterUseCase,
	proposalUC *usecase.ProposalUseCase,
	participationUC *usecase.ParticipationUseCase,
	tallyUC *usecase.TallyUseCase,
	logger *zap.Logger,
) *Handler {
	return NewHandlerWithTallyAndMode(votingUC, registerUC, proposalUC, participationUC, tallyUC, false, logger)
}

// NewHandlerWithTallyAndMode 装配 handler，并决定开发调试能力是否可达。
//
// debugMode 为 false 时，ballot-draft 端点按 404 处理 —— 用 404 而不是
// 403，避免把端点存在性暴露出去。
func NewHandlerWithTallyAndMode(
	votingUC *usecase.VotingUseCase,
	registerUC *usecase.RegisterUseCase,
	proposalUC *usecase.ProposalUseCase,
	participationUC *usecase.ParticipationUseCase,
	tallyUC *usecase.TallyUseCase,
	debugMode bool,
	logger *zap.Logger,
) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		votingUC:        votingUC,
		registerUC:      registerUC,
		proposalUC:      proposalUC,
		participationUC: participationUC,
		tallyUC:         tallyUC,
		validate:        validator.New(),
		debugMode:       debugMode,
		logger:          logger,
	}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/votes", h.SubmitVote)
	r.GET("/proposals", h.ListProposals)
	r.GET("/proposals/open", h.ListOpenProposals)
	r.POST("/proposals/:proposal_id/vote-status", h.GetVoteStatus)
	r.POST("/proposals/:proposal_id/ballot-draft", h.PrepareBallotDraft)
	r.GET("/proposals/:proposal_id/election-key", h.GetElectionKey)
	// 匿名领取凭证：真两段式盲签名，三步走。
	//   ① claim      —— 预留一个池槽位，拿到 user_id 与签发方公钥（不签名）
	//   ② blind-sign —— 服务端对**盲化后**的消息做盲签名
	//   ③ attest     —— 客户端交回解盲后的签名，服务端验签并落库
	// 请求体里刻意没有任何身份字段：服务端一旦知道公钥，盲化当场失效。
	r.POST("/proposals/:proposal_id/credentials/claim", h.ReserveCredential)
	r.POST("/proposals/:proposal_id/credentials/blind-sign", h.BlindSignCredential)
	r.POST("/proposals/:proposal_id/credentials/attest", h.AttestCredential)
	r.GET("/proposals/:proposal_id/results", h.GetResult)
	r.GET("/proposals/:proposal_id/audit-anchors", h.ListAuditAnchors)
}

type submitVotePayload struct {
	ProposalID          string `json:"proposal_id" validate:"required"`
	EncryptedVote       string `json:"encrypted_vote" validate:"required,base64"`
	Proof               string `json:"proof" validate:"required,base64"`
	Nullifier           string `json:"nullifier" validate:"required"`
	UserID              string `json:"user_id" validate:"required"`
	UserPubKey          string `json:"user_pub_key" validate:"required,base64"`
	CredentialSignature string `json:"credential_signature" validate:"required,base64"`
}

func (h *Handler) SubmitVote(c *gin.Context) {
	if h.votingUC == nil {
		c.Error(fmt.Errorf("voting usecase is not configured"))
		return
	}

	var payload submitVotePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	encryptedVote, err := base64.StdEncoding.DecodeString(payload.EncryptedVote)
	if err != nil {
		c.Error(fmt.Errorf("%w: encrypted_vote must be base64", usecase.ErrInvalidRequest))
		return
	}
	proofBytes, err := base64.StdEncoding.DecodeString(payload.Proof)
	if err != nil {
		c.Error(fmt.Errorf("%w: proof must be base64", usecase.ErrInvalidRequest))
		return
	}
	userPubKey, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		c.Error(fmt.Errorf("%w: user_pub_key must be base64", usecase.ErrInvalidRequest))
		return
	}
	credentialSignature, err := base64.StdEncoding.DecodeString(payload.CredentialSignature)
	if err != nil {
		c.Error(fmt.Errorf("%w: credential_signature must be base64", usecase.ErrInvalidRequest))
		return
	}
	req := &usecase.SubmitVoteRequest{
		ProposalID:          payload.ProposalID,
		EncryptedVote:       encryptedVote,
		Proof:               &entity.Proof{Data: proofBytes},
		Nullifier:           payload.Nullifier,
		UserID:              payload.UserID,
		UserPubKey:          userPubKey,
		CredentialSignature: credentialSignature,
	}
	if err := h.votingUC.Submit(c.Request.Context(), req); err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "ok",
	})
}

// reserveCredentialPayload 是管理端预留签发权的请求体。
//
// **没有 user_pub_key**：服务端一旦知道公钥，就能算出凭证消息的原文 ——
// 盲化当场失效（它手上有私钥，根本不需要解盲）。
type reserveCredentialPayload struct {
	ProposalID string `json:"proposal_id" validate:"required"`
	UserID     string `json:"user_id" validate:"required"`
}

// blindSignPayload 是盲签名请求体。
type blindSignPayload struct {
	UserID string `json:"user_id" validate:"required"`
	// BlindedMessage 是客户端盲化后的消息 —— 签发方只见得到它。
	BlindedMessage string `json:"blinded_message" validate:"required,base64"`
	// IssuerKeyVersion 是客户端盲化时用的那一版密钥。
	IssuerKeyVersion int `json:"issuer_key_version" validate:"required,min=1"`
}

// adminBlindSignPayload 是管理端盲签名请求体。
type adminBlindSignPayload struct {
	ProposalID       string `json:"proposal_id" validate:"required"`
	UserID           string `json:"user_id" validate:"required"`
	BlindedMessage   string `json:"blinded_message" validate:"required,base64"`
	IssuerKeyVersion int    `json:"issuer_key_version" validate:"required,min=1"`
}

// attestCredentialPayload 是 attest 请求体：客户端解盲出来的最终签名。
type attestCredentialPayload struct {
	UserID     string `json:"user_id" validate:"required"`
	UserPubKey string `json:"user_pub_key" validate:"required,base64"`
	Signature  string `json:"signature" validate:"required,base64"`
}

// reservationResponse 是「预留签发权」的响应。
//
// 它带出签发方公钥：客户端要拿它把凭证消息盲化。公钥本来就是公开值。
func reservationResponse(reservation *usecase.CredentialReservation) gin.H {
	return gin.H{
		"proposal_id":        reservation.ProposalID,
		"user_id":            reservation.UserID,
		"issuer_key_version": reservation.IssuerKey.Version,
		"variant":            reservation.IssuerKey.Variant,
		"modulus":            base64.StdEncoding.EncodeToString(reservation.IssuerKey.Modulus),
		"exponent":           reservation.IssuerKey.Exponent,
	}
}

func credentialResponse(credential *entity.Credential) gin.H {
	return gin.H{
		"id":                 credential.ID,
		"proposal_id":        credential.ProposalID,
		"user_id":            credential.UserID,
		"user_pub_key":       base64.StdEncoding.EncodeToString(credential.UserPubKey),
		"signature":          base64.StdEncoding.EncodeToString(credential.Signature),
		"issuer_key_version": credential.IssuerKeyVersion,
		"issued_at":          credential.IssuedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ReserveCredentialForUser 管理端为一个指定的 user_id 预留签发权。
//
// 它**不签名**：管理端客户端拿 user_id 与签发方公钥后本地盲化，再走
// 「盲签名 → attest」两步。服务端因此在签名阶段只见盲化消息。
func (h *Handler) ReserveCredentialForUser(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload reserveCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	reservation, err := h.registerUC.ReserveCredentialForUser(
		c.Request.Context(),
		payload.ProposalID,
		payload.UserID,
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, reservationResponse(reservation))
}

type voteStatusPayload struct {
	UserID              string `json:"user_id" validate:"required"`
	UserPubKey          string `json:"user_pub_key" validate:"required,base64"`
	CredentialSignature string `json:"credential_signature" validate:"required,base64"`
}

// fillCredentialPoolPayload 是管理端备池的请求体。
type fillCredentialPoolPayload struct {
	Count int `json:"count" validate:"required,min=1,max=10000"`
}

// ReserveCredential 匿名预留一个凭证池槽位。
//
// **请求体里刻意没有任何字段**（旧版的 `user_pub_key` 一律忽略）：服务端
// 一旦知道公钥，就能算出凭证消息的原文，盲化当场失效。user_id 由服务端从
// 凭证池里随机分配 —— 服务端因此无法把凭证和投票人对应起来。
//
// 这一步**不签名**：客户端拿 user_id 与签发方公钥后本地盲化，再走盲签名与
// attest 两步。
func (h *Handler) ReserveCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	reservation, err := h.registerUC.ReserveCredential(
		c.Request.Context(),
		c.Param("proposal_id"),
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, reservationResponse(reservation))
}

// BlindSignCredential 公开端点的盲签名。
//
// 只覆盖「池里还空着的槽位」：否则签发方会变成任意消息的签名预言机。
func (h *Handler) BlindSignCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload blindSignPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	blindedMessage, err := base64.StdEncoding.DecodeString(payload.BlindedMessage)
	if err != nil {
		c.Error(fmt.Errorf("%w: blinded_message must be base64", usecase.ErrInvalidRequest))
		return
	}

	blindSignature, err := h.registerUC.BlindSignCredential(
		c.Request.Context(),
		c.Param("proposal_id"),
		payload.UserID,
		blindedMessage,
		payload.IssuerKeyVersion,
		usecase.BlindSignScopePoolSlot,
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"blind_signature": base64.StdEncoding.EncodeToString(blindSignature),
	})
}

// AdminBlindSignCredential 管理端的盲签名。
//
// 可以覆盖任意 user_id：运营方本来就有服务端的完整权限，这条不给他新增能力。
func (h *Handler) AdminBlindSignCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload adminBlindSignPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	blindedMessage, err := base64.StdEncoding.DecodeString(payload.BlindedMessage)
	if err != nil {
		c.Error(fmt.Errorf("%w: blinded_message must be base64", usecase.ErrInvalidRequest))
		return
	}

	blindSignature, err := h.registerUC.BlindSignCredential(
		c.Request.Context(),
		payload.ProposalID,
		payload.UserID,
		blindedMessage,
		payload.IssuerKeyVersion,
		usecase.BlindSignScopeAnyUser,
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"blind_signature": base64.StdEncoding.EncodeToString(blindSignature),
	})
}

// AttestCredential 校验客户端解盲出来的最终签名，通过后落库。
//
// 验签绑定的消息是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)`：拿 A 的盲签名
// 去签 B 必然验不过 —— 交叉攻击就是被这一步挡住的。
func (h *Handler) AttestCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload attestCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	userPubKey, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		c.Error(fmt.Errorf("%w: user_pub_key must be base64", usecase.ErrInvalidRequest))
		return
	}
	signature, err := base64.StdEncoding.DecodeString(payload.Signature)
	if err != nil {
		c.Error(fmt.Errorf("%w: signature must be base64", usecase.ErrInvalidRequest))
		return
	}

	credential, err := h.registerUC.AttestCredential(
		c.Request.Context(),
		c.Param("proposal_id"),
		payload.UserID,
		userPubKey,
		signature,
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, credentialResponse(credential))
}

// FillCredentialPool 管理端为提案备好待领取的凭证槽位。
func (h *Handler) FillCredentialPool(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload fillCredentialPoolPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	count, err := h.registerUC.FillCredentialPool(
		c.Request.Context(),
		c.Param("proposal_id"),
		payload.Count,
	)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":      "ok",
		"proposal_id": c.Param("proposal_id"),
		"count":       count,
	})
}

type createProposalPayload struct {
	Title       string   `json:"title" validate:"required"`
	Description string   `json:"description"`
	Options     []string `json:"options" validate:"required,min=2,dive,required"`
	StartsAt    string   `json:"starts_at" validate:"required"`
	EndsAt      string   `json:"ends_at" validate:"required"`
}

func (h *Handler) CreateProposal(c *gin.Context) {
	if h.proposalUC == nil {
		c.Error(fmt.Errorf("proposal usecase is not configured"))
		return
	}

	var payload createProposalPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	startsAt, err := time.Parse(time.RFC3339, payload.StartsAt)
	if err != nil {
		c.Error(fmt.Errorf("%w: starts_at must be RFC3339", usecase.ErrInvalidRequest))
		return
	}
	endsAt, err := time.Parse(time.RFC3339, payload.EndsAt)
	if err != nil {
		c.Error(fmt.Errorf("%w: ends_at must be RFC3339", usecase.ErrInvalidRequest))
		return
	}

	req := &usecase.CreateProposalRequest{
		Title:       payload.Title,
		Description: payload.Description,
		Options:     payload.Options,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}
	proposal, err := h.proposalUC.Create(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, proposalResponse(proposal))
}

func proposalResponse(proposal *entity.Proposal) gin.H {
	return gin.H{
		"id":          proposal.ID,
		"title":       proposal.Title,
		"description": proposal.Description,
		"options":     proposal.Options,
		"status":      proposal.Status,
		"starts_at":   proposal.StartsAt.Format(time.RFC3339),
		"ends_at":     proposal.EndsAt.Format(time.RFC3339),
		"created_at":  proposal.CreatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) ListProposals(c *gin.Context) {
	if h.proposalUC == nil {
		c.Error(fmt.Errorf("proposal usecase is not configured"))
		return
	}

	proposals, err := h.proposalUC.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	out := make([]gin.H, 0, len(proposals))
	for _, proposal := range proposals {
		out = append(out, proposalResponse(proposal))
	}

	c.JSON(http.StatusOK, gin.H{
		"items": out,
	})
}

func (h *Handler) ListOpenProposals(c *gin.Context) {
	if h.proposalUC == nil {
		c.Error(fmt.Errorf("proposal usecase is not configured"))
		return
	}

	proposals, err := h.proposalUC.ListOpen(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	out := make([]gin.H, 0, len(proposals))
	for _, proposal := range proposals {
		out = append(out, proposalResponse(proposal))
	}

	c.JSON(http.StatusOK, gin.H{
		"items": out,
	})
}

type setupDecryptionPayload struct {
	Threshold int `json:"threshold" validate:"required,min=1"`
	Trustees  int `json:"trustees" validate:"required,min=1"`
}

type prepareBallotDraftPayload struct {
	Option string `json:"option" validate:"required"`
}

const developmentProofBase64 = "ZGV2LXByb29m"

func (h *Handler) PrepareBallotDraft(c *gin.Context) {
	// 非 debug 模式按 404 处理：ballot-draft 让服务端拿明文选项去加密，
	// 这条链路在生产环境不该存在，也不该暴露端点存在性。
	if !h.debugMode {
		c.Status(http.StatusNotFound)
		return
	}
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	var payload prepareBallotDraftPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	draft, err := h.tallyUC.PrepareBallotDraft(c.Request.Context(), proposalID, payload.Option)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"proposal_id":    proposalID,
		"option":         draft.Option,
		"encrypted_vote": base64.StdEncoding.EncodeToString(draft.EncryptedVote),
		"proof":          developmentProofBase64,
		"nullifier":      draft.Nullifier,
	})
}

// GetElectionKey 返回提案的计票公钥，供客户端本地加密选票。
//
// 公钥是公开值：客户端拿到它就能在本地把选项下标加密成密文，服务端全程
// 看不到明文选项。公钥以**未压缩**形式返回（X 与 Y 各 32 字节），浏览器端
// 不必实现点解压。
func (h *Handler) GetElectionKey(c *gin.Context) {
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}
	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}
	key, err := h.tallyUC.ElectionPublicKey(c.Request.Context(), proposalID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"proposal_id": proposalID,
		"public_key":  base64.StdEncoding.EncodeToString(key),
		"encoding":    "uncompressed",
	})
}

func (h *Handler) SetupDecryptionSession(c *gin.Context) {
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	var payload setupDecryptionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	setup, err := h.tallyUC.SetupThresholdSession(
		c.Request.Context(),
		proposalID,
		payload.Threshold,
		payload.Trustees,
	)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"proposal_id": setup.ProposalID,
		"threshold":   setup.Threshold,
		"trustees":    setup.Trustees,
		"public_key":  setup.PublicKey,
		"shares":      setup.Shares,
	})
}

type publishResultPayload struct {
	Shares []string `json:"shares" validate:"required,min=1,dive,required"`
}

func (h *Handler) PublishResult(c *gin.Context) {
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	var payload publishResultPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	result, err := h.tallyUC.PublishResult(c.Request.Context(), proposalID, payload.Shares)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"proposal_id":  result.ProposalID,
		"total_votes":  result.TotalVotes,
		"tally":        result.Tally,
		"merkle_root":  base64.StdEncoding.EncodeToString(result.MerkleRoot),
		"anchor_hash":  result.AnchorHash,
		"published_at": result.PublishedAt.Format(time.RFC3339),
	})
}

func (h *Handler) GetResult(c *gin.Context) {
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	result, err := h.tallyUC.GetResult(c.Request.Context(), proposalID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"proposal_id":  result.ProposalID,
		"total_votes":  result.TotalVotes,
		"tally":        result.Tally,
		"merkle_root":  base64.StdEncoding.EncodeToString(result.MerkleRoot),
		"anchor_hash":  result.AnchorHash,
		"published_at": result.PublishedAt.Format(time.RFC3339),
	})
}

func (h *Handler) ListAuditAnchors(c *gin.Context) {
	if h.tallyUC == nil {
		c.Error(fmt.Errorf("tally usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	anchors, err := h.tallyUC.ListAuditAnchors(c.Request.Context(), proposalID)
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]gin.H, 0, len(anchors))
	for _, anchor := range anchors {
		items = append(items, gin.H{
			"id":          anchor.ID,
			"proposal_id": anchor.ProposalID,
			"merkle_root": base64.StdEncoding.EncodeToString(anchor.MerkleRoot),
			"anchor_hash": anchor.AnchorHash,
			"created_at":  anchor.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

func (h *Handler) GetVoteStatus(c *gin.Context) {
	if h.participationUC == nil {
		c.Error(fmt.Errorf("participation usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	var payload voteStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	userPubKey, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		c.Error(fmt.Errorf("%w: user_pub_key must be base64", usecase.ErrInvalidRequest))
		return
	}
	credentialSignature, err := base64.StdEncoding.DecodeString(payload.CredentialSignature)
	if err != nil {
		c.Error(fmt.Errorf("%w: credential_signature must be base64", usecase.ErrInvalidRequest))
		return
	}

	hasVoted, err := h.participationUC.HasVotedWithCredential(
		c.Request.Context(),
		proposalID,
		payload.UserID,
		userPubKey,
		credentialSignature,
	)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"proposal_id": proposalID,
		"user_id":     payload.UserID,
		"has_voted":   hasVoted,
	})
}

func (h *Handler) ListProposalCredentialStatuses(c *gin.Context) {
	if h.participationUC == nil {
		c.Error(fmt.Errorf("participation usecase is not configured"))
		return
	}

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
		return
	}

	items, err := h.participationUC.ListCredentialStatuses(c.Request.Context(), proposalID)
	if err != nil {
		c.Error(err)
		return
	}

	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{
			"credential_id":      item.CredentialID,
			"proposal_id":        item.ProposalID,
			"user_id":            item.UserID,
			"user_pub_key":       base64.StdEncoding.EncodeToString(item.UserPubKey),
			"issuer_key_version": item.IssuerKeyVersion,
			"issued_at":          item.IssuedAt.Format(time.RFC3339),
			"has_voted":          item.HasVoted,
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": out})
}
