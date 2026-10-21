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
	// 匿名领取凭证：请求体不含任何身份信息，服务端因此无从记录
	// 「谁领了哪个凭证」。
	r.POST("/proposals/:proposal_id/credentials/claim", h.ClaimCredential)
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

type issueCredentialPayload struct {
	ProposalID string `json:"proposal_id" validate:"required"`
	UserID     string `json:"user_id" validate:"required"`
	UserPubKey string `json:"user_pub_key" validate:"required,base64"`
}

type voteStatusPayload struct {
	UserID              string `json:"user_id" validate:"required"`
	UserPubKey          string `json:"user_pub_key" validate:"required,base64"`
	CredentialSignature string `json:"credential_signature" validate:"required,base64"`
}

func (h *Handler) IssueCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload issueCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		c.Error(fmt.Errorf("%w: user_pub_key must be base64", usecase.ErrInvalidRequest))
		return
	}

	credential, err := h.registerUC.IssueCredential(c.Request.Context(), payload.ProposalID, payload.UserID, pubKeyBytes)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                 credential.ID,
		"proposal_id":        credential.ProposalID,
		"user_id":            credential.UserID,
		"user_pub_key":       base64.StdEncoding.EncodeToString(credential.UserPubKey),
		"signature":          base64.StdEncoding.EncodeToString(credential.Signature),
		"issuer_key_version": credential.IssuerKeyVersion,
		"issued_at":          credential.IssuedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// claimCredentialPayload 是匿名领取凭证的请求体。
//
// **刻意没有任何身份字段**：领取者只提交自己的公钥，user_id 由服务端
// 从凭证池里随机分配 —— 服务端因此无法把凭证和投票人对应起来。
type claimCredentialPayload struct {
	UserPubKey string `json:"user_pub_key" validate:"required,base64"`
}

// fillCredentialPoolPayload 是管理端备池的请求体。
type fillCredentialPoolPayload struct {
	Count int `json:"count" validate:"required,min=1,max=10000"`
}

// ClaimCredential 匿名领取一个凭证。
func (h *Handler) ClaimCredential(c *gin.Context) {
	if h.registerUC == nil {
		c.Error(fmt.Errorf("register usecase is not configured"))
		return
	}

	var payload claimCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}
	if err := h.validate.Struct(payload); err != nil {
		c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))
		return
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(payload.UserPubKey)
	if err != nil {
		c.Error(fmt.Errorf("%w: user_pub_key must be base64", usecase.ErrInvalidRequest))
		return
	}

	credential, err := h.registerUC.ClaimCredential(
		c.Request.Context(),
		c.Param("proposal_id"),
		pubKeyBytes,
	)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                 credential.ID,
		"proposal_id":        credential.ProposalID,
		"user_id":            credential.UserID,
		"user_pub_key":       base64.StdEncoding.EncodeToString(credential.UserPubKey),
		"signature":          base64.StdEncoding.EncodeToString(credential.Signature),
		"issuer_key_version": credential.IssuerKeyVersion,
		"issued_at":          credential.IssuedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
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
