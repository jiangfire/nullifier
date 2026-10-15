package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type participationRepositoryMock struct {
	mock.Mock
}

func (m *participationRepositoryMock) MarkVoted(
	ctx context.Context,
	proposalID string,
	userHash string,
	votedAt time.Time,
) error {
	args := m.Called(ctx, proposalID, userHash, votedAt)
	return args.Error(0)
}

func (m *participationRepositoryMock) HasVoted(
	ctx context.Context,
	proposalID string,
	userHash string,
) (bool, error) {
	args := m.Called(ctx, proposalID, userHash)
	return args.Bool(0), args.Error(1)
}

var _ repository.ParticipationRepository = (*participationRepositoryMock)(nil)

func TestParticipationUseCase_HasVoted(t *testing.T) {
	repo := new(participationRepositoryMock)
	uc := NewParticipationUseCase(repo, nil, zap.NewNop())

	repo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()

	hasVoted, err := uc.HasVoted(context.Background(), "p-1", "u-1")
	assert.NoError(t, err)
	assert.True(t, hasVoted)
}

func TestParticipationUseCase_HasVoted_InvalidRequest(t *testing.T) {
	repo := new(participationRepositoryMock)
	uc := NewParticipationUseCase(repo, nil, zap.NewNop())

	hasVoted, err := uc.HasVoted(context.Background(), "", "u-1")
	assert.False(t, hasVoted)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestParticipationUseCase_ListCredentialStatuses(t *testing.T) {
	participationRepo := new(participationRepositoryMock)
	credentialRepo := new(credentialRepositoryMock)
	uc := NewParticipationUseCase(participationRepo, credentialRepo, zap.NewNop())

	credentialRepo.On("ListByProposal", mock.Anything, "p-1").Return([]*entity.Credential{
		{
			ID:               "c-1",
			ProposalID:       "p-1",
			UserID:           "u-1",
			UserPubKey:       []byte("pub-1"),
			IssuerKeyVersion: 1,
			IssuedAt:         time.Now().UTC(),
		},
		{
			ID:               "c-2",
			ProposalID:       "p-1",
			UserID:           "u-2",
			UserPubKey:       []byte("pub-2"),
			IssuerKeyVersion: 1,
			IssuedAt:         time.Now().UTC(),
		},
	}, nil).Once()
	participationRepo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()
	participationRepo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-2")).Return(false, nil).Once()

	items, err := uc.ListCredentialStatuses(context.Background(), "p-1")
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.True(t, items[0].HasVoted)
	assert.False(t, items[1].HasVoted)
}

func TestParticipationUseCase_ListCredentialStatuses_ListError(t *testing.T) {
	participationRepo := new(participationRepositoryMock)
	credentialRepo := new(credentialRepositoryMock)
	uc := NewParticipationUseCase(participationRepo, credentialRepo, zap.NewNop())

	credentialRepo.On("ListByProposal", mock.Anything, "p-1").Return(([]*entity.Credential)(nil), errors.New("db down")).Once()

	items, err := uc.ListCredentialStatuses(context.Background(), "p-1")
	assert.Nil(t, items)
	assert.Error(t, err)
}

func TestParticipationUseCase_HasVotedWithCredential(t *testing.T) {
	credential := &entity.Credential{
		ID:         "c-1",
		ProposalID: "p-1",
		UserID:     "u-1",
		UserPubKey: []byte("pub-1"),
		Signature:  []byte("sig-1"),
		IssuedAt:   time.Now().UTC(),
	}

	participationRepo := new(participationRepositoryMock)
	credentialRepo := new(credentialRepositoryMock)
	uc := NewParticipationUseCase(participationRepo, credentialRepo, zap.NewNop())

	credentialRepo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").Return(credential, nil).Once()
	participationRepo.On("HasVoted", mock.Anything, "p-1", hashUserID("u-1")).Return(true, nil).Once()

	hasVoted, err := uc.HasVotedWithCredential(
		context.Background(),
		"p-1",
		"u-1",
		[]byte("pub-1"),
		[]byte("sig-1"),
	)
	assert.NoError(t, err)
	assert.True(t, hasVoted)
}

func TestParticipationUseCase_HasVotedWithCredential_RejectsMismatch(t *testing.T) {
	credential := &entity.Credential{
		ID:         "c-1",
		ProposalID: "p-1",
		UserID:     "u-1",
		UserPubKey: []byte("pub-1"),
		Signature:  []byte("sig-1"),
	}

	cases := []struct {
		name      string
		userID    string
		userKey   []byte
		signature []byte
		wantErr   error
	}{
		{
			name:      "other_public_key",
			userID:    "u-1",
			userKey:   []byte("pub-2"),
			signature: []byte("sig-1"),
			wantErr:   ErrInvalidCredential,
		},
		{
			name:      "other_signature",
			userID:    "u-1",
			userKey:   []byte("pub-1"),
			signature: []byte("sig-2"),
			wantErr:   ErrInvalidCredential,
		},
		{name: "missing_user_id", userKey: []byte("pub-1"), signature: []byte("sig-1"), wantErr: ErrInvalidRequest},
		{name: "missing_public_key", userID: "u-1", signature: []byte("sig-1"), wantErr: ErrInvalidRequest},
		{name: "missing_signature", userID: "u-1", userKey: []byte("pub-1"), wantErr: ErrInvalidRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			credentialRepo := new(credentialRepositoryMock)
			if tc.wantErr == ErrInvalidCredential {
				credentialRepo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").Return(credential, nil).Once()
			}
			uc := NewParticipationUseCase(new(participationRepositoryMock), credentialRepo, zap.NewNop())

			hasVoted, err := uc.HasVotedWithCredential(
				context.Background(),
				"p-1",
				tc.userID,
				tc.userKey,
				tc.signature,
			)
			assert.False(t, hasVoted)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestParticipationUseCase_HasVotedWithCredential_Errors(t *testing.T) {
	uc := NewParticipationUseCase(new(participationRepositoryMock), nil, zap.NewNop())
	_, err := uc.HasVotedWithCredential(context.Background(), "p-1", "u-1", []byte("pub"), []byte("sig"))
	assert.Error(t, err)

	credentialRepo := new(credentialRepositoryMock)
	credentialRepo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").
		Return((*entity.Credential)(nil), repository.ErrNotFound).Once()
	uc = NewParticipationUseCase(new(participationRepositoryMock), credentialRepo, zap.NewNop())

	_, err = uc.HasVotedWithCredential(context.Background(), "p-1", "u-1", []byte("pub"), []byte("sig"))
	assert.Error(t, err)
}
