package usecase

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/usecase/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestVotingUseCase_Submit_WithGoMock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	voteRepo := mocks.NewMockVoteRepository(ctrl)
	participationRepo := mocks.NewMockParticipationRepository(ctrl)
	credentialRepo := mocks.NewMockCredentialRepository(ctrl)
	proposalRepo := mocks.NewMockProposalRepository(ctrl)
	sessionRepo := mocks.NewMockDecryptionSessionRepository(ctrl)
	verifier := mocks.NewMockProofVerifier(ctrl)
	encrypter := mocks.NewMockEncrypter(ctrl)

	uc := NewVotingUseCase(
		voteRepo,
		participationRepo,
		credentialRepo,
		proposalRepo,
		sessionRepo,
		verifier,
		encrypter,
		zap.NewNop(),
	)
	req := &SubmitVoteRequest{
		ProposalID:          "proposal-1",
		EncryptedVote:       []byte("cipher"),
		Proof:               &entity.Proof{Data: []byte("proof")},
		Nullifier:           "nullifier-1",
		UserID:              "u-1",
		UserPubKey:          []byte("pub"),
		CredentialSignature: []byte("sig"),
	}

	gomock.InOrder(
		proposalRepo.EXPECT().GetByID(gomock.Any(), "proposal-1").Return(&entity.Proposal{
			ID:       "proposal-1",
			Status:   "open",
			StartsAt: time.Now().UTC().Add(-1 * time.Hour),
			EndsAt:   time.Now().UTC().Add(1 * time.Hour),
		}, nil),
		sessionRepo.EXPECT().GetByProposal(gomock.Any(), "proposal-1").Return(&entity.DecryptionSession{
			ProposalID: "proposal-1",
			Threshold:  1,
			Trustees:   1,
			SharesJSON: `["c2hhcmUtMQ=="]`,
		}, nil),
		verifier.EXPECT().Verify(req.Proof, gomock.Any()).Return(nil),
		encrypter.EXPECT().ValidateCiphertext(req.EncryptedVote).Return(nil),
		credentialRepo.EXPECT().GetByProposalAndUserID(gomock.Any(), "proposal-1", "u-1").Return(&entity.Credential{
			ProposalID: "proposal-1",
			UserID:     "u-1",
			UserPubKey: []byte("pub"),
			Signature:  []byte("sig"),
		}, nil),
		voteRepo.EXPECT().ExistsByNullifier(gomock.Any(), "nullifier-1").Return(false, nil),
		participationRepo.EXPECT().HasVoted(gomock.Any(), "proposal-1", hashUserID("u-1")).Return(false, nil),
		voteRepo.EXPECT().Save(gomock.Any(), gomock.AssignableToTypeOf(&entity.Vote{})).Return(nil),
		participationRepo.EXPECT().MarkVoted(gomock.Any(), "proposal-1", hashUserID("u-1"), gomock.Any()).Return(nil),
	)

	err := uc.Submit(context.Background(), req)
	require.NoError(t, err)
}
