package usecase

import (
	"context"
	"testing"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type credentialRepositoryMock struct {
	mock.Mock
}

func (m *credentialRepositoryMock) Save(ctx context.Context, credential *entity.Credential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *credentialRepositoryMock) GetByProposalAndUserID(ctx context.Context, proposalID string, userID string) (*entity.Credential, error) {
	args := m.Called(ctx, proposalID, userID)
	credential, _ := args.Get(0).(*entity.Credential)
	return credential, args.Error(1)
}

func (m *credentialRepositoryMock) ListByProposal(ctx context.Context, proposalID string) ([]*entity.Credential, error) {
	args := m.Called(ctx, proposalID)
	items, _ := args.Get(0).([]*entity.Credential)
	return items, args.Error(1)
}

var _ repository.CredentialRepository = (*credentialRepositoryMock)(nil)

type credentialIssuerMock struct {
	mock.Mock
}

func (m *credentialIssuerMock) Issue(ctx context.Context, userID string, userPubKey []byte) (*entity.Credential, error) {
	args := m.Called(ctx, userID, userPubKey)
	credential, _ := args.Get(0).(*entity.Credential)
	return credential, args.Error(1)
}

var _ domaincrypto.CredentialIssuer = (*credentialIssuerMock)(nil)

func TestRegisterUseCase_IssueCredential(t *testing.T) {
	repo := new(credentialRepositoryMock)
	issuer := new(credentialIssuerMock)
	uc := NewRegisterUseCase(repo, issuer, zap.NewNop())

	repo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").Return((*entity.Credential)(nil), repository.ErrNotFound).Once()
	issuer.On("Issue", mock.Anything, "u-1", []byte("pub")).Return(&entity.Credential{
		ProposalID: "p-1",
		UserID:     "u-1",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
	}, nil).Once()
	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Credential")).Return(nil).Once()

	credential, err := uc.IssueCredential(context.Background(), "p-1", "u-1", []byte("pub"))
	assert.NoError(t, err)
	assert.NotNil(t, credential)
	assert.NotEmpty(t, credential.ID)
}

func TestRegisterUseCase_IssueCredential_AlreadyIssued(t *testing.T) {
	repo := new(credentialRepositoryMock)
	issuer := new(credentialIssuerMock)
	uc := NewRegisterUseCase(repo, issuer, zap.NewNop())

	repo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").Return(&entity.Credential{ID: "cred-1"}, nil).Once()

	credential, err := uc.IssueCredential(context.Background(), "p-1", "u-1", []byte("pub"))
	assert.ErrorIs(t, err, ErrCredentialAlreadyIssued)
	assert.Nil(t, credential)
}
