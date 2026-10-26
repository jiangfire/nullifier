package usecase

import (
	"context"
	"errors"
	"testing"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// eligibilityServiceMock 是资格校验的假实现。
type eligibilityServiceMock struct {
	mock.Mock
}

func (m *eligibilityServiceMock) Enabled() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *eligibilityServiceMock) Root() (string, int, error) {
	args := m.Called()
	root, _ := args.Get(0).(string)
	depth, _ := args.Get(1).(int)
	return root, depth, args.Error(2)
}

func (m *eligibilityServiceMock) NewChallenge() (string, error) {
	args := m.Called()
	challenge, _ := args.Get(0).(string)
	return challenge, args.Error(1)
}

func (m *eligibilityServiceMock) Verify(
	proof []byte,
	statement domaincrypto.EligibilityStatement,
) error {
	args := m.Called(proof, statement)
	return args.Error(0)
}

var _ domaincrypto.EligibilityService = (*eligibilityServiceMock)(nil)

// eligibilityUseRepositoryMock 是资格使用记录的假实现。
type eligibilityUseRepositoryMock struct {
	mock.Mock
}

func (m *eligibilityUseRepositoryMock) RecordUse(
	ctx context.Context,
	use *entity.EligibilityUse,
) error {
	args := m.Called(ctx, use)
	return args.Error(0)
}

func (m *eligibilityUseRepositoryMock) CountUses(ctx context.Context, proposalID string) (int64, error) {
	args := m.Called(ctx, proposalID)
	return args.Get(0).(int64), args.Error(1)
}

var _ repository.EligibilityUseRepository = (*eligibilityUseRepositoryMock)(nil)

const (
	testEligibilityRoot      = "2113d5db4b0f0a0f0f9d2c1c1e3b5a7d9c0e1f2a3b4c5d6e7f8091a2b3c4d5e6"
	testEligibilityChallenge = "00aabbccddeeff00112233445566778899aabbccddeeff001122334455667788"
	testQualificationID      = "1122334455667788990011223344556677889900112233445566778899001122"
)

// TestRegisterUseCase_IssueEligibilityChallenge 验证挑战签发。
func TestRegisterUseCase_IssueEligibilityChallenge(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()
	eligibility.On("Root").Return(testEligibilityRoot, 10, nil).Once()
	eligibility.On("NewChallenge").Return(testEligibilityChallenge, nil).Once()

	challenge, err := uc.IssueEligibilityChallenge(context.Background(), "p-1")
	require.NoError(t, err)
	assert.Equal(t, "p-1", challenge.ProposalID)
	assert.Equal(t, testEligibilityRoot, challenge.Root)
	assert.Equal(t, 10, challenge.Depth)
	assert.Equal(t, testEligibilityChallenge, challenge.Challenge)
}

// TestRegisterUseCase_IssueEligibilityChallenge_NotEnabled 验证未开启资格校验
// 时不签发挑战。
func TestRegisterUseCase_IssueEligibilityChallenge_NotEnabled(t *testing.T) {
	eligibility := new(eligibilityServiceMock)
	uc := NewRegisterUseCaseWithEligibility(
		new(credentialRepositoryMock), new(proposalRepositoryMock), new(credentialIssuerMock),
		new(credentialClaimRepositoryMock), new(eligibilityUseRepositoryMock),
		eligibility, zap.NewNop(),
	)

	eligibility.On("Enabled").Return(false).Once()

	challenge, err := uc.IssueEligibilityChallenge(context.Background(), "p-1")
	assert.ErrorIs(t, err, ErrEligibilityNotEnabled)
	assert.Nil(t, challenge)
}

// TestRegisterUseCase_ReserveCredential_RequiresEligibility 验证开启资格校验
// 后，没带资格材料的领取被拒，而且**不消耗池槽位**。
func TestRegisterUseCase_ReserveCredential_RequiresEligibility(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", nil)
	assert.ErrorIs(t, err, ErrEligibilityRequired)
	assert.Nil(t, reservation)
	claimRepo.AssertNotCalled(t, "ReserveAvailable", mock.Anything, mock.Anything, mock.Anything)
}

// TestRegisterUseCase_ReserveCredential_InvalidEligibilityProof 验证资格证明
// 没过时不消耗池槽位。
func TestRegisterUseCase_ReserveCredential_InvalidEligibilityProof(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	material := &EligibilityMaterial{
		Proof:           []byte("proof"),
		QualificationID: testQualificationID,
		Challenge:       testEligibilityChallenge,
	}
	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()
	eligibility.On("Root").Return(testEligibilityRoot, 10, nil).Once()
	eligibility.On("Verify", []byte("proof"), domaincrypto.EligibilityStatement{
		Root:            testEligibilityRoot,
		ProposalID:      "p-1",
		Challenge:       testEligibilityChallenge,
		QualificationID: testQualificationID,
	}).Return(errors.New("boom")).Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", material)
	assert.ErrorIs(t, err, ErrInvalidEligibilityProof)
	assert.Nil(t, reservation)
	claimRepo.AssertNotCalled(t, "ReserveAvailable", mock.Anything, mock.Anything, mock.Anything)
}

// TestRegisterUseCase_ReserveCredential_RecordsEligibilityUse 验证资格使用会被
// 记录（一人一票的强制点）。
func TestRegisterUseCase_ReserveCredential_RecordsEligibilityUse(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	material := &EligibilityMaterial{
		Proof:           []byte("proof"),
		QualificationID: testQualificationID,
		Challenge:       testEligibilityChallenge,
	}
	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()
	eligibility.On("Root").Return(testEligibilityRoot, 10, nil).Once()
	eligibility.On("Verify", mock.Anything, mock.Anything).Return(nil).Once()
	claimRepo.On("ReserveAvailable", mock.Anything, "p-1", mock.Anything).
		Return(&entity.CredentialClaim{ID: "slot-1", ProposalID: "p-1", UserID: "pseudonym-1"}, nil).
		Once()
	eligibilityRepo.On("RecordUse", mock.Anything, mock.MatchedBy(
		func(use *entity.EligibilityUse) bool {
			return use.ProposalID == "p-1" && use.QualificationID == testQualificationID
		},
	)).Return(nil).Once()
	issuer.On("IssuerPublicKey", mock.Anything).Return(testIssuerKey(), nil).Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", material)
	require.NoError(t, err)
	assert.Equal(t, "pseudonym-1", reservation.UserID)
}

// TestRegisterUseCase_ReserveCredential_EligibilityAlreadyUsed 验证同一份资格
// 第二次领取被拒。
func TestRegisterUseCase_ReserveCredential_EligibilityAlreadyUsed(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	material := &EligibilityMaterial{
		Proof:           []byte("proof"),
		QualificationID: testQualificationID,
		Challenge:       testEligibilityChallenge,
	}
	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()
	eligibility.On("Root").Return(testEligibilityRoot, 10, nil).Once()
	eligibility.On("Verify", mock.Anything, mock.Anything).Return(nil).Once()
	claimRepo.On("ReserveAvailable", mock.Anything, "p-1", mock.Anything).
		Return(&entity.CredentialClaim{ID: "slot-1", ProposalID: "p-1", UserID: "pseudonym-1"}, nil).
		Once()
	eligibilityRepo.On("RecordUse", mock.Anything, mock.Anything).
		Return(repository.ErrDuplicateEligibilityUse).Once()
	// 资格已被用过 → 槽位不能被白白消耗：预留要释放掉。
	claimRepo.On("ReleaseReservation", mock.Anything, "slot-1").Return(nil).Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", material)
	assert.ErrorIs(t, err, ErrEligibilityAlreadyUsed)
	assert.Nil(t, reservation)
	claimRepo.AssertExpectations(t)
}

// TestRegisterUseCase_ReserveCredential_RejectsMaterialWhenDisabled 验证服务端
// 没开启资格校验时，不接受「看起来验过了」的资格材料。
func TestRegisterUseCase_ReserveCredential_RejectsMaterialWhenDisabled(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibilityRepo := new(eligibilityUseRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, eligibilityRepo, eligibility, zap.NewNop(),
	)

	material := &EligibilityMaterial{
		Proof:           []byte("proof"),
		QualificationID: testQualificationID,
		Challenge:       testEligibilityChallenge,
	}
	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(false).Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", material)
	assert.ErrorIs(t, err, ErrEligibilityNotEnabled)
	assert.Nil(t, reservation)
}

// TestRegisterUseCase_ReserveCredential_RequiresUseRepository 验证「开了资格
// 校验却没有去重仓储」时直接报错 —— 那样等于一人一票没生效。
func TestRegisterUseCase_ReserveCredential_RequiresUseRepository(t *testing.T) {
	repo := new(credentialRepositoryMock)
	proposalRepo := new(proposalRepositoryMock)
	issuer := new(credentialIssuerMock)
	claimRepo := new(credentialClaimRepositoryMock)
	eligibility := new(eligibilityServiceMock)

	uc := NewRegisterUseCaseWithEligibility(
		repo, proposalRepo, issuer, claimRepo, nil, eligibility, zap.NewNop(),
	)

	material := &EligibilityMaterial{
		Proof:           []byte("proof"),
		QualificationID: testQualificationID,
		Challenge:       testEligibilityChallenge,
	}
	proposalRepo.On("GetByID", mock.Anything, "p-1").Return(&entity.Proposal{ID: "p-1"}, nil).Once()
	eligibility.On("Enabled").Return(true).Once()
	eligibility.On("Root").Return(testEligibilityRoot, 10, nil).Once()
	eligibility.On("Verify", mock.Anything, mock.Anything).Return(nil).Once()
	claimRepo.On("ReserveAvailable", mock.Anything, "p-1", mock.Anything).
		Return(&entity.CredentialClaim{ID: "slot-1", ProposalID: "p-1", UserID: "pseudonym-1"}, nil).
		Once()

	reservation, err := uc.ReserveCredential(context.Background(), "p-1", material)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eligibility use repository")
	assert.Nil(t, reservation)
}
