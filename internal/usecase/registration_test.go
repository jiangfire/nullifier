package usecase

import (
	"context"
	"testing"
	"time"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

type credentialClaimRepositoryMock struct {
	mock.Mock
}

func (m *credentialClaimRepositoryMock) Save(ctx context.Context, claims []*entity.CredentialClaim) error {
	args := m.Called(ctx, claims)
	return args.Error(0)
}

func (m *credentialClaimRepositoryMock) TakeAvailable(ctx context.Context, proposalID string) (*entity.CredentialClaim, error) {
	args := m.Called(ctx, proposalID)
	claim, _ := args.Get(0).(*entity.CredentialClaim)
	return claim, args.Error(1)
}

func (m *credentialClaimRepositoryMock) MarkClaimed(
	ctx context.Context,
	claimID string,
	userPubKey []byte,
	signature []byte,
	claimedAt time.Time,
) (bool, error) {
	args := m.Called(ctx, claimID, userPubKey, signature, claimedAt)
	return args.Bool(0), args.Error(1)
}

func (m *credentialClaimRepositoryMock) CountAvailable(ctx context.Context, proposalID string) (int64, error) {
	args := m.Called(ctx, proposalID)
	return args.Get(0).(int64), args.Error(1)
}

var _ repository.CredentialClaimRepository = (*credentialClaimRepositoryMock)(nil)

func TestRegisterUseCase_IssueCredential(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	uc := NewRegisterUseCase(repo, proposalRepo, issuer, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
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
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	uc := NewRegisterUseCase(repo, proposalRepo, issuer, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	repo.On("GetByProposalAndUserID", mock.Anything, "p-1", "u-1").Return(&entity.Credential{ID: "cred-1"}, nil).Once()

	credential, err := uc.IssueCredential(context.Background(), "p-1", "u-1", []byte("pub"))
	assert.ErrorIs(t, err, ErrCredentialAlreadyIssued)
	assert.Nil(t, credential)
}

func TestRegisterUseCase_FillCredentialPool(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()

	var saved []*entity.CredentialClaim
	claimRepo.On("Save", mock.Anything, mock.AnythingOfType("[]*entity.CredentialClaim")).
		Run(func(args mock.Arguments) {
			saved, _ = args.Get(1).([]*entity.CredentialClaim)
		}).
		Return(nil).
		Once()

	count, err := uc.FillCredentialPool(context.Background(), "p-1", 3)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	require.Len(t, saved, 3)

	// 每个槽位带一个随机的假名，彼此不同。
	seen := map[string]bool{}
	for _, claim := range saved {
		assert.Equal(t, "p-1", claim.ProposalID)
		assert.NotEmpty(t, claim.UserID)
		assert.False(t, seen[claim.UserID], "槽位的 user_id 不能重复")
		seen[claim.UserID] = true
		assert.Nil(t, claim.ClaimedAt)
	}
}

func TestRegisterUseCase_FillCredentialPool_RejectsBadInput(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	_, err := uc.FillCredentialPool(context.Background(), "", 1)
	assert.ErrorIs(t, err, ErrInvalidRequest)

	_, err = uc.FillCredentialPool(context.Background(), "p-1", 0)
	assert.ErrorIs(t, err, ErrInvalidRequest)

	_, err = uc.FillCredentialPool(context.Background(), "p-1", maxCredentialPoolSize+1)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRegisterUseCase_ClaimCredential(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	claimRepo.On("TakeAvailable", mock.Anything, "p-1").
		Return(&entity.CredentialClaim{ID: "slot-1", ProposalID: "p-1", UserID: "pseudonym-1"}, nil).
		Once()
	issuer.On("Issue", mock.Anything, "pseudonym-1", []byte("claimant-pub")).
		Return(&entity.Credential{Signature: []byte("sig"), IssuerKeyVersion: 3}, nil).
		Once()
	claimRepo.On("MarkClaimed", mock.Anything, "slot-1", []byte("claimant-pub"), []byte("sig"), mock.AnythingOfType("time.Time")).
		Return(true, nil).
		Once()

	var saved *entity.Credential
	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Credential")).
		Run(func(args mock.Arguments) {
			saved, _ = args.Get(1).(*entity.Credential)
		}).
		Return(nil).
		Once()

	credential, err := uc.ClaimCredential(context.Background(), "p-1", []byte("claimant-pub"))
	require.NoError(t, err)
	require.NotNil(t, credential)

	// user_id 来自池里的假名，不是领取者提供的；公钥才是领取者带来的。
	assert.Equal(t, "pseudonym-1", credential.UserID)
	assert.Equal(t, []byte("claimant-pub"), credential.UserPubKey)
	assert.Equal(t, "p-1", credential.ProposalID)
	assert.Equal(t, 3, credential.IssuerKeyVersion)
	require.NotNil(t, saved)
	assert.Equal(t, "pseudonym-1", saved.UserID)
}

func TestRegisterUseCase_ClaimCredential_PoolEmpty(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	claimRepo.On("TakeAvailable", mock.Anything, "p-1").
		Return((*entity.CredentialClaim)(nil), repository.ErrNotFound).
		Once()

	credential, err := uc.ClaimCredential(context.Background(), "p-1", []byte("claimant-pub"))
	assert.ErrorIs(t, err, ErrCredentialPoolEmpty)
	assert.Nil(t, credential)
}

func TestRegisterUseCase_ClaimCredential_RetriesWhenSlotTaken(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	// 第一个槽位被别人抢走，第二个才是自己的。
	claimRepo.On("TakeAvailable", mock.Anything, "p-1").
		Return(&entity.CredentialClaim{ID: "slot-1", UserID: "pseudonym-1"}, nil).
		Once()
	claimRepo.On("TakeAvailable", mock.Anything, "p-1").
		Return(&entity.CredentialClaim{ID: "slot-2", UserID: "pseudonym-2"}, nil).
		Once()
	issuer.On("Issue", mock.Anything, mock.Anything, []byte("claimant-pub")).
		Return(&entity.Credential{Signature: []byte("sig")}, nil).
		Twice()
	claimRepo.On("MarkClaimed", mock.Anything, "slot-1", mock.Anything, mock.Anything, mock.Anything).
		Return(false, nil).
		Once()
	claimRepo.On("MarkClaimed", mock.Anything, "slot-2", mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).
		Once()
	repo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Credential")).Return(nil).Once()

	credential, err := uc.ClaimCredential(context.Background(), "p-1", []byte("claimant-pub"))
	require.NoError(t, err)
	assert.Equal(t, "pseudonym-2", credential.UserID)
}

func TestRegisterUseCase_ClaimCredential_RequiresPubKey(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	uc := NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, zap.NewNop())

	_, err := uc.ClaimCredential(context.Background(), "p-1", nil)
	assert.ErrorIs(t, err, ErrInvalidRequest)

	_, err = uc.ClaimCredential(context.Background(), "", []byte("pub"))
	assert.ErrorIs(t, err, ErrInvalidRequest)
}
