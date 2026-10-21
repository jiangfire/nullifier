# 技术说明（当前实现）

本文档描述的是仓库当前已经落地的实现，不再保留目标态伪代码。

## 1. 代码结构

```text
cmd/
├── cli/                    # 命令行客户端，复用 HTTP API
├── keygen/                 # 生成选票电路的 Groth16 密钥（vk / pk）
└── server/                 # 服务启动入口

internal/
├── config/                 # 配置加载与校验
├── di/                     # Wire/provider 组装
├── domain/
│   ├── crypto/             # 密码学接口
│   ├── entity/             # 领域实体
│   └── repository/         # 仓储接口与领域错误
├── infrastructure/
│   ├── crypto/             # gnark proof verifier、kryptology credential issuer、Merkle root builder
│   │   └── ballot/         # 选票密码学栈：曲线运算、v3 密文、Groth16 电路与证明器
│   ├── http/               # Gin handler / server / middleware
│   └── persistence/        # GORM 仓储、迁移、DB 配置
└── usecase/                # 业务编排

tests/
├── e2e/
├── integration/
└── testutil/

web/                        # Vue 3 前端门户
```

## 2. 架构分层

- `domain`
  - 只放实体、仓储接口、密码学接口和领域错误。
  - 不依赖 HTTP、GORM 或具体密码学库。
- `usecase`
  - 编排业务流程，依赖 `domain` 接口。
  - 当前核心用例：
    - `ProposalUseCase`
    - `RegisterUseCase`
    - `VotingUseCase`
    - `ParticipationUseCase`
    - `TallyUseCase`
- `infrastructure`
  - `persistence`：GORM 仓储、SQLite/PostgreSQL 连接与迁移。
  - `http`：Gin 路由、请求校验、中间件、错误映射。
  - `crypto`：gnark proof verifier、kryptology credential issuer、Merkle root builder。
  - `crypto/ballot`：选票密码学栈 —— BN254 twisted Edwards 群运算、v3 选票密文、
    Fr 上的 Shamir 门限份额、Groth16 选票电路与证明/验证器。选票密文与电路
    跑在同一条曲线上，密文关系才能进电路。
- `di`
  - 通过 provider 和 Wire 组装依赖。

## 3. 当前业务模型

### 3.1 Proposal

- 关键字段：
  - `id`
  - `title`
  - `description`
  - `options_json`
  - `status`
  - `starts_at`
  - `ends_at`
- 说明：
  - `options` 是正式业务字段，不再是描述里的附带信息。
  - `ProposalUseCase.Create` 会去重、去空并校验至少两个选项。

### 3.2 Credential

- 凭证按 `proposal_id + user_id` 唯一。
- 凭证包含：
  - `user_id`
  - `user_pub_key`
  - `signature`
  - `issuer_key_version`
  - `issued_at`
- 当前签发流程由管理员接口触发，并落库到提案内台账。

### 3.3 Vote 与 Participation

- `votes` 表只存匿名选票：
  - `proposal_id`
  - `nullifier`（规范编码的 64 位十六进制，全局唯一索引）
  - `cipher`（v3 格式选票密文）
- `vote_participations` 单独记录是否投过票：
  - 使用 `hash(user_id)` 记录参与状态
  - 不直接关联投票内容
- `VotingUseCase` 会同时检查：
  - `nullifier` 是否重复
  - 该用户在该提案下是否已投票
  - 提交的凭证材料是否与提案内凭证匹配
  - 凭证签名是否能用签发方公钥验签通过（落库前）

### 3.4 解密会话、结果与审计锚点

- `decryption_sessions`
  - 保存门限参数、公钥和份额哈希
  - 不落库明文 share
- `tally_results`
  - 保存 `total_votes`、规范化 `tally_json`、`merkle_root`、`anchor_hash`
- `audit_anchors`
  - 单独记录可导出的审计锚点

## 4. 数据库与迁移

### 4.1 环境策略

| 环境 | 数据库 | 说明 |
|------|--------|------|
| 开发 | SQLite 文件库 | 默认 `anonvote.db` |
| 测试 | SQLite 内存库 / PostgreSQL(testcontainers) | 单测快、集成验证真实 PG |
| 生产 | PostgreSQL | 通过环境变量或配置覆盖 |

### 4.2 配置来源

- `internal/config.Load` 同时读取：
  - 配置文件（默认 `configs/config.yaml`；`CONFIG_FILE` 环境变量可显式指定路径）
  - 环境变量
- **`CONFIG_FILE` 显式路径**：`configs/config.production.yaml` 永远不会自动加载
  （viper 只找名为 `config.yaml` 的文件），所以生产环境用
  `CONFIG_FILE=./configs/config.production.yaml` 启动。显式指定的文件**不存在时
  直接启动失败** —— 静默回退到开发配置正是最危险的上线事故。
- 环境变量优先级高于配置文件；`CONFIG_FILE` 只决定**读哪个文件**，不改变优先级。
- 数据库优先来自 `internal/infrastructure/persistence/db_env.go`
- 支持的关键配置：
  - `server.address`
  - `server.mode`
  - `server.cors_allowed_origins`
  - `server.admin_bearer_token`
  - `crypto.vk_path`
  - `crypto.issuer_key_path`
  - `crypto.issuer_rotate_days`
  - `db.driver`
  - `db.path`
  - `db.host`
  - `db.port`
  - `db.name`
  - `db.user`
  - `db.password`
  - `db.sslmode`

### 4.3 运行期校验

- 当 `server.mode` 为 `release/prod/production` 时：
  - `crypto.vk_path` 必填
- `server.admin_bearer_token` 若为空，会在启动期自动生成并写回内存配置
- 自动生成的 token 会通过启动日志打印到控制台，仅对当前进程实例有效
- 启动日志会打印**实际加载的配置文件路径**（`config_file`）—— 「以为加载了生产
  配置、实际跑的是开发配置」是最常见的上线事故，必须能一眼看出来。
- release 模式下 `cors_allowed_origins` 为空时**告警**（不阻止启动）：门户与
  后端同源部署时留空是合法的，但不同源时浏览器会拦掉所有跨域请求。

### 4.4 迁移

- **迁移是独立步骤**：`nullifier-server -migrate` 跑完迁移即退出。
  迁移只需要数据库连接，不要求 `crypto.vk_path` 与 `server.admin_bearer_token`。
- 服务启动时是否自动迁移由 `server.auto_migrate` 决定：
  - 非 `release/prod/production` 模式默认 `true`（开发期省事）
  - `release/prod/production` 模式默认 `false` —— 多副本同时启动会争 DDL 锁，
    改表应当是一个能单独执行、单独回滚的步骤
  - 可用环境变量 `SERVER_AUTO_MIGRATE` 或配置文件 `server.auto_migrate` 覆盖
- 关闭自动迁移时，启动前会检查表结构是否就绪（`persistence.SchemaReady`）；
  **未就绪则服务直接启动失败**，不会等到第一个请求打进来才报「no such table」。
- `persistence.AutoMigrate` 会迁移这些实体：
  - `Vote`
  - `Proposal`
  - `Credential`
  - `VoteParticipation`
  - `DecryptionSession`
  - `TallyResult`
  - `AuditAnchor`
- 同时包含两类兼容迁移：
  - 历史 `credentials` 表补 `proposal_id`
  - 历史 `proposals` 表补 `options_json`

### 4.5 连接池

- PostgreSQL 的连接池参数显式配置，不再依赖 `database/sql` 的默认值（不限制连接数）：
  - `db.max_open_conns`（`DB_MAX_OPEN_CONNS`，默认 25）
  - `db.max_idle_conns`（`DB_MAX_IDLE_CONNS`，默认 5）
  - `db.conn_max_lifetime_seconds`（`DB_CONN_MAX_LIFETIME_SECONDS`，默认 3600）
- 为 0 表示不改动 `database/sql` 的默认值。
- 优先级与其它 `db.*` 一致：环境变量高于配置文件。
- 启动日志打印实际生效的连接池参数。

## 5. HTTP API

### 5.1 公共接口

- `GET /healthz`
- `GET /api/v1/proposals`
- `GET /api/v1/proposals/open`
- `POST /api/v1/votes`
- `POST /api/v1/proposals/:proposal_id/vote-status`
- `POST /api/v1/proposals/:proposal_id/ballot-draft`（仅 debug 模式）
- `GET /api/v1/proposals/:proposal_id/election-key`（计票公钥，公开值）
- `POST /api/v1/proposals/:proposal_id/credentials/claim`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`

### 5.2 管理接口

以下接口统一走 Bearer Token 鉴权中间件：

- `POST /api/v1/credentials`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/:proposal_id/credentials`
- `POST /api/v1/proposals/:proposal_id/credentials/pool`
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`

### 5.3 关键约束

- `GET /healthz`
  - 响应体是 `{"status":"ok","debug":<bool>}`。
  - `debug` 由 `server.mode` 推导（`debug` / `dev` / `development` 为 true）。
    门户据此决定要不要显示开发联调入口；该字段不是敏感信息，服务端是否处于
    调试模式本来就是公开事实。
- `POST /api/v1/votes`
  - 要求 `proposal_id`、`encrypted_vote`、`proof`、`nullifier`、`user_id`、`user_pub_key`、`credential_signature`，全部必填（HTTP 层与 usecase 层都强制，不存在匿名免凭证投票的路径）
  - **请求体不再有 `public_witness`**：公开输入由服务端按请求字段、提案与计票会话
    自行拼装（`usecase.SubmitVoteRequest.PublicInputs`）。客户端无法自选公开输入，
    因此一份 proof 只能用于它被生成的那份密文，也无法跨提案 / 跨 nullifier 复用。
  - `nullifier` 必须是 64 位十六进制编码的 32 字节域元素：请求层先查长度与 hex
    （`usecase.decodeNullifier`），规范性（小于模数、不按模归约）在验签路径上由
    `ballot.BuildPublicInputs` 复查，不合法 → 400。（`crypto.vk_path` 为空的开发模式
    不走验签路径，只查长度与 hex。）
  - **nullifier 落库前归一成 64 位小写 hex**（`decodeNullifier` 同时返回规范形式）。
    票表 `votes.nullifier` 的唯一索引建在**字符串**上：客户端用大写或带空白的形式
    提交同一个 nullifier，若原样落库就能占掉多行、绕开唯一索引 —— 防重投会失效。
    查重与落库都用规范形式，因此这种变体提交会正常命中「已投过票」。
  - 当前服务端会校验 proof、密文格式、凭证归属、凭证签名、防重投
  - **proof 校验失败返回 400 `INVALID_REQUEST`**（客户端输入问题，不归为服务端故障）
  - **凭证签名验签失败返回 401 `INVALID_CREDENTIAL`**
  - 落票与参与记录要么在同一事务里完成，要么都不做；不存在“记了票但没记参与状态”的路径
  - 请求体里的 `selected_option` 服务端**不消费**，也不应发送（明文选项上服务端会抵消
    “选票是加密的”这个属性）；门户与 CLI 均已移除该字段
- `POST /api/v1/proposals/:proposal_id/ballot-draft`
  - **非 debug 模式返回 404**：用 404 而不是 403，避免暴露端点存在性。
    路由始终注册，门控在 handler 里做。
  - 响应字段：`proposal_id`、`option`、`encrypted_vote`、`proof`、`nullifier`；
    **不再返回 `public_witness`**。
  - 返回的 `proof` 是开发用占位值（`handler.developmentProofBase64`），不是真实证明。
  - 密文由服务端按明文选项生成，服务端此时就知道选了哪个 —— 这条链路不匿名，
    只是为了联调（见 §6.7）。
- `POST /api/v1/proposals/:proposal_id/vote-status`
  - 必须带当前凭证材料，不接受裸 `user_id` 枚举
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
  - 当前实现拒绝“投票开放中”的提案
  - 同一提案重复 setup 返回 `409 DECRYPTION_SESSION_EXISTS`，不会覆盖已有会话（已有会话如果被覆盖，已投选票将无法解密）
  - 实际运营应在投票开始前完成初始化
- `POST /api/v1/proposals/:proposal_id/results/publish`
  - 仅允许在提案关闭后执行
  - 同一提案重复发布返回 `409 RESULT_ALREADY_PUBLISHED`
  - 重复提交同一份 share 返回 `400 INVALID_REQUEST`，不会绕过 threshold 计数
  - share 会先校验是否属于当前 session

## 6. 密码学实现边界

- 选票密码学栈：`internal/infrastructure/crypto/ballot`（BN254 twisted Edwards、v3 密文、Groth16 电路）
- proof 验证：`gnark`（Groth16 / BN254）
- 凭证签发：`kryptology`
- 审计根：Merkle root builder

### 6.1 选票密文格式（v3）

| 项 | 取值 |
|---|---|
| 曲线 | BN254 twisted Edwards（素数阶子群，余因子 8） |
| 标量域 | BN254 Fr |
| 点编码 | 压缩点，32 字节 |
| 密文版本 | `0x03` |
| 密文长度 | 65 字节 |
| 密文结构 | `version(1) \| a(32) \| b(32)` |
| 被加密的内容 | 选项**下标** m，不是选项文本 |
| 加密算法 | 指数 ElGamal：`(a, b) = (r·G, m·G + r·Y)` |

- Y 是提案的计票公钥，G 是基点，r 是一次性随机 nonce。
- 只有密文时服务端无法还原 m：解出 m 需要门限私钥 X。
- 解密是 `b - X·a = m·G`，再与提案的选项集合逐个比对（选项数很小，逐个比即可）。
- 加密下标而不是选项文本：下标是电路里能做范围检查的小整数，选项文本可能很长且不唯一。
- 凡是来自外部的点（密文里的两个点、计票公钥）都要过子群检查：这条曲线余因子是 8，
  小阶点会让解密方泄漏私钥的低位比特。
- **v3 与 v0.1.0 的 v2 密文不兼容**：v2 是 kyber `edwards25519` 上的 ElGamal 封装
  （HKDF 派生密钥 + salt + nonce + tag），v3 是 BN254 twisted Edwards 上的指数 ElGamal。
  旧密文解不开（`unsupported version`），也不该再用于真实投票。
- 换曲线的原因：零知识电路的算术发生在 BN254 的标量域里，电路内无法复算 ed25519
  的群运算，也就无法证明「这份密文确实是本提案计票公钥加密的某个合法选项」。
  仓库原先的 `internal/infrastructure/crypto/kyber_elgamal.go` 及其两个测试已删除。

### 6.2 计票密钥与门限份额
- 计票私钥是 Fr 上的标量（32 字节），公钥是压缩点（32 字节）。
- Shamir 拆份额与门限恢复都在 Fr 上做（`ballot.SplitSecret` / `ballot.RecoverSecret`），
  恢复用拉格朗日插值。门限解密与密文格式因此不需要任何域转换。
- 份额格式：4 字节大端下标 + 32 字节取值，共 36 字节。
- 份额不落明文：`decryption_sessions.shares_json` 只存份额的哈希。

### 6.3 选票电路（真实 Groth16）

电路定义在 `internal/infrastructure/crypto/ballot/circuit.go`。

公开输入（9 个，服务端自行拼装）：

| 字段 | 含义 |
|---|---|
| `ProposalID` | 提案 ID 经 SHA-256 映射到 Fr（`DeriveProposalField`） |
| `Nullifier` | nullifier 域元素 |
| `ElectionKeyX` / `ElectionKeyY` | 提案计票公钥的坐标 |
| `CipherAX` / `CipherAY` | 密文第一个点 a 的坐标 |
| `CipherBX` / `CipherBY` | 密文第二个点 b 的坐标 |
| `NumOptions` | 提案的选项数 |

私有见证（3 个）：

| 字段 | 含义 |
|---|---|
| `OptionIndex` | 选项下标 m |
| `Nonce` | 一次性随机数 r |
| `NullifierSecret` | nullifier 秘密 |

电路断言四件事：

1. `a = r·G`
2. `b = m·G + r·Y`
3. `m < NumOptions` —— 8 位分解 + 最高位检查（位宽上限 256 个选项）
4. `nullifier = MiMC(NullifierSecret, ProposalID)`

**公开输入里没有 option_index，这是刻意的。** 把选项下标放进公开输入等于把明文选项
交给服务端：服务端拼公开输入时就拿到了选了哪个，与「选票内容对服务端不可知」直接
冲突。下标降为私有见证，用范围检查 `m < NumOptions` 代替公开披露 —— 服务端只知道
「投的是某个合法选项」，不知道是哪个。

实测（本机 Go 1.26）：7917 条约束；证明与验证都是毫秒量级（本机实测证明约 20ms、验证约 1–2ms）。

### 6.4 nullifier

- nullifier 是防重投的公开标识：必须由投票人的秘密单向推导，且编码唯一。
- 规范编码是 32 字节大端、十六进制 64 位（`ballot.EncodeScalarHex`）。
- 非规范编码一律拒绝：`ballot.ParseScalarHex` 与 `ballot.BuildPublicInputs` 都会拒绝
  「大于等于模数」或「按模归约过的表示」。否则同一个 nullifier 会有多种字节表示，
  票表上的唯一索引就拦不住重投。
- 派生分两步：`DeriveProposalField`（SHA-256 → Fr）与 `DeriveNullifier`（MiMC，
  与 gnark `std/hash/mimc` 逐位一致的 Go 实现）。
- **两侧都有实现**：Go 侧是 `ballot/nullifier.go`；浏览器侧是纯 TypeScript 实现 ——
  `web/src/lib/sha256.ts`（SHA-256）、`web/src/lib/keccak.ts`（legacy Keccak-256，
  MiMC 轮常量由它派生）、`web/src/lib/mimc.ts`（Fr 上的 MiMC）、
  `web/src/lib/nullifier.ts`（`deriveProposalField` / `deriveNullifier` /
  `generateNullifierSecret`）。SHA-256 没有用 WebCrypto：`crypto.subtle` 要求安全
  上下文，门户被部署在内网 http 地址下会直接不可用。
- **跨语言一致性由 KAT 锁死**：`internal/infrastructure/crypto/ballot/nullifier_interop_test.go`
  与 `web/src/lib/mimc.test.ts` / `web/src/lib/nullifier.test.ts` 断言同一组值
  （提案域元素、nullifier、MiMC 首尾轮常量）。Go 侧还有一个测试真的用这组值
  出了一份证明并验过 —— 即「浏览器算出的 nullifier，电路认」。改任一侧的哈希或
  域运算，必须重新生成向量并同步两处。
- 请求侧先查长度与 hex（`usecase.decodeNullifier`），规范性再查一遍
  （`ballot.BuildPublicInputs`，只在加载 vk 的验签路径上执行）。
- **当前实现边界**：`DeriveNullifier` 服务端侧还没人调用 ——
  `BallotNullifierGenerator` 生成的只是一个随机域元素，没有做 MiMC 绑定；浏览器侧
  的 `generateNullifier` 同理，门户（`useVotePortal.ts`）还在用它。**按当前电路
  约束，这样的 nullifier 出不了证明**。与电路一致的是两条路：CLI 的
  `ballot-encrypt`（秘密留在选票文件里）与浏览器侧的 `deriveNullifier`（秘密留在
  本地，门户接线到 prover 时改用）。

### 6.5 密钥生成与分发

`cmd/keygen` 生成 Groth16 密钥：

| 文件 | 权限 | 去向 |
|---|---|---|
| `verifying.key` | 0644 | 部署到服务端，`crypto.vk_path` 指向它（生产配置默认 `./keys/verifying.key`） |
| `proving.key` | 0600 | 只在客户端侧使用，**不入库、不进服务端镜像** |

- 命令会打印 verifying key 的 sha256 指纹，便于核对部署的是哪一把。
- 默认不覆盖已存在的文件；要覆盖得加 `-force`。
- 轮换密钥等于重新生成一对并让服务端加载新的 vk。旧 proof 用新 vk 验证必然失败，
  因此轮换要挑在投票窗口之外。

### 6.6 服务端证明链接线

- `entity.BallotStatement{ProposalID, Nullifier, ElectionPublicKey, Ciphertext, NumOptions}`
  取代了原先的 `entity.PublicInputs`。
- `domaincrypto.ProofVerifier.Verify(proof, statement)`：验证器吃声明，不吃客户端
  自由提交的公开输入字典。
- 公开输入由服务端拼装（`usecase.SubmitVoteRequest.PublicInputs`）：提案 ID、请求里的
  nullifier 与密文、提案的选项数、计票会话里的计票公钥。
- 客户端不能自选公开输入，因此一份 proof 只能用于它被生成的那份密文，也无法跨提案 /
  跨 nullifier 复用。
- `GnarkProofVerifier`：`crypto.vk_path` 为空时只要求 proof 非空（开发模式）；加载 vk 后
  走真实 Groth16 验签（`ballot.Verifier.Verify`）。release 模式下 `vk_path` 必填，
  这条放行分支不会生效。

### 6.7 debug 模式下的妥协

- `config.Config.DebugMode` 由 `server.mode` 推导：`debug` / `dev` / `development` 为 true，
  其余为 false。
- `POST /api/v1/proposals/:proposal_id/ballot-draft` 只在 debug 模式可达；非 debug 模式
  返回 404（用 404 而不是 403，避免暴露端点存在性）。
- 该端点的密文由服务端按明文选项生成（`usecase.TallyUseCase.EncryptVoteOption`），
  返回的 `proof` 是开发用占位值，不是真实证明。
- 性质要说清楚：debug 模式下服务端知道选了哪个，这条链路**不匿名**，只是换联调速度。
  release 模式下这个端点不存在。

### 6.8 客户端加密（选票密文在客户端生成）

- **计票公钥是公开的**，客户端拿到它就能在本地把选项下标加密成密文，服务端全程
  看不到明文选项。服务端新增公开接口 `GET /api/v1/proposals/:proposal_id/election-key`，
  返回**未压缩**形式（X 与 Y 各 32 字节大端，base64）—— 客户端因此不必实现点解压
  （那需要在标量域里求平方根）。
- **浏览器端实现**：`web/src/lib/ballot-cipher.ts`，BN254 twisted Edwards 上的指数
  ElGamal，与 Go 侧**逐字节一致**。一致性由一组跨语言已知答案向量保证：
  `internal/infrastructure/crypto/ballot/kat_interop_test.go`（Go 侧解出同一个选项
  下标）与 `web/src/lib/ballot-cipher.test.ts`（TS 侧产出同一串密文）。改任何一边
  都要重新生成并同步这组向量。
- **CLI 实现**：`cmd/cli` 的 `ballot-encrypt` 命令 —— 取公钥 → 本地加密 → 落盘选票
  文件。选票文件里带 `nullifier_secret`（nullifier 秘密，只留本地），prover 需要它
  才能满足电路里的 nullifier 约束。
- **服务端不再需要知道明文选项**：`usecase.TallyUseCase.EncryptVoteOption`（服务端
  代加密）只剩 debug 模式的 `ballot-draft` 在用。
- **还没闭合的部分**：门户还没接上客户端加密（缺客户端 prover，接了也投不出去），
  debug 模式的 `ballot-draft` 仍是「服务端知道选项」的那条路径。

### 6.9 凭证验签

- `domaincrypto.CredentialIssuer.VerifyCredential`：用签发方公钥校验凭证签名。
  投票落库前调用（`usecase.VotingUseCase.Submit`），验签失败 → `ErrInvalidCredential`
  → HTTP 401 `INVALID_CREDENTIAL`。
- 此前投票路径只比对「库里存的签名」与「请求里带的签名」是否逐字节相同 ——
  拿一张合法凭证换个 user_id 重放就能过那一关。
- **凭证签发不是真正的盲签名。** `kryptology_credential_issuer.go` 里
  `Prepare → Blind → BlindSign → Finalize` 全在服务端进程内完成，服务端全程持有
  被签消息 —— 等同于普通签名，签发方看得见签的是什么。

> 补齐上述链路的顺序与工作量评估见 [`docs/roadmap.md`](./roadmap.md)（产品路线图）。

## 7. 前端与 CLI

### 7.1 前端门户

- 单页 Vue 3 应用
- 包含两个工作区：
  - 用户端
  - 运营端
- 用户端能力：
  - 查看开放提案
  - 领取或导入凭证
  - 按选项生成投票草稿（**仅 debug 模式可见**）
  - 提交投票
  - 查询是否已投票
- 运营端能力：
  - 创建提案
  - 初始化解密会话
  - 发布结果
  - 查询结果
  - 查看凭证状态
  - 查看审计锚点
- nullifier 在浏览器本地生成（`web/src/lib/nullifier.ts`）：32 字节随机值先按 BN254
  标量域模数归约，再编码成 64 位十六进制，保证一定是规范编码（有对应单元测试）。
- 「按选项生成凭据」按钮只在 debug 模式显示；非 debug 模式改为一句说明：这个入口会让
  服务端看到你选了哪个选项，与匿名投票的前提冲突。debug 标志来自 `GET /healthz` 的
  `debug` 字段，读不到该字段时按 false 处理（宁可把开发入口藏起来）。
- 门户与 CLI 都不再发送 `public_witness`；公开输入由服务端自拼。
- **浏览器端 prover 还没做**：门户不能在本地生成密文与证明，release 模式下门户
  依然无法完成投票（见 §9）。

### 7.2 前端信息架构与设计系统

- 外壳结构：左侧导航（共享页 + 当前工作区分组）+ 顶栏（页面标题与上下文状态）+ 内容区。
  `src/App.vue` 只负责外壳、路由与新手引导编排，业务页面在 `src/components/panels/`。
- 页面分区（`SectionId`）：
  - 共享：`overview`（概览）、`setup`（连接配置）、`guide`（术语参考）
  - 用户端：`hall`（投票大厅）、`identity`（我的身份）、`vote`（参与投票）
  - 运营端：`ops`（计票与审计，内部再分 `OpsStepId` 六个步骤）
- 引导策略：只保留一种上下文引导——当用户被前置条件卡住时，页面顶部出现一条
  带直达动作的提示条（缺 Admin Token、缺凭证、缺解密份额等），条件满足即消失。
  新手引导浮层只负责选角色，具体步骤由内容区顶部的引导条承载；
  术语说明集中在「术语参考」页，不再在每个字段旁重复解释。
- 设计系统：`src/styles/` 分为 `tokens.css`（颜色/圆角/间距/字体令牌）、
  `base.css`（重置与基础排版）、`components.css`（通用 UI 词汇表）、
  `shell.css`（外壳布局）。组件只引用令牌变量，不写裸色值。
- 视图推导规则（工作区归属、页面标题、引导步进）集中在 `src/lib/portal-view.ts`，
  有对应单元测试。
- 候选项用可增删的行式输入：`createProposalForm.options` 是字符串数组，不是按行分割的文本。
  每行一个输入框 + 「移除」按钮，底部「+ 添加选项」；界面至少保留 2 行（后端要求 ≥ 2 个候选项），
  增删后焦点自动落到相邻输入框。行编辑规则在 `src/lib/options.ts`，有对应单元测试。

### 7.3 前端错误文案中文化

- 后端 `ErrorHandler` 统一返回 `{ error, code }`：`code` 是稳定枚举
  （`INVALID_REQUEST` / `NOT_FOUND` / `UNAUTHORIZED` / `DOUBLE_VOTING` /
  `CREDENTIAL_EXISTS` / `USER_ALREADY_VOTED` / `INVALID_CREDENTIAL` /
  `DECRYPTION_SESSION_EXISTS` / `RESULT_ALREADY_PUBLISHED` / `INTERNAL_ERROR`），
  `error` 是面向开发者的英文或原始 Go error 细节。
- 前端不再直接透传 `error`：`src/lib/api.ts` 把 `code` 一并带出，
  `src/lib/error-messages.ts` 的 `describeApiError(message, code, fallback)` 负责翻译，
  优先级为 **业务短语 > 错误码 > 调用方兜底**。
  - 业务短语：按后端透出的英文短语正则匹配（如 `proposal must be closed before tally`），
    映射成带操作指引的中文。
  - 错误码：与操作无关的失败直接用错误码的通用说法。
  - 兜底：调用方按当前操作给出上下文文案（如「该提案还没有发布结果，或结果尚未成功解密」）。
  - 未命中已知短语的 `INVALID_REQUEST` 去掉 `invalid request: ` 前缀后透出后端细节，
    避免吞掉真实原因。
- 所有失败出口统一走 `describeApiError`，不再有组件内的英文硬编码特例。
- CLI 输出保持纯文本命令结果，不做中文化。

### 7.4 CLI

- CLI 直接复用 HTTP API，不绕过服务端校验。
- `decrypt-setup` 强制要求 `-out`：门限份额只返回一次、服务端不保存明文，
  必须落到文件里（`0o600`），不能默认打到 stdout。
- CLI 不发送 `selected_option`：服务端不消费该字段，明文选项上服务端会抵消
  「选票是加密的」这个属性。
- CLI 不发送 `public_witness`：公开输入由服务端按请求字段自拼，客户端无从选择。
- `ballot-draft` 只在 debug 模式可用（服务端在非 debug 模式返回 404），返回的 `proof`
  是开发用占位值，不是真实证明。
- `ballot-encrypt` 在**客户端本地加密**选票：取公开的计票公钥 → 本地加密选项下标 →
  落盘选票文件。选票文件里的 `nullifier_secret` 只留本地，prover 需要它。
- 当前命令：
  - `health`
  - `version`
  - `proposals-open`
  - `proposal-create`
  - `credential-issue`
  - `credential-show`
  - `ballot-draft`
  - `ballot-encrypt`
  - `vote-submit`
  - `vote-status`
  - `credentials-list`
  - `decrypt-setup`
  - `result-publish`
  - `result-get`
  - `anchors-list`

## 8. 测试策略

- 后端统一使用 `go test ./...`
- 当前覆盖：
  - `usecase` 单元测试
  - `persistence` 仓储测试
  - `http` / middleware 测试
  - `config` / `di` 测试
  - `internal/infrastructure/crypto/ballot` 单元测试（密文往返、证明往返、篡改任一公开输入
    验证失败、选项越界无法出证、错 nullifier 秘密无法出证、份额拆分与恢复、未压缩点与
    客户端加密入口）
  - **跨语言一致性测试**：`internal/infrastructure/crypto/ballot/kat_interop_test.go` 与
    `web/src/lib/ballot-cipher.test.ts` 共用一组已知答案向量 —— 浏览器端生成的密文
    必须能被 Go 侧解回同一个选项下标
  - **nullifier 派生的跨语言测试**：`internal/infrastructure/crypto/ballot/nullifier_interop_test.go`
    与 `web/src/lib/mimc.test.ts` / `web/src/lib/nullifier.test.ts` 共用一组向量
    （提案域元素、nullifier、MiMC 轮常量）；Go 侧另有测试用这组值真的出证明并验过
  - `cmd/keygen` 单元测试
  - `tests/integration` 集成测试
  - `tests/e2e` 端到端流程测试（含「客户端本地加密 → 落库只有密文 → 计票还原」）
- PostgreSQL 集成测试使用 `testcontainers`
- 前端最小校验是：
  - `pnpm --dir web exec tsc --noEmit`
  - `pnpm --dir web run lint`
  - `pnpm --dir web run test`

## 9. 当前已知限制

- **证明链已通，但浏览器端 prover 还没做。** 服务端能真实验签：真实 Groth16 电路 +
  `cmd/keygen` + 加载 `crypto.vk_path` 后的 Groth16 验证都落地了。但门户没有本地
  prover，**release 模式下门户依然无法完成投票**；dev 模式走的是服务端代加密的
  草稿链路（`ballot-draft`）。
- **明文选项对服务端不可知：客户端加密已落地，但门户还没接上。** 浏览器端
  （`web/src/lib/ballot-cipher.ts`）与 CLI（`cmd/cli` 的 `ballot-encrypt`）都能在本地
  生成选票密文，服务端只提供公开的计票公钥（`GET .../election-key`），落库记录里
  只有密文。**但门户还没接上**（缺客户端 prover），debug 模式的 `ballot-draft` 仍是
  「服务端知道选项」的那条路径 —— 能力 A 因此尚未完全闭合。
- **凭证签发仍不是盲签名。** 盲化在服务端进程内完成，签发方看得见被签内容（见 §6.9）。
- **nullifier 的 MiMC 绑定：浏览器端已实现，门户还没用上。** 浏览器端
  （`web/src/lib/nullifier.ts` 的 `deriveNullifier`）已能与 Go 侧逐位一致地算出
  电路认可的 nullifier，KAT 也把它钉死了（见 §6.4）。但门户
  （`useVotePortal.ts`）仍在用 `generateNullifier` 生成随机域元素 —— 缺客户端
  prover 时门户也投不出去，接线时一起改。服务端的 `BallotNullifierGenerator`
  同样仍是随机域元素。
- `ballot-draft` 仍是开发联调接口，不应被写成生产能力。
- **v3 密文与 v0.1.0 的 v2 密文不兼容**，旧的加密选票解不开（见 §6.1）。
- 解密会话的初始化时机是当前系统最容易被文档写错的点：
  - 服务端禁止在投票开放中 setup
  - 实际业务上应在开始前准备好 session
- `GET /api/v1/proposals/open` 只返回“状态 open 且当前处于时间窗口内”的提案：
  - 尚未开始的提案不会出现在大厅列表里
  - 管理端需要看到未开始/已关闭提案时，改用 `GET /api/v1/proposals`（全部提案，按创建时间倒序）
- 前端仍保留对历史 JSON 描述字段的兼容解析，仅用于读取旧数据；新写路径已统一为纯文本 `description` + 独立 `options`。
