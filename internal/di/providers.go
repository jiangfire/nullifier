package di

import (
	"anonvote/internal/config"
	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/repository"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

func NewLogger(cfg *config.Config) (*zap.Logger, error) {
	if cfg != nil {
		switch cfg.GinMode {
		case "debug", "test":
			loggerCfg := zap.NewDevelopmentConfig()
			loggerCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
			return loggerCfg.Build()
		}
	}
	return zap.NewProduction()
}

func NewDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := persistence.NewDB(cfg.DB)
	if err != nil {
		return nil, err
	}
	if err := persistence.AutoMigrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func NewVoteRepository(db *gorm.DB) repository.VoteRepository {
	return persistence.NewVoteRepository(db)
}

func NewCredentialRepository(db *gorm.DB) repository.CredentialRepository {
	return persistence.NewCredentialRepository(db)
}

func NewProposalRepository(db *gorm.DB) repository.ProposalRepository {
	return persistence.NewProposalRepository(db)
}

func NewParticipationRepository(db *gorm.DB) repository.ParticipationRepository {
	return persistence.NewParticipationRepository(db)
}

func NewDecryptionSessionRepository(db *gorm.DB) repository.DecryptionSessionRepository {
	return persistence.NewDecryptionSessionRepository(db)
}

func NewTallyResultRepository(db *gorm.DB) repository.TallyResultRepository {
	return persistence.NewTallyResultRepository(db)
}

func NewAuditAnchorRepository(db *gorm.DB) repository.AuditAnchorRepository {
	return persistence.NewAuditAnchorRepository(db)
}

func NewProofVerifier(cfg *config.Config) (domaincrypto.ProofVerifier, error) {
	return cryptoinfra.NewGnarkProofVerifier(cfg.VerifyingKeyPath)
}

func NewEncrypter() domaincrypto.Encrypter {
	return cryptoinfra.NewKyberElGamal()
}

func NewThresholdCrypto() usecase.ThresholdCrypto {
	return cryptoinfra.NewKyberElGamal()
}

func NewMerkleRootBuilder() usecase.MerkleRootBuilder {
	return cryptoinfra.NewGnarkMerkleRootBuilder()
}

func NewCredentialIssuer(cfg *config.Config) domaincrypto.CredentialIssuer {
	return cryptoinfra.NewKryptologyCredentialIssuerWithFile(
		cfg.CredentialIssuerKeyPath,
		cfg.CredentialIssuerRotateAfter,
	)
}

func NewVotingUseCase(
	repo repository.VoteRepository,
	participationRepo repository.ParticipationRepository,
	credentialRepo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	sessionRepo repository.DecryptionSessionRepository,
	verifier domaincrypto.ProofVerifier,
	encrypter domaincrypto.Encrypter,
	logger *zap.Logger,
) *usecase.VotingUseCase {
	return usecase.NewVotingUseCase(
		repo,
		participationRepo,
		credentialRepo,
		proposalRepo,
		sessionRepo,
		verifier,
		encrypter,
		logger,
	)
}

func NewRegisterUseCase(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	logger *zap.Logger,
) *usecase.RegisterUseCase {
	return usecase.NewRegisterUseCase(repo, proposalRepo, issuer, logger)
}

func NewProposalUseCase(
	repo repository.ProposalRepository,
	logger *zap.Logger,
) *usecase.ProposalUseCase {
	return usecase.NewProposalUseCase(repo, logger)
}

func NewParticipationUseCase(
	repo repository.ParticipationRepository,
	credentialRepo repository.CredentialRepository,
	logger *zap.Logger,
) *usecase.ParticipationUseCase {
	return usecase.NewParticipationUseCase(repo, credentialRepo, logger)
}

func NewTallyUseCase(
	voteRepo repository.VoteRepository,
	proposalRepo repository.ProposalRepository,
	sessionRepo repository.DecryptionSessionRepository,
	resultRepo repository.TallyResultRepository,
	anchorRepo repository.AuditAnchorRepository,
	crypto usecase.ThresholdCrypto,
	merkleBuilder usecase.MerkleRootBuilder,
	logger *zap.Logger,
) *usecase.TallyUseCase {
	return usecase.NewTallyUseCase(
		voteRepo,
		proposalRepo,
		sessionRepo,
		resultRepo,
		anchorRepo,
		crypto,
		merkleBuilder,
		logger,
	)
}

func NewHTTPHandler(
	votingUC *usecase.VotingUseCase,
	registerUC *usecase.RegisterUseCase,
	proposalUC *usecase.ProposalUseCase,
	participationUC *usecase.ParticipationUseCase,
	tallyUC *usecase.TallyUseCase,
	logger *zap.Logger,
) *httpapi.Handler {
	return httpapi.NewHandlerWithTally(votingUC, registerUC, proposalUC, participationUC, tallyUC, logger)
}

func NewHTTPServer(cfg *config.Config, handler *httpapi.Handler, logger *zap.Logger) *httpapi.Server {
	return httpapi.NewServer(cfg, handler, logger)
}
