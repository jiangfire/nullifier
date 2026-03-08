package usecase

import (
	"context"
	"testing"

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
	verifier := mocks.NewMockProofVerifier(ctrl)
	encrypter := mocks.NewMockEncrypter(ctrl)

	uc := NewVotingUseCase(voteRepo, participationRepo, credentialRepo, verifier, encrypter, zap.NewNop())
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
		verifier.EXPECT().Verify(req.Proof, gomock.Any()).Return(nil),
		encrypter.EXPECT().ValidateCiphertext(req.EncryptedVote).Return(nil),
		credentialRepo.EXPECT().GetByUserID(gomock.Any(), "u-1").Return(&entity.Credential{
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
