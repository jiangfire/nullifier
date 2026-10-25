# nullifier 项目地图（上下文索引）

> 本文档是 **Level 3 上下文**（分层摘要）。目的是让 agent / 人在进入某个子系统前，
> 只加载该子系统需要的那一节，而不是把整个仓库塞进上下文。
>
> 上下文层级：
> - **Level 1 规则文件**：`AGENTS.md`（仓库根）、`web/AGENTS.md`
> - **Level 2 规格文档**：`docs/techspec.md`（当前实现口径）、`README.md`（对外说明）、
>   `docs/deployment-checklist.md`（上线前的逐项核对清单）
> - **Level 3 项目地图**：本文档 —— 分层索引 + 模式 + 陷阱
> - **Level 4 具体源码**：按需读取，本文每节都给出精确路径
>
> 改接口或流程时，**先更新 `docs/techspec.md`**，再更新本文档。

---

## 0. 一句话架构

匿名投票系统：**发资格的人看不到选谁**（凭证与选票内容完全无关）→ **记选票的人看不到是谁**（votes 表无用户标识）→
**验票真伪但不验身份**（零知识证明）。Go 后端（Gin + GORM + Wire）+ Vue 3 门户 + 复用同一套 HTTP API 的 CLI。

技术栈：Go 1.25.4 · Gin 1.12 · GORM 1.31 · Wire · gnark 0.7.1（BN254：Groth16 + twisted Edwards）· circl blindrsa（kryptology 1.8）· SQLite/PostgreSQL · Vue 3.5 + TypeScript 5.9 + Rsbuild

选票密码学栈（`internal/infrastructure/crypto/ballot/`）整体跑在 **BN254 twisted Edwards（jubjub）** 上：
选票密文 v3、门限份额、零知识电路三者同曲线，服务端因此既能「拿不到明文选项」，又能「验证明真伪」。

---

## 1. 分层地图

### 1.1 启动与装配 —— `cmd/` + `internal/di/`

| 位置 | 职责 |
|---|---|
| `cmd/server/main.go` | 服务入口。加载配置 → 生成/校验 admin token → 建 logger → `di.InitializeServer` → `server.Run()`；`-migrate` 只跑迁移后退出（`serve(migrateOnly)`） |
| `cmd/keygen/main.go` | 生成选票电路的 Groth16 密钥：`verifying.key`（0644，服务端用）/ `proving.key`（0600，**只在客户端侧**）；打印 verifying key 的 sha256 指纹；默认不覆盖已有文件（`-force` 才覆盖） |
| `cmd/cli/main.go` | 15 个子命令的 CLI 客户端，**只走 HTTP API**，不绕过服务端校验 |
| `internal/di/providers.go` | 全部 Wire provider 的实现体（logger/db/各仓储/各 usecase/handler）；密码学的装配点在这里：`NewProofVerifier` `NewEncrypter` `NewThresholdCrypto` `NewNullifierGenerator` `NewCredentialIssuer` |
| `internal/di/wire.go` + `wire_gen.go` | Wire 声明与生成产物。**当前是手工同步的**（见陷阱） |
| `internal/config/config.go` | viper 加载 + 运行期校验；`server.mode` 推导 `Config.DebugMode`（`isDebugMode`），release/prod/production 走 `IsReleaseMode`；`CONFIG_FILE` 显式指定配置文件路径 |
| `internal/config/admin_token.go` | admin bearer token 兜底生成（32 字节随机，base64url） |

**模式**：usecase 构造函数是纯 DI 传参，无全局变量；logger 允许为 nil（自动降级 `zap.NewNop()`）。

**陷阱**：
- `cmd/server/main.go` 的 `serve()` —— **只有「临时生成」的 admin token 会进日志**（`generatedAdminToken` 分支的 Warn）；配置态的 token 不写日志。启动日志里有 `config_file`（实际加载的配置文件路径）、`proof_verification_enabled` 字段，以及 `crypto.vk_path is empty` 的告警。
- `internal/config/config.go` 的 `load()` —— `server.mode` 默认 `release`；release/prod/production **强制要求** `crypto.vk_path` **和** `server.admin_bearer_token`（`IsReleaseMode`）。配置文件缺失时服务直接启动失败，这是有意的兜底。
- `internal/config/config.go` 的 `load()` —— 配置文件默认找名为 `config.*` 的文件（`SetConfigName("config")`）；**`CONFIG_FILE` 环境变量可以显式指定路径**（`ConfigFileEnvVar`），生产环境用它加载 `configs/config.production.yaml`。显式文件不存在 → 启动失败。
- **`internal/di/wire_gen.go` 目前是手工同步的**：本仓库离线跑不了 wire 生成器（`go.sum` 里 `github.com/google/subcommands` 只有 `/go.mod` 哈希，缺模块内容哈希，`go run github.com/google/wire/cmd/wire` 需要联网补依赖）。改了 `internal/di/wire.go` 必须**手工**改 `wire_gen.go`，provider 的声明顺序与实参顺序要一起对齐。
- `go.mod` 里还留着 `go.dedis.ch/kyber/v3`，但仓库里已经**没有任何 Go 文件引用它**（`kyber_elgamal.go` 已删）；`go mod tidy` 会把它摘掉。

### 1.2 领域层 —— `internal/domain/`

只放实体、接口、领域错误。**不依赖 HTTP / GORM / 具体密码学库**。

| 子包 | 内容 |
|---|---|
| `domain/entity/` | `Proposal` `Vote` `Credential` `CredentialClaim` `VoteParticipation` `DecryptionSession` `TallyResult` `AuditAnchor` `Proof` `BallotStatement` |
| `domain/repository/` | 8 个仓储接口 + `errors.go`（`ErrNotFound` `ErrDuplicateVote` `ErrDuplicateParticipation`） |
| `domain/crypto/` | 4 个接口：`ProofVerifier` `Encrypter` `CredentialIssuer` `NullifierGenerator` |

**本轮的关键类型变更**：
- `domain/entity/proof.go` 的 `BallotStatement` 取代了旧的 `PublicInputs`：它是**服务端拼装**的公开输入声明（`ProposalID` `Nullifier` `ElectionPublicKey` `Ciphertext` `NumOptions`）。客户端不提交公开输入。
- `domain/crypto/proof_verifier.go`：`Verify(proof *entity.Proof, statement entity.BallotStatement) error` —— 不再接受客户端自由提交的公开输入。
- `domain/crypto/credential_issuer.go`：`CredentialIssuer` 增加 `VerifyCredential(userID, userPubKey, signature)`；投票路径落库前必须调用它。
- `domain/crypto/nullifier.go`：`NullifierGenerator.GenerateNullifier() (string, error)` —— 要求 nullifier 的**编码规范唯一**（见 §3 陷阱 2）。

**模式**：GORM tag 直接写在实体上（`entity.Vote` 的 `Nullifier` 是 `uniqueIndex`，`entity.VoteParticipation` 的 `(proposal_id, user_hash)` 是复合 `uniqueIndex:idx_vote_participation`）。数据库唯一约束是防重投的**最终防线**，不是 usecase 里的先查后写。

**陷阱**：`entity.Proposal.Options` 是 `gorm:"-"`（不落库），真正落库的是 `OptionsJSON`（`type:text`）。读出来需要仓储层反序列化。

### 1.3 用例层 —— `internal/usecase/`

5 个用例，依赖方向只朝内（只依赖 `domain` 接口）：

| 用例 | 文件 | 负责 |
|---|---|---|
| `ProposalUseCase` | `proposal.go` | 创建/列表；创建时去重、去空、校验 ≥2 选项、`ends_at > starts_at`；新提案 `Status` 固定为 `open` |
| `RegisterUseCase` | `registration.go` | 签发凭证；凭证池 + 投票人匿名领取 |
| `VotingUseCase` | `voting.go` | **核心**。见 §2.1 投票链路 |
| `ParticipationUseCase` | `participation.go` | 「你投过票吗」；`HasVotedWithCredential` 必须带凭证材料 |
| `TallyUseCase` | `tally.go` | 门限会话初始化、发布结果、审计锚点。见 §2.2 计票链路 |

**投票路径的两个新环节**：
- `SubmitVoteRequest.PublicInputs(proposal, session)` —— 用**请求字段 + 提案 + 计票会话**拼出 `entity.BallotStatement`。客户端不提交公开输入，因此一份 proof 只能用于它被生成的那份密文。
- `decodeNullifier` —— 请求侧早期校验：十六进制可解码 + 恰好 32 字节（`entity.NullifierByteSize`）。**规范编码的检查在密码学层**（`ballot.ParseScalarHex` / `ballot.BuildPublicInputs`）。
- `issuer.VerifyCredential(userID, userPubKey, signature)` —— 落库前验签；失败 → `ErrInvalidCredential` → HTTP 401。

**计票链路的密码学口径变了**：`tally.go` 的 `ThresholdCrypto` 接口现在是 `Encrypt(publicKey []byte, optionIndex int)` / `Decrypt(privateKey, ciphertext []byte, numOptions int) (int, error)` —— 加密与解密的单位是**选项下标**，不是任意明文。

**模式**：错误用 `fmt.Errorf("%w: ...", ErrXxx)` 包装 `internal/usecase/errors.go` 里的哨兵错误；HTTP 层靠 `errors.Is` 映射状态码。

**陷阱**：`voting.go` 有**两条保存路径**——仓储实现了 `atomicVoteParticipationSaver` 就走事务（`SaveWithParticipation`），否则退化成先存票再存参与状态的非原子路径。目前 GORM 仓储实现了事务路径，mock 仓储没有。

### 1.4 HTTP 层 —— `internal/infrastructure/http/`

| 文件 | 职责 |
|---|---|
| `server.go` | 建 engine、挂 4 个中间件、注册公共组与 admin 组；`GET /healthz` 返回 `{"status":"ok","debug":<bool>}` |
| `handler.go` | 全部 handler；DTO 定义与绑定都在这里 |
| `middleware/admin_auth.go` | Bearer Token 鉴权 |
| `middleware/cors.go` | 白名单 Origin 精确匹配 |
| `middleware/error_handler.go` | 错误 → `{error, code}` 统一响应 |
| `middleware/recovery.go` | panic 兜底 |
| `middleware/request_logger.go` | 请求日志（只记 method/path/status/latency/ip，**不记 header 和 body**） |

**路由表**（`server.go` 的 `NewServer` + `handler.go` 的 `RegisterRoutes`）：

| 方法 | 路径 | 鉴权 |
|---|---|---|
| GET | `/healthz` | 无 |
| POST | `/api/v1/votes` | 无 |
| GET | `/api/v1/proposals` | 无 |
| GET | `/api/v1/proposals/open` | 无 |
| POST | `/api/v1/proposals/:id/vote-status` | 无（需凭证材料） |
| POST | `/api/v1/proposals/:id/ballot-draft` | 无（**仅 debug 模式**，否则 404） |
| POST | `/api/v1/proposals/:id/credentials/claim` | 无（匿名领取凭证，**只预留**） |
| POST | `/api/v1/proposals/:id/credentials/blind-sign` | 无（**只签池里还空着的槽位**） |
| POST | `/api/v1/proposals/:id/credentials/attest` | 无（先验签再落库） |
| GET | `/api/v1/proposals/:id/results` | 无 |
| GET | `/api/v1/proposals/:id/audit-anchors` | 无 |
| POST | `/api/v1/credentials` | **admin**（为指定 `user_id` 预留） |
| POST | `/api/v1/credentials/blind-sign` | **admin**（可为**任意** `user_id` 盲签） |
| POST | `/api/v1/proposals` | **admin** |
| GET | `/api/v1/proposals/:id/credentials` | **admin** |
| POST | `/api/v1/proposals/:id/credentials/pool` | **admin**（备凭证池） |
| POST | `/api/v1/proposals/:id/decryption/setup` | **admin** |
| POST | `/api/v1/proposals/:id/results/publish` | **admin** |

**模式**：handler 里 payload 结构体用 `validate:"..."` tag 校验，绑定失败统一 `c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))`，由 error_handler 统一出参。

**本轮的接口变更**：
- `submitVotePayload` **没有 `public_witness` 字段** —— 旧的「客户端自由提交公开输入」入口彻底消失，公开输入由服务端拼装。
- `PrepareBallotDraft`：`h.debugMode` 为 false 时直接 `c.Status(http.StatusNotFound)` —— 用 **404 而不是 403**，避免把端点存在性暴露出去。响应体只有 `proposal_id` `option` `encrypted_vote` `proof`（开发占位常量 `developmentProofBase64`）`nullifier`。
- `NewHandlerWithTallyAndMode(..., debugMode bool, ...)` 是 handler 的装配入口；`debugMode` 由 `config.Config.DebugMode` 传进来。

**陷阱**：
- `middleware/admin_auth.go` 用 `!=` 做字符串比较，**不是常数时间比较**。
- `middleware/error_handler.go` 的 `publicErrorMessage` —— `INVALID_REQUEST` 直接回 `err.Error()`，会把内部错误细节透出；其他 code 都做了归一化。改文案时注意前后端 `error-messages.ts` 的映射是按英文短语正则匹配的。
- 错误码枚举是**前后端契约**：`INVALID_REQUEST` `NOT_FOUND` `UNAUTHORIZED` `DOUBLE_VOTING` `CREDENTIAL_EXISTS` `CREDENTIAL_POOL_EMPTY` `USER_ALREADY_VOTED` `INVALID_CREDENTIAL` `DECRYPTION_SESSION_EXISTS` `RESULT_ALREADY_PUBLISHED` `INTERNAL_ERROR`。

### 1.5 持久化层 —— `internal/infrastructure/persistence/`

| 文件 | 职责 |
|---|---|
| `db.go` | SQLite / PostgreSQL 连接（`NewDB` `newPostgresDB` `newSQLiteDB` `applyPoolConfig`） |
| `db_env.go` | 从环境变量读 DB 配置（`DB_DRIVER` 等，`GetDBConfigFromEnv`） |
| `base.go` | 泛型仓储基类（`BaseRepository[T]`） |
| `migration.go` | `AutoMigrate` + `SchemaReady` + 历史 schema 兼容迁移（`migrateLegacyCredentialSchema`） |
| `*_repository.go` | 8 个仓储实现（vote / proposal / credential / credential_claim / participation / decryption_session / tally_result / audit_anchor） |

**模式**：仓储接口方法签名都带 `ctx`；`SaveWithParticipation` 用 `clause.OnConflict{DoNothing}` + `RowsAffected == 0` 检测重复参与，冲突时返回 `ErrDuplicateParticipation`。

**陷阱**：
- `db.go` 的 `applyPoolConfig` —— PostgreSQL 的**连接池参数**由 `db.max_open_conns` / `db.max_idle_conns` / `db.conn_max_lifetime_seconds` 配置（默认 25 / 5 / 3600 秒），为 0 表示不改动 `database/sql` 默认值。
- `db.go` 的 `newSQLiteDB` —— SQLite 文件库**没开 WAL**，也没设 busy timeout。
- `migration.go` 的 `AutoMigrate` —— 每次迁移都会回填 `options_json`（`WHERE options_json IS NULL OR = ''` → `'[]'`）。**release 模式启动不再自动迁移**，改表要走独立的 `nullifier-server -migrate` 步骤（`config.AutoMigrate` / `di.Migrate`）。
- `db_env.go` 的 `GetDBConfigFromEnv` —— `GO_ENV=test` 时 SQLite 强制切 `:memory:`。

### 1.6 密码学层 —— `internal/infrastructure/crypto/`

**`crypto/ballot/` —— 选票密码学栈（本轮新增，是选票能力的事实核心）**

| 文件 | 内容 |
|---|---|
| `ballot/group.go` | 曲线点与标量工具：`Point`（仿射坐标）、`parsePoint`（**在曲线上 + 在素数阶子群里**）、`scalarMul`、随机标量采样 |
| `ballot/keys.go` | 计票密钥对（`GenerateKeyPair`）、**Fr 上的 Shamir 拆份额 / 恢复**（`SplitSecret` `RecoverSecret`）；份额编码 = **4 字节下标（大端 uint32）+ 32 字节取值** |
| `ballot/cipher.go` | **v3 选票密文**：`EncryptBallot` / `DecryptBallotOption` / `ValidateBallotCiphertext` / `parseCiphertext` |
| `ballot/nullifier.go` | `DeriveProposalField`（SHA-256 → Fr）、`DeriveNullifier`（**MiMC**）、`mimcHash`（与 gnark `std/hash/mimc` 逐位一致的镜像实现） |
| `ballot/encoding.go` | `EncodeScalarHex` / `ParseScalarHex`（**拒绝式**规范编码检查）/ `GenerateNullifierSecret` |
| `ballot/circuit.go` | 真实 Groth16 选票电路 `ballotCircuit` + `assertLessThan`（8 位分解的范围检查） |
| `ballot/statement.go` | `PublicInputs`（电路公开输入的域元素形式）与 `BuildPublicInputs(statement entity.BallotStatement)` |
| `ballot/prover.go` | `Prover`（`NewProver` / `Prove`）、`Verifier`（`NewVerifier` / `Verify`）、`GenerateKeys`、`compileCircuit` |

**密文格式 v3**：1 字节版本 `0x03` + 两个压缩点（各 32 字节）= **65 字节**。
密文是指数 ElGamal `(a, b) = (r·G, m·G + r·Y)`，其中 `m` 是**选项下标**、`r` 是一次性随机数、`Y` 是提案的计票公钥。
解密是 `b - X·a = m·G`，再在提案的选项集合里逐个比对出 `m`。

**电路形状**（`ballot/circuit.go`）：公开输入 **9 个**（提案 ID 域元素、nullifier、计票公钥 XY、密文 a 的 XY、密文 b 的 XY、选项数），私有见证 **3 个**（选项下标 `m`、随机数 `r`、nullifier 秘密）。约束三条：`a = r·G`、`b = m·G + r·Y`、`m < NumOptions`（8 位分解 + 最高位检查）、`nullifier = MiMC(secret, proposal_id)`。实测 **7917 条约束**，证明与验证都是毫秒量级（本机实测证明约 20ms、验证约 1–2ms）。

**对外适配层**

| 文件 | 内容 |
|---|---|
| `crypto/gnark_proof_verifier.go` | `GnarkProofVerifier`：实现 `domaincrypto.ProofVerifier`；`vk_path` 为空时走 `skipVerify` 分支 |
| `crypto/ballot_threshold_crypto.go` | `BallotThresholdCrypto`：同时实现 `domaincrypto.Encrypter` 与 `usecase.ThresholdCrypto` |
| `crypto/ballot_nullifier.go` | `BallotNullifierGenerator`：实现 `domaincrypto.NullifierGenerator` |
| `crypto/kryptology_credential_issuer.go` | RFC9474 RSABSSA 签发（**`BlindSign` 对盲化消息签名**）、**`IssuerPublicKey` 出口公钥**、**`VerifyCredential` 验签**、密钥版本轮换、落盘 |
| `crypto/credential_client.go` | **盲签名的客户端侧**（`CredentialBlindClient.Blind` / `Finalize`）：CLI 与端到端测试共用，只需签发方公钥 |
| `crypto/merkle_root.go` | Merkle root（`GnarkMerkleRootBuilder`） |
| `cmd/keygen/main.go` | 选票电路密钥生成（见 §1.1） |

**模式**：密钥材料统一 `append([]byte(nil), ...)` 拷贝后返回，不外泄内部 slice；issuer 落盘权限 `0o600`。

**陷阱**：
- `gnark_proof_verifier.go` 的 `NewGnarkProofVerifier` —— **`vk_path` 为空时 `Verify` 直接 `return nil`**，任何非空 proof 都通过。release 模式强制要求 `crypto.vk_path`，这条分支只在开发模式生效。
- `ballot/group.go` 的 `parsePoint` —— **子群检查不能省**：BN254 twisted Edwards 的余因子是 8，小阶点会让解密方泄漏私钥低位比特。所有外部点（密文里的两个点、计票公钥）都要过 `isInSubGroup`。
- `ballot/circuit.go` —— **公开输入里绝不能有选项下标**（见 §3 陷阱 1）。
- `ballot/nullifier.go` 的 `mimcHash` —— 必须与 gnark `std/hash/mimc` 逐位一致（见 §3 陷阱 4）。
- `ballot/encoding.go` 的 `ParseScalarHex` —— 与 `fr.Element.SetBytes` 的区别就在这里：后者按模归约，前者拒绝非规范编码（见 §3 陷阱 2）。
- **密文格式 v2 → v3 与 v0.1.0 不兼容**：v2 是 ed25519 上的 DH 混合封装（HKDF-SHA256 派生），v3 是 BN254 twisted Edwards 上的指数 ElGamal。旧密文解不开，也不该再用于真实投票。`kyber_elgamal.go` 已删除。
- `kryptology_credential_issuer.go` —— **没有 `Issue` 了**：一次性签发已删除。签发方现在只做两件事：给出公钥（`IssuerPublicKey`）与对**盲化消息**签名（`BlindSign`）。轮换挂在 `IssuerPublicKey` 上（客户端每次预留槽位都要拿公钥，轮换时机跟着它）。
- `kryptology_credential_issuer.go` 的 `VerifyCredential` —— 校验凭证签名，**按 key version 从新到旧逐个试**；投票路径落库前调用它（这是能力 E 的落点）。
- `crypto/credential_client.go` 的 `NewCredentialBlindClient` —— **要求变体必须是 `SHA384PSSDeterministic`**，否则报错。变体不同则盲化结果不同，签名验不过。
- `crypto/credential_client.go` 的 `BlindedCredential.valid` —— CIRCL 的 `blindrsa.State` 是**值类型**不是指针，零值无法用 `== nil` 判断，所以另加了一个 `valid` 布尔。

### 1.7 前端门户 —— `web/`

| 位置 | 职责 |
|---|---|
| `src/App.vue` | 只做外壳、路由与新手引导编排 |
| `src/components/panels/` | 7 个页面：`OverviewPanel` `SetupPanel` `ReferencePanel` `HallPanel` `IdentityPanel` `VotePanel` `OpsPanel` |
| `src/components/` | `AppSidebar` `AppTopbar` `GuideOverlay` `JourneyBar` `NoticeStack` |
| `src/components/ui/` | `EmptyState` `NoticeBanner` `StatTile` |
| `src/composables/useVotePortal.ts` | **全部状态与 API 编排**（单文件约 1800 行，是前端的事实核心） |
| `src/lib/` | `api.ts` `ballot-cipher.ts` `ballot.ts` `blind-rsa.ts` `credential.ts` `datetime.ts` `error-messages.ts` `field.ts` `focus-trap.ts` `keccak.ts` `mimc.ts` `navigation.ts` `nullifier.ts` `options.ts` `portal-view.ts` `sha256.ts` `sha384.ts` —— 除 `api.ts` 外都有单测 |
| `src/styles/` | `tokens.css` `base.css` `components.css` `shell.css` |
| `src/types/` | `portal.ts` `portal-api.ts` |
| `vitest.config.ts` | 测试配置 + **自写的 `.vue` 编译插件**（见下） |
| `test/setup.ts` | 测试环境的浏览器全局对象（`localStorage` / `sessionStorage` / `window`） |

**面板渲染测试**（`src/components/panels/panels.test.ts`）：

- 7 个面板此前**既没有类型检查也没有任何测试** —— `tsc` 不认 `.vue` 文件（只有
  `src/env.d.ts` 里的 `declare module '*.vue'` 让导入不报错），所以模板里的笔误
  只会在运行时炸。
- 现在用 Vue 的**服务端渲染**（`vue/server-renderer`）把面板渲染成 HTML 字符串：
  模板、setup、computed 都会真的跑一遍。助手在 `src/lib/render-panel.ts`。
- **这不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。抓的是
  「模板引用了不存在的变量」「setup 抛错」「条件分支写反」这一类问题。
- 覆盖了路线图 T2 的门控：非 debug 模式下「按选项生成凭据」按钮**不渲染**
  （断言的是按钮本身，不是文案里出现的那个词）。

**`.vue` 编译插件**（`vitest.config.ts` 内联）：

- 离线环境装不了 `@vitejs/plugin-vue`（模块缓存里没有），但 `vue` 自带
  `vue/compiler-sfc`（再导出 `@vue/compiler-sfc`），`typescript` 又是已有的
  devDependency —— 三样凑齐就不必新装包。
- 走 `compileScript({ inlineTemplate: true })`：模板被编译进 `setup` 的返回值，
  产物是单个模块。仓库里 17 个 `.vue` **全是 `<script setup>`**，没有别的形态。
- 两个踩过的坑：① `ts.transpileModule` 按**扩展名**判断脚本类型，`.vue` 是它不
  认识的扩展名 → 会按 JS 解析，`import type { X } from '...'` 直接语法错误。解法是
  给它一个 `.ts` 后缀。② `rewriteDefault` 的默认 babel 解析器**不认 TypeScript**，
  而且它只对「函数声明」形式的默认导出生效 —— 对 `_defineComponent({...})` 是空
  操作，所以这里根本不需要它（`compileScript` 的产物自带合法 `export default`）。

**模式**：设计令牌在 `styles/tokens.css`，组件只引用变量不写裸色值；视图推导（工作区归属/标题/引导步进）集中在 `lib/portal-view.ts`。

**本轮的 debug 门控**：
- `useVotePortal.ts` 的 `readDebugFlag` 从 `GET /healthz` 的响应里读 `debug` 字段 → `debugMode` ref。
- `VotePanel.vue` 的「按选项生成凭据」按钮整块包在 `v-if="portal.debugMode"` 里；非 debug 模式改为提示文案（说明这条入口会让服务端看到选项）。
- 门户**不再发送** `selected_option`，也**不再发送** `public_witness`（服务端本来就不消费前者）。
- `web/src/lib/nullifier.ts` —— 两类 nullifier：**`generateNullifier`（随机域元素，门户现在还在用）**与 **`deriveNullifier`（MiMC 绑定，与电路一致）**。前者生成 32 字节随机数后**按模 r 归约**，输出 64 位小写十六进制，保证是规范域元素编码；⚠️ 它**没有**做 MiMC 绑定，因此这样的 nullifier 按电路约束出不了证明。后者按 `MiMC(秘密, 提案域元素)` 派生，与 Go 侧 `ballot/nullifier.go` 逐位一致（见 §3 陷阱 23）。
- `web/src/lib/ballot-cipher.ts` —— **浏览器端选票加密**：BN254 twisted Edwards 上的指数 ElGamal（加密选项下标），点编码与 Go 侧逐字节一致（见 §3 陷阱 21）。计票公钥以**未压缩**形式传入，因此浏览器端不需要在域内求平方根。
- `web/src/lib/credential.ts` —— **浏览器端凭证盲签名的客户端侧**：`credentialMessage`（`sha256(user_id ‖ 0x00 ‖ user_pub_key)`）、`parseIssuerKey`、`blindCredentialMessage`、`finalizeCredentialSignature`。与 Go 侧 `crypto.CredentialMessage` 同口径，由跨语言向量钉死。
- `web/src/lib/{field,sha256,keccak,mimc}.ts` —— **浏览器端 nullifier 派生的底座**：域运算与标量规范编码、SHA-256、legacy Keccak-256（MiMC 轮常量由它派生）、Fr 上的 MiMC。全部是纯 TS 实现，不依赖任何外部密码学包（离线环境下装不了），也不依赖 WebCrypto（`crypto.subtle` 要求安全上下文，内网 http 部署会失效）。

**陷阱**：
- `lib/api.ts` 是唯一的 HTTP 出口，返回 `{ok, status, data, error, code, rawText}`；错误文案统一走 `lib/error-messages.ts` 的 `describeApiError(message, code, fallback)`，优先级 **业务短语 > 错误码 > 调用方兜底**。
- **凭据与选项的绑定已经补上**：`lib/ballot.ts` 的 `hasBallotTargetDrifted` / `isBallotTargetCurrent` + `useVotePortal.ts` 里选项/提案变化的 watcher（清空 `encryptedVote` / `proof`）+ 提交前的 `ballotTargetCurrent` 校验。改了选项必须重新生成凭据。
- **admin token 存 `sessionStorage`**（不落 `localStorage`），但仍是明文；同源脚本能读到。
- **`ballot-draft` 在非 debug 模式返回 404**：门户此时拿不到任何可提交的密文/证明 —— **浏览器端 prover 还没做**（见 §5）。
- **门户的匿名性文案仍与服务端能力有落差**：非 debug 模式下的提示文案说「正式客户端会在浏览器本地生成密文与证明」，但客户端 prover 尚未落地。
- `.vue` 面板**有渲染测试，但不是浏览器渲染**（见 §1.7 的「面板渲染测试」）。没有 CSS、没有事件绑定、没有客户端挂载 —— 改界面的视觉与交互仍要靠人工核对。
- `pnpm run format`（prettier）会重写 `.pnpm-store`，必须先确认 `.prettierignore` 覆盖（当前已覆盖 `.pnpm-store/`）。

### 1.8 CLI —— `cmd/cli/main.go`

16 个命令，全部走 HTTP API：
`health` `version` `proposals-open` `proposal-create` `credential-issue` `credential-claim` `credential-show` `ballot-draft` `ballot-encrypt` `vote-submit` `vote-status` `credentials-list` `decrypt-setup` `result-publish` `result-get` `anchors-list`

**模式**：`-admin-token` flag 或 `NULLIFIER_ADMIN_TOKEN` 环境变量；`-base-url` 或 `NULLIFIER_BASE_URL`。

**凭证命令走真两段式盲签名**：`credential-claim`（匿名，公开端点）与 `credential-issue`
（管理端）都是「预留 → 本地盲化 → 盲签名 → 本地解盲 → attest」。Go 侧的盲签名
客户端是 `internal/infrastructure/crypto/credential_client.go` 的 `CredentialBlindClient`
（CLI 与端到端测试共用同一个实现）。

**陷阱**：
- CLI 输出是纯文本命令结果，**不做中文本地化**（与前端相反，是有意为之）。
- **CLI 的 `postJSON` 返回 `*httpError`**（带 HTTP 状态码）：领取凭证时「槽位被别人
  抢走」是 409 `CREDENTIAL_EXISTS`，客户端据此换槽位重试。409 还有另一种语义
  `CREDENTIAL_POOL_EMPTY`（池领完了），那个**不重试**。
- **URL 路径二次转义**：`main.go` 的 `cmdResultGet` 把已 `url.PathEscape` 过的 path 塞进 `base.Path`，`url.URL.String()` 会再转义一次（`%` → `%25`）。含空格 / `/` / `%` 的 proposal id 必然 404。实测：`result-get -proposal "a b/c"` → `/api/v1/proposals/a%2520b%252Fc/results`。
- **CLI 不再发送 `selected_option`**（旧版本会多发这个字段，明文选项上过网）。`cmdVoteSubmit` 的 body 只有 `proposal_id` `encrypted_vote` `proof` `nullifier` `user_id` `user_pub_key` `credential_signature`。
- **CLI 没有 `-public-witness` 入口**，服务端也不再收这个字段。CLI 只「带 proof 提交」，不本地生成 proof（路线图 G9 决策）。
- **`decrypt-setup` 不带 `-out` 时会把门限份额打印到 stdout**。
- **`ballot-draft` 只在 debug 模式可用**：release 模式下服务端返回 404，CLI 拿不到草稿。

### 1.9 压测工具 —— `cmd/loadtest/main.go`

纯 HTTP API 的并发投票压测工具：不碰数据库，也不绕过服务端的任何校验。

**流程**：建「窗口稍后开」的提案 → 初始化门限解密会话 → 投凭证池 → 匿名领凭证 →
等窗口开 → 并发投票（每请求一个不同凭证与不同 nullifier）→ 同 nullifier 并发复核防重投 →
用 `GET /proposals/:id/credentials` 核对已投票凭证数。

**输出**：吞吐（req/s）、延迟 p50/p90/p99/max、状态分布、防重投结论、落库核对。`-json` 输出机器可读报告。

**退出码**：`0` = 防重投守住（恰好放行 1 张）且落库数对得上；`1` = 防重投失守或落库数不符；
`2` = 用法错误。**结论能当门禁用** —— 这是它用退出码而不是只打印报告的原因。

**陷阱**：
- **只能在 debug 模式跑**：客户端没有 prover，release 模式下所有投票请求都会因验签失败被拒
  （工具发的是 `ZGV2LXByb29m`，即 `"dev-proof"` 的 base64）。工具不会替你伪造真证明。
- **解密会话必须在投票窗口打开之前初始化**（服务端的硬性约束），所以工具先建「窗口稍后开」
  的提案再初始化会话，最后才等到窗口开始。
- **服务端没有 `GET /api/v1/proposals/:id` 端点** —— 工具靠 `GET /api/v1/proposals` 列表定位提案。
- **SQLite 并发写会撞 `database is locked`**：服务端要用带 `_busy_timeout` 的 DSN
  （如 `anonvote.db?_busy_timeout=10000&_journal_mode=WAL`），否则压测结果里全是 500 ——
  那是存储层的问题，不是投票逻辑的问题。
- **吞吐按墙钟算，不按延迟求和** —— 并发请求的延迟总和不是总耗时。

---

## 2. 关键流程

### 2.1 投票链路（`internal/usecase/voting.go` 的 `Submit`）

```
HTTP 绑定 + validate（请求体已无 public_witness）
  → nullifier 早期校验：hex 可解码 + 恰好 32 字节，并归一成规范形式（decodeNullifier）
  → proposal 必须处于 open 且在时间窗口内（isProposalOpenForVoting）
  → 必须已存在 decryption session（否则「decryption session is not ready」）
  → req.PublicInputs(proposal, session)  ← 服务端自己拼 entity.BallotStatement
  → verifier.Verify(proof, statement)   ← vk_path 为空时此步只查 proof 非空
  → encrypter.ValidateCiphertext(encrypted_vote)  ← v3 密文结构 + 曲线点子群检查
  → 查 (proposal_id, user_id) 的凭证，比对 user_pub_key 与 signature
  → issuer.VerifyCredential(user_id, user_pub_key, signature)  ← 验签失败 → 401
  → hash(user_id) = sha256
  → 查 nullifier 是否已存在（规范形式，全局唯一）
  → 查该 user_hash 是否已参与
  → 事务内：先插 vote_participations（OnConflict DoNothing），再插 votes
```

**公开输入由服务端拼装**是本轮的关键变化：客户端不提交公开输入，一份 proof 因此只能用于它被生成的那份密文，跨提案 / 换密文 / 改 nullifier 都会验证失败。

**防重投依赖两件事**：usecase 里的先查后写 + 数据库唯一索引兜底。真正的并发安全靠后者 —— 但唯一索引兜底的前提是 nullifier 的**字符串表示唯一**，所以落库前必须归一成规范形式（见 §3 陷阱 2）。

**并发下的兜底已被验证**：`tests/e2e/concurrent_nullifier_test.go` 用 8 个并发请求、**同一个 nullifier**、各自不同的凭证提交，断言恰好 1 张票入库、其余 7 个是 409 `DOUBLE_VOTING`。
- ⚠️ 那个用例的 SQLite 必须用**文件库 + WAL**（`?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate`）：共享缓存的内存库用表级锁，并发写会直接抛 "table is locked"，而 busy_timeout 对它无效 —— 那不是防重投的锅，是 SQLite 的锁语义。

### 2.2 计票链路（`internal/usecase/tally.go`）

```
decrypt-setup  → 生成 BN254 计票密钥对（私钥 = 标量，公钥 = 压缩点）
                → Shamir 拆成 threshold/trustees 份（Fr 上插值）
                → 份额编码 = 4 字节下标 + 32 字节取值
                → 库里只存份额的 sha256 哈希（SharesJSON）
                → 响应体里把明文份额一次性返回给 admin
publish        → 提案必须已关闭（isProposalClosed）
                → 重复发布返回 409
                → 份额必须属于本 session，且不能重复计数
                → RecoverSecret → 逐票解密（b - X·a = m·G，再在选项集合里比对出下标）
                → 统计 → 校验下标在提案白名单内
                → Merkle root → anchor_hash = sha256(proposal_id|root|tally)
                → 写 tally_results + audit_anchors
```

**陷阱**：`decrypt-setup` 在**投票开放中会被拒绝**（`tally.go` 的 `SetupThresholdSession`）。运营上必须在 `starts_at` 之前完成初始化。同理 `publish` 只能在提案关闭后执行。

### 2.3 门限解密会话的生命周期

- 一个提案一个 session，重复 setup → `409 DECRYPTION_SESSION_EXISTS`（**不会覆盖**）。
- 覆盖会导致已投选票无法解密，所以这是硬约束。
- 份额明文只在 setup 响应里出现一次，库里只有哈希。

---

## 3. 已知陷阱（改动时优先检查）

> 本清单随代码更新。**已修掉的问题不再列在这里**，但历史结论保留在
> `docs/release-review-2026-10-20.md`。

1. **公开输入里绝不能有选项下标。** `ballot/circuit.go` 的 `ballotCircuit` 把 `OptionIndex` 放在**私有见证**里，用 `assertLessThan`（8 位分解 + 最高位检查）做范围检查。公开输入一旦带上选项下标，服务端就等于拿到明文选项 —— 这与「选票内容对服务端不可知」直接冲突。
2. **`fr.Element.SetBytes` 是按模归约，不是拒绝。** 超范围输入会被折回域内，于是同一个域元素有多种字节表示。`ballot/encoding.go` 的 `ParseScalarHex` / `isCanonicalScalar` 才是拒绝式检查，`ballot/statement.go` 的 `BuildPublicInputs` 会再查一次规范编码。**漏查的后果**：票表 `votes.nullifier` 上的唯一索引存的是**字符串**，同一个 nullifier 的两种写法会绕开唯一索引 → 重投拦不住。**本轮已修**：`usecase/voting.go` 的 `decodeNullifier` 除了 hex 与长度，还返回**规范形式**（64 位小写 hex），`Submit` 用规范形式查重与落库 —— 客户端提交大写或带空白的变体也会归一到同一串，唯一索引因此拦得住。仍需注意：规范**字节**检查（值 < 模数）只在 `vk_path` 加载时由验证器执行（见陷阱 3）。
3. **`vk_path` 为空 = 关闭证明验证。** `NewGnarkProofVerifier("")` → `skipVerify=true`，只要求 proof 非空。本地「验证通过」不代表真的验过。开发配置 `configs/config.yaml` 就是 `vk_path: ""`；release 模式强制要求 `crypto.vk_path`（`config.requiresVerifyingKey`）。
4. **Go 侧的 MiMC 必须与 gnark `std/hash/mimc` 逐位一致。** `ballot/nullifier.go` 的 `mimcHash` 是电路 `hash.Write(c.NullifierSecret, c.ProposalID)` 的镜像（轮函数 `x = (x + h + c)^5`，外层 Miyaguchi–Preneel）。常量或轮次差一点，电路里的 nullifier 约束就永远不成立。
5. **密文格式 v2 → v3 与 v0.1.0 不兼容。** v3 是 BN254 twisted Edwards 上的指数 ElGamal（`0x03` + 两个压缩点 = 65 字节），v2 是 ed25519 上的 DH 混合封装（HKDF-SHA256 派生）。旧密文既解不开，也不该再用于真实投票。
6. **加密的是选项下标，不是选项文本。** 下标是电路里可做范围检查的小整数（上限 `ballot.MaxOptions` = 256），选项文本可能很长且不唯一。解密方 `DecryptBallotOption` 拿 `numOptions` 在选项集合里逐个比对。
7. **曲线点的子群检查不能省。** BN254 twisted Edwards 的余因子是 8，小阶点会让解密方泄漏私钥低位比特。`ballot/group.go` 的 `parsePoint` 对每个外部点都做 `isInSubGroup`（用子群阶乘点看是否回到单位元）。
8. **`internal/di/wire_gen.go` 是手工同步的。** wire 生成器在本仓库离线不可用（`go.sum` 里 `github.com/google/subcommands` 只有 `/go.mod` 哈希，缺模块内容哈希）。改 `internal/di/wire.go` 必须手工同步 `wire_gen.go`。
9. **`ballot-draft` 是明确的妥协，且只在 debug 模式可达。** 它由**服务端拿明文选项去加密**（`usecase/tally.go` 的 `EncryptVoteOption` → `proposalOptionIndex`），所以这条链路上服务端知道每一票的明文选项。非 debug 模式按 404 处理（`handler.go` 的 `PrepareBallotDraft`），用 404 而不是 403，避免暴露端点存在性。它**不是生产能力**，客户端 prover 落地后要被删掉。
10. **只比对签名是不够的。** 拿一张合法凭证换个 `user_id` 重放，逐字节比对签名会通过。`usecase/voting.go` 因此在落库前调 `issuer.VerifyCredential`，失败 → 401。
11. **两段式盲签名的服务端只做两件事。** `kryptology_credential_issuer.go` 没有 `Issue` 了：签发方只给公钥（`IssuerPublicKey`）与对**盲化消息**签名（`BlindSign`）。**服务端在 attest 之前绝不能拿到 `user_pub_key`** —— 它一旦知道公钥就能算出凭证消息原文，盲化当场失效（它手上有私钥，根本不需要解盲）。因此 `credentials/claim` 的请求体是空的，旧版本收 `user_pub_key` 的字段一律忽略。
12. **公开的盲签名端点必须绑定池槽位。** `credentials/blind-sign` 只为该提案里**还空着的** `user_id` 签名，否则签发方就是「任意消息」的签名预言机。管理端可以覆盖任意 `user_id`，走的是另一个路径 `/api/v1/credentials/blind-sign`（gin 的路由树不允许同一路径挂两套中间件）。
13. **预留不占位 → 并发领取会抢同一个槽位。** 服务端取槽位时不改状态，所以两个客户端可能拿到同一个 `user_id`，后到 attest 的那个会撞 409 `CREDENTIAL_EXISTS`。**重试必须在客户端**（新协议下重试要重新盲化，服务端代劳不了）：换槽位重来，最多 5 次，只在 409 `CREDENTIAL_EXISTS` 时重试 —— 409 还有 `CREDENTIAL_POOL_EMPTY`（池领完了），那个不重试。
    - 为什么不改成「预留即占位」：那会让「只预留不 attest」的客户端永久消耗池槽位，变成一个 DoS。
14. **凭证改为匿名随机领取后，服务端无法强制一人一票** —— 每次领到的 `user_id` 都是新的随机假名，同一个人领两次会拿到两张凭证、可以投两票。路线图 T22 用资格证明补回（见 [`docs/roadmap.md`](./roadmap.md) §2.3）。
15. **`configs/config.production.yaml` 不会被自动加载** —— viper 默认只找 `config.*`。**已给出一条明确路径**：`CONFIG_FILE=./configs/config.production.yaml` 启动即可（`internal/config/config.go` 的 `ConfigFileEnvVar`）。显式指定的文件不存在时**直接启动失败**，不会静默回退到开发配置。启动日志的 `config_file` 字段会打印实际加载的文件路径。
16. **生产配置 `cors_allowed_origins: []`** → 门户跨域会被拒。这是部署项，不是代码项 —— release 模式下留空会打一条启动告警（同源部署留空是合法的，所以不阻止启动）。
17. **覆盖率门槛是逐包分档的**，门槛值写在 `scripts/coverage-thresholds.txt`（**11 个受控包**：usecase / config / di / http / http-middleware / crypto / crypto-ballot / persistence / cmd-cli / cmd-keygen / cmd-server），合计门槛见表内 `total` 行（77）。单个包回退会直接让 CI 变红。门槛本身还比各包当前水位低约 2 个百分点 —— 抬门槛是逐版本的事，别一次拉满。最低的一档是 `internal/infrastructure/persistence`（门槛 58）。
18. **窗口期语义在两个用例里各写了一份**：`isProposalOpenForVoting`（`voting.go`）和 `isProposalClosed`（`tally.go`）。改时间规则要同时改两处。
19. **release 模式不再启动即改表** —— 必须先跑 `nullifier-server -migrate`，忘了会直接启动失败（`persistence.SchemaReady` 检查）。开发模式仍然自动迁移。
20. **admin token 的存放位置**：CLI 优先读 `NULLIFIER_ADMIN_TOKEN` 环境变量（`cmd/cli/main.go`），`-admin-token` 只是兜底；门户存 `sessionStorage`，不落 `localStorage`。
21. **浏览器端与 Go 侧的选票密文必须逐字节一致。** 密文点编码一旦有一处偏差，服务端就解不开客户端投的票，而且**不会报错**（只是计票结果对不上）。唯一防线是那组跨语言已知答案向量：`internal/infrastructure/crypto/ballot/kat_interop_test.go`（Go 侧断言向量并解出选项下标）与 `web/src/lib/ballot-cipher.test.ts`（TS 侧断言产出同一串密文）。**改任何一边的曲线参数或编码，都要重新生成并同步这两处。**
22. **计票公钥以未压缩形式给客户端**：`GET /api/v1/proposals/:id/election-key` 返回 X 与 Y 各 32 字节。原因是压缩点要在标量域里求平方根才能解压（`r ≡ 1 mod 4`，走 Tonelli-Shanks），浏览器端不必背这个包；代价是接口多一种编码方式，改接口时别改回压缩形式。
23. **浏览器端的 nullifier 派生必须与 Go 侧逐位一致。** 电路约束是 `nullifier = MiMC(秘密, 提案域元素)`：SHA-256 映射提案 ID、Keccak-256 链派生 91 个 MiMC 轮常量、轮函数 `(x+h+c)^5`、外层 Miyaguchi–Preneel。差一位，浏览器就出不了证明（而且**不会报错**，只是验签失败）。防线是同一组跨语言向量：`internal/infrastructure/crypto/ballot/nullifier_interop_test.go`（Go 侧断言向量，并真的用这组值出一份证明验过）与 `web/src/lib/mimc.test.ts` / `web/src/lib/nullifier.test.ts`（TS 侧断言同一组值）。**改任一侧的哈希、轮常量或域运算，都要重新生成并同步这几处。**
24. **浏览器端的盲签名 client 侧必须与 CIRCL 逐字节一致。** `web/src/lib/blind-rsa.ts` 实现 RFC 9474 的 `Prepare` / `Blind` / `Finalize` 与 EMSA-PSS 编码，服务端用 CIRCL `blindsign/blindrsa`（变体 `SHA384PSSDeterministic`）。两侧对同一条消息盲化出的消息必须一模一样，否则解出来的签名验不过 —— **而且不会报错**，只是验签失败。防线是 `internal/infrastructure/crypto/blind_rsa_interop_test.go`（Go 侧用 CIRCL 真库，固定密钥 + 固定盐 + 固定盲化因子）与 `web/src/lib/blind-rsa.test.ts`。
    - 关键事实：**client 侧只需要签发方的 RSA 公钥**（`n`、`e`），不需要私钥 —— 这是两段式盲签名能落地的前提。
    - 改任一侧的 PSS 编码 / MGF1 / 哈希 / 盐长度，都要重新生成并同步这两处。
25. **凭证消息的两端实现必须逐字节一致。** 凭证消息是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)`：Go 侧 `crypto.CredentialMessage`、TS 侧 `web/src/lib/credential.ts` 的 `credentialMessage`。服务端验签时重算的是同一条消息，对不上就是验签失败 —— **而且不会报错**，只是凭证被判无效。防线是 `internal/infrastructure/crypto/blind_rsa_interop_test.go`（Go 侧断言向量）与 `web/src/lib/credential.test.ts`（TS 侧断言同一组值）。**分隔符 `0x00` 不能省**：没有它，`(a, b)` 与 `(a‖0x00, b)` 会撞。

---

## 4. 验证手册

```bash
# 后端
go build ./...
go vet ./...
gofmt -l ./cmd ./internal ./tests      # 应无输出
go test ./...                          # Windows 需 CGO_ENABLED=1
bash ./scripts/check-coverage.sh 80 coverage.ci.out   # 用 Git Bash，不是 WSL
#   覆盖率门槛按包分档，逐包判定；门槛值见 scripts/coverage-thresholds.txt（11 个包）

# 只跑选票密码学栈（真实 Groth16：加解密往返、证明往返、篡改公开输入、越界选项、错 nullifier 秘密）
go test ./internal/infrastructure/crypto/ballot/ -v
go test ./internal/infrastructure/crypto/ -run TestBallot -v

# 真实证明的 HTTP 回归：真实证明 → 201；跨提案复用 / 篡改 nullifier / 篡改密文 → 400；伪造凭证签名 → 401
go test ./internal/infrastructure/http/ -run TestServer_RealBallotProof -v

# 跨语言一致性：Go 侧解出浏览器端生成的密文（改了曲线参数或点编码就会红）
go test ./internal/infrastructure/crypto/ballot/ -run TestKAT -v
go test ./tests/e2e/ -run TestAPI_ClientSideEncryption -v   # 客户端本地加密 → 计票还原

# nullifier 派生的跨语言一致性（改了 SHA-256 / Keccak / MiMC 轮常量就会红）
go test ./internal/infrastructure/crypto/ballot/ -run 'TestKAT_Nullifier|TestKAT_Mimc|TestKAT_BrowserNullifier' -v

# 盲签名 client 侧的跨语言一致性（改了 PSS 编码 / MGF1 / SHA-384 就会红）
go test ./internal/infrastructure/crypto/ -run TestKAT_BlindRsa -v

# 真两段式盲签名：验收 ①（服务端只见盲化消息）、验收 ②（交叉攻击失败）、槽位重试
go test ./tests/e2e/ -run 'TestAPI_TwoPhaseBlindSignature|TestAPI_BlindSignature|TestAPI_ClaimRetries|TestAPI_BlindSignRequires' -v
go test ./internal/infrastructure/crypto/ -run TestCredentialBlindClient -v   # Go 侧盲签名客户端往返
go test ./cmd/cli/ -run TestRun_Credential -v   # CLI 的两段式与重试

# 凭证消息的跨语言一致性（改了消息拼法或分隔符就会红）
go test ./internal/infrastructure/crypto/ -run TestKAT_BlindRsa -v
pnpm --dir web run test -- src/lib/credential.test.ts

# 密钥生成（默认不覆盖已有文件；-force 才覆盖）
go run ./cmd/keygen -dir ./keys
go run ./cmd/keygen -dir ./keys -force
go test ./cmd/keygen/ -v

# 并发投票压测（需要一个本地 debug 服务；SQLite 建议用带 busy_timeout 的 DSN）
go test ./cmd/loadtest/ -v        # 工具自身的测试（httptest 模拟服务端）
go run ./cmd/loadtest -admin-token dev-admin-token -votes 200 -concurrency 20
go run ./cmd/loadtest -admin-token dev-admin-token -votes 200 -concurrency 20 -json
#   防重投没守住时退出码非 0 —— 可以直接当门禁用

# PostgreSQL 集成测试：本地用 compose 起库（需要 Docker），CI 走 testcontainers
make pg-test                       # 起库 → 跑集成测试 → 关库
go test ./tests/integration/ -run TestExternalPostgresSettingsFromEnv -v   # 连接参数解析（不需要数据库）

# 前端
pnpm --dir web run test
pnpm --dir web run test -- src/lib/nullifier.test.ts   # 只跑 nullifier 单测
pnpm --dir web run test -- src/lib/ballot-cipher.test.ts   # 客户端加密（含跨语言向量）
pnpm --dir web run test -- src/lib/mimc.test.ts   # MiMC（含跨语言向量）
pnpm --dir web run test -- src/lib/blind-rsa.test.ts   # 盲签名 client 侧（含跨语言向量）
pnpm --dir web run test -- src/lib/credential.test.ts   # 凭证消息（含跨语言向量）
pnpm --dir web run test -- src/components/panels/panels.test.ts   # 7 个面板的渲染测试
pnpm --dir web run lint
pnpm --dir web exec tsc --noEmit
pnpm --dir web run build

# 一次性门禁
make check      # vet + gofmt + web-lint + web-typecheck
make ci         # go test ./... + 覆盖率门槛
```

Windows 注意：仓库根的 `Makefile` 需要在 **Git for Windows 的 bash** 里用 `mingw32-make` 跑；`bash` 命令在 Windows 上默认可能指向未安装的 WSL。

手工验证 debug 门控：`GET /healthz` 看 `debug` 字段；`server.mode` 是 `debug` / `dev` / `development` 时 `POST /api/v1/proposals/:id/ballot-draft` 可达，其余模式返回 404。

---

## 5. 尚未完成的部分（不要写成生产能力）

- **浏览器端 prover 没做。** 真实 Groth16 电路与真实验签在**服务端侧**已成立（`ballot/prover.go` + `cmd/keygen` + `GnarkProofVerifier`），但**没有任何客户端**在本地生成 proof：门户与 CLI 都拿不到可提交的证明。因此 **release 模式下门户仍无法完成投票**（非 debug 模式 `ballot-draft` 404，本地又没有 prover）。路线图 T14。
- **明文选项的彻底消除还差门户接线（T16 完成一半）。** 客户端本地加密已落地（浏览器端
  `lib/ballot-cipher.ts` + CLI `ballot-encrypt`），服务端只提供公开的计票公钥；但**门户
  还没接上**（缺客户端 prover），debug 链路上的 `ballot-draft` → `EncryptVoteOption`
  仍是服务端代加密 —— 这条路径是明确的妥协，不是生产能力。
- **门户的 nullifier 还是随机域元素。** 浏览器端已能算出与电路一致的 nullifier
  （`lib/nullifier.ts` 的 `deriveNullifier`），但门户仍在用 `generateNullifier`；
  接线 prover 时要一起改，并把 nullifier 秘密留在本地。
- **盲签名的两段式协议已落地（T17 完成）**：客户端本地盲化，服务端在签名阶段只见盲化
  消息，attest 阶段才拿到公钥并验签。**口径**：attest 阶段服务端仍然看得到 `user_id`
  与 `user_pub_key`（验签必须重算凭证消息）—— 这是「签发方不知道被签内容」，不是
  「服务端全程不知道客户端是谁」。
- **一人一票没有强制手段**（T22）：凭证匿名随机领取，同一个人可以领两张票投两次。
- **`ballot-draft` 返回的 proof 是开发占位常量**（`developmentProofBase64`），不是真证明；release 模式下即使端点可达也会被验签拒。
- **门户与 CLI 的匿名性文案与实现仍有落差**（见 §1.7 陷阱）。
- **容量数字只有 SQLite 一份，且不是规划。** `cmd/loadtest` 能跑出可复现的吞吐与防重投结论，
  但 SQLite 的单写者模型会把并发写串行化，数字反映的是存储层；客户端 prover 还没做，
  压测只能在 debug 模式跑。按路线图 §2.2，真正的容量规划（阈值、限流、PG 基准）属 v0.3.0。

> 完整的问题清单、证据与修复顺序见 `docs/release-review-2026-10-20.md`；v0.2.0 的能力矩阵与任务状态见 [`docs/roadmap.md`](./roadmap.md) §4.1。
