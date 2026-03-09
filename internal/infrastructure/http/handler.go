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
	"github.com/google/uuid"
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
	r.POST("/votes", h.SubmitVote)
	r.POST("/credentials", h.IssueCredential)
	r.POST("/proposals", h.CreateProposal)
	r.GET("/proposals/open", h.ListOpenProposals)
	r.GET("/proposals/:proposal_id/vote-status", h.GetVoteStatus)
	r.POST("/proposals/:proposal_id/ballot-draft", h.PrepareBallotDraft)
	r.POST("/proposals/:proposal_id/decryption/setup", h.SetupDecryptionSession)
	r.POST("/proposals/:proposal_id/results/publish", h.PublishResult)
	r.GET("/proposals/:proposal_id/results", h.GetResult)
	r.GET("/proposals/:proposal_id/audit-anchors", h.ListAuditAnchors)
}

type submitVotePayload struct {
	ProposalID          string `json:"proposal_id" validate:"required"`
	EncryptedVote       string `json:"encrypted_vote" validate:"required,base64"`
	Proof               string `json:"proof" validate:"required,base64"`
	Nullifier           string `json:"nullifier" validate:"required"`
	PublicWitness       string `json:"public_witness" validate:"omitempty,base64"`
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
	var publicWitness []byte
	if payload.PublicWitness != "" {
		publicWitness, err = base64.StdEncoding.DecodeString(payload.PublicWitness)
		if err != nil {
			c.Error(fmt.Errorf("%w: public_witness must be base64", usecase.ErrInvalidRequest))
			return
		}
	}

	req := &usecase.SubmitVoteRequest{
		ProposalID:          payload.ProposalID,
		EncryptedVote:       encryptedVote,
		Proof:               &entity.Proof{Data: proofBytes},
		Nullifier:           payload.Nullifier,
		PublicWitness:       publicWitness,
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
	UserID     string `json:"user_id" validate:"required"`
	UserPubKey string `json:"user_pub_key" validate:"required,base64"`
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

	credential, err := h.registerUC.IssueCredential(c.Request.Context(), payload.UserID, pubKeyBytes)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                 credential.ID,
		"user_id":            credential.UserID,
		"user_pub_key":       base64.StdEncoding.EncodeToString(credential.UserPubKey),
		"signature":          base64.StdEncoding.EncodeToString(credential.Signature),
		"issuer_key_version": credential.IssuerKeyVersion,
		"issued_at":          credential.IssuedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

type createProposalPayload struct {
	Title       string `json:"title" validate:"required"`
	Description string `json:"description"`
	StartsAt    string `json:"starts_at" validate:"required"`
	EndsAt      string `json:"ends_at" validate:"required"`
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
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}
	proposal, err := h.proposalUC.Create(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          proposal.ID,
		"title":       proposal.Title,
		"description": proposal.Description,
		"status":      proposal.Status,
		"starts_at":   proposal.StartsAt.Format(time.RFC3339),
		"ends_at":     proposal.EndsAt.Format(time.RFC3339),
		"created_at":  proposal.CreatedAt.Format(time.RFC3339),
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
		out = append(out, gin.H{
			"id":          proposal.ID,
			"title":       proposal.Title,
			"description": proposal.Description,
			"status":      proposal.Status,
			"starts_at":   proposal.StartsAt.Format(time.RFC3339),
			"ends_at":     proposal.EndsAt.Format(time.RFC3339),
			"created_at":  proposal.CreatedAt.Format(time.RFC3339),
		})
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

	cipher, err := h.tallyUC.EncryptVoteOption(c.Request.Context(), proposalID, payload.Option)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"proposal_id":    proposalID,
		"option":         payload.Option,
		"encrypted_vote": base64.StdEncoding.EncodeToString(cipher),
		"proof":          developmentProofBase64,
		"nullifier":      "null-" + uuid.NewString(),
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
	userID := c.Query("user_id")
	if proposalID == "" || userID == "" {
		c.Error(fmt.Errorf("%w: proposal_id and user_id are required", usecase.ErrInvalidRequest))
		return
	}

	hasVoted, err := h.participationUC.HasVoted(c.Request.Context(), proposalID, userID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"proposal_id": proposalID,
		"user_id":     userID,
		"has_voted":   hasVoted,
	})
}
