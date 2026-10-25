package httpapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
		logger:          logger,
	}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// 投票提交与其他提案级端点同形：提案 ID 走路径，不走请求体 ——
	// 同一个提案 ID 只有**一个**来源，客户端不可能把两个提案搞混。
	// 这条路径在 v0.3.0 被冻结（见 docs/api-contract-v1.md）。
	r.POST("/proposals/:proposal_id/votes", h.SubmitVote)
	r.GET("/proposals", h.ListProposals)
	r.GET("/proposals/open", h.ListOpenProposals)
	r.POST("/proposals/:proposal_id/vote-status", h.GetVoteStatus)
	r.GET("/proposals/:proposal_id/election-key", h.GetElectionKey)
	// 匿名领取凭证：真两段式盲签名，三步走。
	//   ① claim      —— 预留一个池槽位，拿到 user_id 与签发方公钥（不签名）
	//   ② blind-sign —— 服务端对**盲化后**的消息做盲签名
	//   ③ attest     —— 客户端交回解盲后的签名，服务端验签并落库
	// 请求体里刻意没有任何身份字段：服务端一旦知道公钥，盲化当场失效。
	//
	// 开启资格校验时，claim 之前多一步：先拿一次性的领取挑战（④ eligibility-
	// challenge），把资格证明钉在本次会话上。
	r.POST("/proposals/:proposal_id/credentials/eligibility-challenge", h.IssueEligibilityChallenge)
	r.POST("/proposals/:proposal_id/credentials/claim", h.ReserveCredential)
	r.POST("/proposals/:proposal_id/credentials/blind-sign", h.BlindSignCredential)
	r.POST("/proposals/:proposal_id/credentials/attest", h.AttestCredential)
	r.GET("/proposals/:proposal_id/results", h.GetResult)
	r.GET("/proposals/:proposal_id/audit-anchors", h.ListAuditAnchors)
}

// submitVotePayload 是投票提交的请求体。
//
// ⚠️ 这里**没有** `proposal_id`：它只从路径来（`/proposals/:proposal_id/votes`）。
// 一个提案 ID 只有**一个**来源 —— 请求体里再放一份，两份不一致时
// 服务端只能猜，而猜错就是「把票投到别的提案去」。
//
// 字段名在 v0.3.0 被冻结（见 docs/api-contract-v1.md）。
type submitVotePayload struct {
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

	proposalID := c.Param("proposal_id")
	if proposalID == "" {
		c.Error(fmt.Errorf("%w: proposal_id is required", usecase.ErrInvalidRequest))
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
		ProposalID:          proposalID,
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

// claimCredentialPayload 是匿名领取凭证的请求体。
//
// 开启资格校验时三件材料必填；没开启时整个请求体可以为空（既有客户端就是
// 这么发的）。
//
// ⚠️ 这里**不能**出现真人身份字段（工号、姓名、证件号……）：资格材料里只有
// 零知识证明与两个公开输入，服务端从头到尾不知道领票的是谁。
type claimCredentialPayload struct {
	// EligibilityProof 是资格证明（base64 编码的 Groth16 证明）。
	EligibilityProof string `json:"eligibility_proof"`
	// QualificationID 是去重键（十六进制的域元素）。
	QualificationID string `json:"qualification_id"`
	// Challenge 是服务端签发的领取挑战（十六进制的域元素）。
	Challenge string `json:"challenge"`
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

// IssueEligibilityChallenge 签发一次领取挑战。
//
// 客户端拿它把资格证明钉在本次会话上。服务端不记账 —— 重放由去重键挡住。
func (h *Handler) IssueEligibilityChallenge(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	challenge, err := h.registerUC.IssueEligibilityChallenge(
		c.Request.Context(),
		c.Param("proposal_id"),
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"proposal_id": challenge.ProposalID,
		"root":        challenge.Root,
		"depth":       challenge.Depth,
		"challenge":   challenge.Challenge,
	})
}

// ReserveCredential 匿名预留一个凭证池槽位。
//
// **请求体里刻意没有任何身份字段**（旧版的 `user_pub_key` 一律忽略）：服务端
// 一旦知道公钥，就能算出凭证消息的原文，盲化当场失效。user_id 由服务端从
// 凭证池里随机分配 —— 服务端因此无法把凭证和投票人对应起来。
//
// 这一步**不签名**：客户端拿 user_id 与签发方公钥后本地盲化，再走盲签名与
// attest 两步。
//
// 开启资格校验时，请求体里必须带资格材料（证明 + 去重键 + 挑战）：服务端
// 先验资格、再记一次「资格已用过」，然后才给槽位 —— **资格不过不消耗槽位**。
func (h *Handler) ReserveCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload claimCredentialPayload
	// 空请求体是合法的（没开启资格校验时的既有形态）：EOF 不算错误。
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	material, err := parseEligibilityMaterial(payload)
	if err != nil {
		c.Error(err)
		return
	}

	reservation, err := h.registerUC.ReserveCredential(
		c.Request.Context(),
		c.Param("proposal_id"),
		material,
	)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, reservationResponse(reservation))
}

// parseEligibilityMaterial 从请求体里解析资格材料。
//
// 三件材料要么都给、要么都不给：只给一半（例如只带去重键）说明客户端出了错 ——
// 那种情况下不能当成「没带资格」放行。
func parseEligibilityMaterial(payload claimCredentialPayload) (*usecase.EligibilityMaterial, error) {
	if payload.EligibilityProof == "" && payload.QualificationID == "" && payload.Challenge == "" {
		return nil, nil
	}
	if payload.EligibilityProof == "" || payload.QualificationID == "" || payload.Challenge == "" {
		return nil, fmt.Errorf(
			"%w: eligibility_proof, qualification_id and challenge must be provided together",
			usecase.ErrInvalidRequest,
		)
	}
	proof, err := base64.StdEncoding.DecodeString(payload.EligibilityProof)
	if err != nil {
		return nil, fmt.Errorf("%w: eligibility_proof must be base64", usecase.ErrInvalidRequest)
	}
	return &usecase.EligibilityMaterial{
		Proof:           proof,
		QualificationID: payload.QualificationID,
		Challenge:       payload.Challenge,
	}, nil
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
