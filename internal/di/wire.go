//go:build wireinject
// +build wireinject

package di

import (
	"anonvote/internal/config"
	httpapi "anonvote/internal/infrastructure/http"

	"github.com/google/wire"
)

func InitializeServer(cfg *config.Config) (*httpapi.Server, error) {
	wire.Build(
		NewLogger,
		NewDB,
		NewVoteRepository,
		NewCredentialRepository,
		NewCredentialClaimRepository,
		NewEligibilityUseRepository,
		NewProposalRepository,
		NewParticipationRepository,
		NewDecryptionSessionRepository,
		NewTallyResultRepository,
		NewAuditAnchorRepository,
		NewProofVerifier,
		NewEncrypter,
		NewThresholdCrypto,
		NewMerkleRootBuilder,
		NewMetrics,
		NewCredentialIssuer,
		NewEligibilityService,
		NewVotingUseCase,
		NewRegisterUseCase,
		NewProposalUseCase,
		NewParticipationUseCase,
		NewTallyUseCase,
		NewHTTPHandler,
		NewHTTPServer,
	)
	return nil, nil
}
