## 1. 架构设计（Clean Architecture）

```
anonvote/
├── internal/
│   ├── domain/              # 实体与接口（零外部依赖，纯Go标准库）
│   │   ├── entity/          # 聚合根（Vote, Proposal, Credential）
│   │   └── repository/      # 仓储接口（依赖倒置）
│   ├── usecase/             # 应用层（业务逻辑，依赖domain接口）
│   ├── infrastructure/      # 基础设施（具体实现，可替换）
│   │   ├── crypto/          # gnark/kyber封装
│   │   ├── persistence/     # gorm实现
│   │   └── http/            # gin handler
│   └── di/                  # wire依赖注入容器
├── cmd/server/              # 启动入口
├── tests/integration/       # 集成测试（docker-compose）
└── web/                     # Vue3前端
```

**核心原则**：
- **SOLID**：`domain`层定义接口，`infrastructure`层实现，通过`wire`注入
- **KISS**：单体应用，无需消息队列，PostgreSQL存储所有状态
- **DRY**：通用错误处理、通用CRUD基类、共享的gnark电路配置

## 2. 多环境数据库架构

### 环境配置策略

| 环境 | 数据库 | 用途 | 配置方式 |
|------|--------|------|----------|
| **测试** | SQLite3 (:memory:) | 单元测试、快速反馈 | 代码自动选择 |
| **开发** | SQLite3 (file) | 本地开发、无需Docker | 默认配置 |
| **生产** | PostgreSQL | 高并发、数据持久化 | 环境变量覆盖 |

### GORM多驱动配置

```go
// internal/infrastructure/persistence/db.go
package persistence

import (
    "fmt"
    "os"

    "gorm.io/driver/postgres"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

// DBConfig 数据库配置
type DBConfig struct {
    Driver   string // "sqlite" | "postgres"
    Host     string
    Port     int
    Database string
    Username string
    Password string
    SSLMode  string
}

// NewDB 根据配置创建对应的数据库连接
func NewDB(cfg *DBConfig) (*gorm.DB, error) {
    switch cfg.Driver {
    case "postgres":
        return newPostgresDB(cfg)
    case "sqlite":
        return newSQLiteDB(cfg)
    default:
        return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
    }
}

func newPostgresDB(cfg *DBConfig) (*gorm.DB, error) {
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode)

    return gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })
}

func newSQLiteDB(cfg *DBConfig) (*gorm.DB, error) {
    // :memory: 用于测试，文件用于开发
    dsn := cfg.Database
    if dsn == "" {
        dsn = ":memory:"
    }

    return gorm.Open(sqlite.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })
}
```

### 环境检测工具函数

```go
// internal/infrastructure/persistence/db_env.go
package persistence

import "os"

// GetDBConfigFromEnv 从环境变量读取数据库配置
func GetDBConfigFromEnv() *DBConfig {
    driver := os.Getenv("DB_DRIVER")
    if driver == "" {
        // 默认使用SQLite3（开发环境）
        driver = "sqlite"
    }

    cfg := &DBConfig{Driver: driver}

    switch driver {
    case "postgres":
        cfg.Host = getEnv("DB_HOST", "localhost")
        cfg.Port = getEnvInt("DB_PORT", 5432)
        cfg.Database = getEnv("DB_NAME", "anonvote")
        cfg.Username = getEnv("DB_USER", "postgres")
        cfg.Password = getEnv("DB_PASSWORD", "")
        cfg.SSLMode = getEnv("DB_SSLMODE", "disable")
    case "sqlite":
        cfg.Database = getEnv("DB_PATH", "anonvote.db")
        // 测试环境使用 :memory:
        if os.Getenv("GO_ENV") == "test" {
            cfg.Database = ":memory:"
        }
    }

    return cfg
}

func getEnv(key, defaultVal string) string { /* ... */ }
func getEnvInt(key string, defaultVal int) int { /* ... */ }
```

### 迁移兼容性

```go
// internal/infrastructure/persistence/migration.go
package persistence

import (
    "gorm.io/gorm"
    "anonvote/internal/domain/entity"
)

// AutoMigrate 自动迁移所有实体（支持SQLite和PostgreSQL）
func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &entity.Vote{},
        &entity.Proposal{},
        &entity.Credential{},
    )
}

// 注意：SQLite部分字段类型有差异，GORM已处理大部分兼容性问题
// 如需原生SQL，使用db.Dialector.Name()判断数据库类型
```

## 4. 技术栈（全部选成熟工业级库）

| 用途 | 库 | 理由 |
|------|----|----|
| **ZKP** | `github.com/consensys/gnark` | Consensys官方，性能最优，支持Groth16/PLONK |
| **高级密码学** | `github.com/drand/kyber` | DEDIS实验室出品，支持ElGamal、阈值加密 |
| **盲签名** | `github.com/coinbase/kryptology` | Coinbase开源，生产级盲RSA实现（RFC 9474） |
| **HTTP** | `github.com/gin-gonic/gin` | 性能强，中间件生态成熟 |
| **ORM** | `gorm.io/gorm` | 支持PostgreSQL/SQLite，迁移方便 |
| **数据库驱动** | `gorm.io/driver/postgres` + `gorm.io/driver/sqlite` | PostgreSQL生产环境，SQLite3测试/开发环境 |
| **配置** | `github.com/spf13/viper` | 支持YAML/ENV，热加载 |
| **日志** | `go.uber.org/zap` | 结构化日志，性能极致 |
| **测试** | `github.com/stretchr/testify` + `gomock` | 断言、Mock、Suite测试 |
| **DI** | `github.com/google/wire` | 编译期依赖注入，零运行时开销 |
| **验证** | `github.com/go-playground/validator/v10` | 结构体验证，标签驱动 |

**绝不自研**：
- 不自己写ZKP电路算法（用gnark标准库）
- 不自己写盲签名（用kryptology）
- 不自己写Merkle树（用gnark标准库merkle包）

## 5. TDD实现流程（红-绿-重构）

### 步骤1：先写接口与测试（红）
```go
// internal/domain/repository/vote_repo.go
package repository

import "context"

type VoteRepository interface {
    Save(ctx context.Context, vote *entity.Vote) error
    ExistsByNullifier(ctx context.Context, nullifier string) (bool, error)
    GetByProposal(ctx context.Context, proposalID string) ([]*entity.Vote, error)
}

// internal/usecase/voting_test.go（TDD先行）
package usecase

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestVotingUseCase_Submit(t *testing.T) {
    // Arrange
    mockRepo := new(mock.VoteRepositoryMock)
    mockVerifier := new(mock.ProofVerifierMock)
    mockEncrypter := new(mock.EncrypterMock)
    
    uc := NewVotingUseCase(mockRepo, mockVerifier, mockEncrypter, zap.NewNop())
    
    // 输入数据
    req := &SubmitVoteRequest{
        ProposalID:    "prop-001",
        EncryptedVote: []byte("encrypted"),
        Proof:         &entity.Proof{Data: []byte("proof")},
        Nullifier:     "null-123",
    }
    
    // 期望行为（Mock）
    mockVerifier.On("Verify", req.Proof, mock.Anything).Return(nil)
    mockRepo.On("ExistsByNullifier", mock.Anything, "null-123").Return(false, nil)
    mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*entity.Vote")).Return(nil)
    
    // Act
    err := uc.Submit(context.Background(), req)
    
    // Assert
    assert.NoError(t, err)
    mockRepo.AssertExpectations(t)
}
```

### 步骤2：实现业务逻辑（绿）
```go
// internal/usecase/voting.go
package usecase

type VotingUseCase struct {
    repo      repository.VoteRepository
    verifier  crypto.ProofVerifier
    encrypter crypto.Encrypter
    logger    *zap.Logger
}

// 构造函数（依赖注入，SOLID的D原则）
func NewVotingUseCase(
    repo repository.VoteRepository,
    verifier crypto.ProofVerifier,
    encrypter crypto.Encrypter,
    logger *zap.Logger,
) *VotingUseCase {
    return &VotingUseCase{
        repo:      repo,
        verifier:  verifier,
        encrypter: encrypter,
        logger:    logger,
    }
}

func (uc *VotingUseCase) Submit(ctx context.Context, req *SubmitVoteRequest) error {
    // 1. 验证ZKP证明（接口抽象，可替换实现）
    if err := uc.verifier.Verify(req.Proof, req.PublicInputs()); err != nil {
        return fmt.Errorf("invalid proof: %w", err)
    }
    
    // 2. 检查重复投票（单一职责，S原则）
    exists, err := uc.repo.ExistsByNullifier(ctx, req.Nullifier)
    if err != nil {
        return err
    }
    if exists {
        return ErrDoubleVoting
    }
    
    // 3. 存储（开闭原则，O：以后换存储只需改repo实现）
    vote := &entity.Vote{
        ID:        uuid.New().String(),
        ProposalID: req.ProposalID,
        Nullifier: req.Nullifier,
        Cipher:    req.EncryptedVote,
        CreatedAt: time.Now(),
    }
    
    return uc.repo.Save(ctx, vote)
}
```

### 步骤3：基础设施实现（隔离具体技术）
```go
// internal/infrastructure/crypto/gnark_prover.go
package crypto

import (
    "github.com/consensys/gnark/backend"
    "github.com/consensys/gnark/backend/groth16"
    "github.com/consensys/gnark/frontend"
)

// GnarkProofVerifier 具体实现（符合接口即可替换）
type GnarkProofVerifier struct {
    verifyingKey groth16.VerifyingKey
}

// 构造函数（工厂模式）
func NewGnarkProofVerifier(vkPath string) (*GnarkProofVerifier, error) {
    vk := groth16.NewVerifyingKey(ecc.BN254)
    // 从文件加载vk...
    return &GnarkProofVerifier{verifyingKey: vk}, nil
}

// Verify 实现domain接口（Liskov替换，L原则）
func (v *GnarkProofVerifier) Verify(proof *domain.Proof, inputs domain.PublicInputs) error {
    // 使用gnark标准库验证，不自研算法（KISS）
    grothProof := groth16.NewProof(ecc.BN254)
    // 反序列化...
    return groth16.Verify(grothProof, v.verifyingKey, inputs.ToAssignment())
}
```

## 6. DRY原则落地（通用抽象）

### 通用CRUD基类（避免重复数据库代码）
```go
// internal/infrastructure/persistence/base.go
package persistence

import (
    "gorm.io/gorm"
    "context"
)

// BaseRepository 泛型基类（DRY）
type BaseRepository[T any] struct {
    db *gorm.DB
}

func NewBaseRepository[T any](db *gorm.DB) *BaseRepository[T] {
    return &BaseRepository[T]{db: db}
}

func (r *BaseRepository[T]) Create(ctx context.Context, entity *T) error {
    return r.db.WithContext(ctx).Create(entity).Error
}

func (r *BaseRepository[T]) GetByID(ctx context.Context, id string) (*T, error) {
    var entity T
    err := r.db.WithContext(ctx).First(&entity, "id = ?", id).Error
    return &entity, err
}

// VoteRepository 继承基类，只写特有逻辑
type VoteRepository struct {
    *BaseRepository[entity.Vote]  // 嵌入（Go的组合代替继承）
    db *gorm.DB
}

func (r *VoteRepository) ExistsByNullifier(ctx context.Context, nullifier string) (bool, error) {
    var count int64
    err := r.db.WithContext(ctx).Model(&entity.Vote{}).
        Where("nullifier = ?", nullifier).Count(&count).Error
    return count > 0, err
}
```

### 通用错误处理中间件（DRY）
```go
// internal/infrastructure/http/middleware/error_handler.go
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        
        if len(c.Errors) > 0 {
            err := c.Errors.Last().Err
            logger.Error("request failed", zap.Error(err))
            
            // 统一错误格式
            c.JSON(500, gin.H{
                "error": err.Error(),
                "code":  "INTERNAL_ERROR",
            })
        }
    }
}
```

## 7. KISS简化策略

**数据库选型原则**：
- **开发/测试**：SQLite3（零配置，开箱即用）
- **生产**：PostgreSQL（高性能、ACID、并发安全）
- **代码层无差异**：GORM抽象所有差异，业务代码无需关心底层数据库

**极简密码学方案**：
- **无需完整ZKP电路**：使用**zk-SNARKs**证明"我持有盲签名"，而非复杂成员证明
- **简化加密**：使用ElGamal（kyber实现），无需同态加密，投票结束阈值解密即可
- **无需区块链**：PostgreSQL + 定期导出的**Merkle树根哈希**（作为审计锚点）足够满足大多数企业场景

**盲签名流程（极简3步）**：
```go
// internal/usecase/registration.go
// 使用kryptology库，不自己实现RSA盲签名（KISS）

func (uc *RegisterUseCase) IssueCredential(ctx context.Context, userID string) (*entity.Credential, error) {
    // 1. 验证用户身份（调用HR系统SSO，此处省略）
    
    // 2. 生成盲化因子（使用kryptology/blindsig）
    blindedMsg, secretFactor, err := blindsig.Blind(userPubKey)
    if err != nil {
        return nil, err
    }
    
    // 3. 管理员签名（使用kryptology的RSA私钥）
    blindSig, err := uc.issuer.Sign(blindedMsg) // 调用kryptology API
    
    // 4. 去盲化（本地完成，管理员无法关联）
    clearSig, err := blindsig.Unblind(blindSig, secretFactor)
    
    return &entity.Credential{
        UserPubKey: userPubKey,
        Signature:  clearSig,
    }, nil
}
```

## 8. 依赖注入配置

```go
// internal/di/wire.go
//go:build wireinject
// +build wireinject

package di

import (
    "github.com/google/wire"
)

func InitializeServer(cfg *config.Config) (*http.Server, error) {
    wire.Build(
        // 基础设施层
        persistence.NewDB,                    // GORM连接
        persistence.NewVoteRepository,        // 仓储实现
        crypto.NewGnarkProofVerifier,         // ZKP验证器
        crypto.NewKryptologyCredentialIssuer, // 盲签名颁发
        
        // 应用层
        usecase.NewVotingUseCase,
        usecase.NewRegisterUseCase,
        
        // 接口层
        http.NewHandler,
        http.NewServer,
        
        // 第三方库
        zap.NewProduction,
    )
    return &http.Server{}, nil
}
```

**生成命令**：
```bash
wire ./internal/di
```

## 9. 测试策略

### 测试数据库选择

| 测试类型 | 数据库 | 理由 |
|----------|--------|------|
| **单元测试** | SQLite3 :memory: | 速度快，无外部依赖 |
| **集成测试** | testcontainers(PostgreSQL) | 验证真实PG行为 |
| **E2E测试** | 可配置（默认SQLite） | 简化CI/CD |

### 测试工具函数

```go
// tests/testutil/db.go
package testutil

import (
    "testing"
    "anonvote/internal/infrastructure/persistence"
)

// SetupTestDB 为测试创建内存数据库
func SetupTestDB(t *testing.T) *gorm.DB {
    cfg := &persistence.DBConfig{
        Driver:   "sqlite",
        Database: ":memory:",
    }

    db, err := persistence.NewDB(cfg)
    if err != nil {
        t.Fatalf("failed to create test db: %v", err)
    }

    // 自动迁移
    if err := persistence.AutoMigrate(db); err != nil {
        t.Fatalf("failed to migrate: %v", err)
    }

    return db
}
```

### 目录结构

```
tests/
├── unit/                    # 单元测试（80%）
│   ├── domain/              # 实体业务规则（纯Go，无依赖）
│   └── usecase/             # 用例层（使用gomock + SQLite内存DB）
├── integration/             # 集成测试（15%）
│   ├── crypto_test.go       # 测试gnark电路（真实证明/验证）
│   └── persistence_test.go  # 测试GORM+PG（testcontainers）
├── testutil/                # 测试工具
│   └── db.go                # 数据库测试辅助函数
└── e2e/                     # 端到端测试（5%）
    └── api_test.go          # 测试完整投票流程
```

**集成测试示例**（使用`testcontainers-go`启动真实PG）：
```go
func TestVoteRepository_Postgres(t *testing.T) {
    ctx := context.Background()
    req := testcontainers.ContainerRequest{
        Image:        "postgres:16-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER":     "test",
            "POSTGRES_PASSWORD": "test",
            "POSTGRES_DB":       "anonvote",
        },
        WaitingFor: wait.ForListeningPort("5432/tcp"),
    }
    pgC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    require.NoError(t, err)
    defer pgC.Terminate(ctx)
    
    // 运行实际测试...
}
```

## 10. 开发Checklist

**开发前必读**：
- [ ] 所有密码学操作必须使用`kryptology`或`gnark`标准API，禁止自研算法
- [ ] 所有数据访问必须通过`repository`接口，禁止在handler写SQL（SOLID）
- [ ] 每个接口必须有Mock实现（使用`mockgen`），确保可测试
- [ ] 业务逻辑必须先写`xxx_test.go`，覆盖率>80%才能合并（TDD）
- [ ] 通用逻辑必须抽离到`pkg/`或基类（DRY）

**代码审查红线**：
- 出现`import "crypto/rsa"` + 自定义盲签名逻辑 → **驳回**（必须用kryptology）
- Handler直接调用`db.Create` → **驳回**（必须走repository接口）
- 超过200行的函数 → **必须拆分**（KISS）