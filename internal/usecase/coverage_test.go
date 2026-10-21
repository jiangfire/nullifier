package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestConstructors_DefaultLogger(t *testing.T) {
	require.NotNil(t, NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, nil))
	require.NotNil(t, NewRegisterUseCase(nil, nil, nil, nil))
	require.NotNil(t, NewProposalUseCase(nil, nil))
	require.NotNil(t, NewParticipationUseCase(nil, nil, nil))
	require.NotNil(t, NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, nil, nil))
}

func TestSubmitVoteRequest_Validate(t *testing.T) {
	var req *SubmitVoteRequest
	require.ErrorIs(t, req.Validate(), ErrInvalidRequest)

	req = &SubmitVoteRequest{}
	require.ErrorIs(t, req.Validate(), ErrInvalidRequest)

	valid := SubmitVoteRequest{
		ProposalID:          "p1",
		Nullifier:           strings.Repeat("ab", 32),
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		UserID:              "u1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}
	require.NoError(t, valid.Validate())

	// The credential material is mandatory: a vote cannot be cast anonymously.
	missingUserID := valid
	missingUserID.UserID = ""
	require.ErrorIs(t, missingUserID.Validate(), ErrInvalidRequest)

	missingPubKey := valid
	missingPubKey.UserPubKey = nil
	require.ErrorIs(t, missingPubKey.Validate(), ErrInvalidRequest)

	missingSignature := valid
	missingSignature.CredentialSignature = nil
	require.ErrorIs(t, missingSignature.Validate(), ErrInvalidRequest)
}

func TestCreateProposalRequest_Validate(t *testing.T) {
	var req *CreateProposalRequest
	require.ErrorIs(t, req.Validate(), ErrInvalidRequest)

	req = &CreateProposalRequest{
		Title:    "title",
		Options:  []string{"A", "B"},
		StartsAt: time.Now().UTC(),
		EndsAt:   time.Now().UTC().Add(-time.Minute),
	}
	require.ErrorIs(t, req.Validate(), ErrInvalidRequest)
}

func TestRegisterUseCase_IssueCredential_InvalidAndMissingDeps(t *testing.T) {
	uc := NewRegisterUseCase(nil, nil, nil, nil)
	_, err := uc.IssueCredential(context.Background(), "p1", "u1", []byte("pub"))
	require.Error(t, err)

	repo := new(credentialRepositoryMock)
	uc = NewRegisterUseCase(repo, nil, nil, nil)
	_, err = uc.IssueCredential(context.Background(), "", "", []byte("pub"))
	require.Error(t, err)
}

func TestRegisterUseCase_IssueCredential_CheckExistingError(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	uc := NewRegisterUseCase(repo, proposalRepo, issuer, nil)

	proposalRepo.On("GetByID", mock.Anything, "p1").Return(&entity.Proposal{ID: "p1"}, nil).Once()
	repo.On("GetByProposalAndUserID", mock.Anything, "p1", "u1").Return((*entity.Credential)(nil), errors.New("db down")).Once()
	_, err := uc.IssueCredential(context.Background(), "p1", "u1", []byte("pub"))
	require.Error(t, err)
}

func TestProposalUseCase_ListOpen_RepoNil(t *testing.T) {
	uc := NewProposalUseCase(nil, nil)
	_, err := uc.ListOpen(context.Background())
	require.Error(t, err)
}

func TestParticipationUseCase_HasVoted_RepoError(t *testing.T) {
	repo := new(participationRepositoryMock)
	repo.On("HasVoted", mock.Anything, "p1", hashUserID("u1")).Return(false, repository.ErrNotFound).Once()
	uc := NewParticipationUseCase(repo, nil, nil)
	_, err := uc.HasVoted(context.Background(), "p1", "u1")
	require.Error(t, err)
}

func TestVotingUseCase_Submit_MissingDeps(t *testing.T) {
	uc := NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	err := uc.Submit(context.Background(), &SubmitVoteRequest{})
	require.Error(t, err)

	uc = NewVotingUseCase(new(voteRepositoryMock), nil, nil, nil, nil, nil, nil, nil, nil)
	err = uc.Submit(context.Background(), &SubmitVoteRequest{})
	require.Error(t, err)
}
