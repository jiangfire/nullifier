package di

import (
	"fmt"

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
	// 自动迁移默认只在非 release 模式开启。release 模式下改表是一个
	// 独立的步骤（见 Migrate），不该是服务启动的副作用。
	if cfg.AutoMigrate {
		if err := persistence.AutoMigrate(db); err != nil {
			return nil, err
		}
		return db, nil
	}
	// 关掉自动迁移时，表结构必须已经就绪，否则服务起来之后第一个请求
	// 才会报「no such table」。这里直接启动失败，把问题留在启动日志里。
	if !persistence.SchemaReady(db) {
		return nil, fmt.Errorf(
			"database schema is not initialized: run `nullifier-server -migrate` before starting the service",
		)
	}
	return db, nil
}

// Migrate 执行数据库结构迁移，是一个可以单独跑、单独回滚的步骤。
func Migrate(cfg *config.Config) error {
	db, err := persistence.NewDB(cfg.DB)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer func() { _ = sqlDB.Close() }()
	}
	return persistence.AutoMigrate(db)
}

func NewVoteRepository(db *gorm.DB) repository.VoteRepository {
	return persistence.NewVoteRepository(db)
}

func NewCredentialRepository(db *gorm.DB) repository.CredentialRepository {
	return persistence.NewCredentialRepository(db)
}

func NewCredentialClaimRepository(db *gorm.DB) repository.CredentialClaimRepository {
	return persistence.NewCredentialClaimRepository(db)
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
	return cryptoinfra.NewBallotThresholdCrypto()
}

func NewThresholdCrypto() usecase.ThresholdCrypto {
	return cryptoinfra.NewBallotThresholdCrypto()
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
	issuer domaincrypto.CredentialIssuer,
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
		issuer,
		logger,
	)
}

func NewRegisterUseCase(
	repo repository.CredentialRepository,
	proposalRepo repository.ProposalRepository,
	issuer domaincrypto.CredentialIssuer,
	claimRepo repository.CredentialClaimRepository,
	logger *zap.Logger,
) *usecase.RegisterUseCase {
	return usecase.NewRegisterUseCaseWithPool(repo, proposalRepo, issuer, claimRepo, logger)
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
	return httpapi.NewHandlerWithTally(
		votingUC,
		registerUC,
		proposalUC,
		participationUC,
		tallyUC,
		logger,
	)
}

func NewHTTPServer(cfg *config.Config, handler *httpapi.Handler, logger *zap.Logger) *httpapi.Server {
	return httpapi.NewServer(cfg, handler, logger)
}
