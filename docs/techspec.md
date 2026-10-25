# 技术说明（当前实现）

本文档描述的是仓库当前已经落地的实现，不再保留目标态伪代码。

## 1. 代码结构

```text
cmd/
├── cli/                    # 命令行客户端，复用 HTTP API
├── keygen/                 # 生成 Groth16 密钥（vk / pk）：选票与资格两套电路
├── ballot-wasm/            # 浏览器端 prover 的 WASM 模块（选票 + 资格两套电路）
├── eligibility-list/       # 离线名单工具：成员名单 → Merkle 根 + 每人的资格包
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
│   │   ├── ballot/         # 选票密码学栈：曲线运算、v3 密文、Groth16 电路与证明器
│   │   ├── eligibility/    # 资格密码学栈：名单 Merkle 承诺、资格电路与证明器
│   │   └── mimc/           # 选票电路与资格电路共用的 MiMC 实现
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
  - `crypto/eligibility`：资格密码学栈 —— 资格名单的 Merkle 承诺、Groth16 资格
    电路与证明/验证器。服务端只持有名单根与资格验证密钥。
  - `crypto/mimc`：选票电路与资格电路**共用**的 MiMC 实现 —— 两处必须同一份，
    差一位客户端就出不了证明（且不会报错，只是验签失败）。
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

### 3.5 资格（一人一票）

- 资格是**权威名单分配**的：名单离线生成 Merkle 承诺，服务端只拿到**树根**，
  成员各自拿到「资格秘密 + 叶子 + Merkle 路径」的**资格包**。
- 成员领凭证时在本地出一份零知识**资格证明**，证明「我的叶子在这棵树里」。
  服务端据此强制「一人一票」，却**不知道**这份资格属于谁。
- 去重键 `QualificationID = MiMC(Leaf, ProposalID)`：
  - 同一成员在同一提案里稳定 → 第二次领取撞唯一索引
  - 跨提案不同 → 跨提案不可关联
- `eligibility_uses` 表只存**去重键**：
  - `id`
  - `proposal_id`
  - `qualification_id`（TEXT，64 位十六进制）
  - `used_at`
  - 唯一索引 `(proposal_id, qualification_id)` —— **这就是「一人一票」的强制点**
- **不存**叶子、资格秘密、成员 ID；也**不与凭证、票据关联** —— 一旦能把资格连到
  票，「分发不泄露身份」这条底线就破了。

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
  - `crypto.eligibility_root`（资格名单的 Merkle 树根，64 位十六进制的域元素）
  - `crypto.eligibility_vk_path`（资格电路的 verifying key 路径）
  - `db.driver`
  - `db.path`
  - `db.host`
  - `db.port`
  - `db.name`
  - `db.user`
  - `db.password`
  - `db.sslmode`
- 资格校验的两个配置项有环境变量：`CRYPTO_ELIGIBILITY_ROOT` /
  `CRYPTO_ELIGIBILITY_VK_PATH`（优先级与其它 `crypto.*` 一致：环境变量高于配置文件）。

### 4.3 运行期校验

- 当 `server.mode` 为 `release/prod/production` 时：
  - `crypto.vk_path` 必填
  - **`crypto.eligibility_root` 与 `crypto.eligibility_vk_path` 必须同时必填** ——
    没有名单根与资格验证密钥，服务端就无法强制「同一个人只能领一张票」，
    那时它只是个能发凭证的服务器。这条与 `crypto.vk_path` 是同级的硬要求。
- `crypto.eligibility_root` 与 `crypto.eligibility_vk_path` **必须成对出现**
  （任何模式都校验）：只配根等于「不校验」，只配密钥则无从校验 ——
  两者只配一个时服务**启动失败**，不静默降级。
- `server.admin_bearer_token` 若为空，会在启动期自动生成并写回内存配置
- 自动生成的 token 会通过启动日志打印到控制台，仅对当前进程实例有效
- 启动日志会打印**实际加载的配置文件路径**（`config_file`）—— 「以为加载了生产
  配置、实际跑的是开发配置」是最常见的上线事故，必须能一眼看出来。
- release 模式下 `cors_allowed_origins` 为空时**告警**（不阻止启动）：门户与
  后端同源部署时留空是合法的，但不同源时浏览器会拦掉所有跨域请求。

### 4.4 迁移

- **迁移是独立步骤**：`nullifier-server -migrate` 跑完迁移即退出。
  迁移只需要数据库连接，不要求 `crypto.vk_path`、`server.admin_bearer_token`
  与资格校验的两个配置。
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
  - `CredentialClaim`
  - `EligibilityUse`（资格使用记录 —— 「一人一票」的强制点，见 §3.5）
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

### 4.6 SQLite 的两条硬约束

SQLite 只在开发/单机形态使用，但它的两个限制会**直接打穿管理面接口**，
所以在这里写死口径：

1. **批量写入必须分批**（`persistence.claimSaveBatchSize = 100` 行/条语句）。
   - 原因：一条语句的绑定参数数量有上限（旧版 SQLite 999，新版 32766）。
     `credential_claims` 有 7 列，一次性写 **4681 行以上**就会报
     `too many SQL variables` —— 而 `credentials/pool` 接口允许一次投到
     **10000** 个槽位，不分批那条路径必然失败。
   - 批次都在**同一个事务**里：要么整池成功，要么整池回滚，不会留下半个池子。
2. **临时表与临时索引固定放内存**（`PRAGMA temp_store=MEMORY`，通过驱动
   的 ConnectHook 对**每条新连接**设置，见 `persistence/db.go`）。
   - 原因：多行 INSERT 做主键去重时 SQLite 会建一个临时索引；索引撑破内存后
     会**溢写临时文件**，而本机（Windows，用户目录下的临时目录）实测过这个
     临时文件建不出来 —— 整条 INSERT 于是以 `SQLITE_CANTOPEN` 失败，接口报成
     一句看不懂的 `unable to open database file: The system cannot find the
     file specified.`（备池、投票这类批量写都会中招）。
   - 临时索引的大小只跟**单批行数**有关（几十行），放进内存不会把内存吃穿；
     而依赖操作系统临时目录的风险已经被实测过了。
   - 用 ConnectHook 而不是「启动时执行一次 PRAGMA」：`database/sql` 的连接是
     按需建立的，启动时执行只会落在当时那一条连接上。

## 5. HTTP API

### 5.1 公共接口

- `GET /healthz`
- `GET /api/v1/proposals`
- `GET /api/v1/proposals/open`
- `POST /api/v1/votes`
- `POST /api/v1/proposals/:proposal_id/vote-status`
- `GET /api/v1/proposals/:proposal_id/election-key`（计票公钥，公开值）
- `POST /api/v1/proposals/:proposal_id/credentials/eligibility-challenge`（领取挑战，公开）
- `POST /api/v1/proposals/:proposal_id/credentials/claim`
- `POST /api/v1/proposals/:proposal_id/credentials/blind-sign`
- `POST /api/v1/proposals/:proposal_id/credentials/attest`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`

### 5.2 管理接口

以下接口统一走 Bearer Token 鉴权中间件：

- `POST /api/v1/credentials`
- `POST /api/v1/credentials/blind-sign`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/:proposal_id/credentials`
- `POST /api/v1/proposals/:proposal_id/credentials/pool`
  - `count` 上限 **10000**（`usecase.maxCredentialPoolSize`）；服务端一次性把
    `count` 个槽位写进池子（SQLite 下分批写，见 §4.6）。
  - 池子只增不减：重复调用是**继续往池里加**槽位，不是补齐到 `count`。
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`

### 5.3 关键约束

- `GET /healthz`
  - 响应体是 `{"status":"ok","debug":<bool>,"eligibility":<bool>}`。
  - `debug` 由 `server.mode` 推导（`debug` / `dev` / `development` 为 true）。
    门户据此决定要不要显示开发联调入口；该字段不是敏感信息，服务端是否处于
    调试模式本来就是公开事实。
  - `eligibility` 报出服务端**是否在强制一人一票**（`crypto.eligibility_root`
    是否已配置）。没开的话任何人都能反复领票 —— 这是运营必须能一眼看到的
    运行状态，同样不是敏感信息。
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
- `POST /api/v1/proposals/:proposal_id/vote-status`
  - 必须带当前凭证材料，不接受裸 `user_id` 枚举
- `POST /api/v1/proposals/:proposal_id/credentials/eligibility-challenge`（领取挑战，公开）
  - 成功 200，响应体是 `{"proposal_id","root","depth","challenge"}`：
    `root` 是服务端配置的名单树根（64 位十六进制的域元素）、`depth` 是名单树深
    （当前恒为 10）、`challenge` 是本次会话的一次性挑战（同样 64 位十六进制）。
  - **服务端不记账**：挑战无状态，重放由去重键的唯一索引挡住（见 §3.5）。
  - 409 `ELIGIBILITY_NOT_ENABLED`：服务端没开启资格校验。
  - 404 `NOT_FOUND`：提案不存在。
- `POST /api/v1/proposals/:proposal_id/credentials/claim`（预留签发权，公开）
  - 请求体**仍然不接受 `user_pub_key`**。服务端一旦知道公钥就能算出凭证消息
    的原文，盲化当场失效（它手上有私钥，根本不需要解盲）。旧版本这个端点收
    `user_pub_key`，现在一律忽略。
  - **请求体从「空」变成「可选的三件资格材料」**：
    `{"eligibility_proof": "<base64>", "qualification_id": "<64 位 hex>",
    "challenge": "<64 位 hex>"}`。
    - 三件**要么都给、要么都不给**；只给一半 → 400 `INVALID_REQUEST`。
    - 开了资格校验却没给材料 → 400 `ELIGIBILITY_REQUIRED`。
    - 给了材料但服务端没开资格校验 → 409 `ELIGIBILITY_NOT_ENABLED`
      （不能让它「看起来被验过」）。
    - 证明验不过（伪造、名单不对、提案不对、挑战不对）→ 403
      `INVALID_ELIGIBILITY_PROOF`。
    - 同一份资格第二次领 → 409 `ELIGIBILITY_ALREADY_USED`。
  - **资格不过不消耗池槽位**：顺序是「验资格 → 取槽位（不占位）→ 记资格使用」。
    唯一约束冲突时槽位还没被标记领取，不会被白白消耗。
  - 请求体里**不许出现真人身份字段**（工号、姓名、证件号……）：资格材料里只有
    零知识证明与两个公开输入，服务端从头到尾不知道领票的是谁。
  - 响应：`proposal_id`、`user_id`（池槽位的假名）、`issuer_key_version`、`variant`、
    `modulus`（base64）、`exponent`。
  - **预留不占位**：服务端取槽位时不改状态，所以并发领取会抢到同一个槽位。撞车的
    那个由**客户端**换一个槽位重来（最多 5 次，只在 attest 返回 409
    `CREDENTIAL_EXISTS` 时重试）—— 重试要重新盲化，服务端代劳不了。
    「预留即占位」没有采用：那会让「只预留不 attest」的客户端永久消耗池槽位，
    变成一个 DoS。
- `POST /api/v1/proposals/:proposal_id/credentials/blind-sign`（公开）
  - 请求体：`user_id`、`blinded_message`（base64）、`issuer_key_version`。
  - `issuer_key_version` 必填：服务端要用客户端盲化时那一版的私钥签，版本对不上
    解盲结果作废。用请求携带版本，避免改 schema。
  - **只覆盖池里还空着的槽位**（`user_id` 必须在该提案的可用槽位里），否则签发方就
    成了「任意消息」的签名预言机。
  - 服务端在这一步只见盲化消息，见不到被签内容。
- `POST /api/v1/proposals/:proposal_id/credentials/attest`（公开）
  - 请求体：`user_id`、`user_pub_key`（base64）、`signature`（base64）。
  - **先验签再落库**：验签失败 → 401 `INVALID_CREDENTIAL`；同一 `(proposal, user_id)`
    已有凭证 → 409 `CREDENTIAL_EXISTS`。
  - 验签绑定的消息是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)`，交叉攻击（拿 A 的盲
    签名去签 B）必然验不过。
- `POST /api/v1/credentials`（管理端预留，公开路径之外的 `/api/v1/credentials`）
  - 请求体：`proposal_id`、`user_id`。同样**不收公钥**。
  - 与公开端的区别：可以指定任意 `user_id`，且该 user_id 不必在池里（池槽位是
    「匿名领取」的机制，管理端签发不需要它）。
  - **管理端签发不校验资格证明**（请求体里没有资格材料的位置）：运营方可以为指定
    `user_id` 签发凭证。因此「一人一票」只在**公开领取端点**上被强制 —— 运营方自己
    签发的那条路不在其内。
- `POST /api/v1/credentials/blind-sign`（管理端盲签名）
  - 请求体：`proposal_id`、`user_id`、`blinded_message`（base64）、
    `issuer_key_version`。
  - 与公开端的区别：可以覆盖任意 `user_id`（运营方指定的人），不要求池槽位。
  - 路径与公开端不同是因为 gin 的路由树不允许同路径挂两套中间件。
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
- **proving key 要分发给投票人**（CLI 用户拿它跑 `ballot-prove`；浏览器端 prover 落地后
  由门户提供）。这一点要说清楚：拿到 proving key 的人可以自己出证明，**但证明只绑定
  密文、不绑定身份** —— 它不携带任何「谁投的」信息，因此分发它不构成新的身份泄露；
  真正的身份约束来自凭证签名与「一张凭证只能投一票」。反过来，**服务端永远不该拿到
  proving key**（它拿到就能替任何人造出合法密文与证明）。

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

### 6.7 debug 模式

- `config.Config.DebugMode` 由 `server.mode` 推导：`debug` / `dev` / `development` 为 true，
  其余为 false。它**只作为运行状态暴露**（`GET /healthz` 的 `debug` 字段）。
- **服务端在两种模式下的行为完全一致**：没有「替用户选票」的调试能力。
  曾存在的 `POST /api/v1/proposals/:proposal_id/ballot-draft`（服务端代加密）已删除，
  路由不再注册，任何模式都返回 404。
- 这条底线由 `tests/architecture/plaintext_option_test.go` 持续守卫：请求体结构体不得有
  明文选项字段、usecase 不得有接受明文选项的入口、服务端代加密那条链不许复活、
  ballot-draft 端点不许复活。

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
- **CLI 侧本地 prover**：`cmd/cli` 的 `ballot-prove` 命令 —— 读选票文件 → 用本地
  proving key 生成 Groth16 证明 → 把证明写回选票文件。**证明全程不经过服务端**，
  proving key 也只留在客户端（服务端只拿 verifying key）。
  - 选票文件因此多存四项本地见证材料：`nonce`（加密用的一次性随机数，电路约束
    `a = r·G` 要用）、`option_index`（选项下标，电路的私有见证）、`num_options`
    （提案选项数，公开输入之一）、`election_public_key`（计票公钥未压缩形式）。
    这四项**都不提交给服务端**，`vote-submit` 也不会发它们。
  - 公开输入走**服务端同一套**拼装入口（`ballot.BuildPublicInputs`）：客户端与服务端
    算出同一份公开输入，证明才验得过。计票公钥客户端拿到的是未压缩形式，服务端
    用的是压缩形式，客户端先用 `ballot.CompressPoint` 转成同一形式。
- **服务端不再需要知道明文选项**：服务端代加密的 `usecase.TallyUseCase.EncryptVoteOption`
  已随 `ballot-draft` 一起删除，服务端只剩解密（计票）这一侧。
- **门户接线已闭合**：浏览器端 prover 见 §6.10，门户在本地出票再提交。

### 6.10 浏览器端 prover（WASM）

- **模块**：`cmd/ballot-wasm`，用 `GOOS=js GOARCH=wasm` 构建。它在全局对象上注册
  `nullifierProver`，暴露四个函数（入参出参都是 JSON 字符串 / JS 对象）：

  | 函数 | 入参 | 出参 |
  |---|---|---|
  | `init` | `{"proving_key": "<base64>"}` | `{"ok":true,"init_ms":7608}` |
  | `prove` | `{"proposal_id","election_public_key","option_index","num_options"}` | `{"ok":true,"ciphertext","nullifier","proof","prove_ms":2254}` |
  | `initEligibility` | `{"proving_key": "<base64>"}` | `{"ok":true,"init_ms":5120}` |
  | `proveEligibility` | `{"proposal_id","challenge","root","member_package"}` | `{"ok":true,"proof","qualification_id","prove_ms":...}` |

  - **两套电路在同一个 WASM 模块里**（选票 + 资格），各自一把 proving key：
    `proving.key` 与 `eligibility-proving.key`。两把密钥互不相通。

- **出票在 Go 侧一次做完**：`ballot.Prover.ProveBallot`（`internal/infrastructure/crypto/ballot/client.go`）
  依次做四件事 —— 压缩计票公钥 → 加密选项下标 → 随机 nullifier 秘密并派生 nullifier →
  用 `ballot.BuildPublicInputs` 拼公开输入 → 出证明。**nonce 与 nullifier 秘密只在这一次
  调用里出现**，出了函数就拼不出与密文匹配的见证。
- **客户端与服务端共用同一套代码**：公开输入走 `ballot.BuildPublicInputs`，服务端拼的那份
  与客户端拼的那份逐字段相同 —— 否则证明验不过。计票公钥客户端拿到未压缩形式、服务端用
  压缩形式，差异由 `ballot.CompressPoint` 消除。
- **WASM 里必须关掉 gnark 的日志**：js/wasm 往 stdout / stderr 写日志是「发请求给 JS 侧、
  再等回复」的同步调用，而 JS 侧此刻正卡在这次同步回调里，事件循环转不起来 —— 结果是整个
  运行时死锁（`all goroutines are asleep`）。`cmd/ballot-wasm` 的 `main` 第一件事就是
  `logger.Disable()`。
- **门户侧**：`web/src/workers/ballot-prover.worker.ts` 在 Worker 里加载 Go 官方胶水
  `wasm_exec.js` 与 WASM 模块，主线程 `web/src/lib/wasm-prover.ts` 只负责发消息、收结果。
  放进 Worker 的原因：加载 proving key 会编译一次电路（浏览器上约 7~8 秒），出证明再约
  2 秒，放在主线程会把界面卡死。
- **依赖的静态资源**（门户静态目录，见 `web/public/README.md`）：
  `wasm_exec.js`（Go 官方胶水，BSD 许可）、`ballot-prover.wasm`（约 11.9 MB，gzip 后约
  2.8 MB）、`proving.key`（约 1.2 MB）、`eligibility-proving.key`（资格电路的 proving
  key）。由 `make web-wasm`、`make web-proving-key PK=<路径>` 与
  `make web-eligibility-key PK=<路径>` 准备，**都不进仓库**。
- **proving key 只在客户端**：服务端只拿 verifying key（`crypto.vk_path`）。发布到门户
  静态目录是给**投票人**下载用的，不进服务端镜像、不进数据库。
- **实测**（node 驱动同一个 WASM 模块，见 `tests/e2e/wasm_prover_test.go`）：
  初始化 7.6 秒、出证明 2.25 秒、证明 128 字节；服务端以真 verifying key 验签通过，
  篡改一个字节的证明被 400 拒绝，计票还原出客户端选的那个选项。
- **真实浏览器实测**（2026-10-25，Chromium / Electron）：在真浏览器里加载
  `wasm_exec.js` + `ballot-prover.wasm` + `proving.key`，初始化 11.8~14.3 秒、出证明 2.9 秒，
  服务端以真 verifying key 验签通过（201），计票还原出浏览器端选的那个选项
  （`{"tally":{"candidate-B":1}}`）。这一次真实浏览器验收还挖出一个只靠 node 测不出来的
  跨语言问题：门户把盲签名变体名写成了 Go 的枚举标识符，服务端发的是 CIRCL 的线路名 ——
  见 §6.9。

### 6.9 凭证验签与真两段式盲签名

- `domaincrypto.CredentialIssuer` 现在是三个方法：
  - `BlindSign(ctx, keyVersion, blindedMessage)` —— 签发方对**盲化后**的消息做盲签名，
    这是签发方在真两段式里**唯一**的签名动作。
  - `IssuerPublicKey(ctx)` —— 返回当前签发方公钥（公开值，客户端盲化本来就需要），
    并顺带做密钥轮换。
  - `VerifyCredential(userID, userPubKey, signature)` —— 校验凭证签名，返回验签通过的
    密钥版本。
  - 一次性 `Issue` 已删除。
- 投票落库前调用 `VerifyCredential`（`usecase.VotingUseCase.Submit`），验签失败 →
  `ErrInvalidCredential` → HTTP 401 `INVALID_CREDENTIAL`。此前投票路径只比对「库里
  存的签名」与「请求里带的签名」是否逐字节相同 —— 拿一张合法凭证换个 user_id 重放
  就能过那一关。
- **协议是三步**（真两段式盲签名，RFC 9474 RSABSSA-PSS-Deterministic / SHA-384）：

  | 步骤 | 端点 | 服务端看到什么 |
  |---|---|---|
  | ① 预留签发权 | `credentials/claim` | 池槽位的假名 `user_id` + 签发方公钥 |
  | ② 盲签名 | `credentials/blind-sign` | 只有**盲化消息** |
  | ③ attest | `credentials/attest` | `user_id`、公钥、最终签名（此时验签） |

  关键：**客户端从头到尾没把公钥交给服务端**，直到第 ③ 步。服务端一旦知道公钥就能
  算出凭证消息原文，盲化当场失效。
- **凭证消息**是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)`（Go 侧 `crypto.CredentialMessage`，
  TS 侧 `credentialMessage`）。`0x00` 分隔符固定 user_id 与公钥的边界 —— 没有它，
  `(a, b)` 与 `(a‖0x00, b)` 会撞。跨语言一致性由
  `internal/infrastructure/crypto/blind_rsa_interop_test.go` 与
  `web/src/lib/credential.test.ts` 用同一组向量锁死。
- **验签绑定是安全锚**：消息里含 `user_id` 与公钥，所以交叉攻击（拿 A 的盲签名去签
  B）必然验不过 —— 这是路线图 T17 的验收 ②。
- **客户端实现**：
  - 浏览器端：`web/src/lib/credential.ts`（`credentialMessage` / `parseIssuerKey` /
    `blindCredentialMessage` / `finalizeCredentialSignature`）+
    `web/src/lib/blind-rsa.ts`（Prepare / Blind / Finalize、EMSA-PSS、MGF1、SHA-384）。
  - Go 端：`internal/infrastructure/crypto/credential_client.go`
    （`CredentialBlindClient.Blind` / `Finalize`）—— CLI 与端到端测试共用。
  - 两边与服务端 CIRCL **逐字节一致**，由
    `internal/infrastructure/crypto/blind_rsa_interop_test.go`（Go 侧用 CIRCL 真库）
    与 `web/src/lib/blind-rsa.test.ts`（TS 侧）共同锁定。
  - 关键事实：CIRCL 的 client 侧**只需要签发方的 RSA 公钥**（`n`、`e`），不需要私钥 ——
    因此客户端可以自己盲化，服务端那一侧只剩 `Signer.BlindSign(blindedMessage)`。
- **变体名是线路上的契约，两边必须一模一样**：`issuer_key` 响应里的 `variant` 字段是
  CIRCL `blindsign/blindrsa` 的**线路名** `RSABSSA-SHA384-PSS-Deterministic`
  （即 `blindrsa.SHA384PSSDeterministic.String()`），**不是** Go 侧的枚举标识符
  `SHA384PSSDeterministic`。门户的 `supportedBlindSignatureVariant` 必须写线路名；
  写错时 `parseIssuerKey` 直接抛错，领取流程停在 ① 之后（服务端只看到 claim，blind-sign
  从未发出），浏览器端表现为「领取失败：签发方公钥不可用」。CLI 不受影响 —— 它比的是同一个
  库的 `String()`。这条契约由 Go 侧 `TestKAT_BlindRsaVariantName` 与 TS 侧
  `credential.test.ts` 的「变体名与服务端线路上的名字一致」一起钉死。
- **管理端也走真两段式**（与匿名领取同一套协议，只是端点不同）：运营方指定
  `user_id` → 管理端客户端本地盲化 → 管理端 blind-sign → attest。管理端 blind-sign
  走全局路径 `/api/v1/credentials/blind-sign`，因为 gin 的路由树不允许同一路径挂
  两套中间件。
- **槽位竞争的解法在客户端**：预留不占位 → 并发领取会抢同一个槽位 → attest 返回
  409 `CREDENTIAL_EXISTS` → 客户端换一个槽位重来（最多 5 次）。409 还有另一种语义
  `CREDENTIAL_POOL_EMPTY`（池领完了），那个**不重试**。
- **「一人一票」已落地**（见 §6.11）：资格证明把「服务端能去重」与「服务端不知道是谁」
  同时成立，门户与 CLI 两侧都能出资格证明。

> 补齐上述链路的顺序与工作量评估见 [`docs/roadmap.md`](./roadmap.md)（产品路线图）。

### 6.11 资格证明（一人一票，T22）

- **要解决的问题**：凭证是匿名随机领取的，同一个人领两次就能投两票。要强制
  「一人一票」，服务端就必须能对某个标识去重；而只要这个标识与真人的对应关系
  是服务端可见的，去重就等于给了服务端关联票与人的能力。
- **做法**：资格由**权威名单**分配，成员以**零知识**的方式出示资格。
  服务端只拿到名单的**树根**，能验证「这个人的叶子在树里」，却无法把叶子对应到
  任何成员。
- **包**：`internal/infrastructure/crypto/eligibility`

  | 文件 | 内容 |
  |---|---|
  | `merkle.go` | 名单树与两种产物：`MemberPackage`（成员本人保管的秘密材料）与 `ListPackage`（给服务端的根）；`BuildTree` / `DeriveLeaf` / `DeriveQualificationID` / `DecodeField`（拒绝非规范编码） |
  | `circuit.go` | 资格电路 `eligibilityCircuit` 与 `TreeDepth` |
  | `statement.go` | 公开输入 `Statement`（**定长 5 个**）与 `BuildStatement` |
  | `prover.go` | Groth16 的 `Prover` / `Verifier` / `GenerateKeys` / `Constraints` |
  | `client.go` | 客户端一站式出证明：`Prover.ProveEligibility`（重算叶子并与包比对、由叶子推出去重键、用**服务端的** root 出证明） |

- **电路形状**：Groth16 / BN254，哈希用 MiMC。**实测 6860 条约束**
  （`eligibility.Constraints()`，护栏测试断言 < 12000）。
- **公开输入（定长 5 个，服务端自拼，客户端无从篡改）**：

  | 字段 | 含义 |
  |---|---|
  | `Root` | 资格名单的 Merkle 树根 |
  | `ProposalID` | 提案 ID 经 SHA-256 映射到 Fr（`ballot.DeriveProposalField`） |
  | `Challenge` | 本次领取会话的一次性挑战 |
  | `QualificationID` | 去重键 |
  | `SessionBinding` | `MiMC(QualificationID, Challenge)`，把挑战钉在这份证明上 |

- **私有见证**：`Secret`（资格秘密）、`Index`（叶子位置）、`Path`（10 个兄弟节点）。
  **叶子、路径、位置全是私有的** —— 服务端因此不需要知道树的结构，公开输入的
  个数也不随树深变化。
- **电路断言四件事**：
  1. `Leaf = MiMC(Secret)` —— 叶子由秘密单向决定；
  2. `QualificationID = MiMC(Leaf, ProposalID)` —— 去重键绑定本提案；
  3. `SessionBinding = MiMC(QualificationID, Challenge)` —— 截获的证明挪不到别的挑战上用；
  4. `Index` 按 `TreeDepth` 位分解并能复原，沿 `Path` 上溯得到 `Root`。
- **去重键**：`QualificationID = MiMC(Leaf, ProposalID)` —— 同一成员同一提案稳定
  （第二次领取撞唯一索引），跨提案不同（跨提案不可关联）。
- **`TreeDepth = 10` 是编译期常量**（电路里 `ToBinary` 的宽度必须是常量）：
  名单上限 **1024 人**；要更大得改常量、重编电路与 WASM、重生成密钥。
- **名单工具**：`cmd/eligibility-list -members <名单文件> -out <目录>` —— 离线运行，
  不联网、不连服务端。输入每行一个成员 ID（空行与 `#` 开头行忽略，重复行去重，
  成员按 ID 排序后建树，因此同一份名单的**成员位置**是确定的；但每次运行都会
  重新随机生成资格秘密，所以**两次运行的树根不一样**）。输出：
  - `root.json` —— 名单根（`root` / `depth` / `count`），给服务端配置
    `crypto.eligibility_root`；
  - `eligibility-<member_id>.json` —— 每成员一份资格包（`member_id` / `secret` /
    `leaf` / `index` / `path` / `root` / `depth`），权限 **0600**，走安全渠道发给本人。
  - **两份产物必须分开保管**：服务端只该拿到 root。
  - **成员标识的约束**：包的文件名由标识归一化而来（字母数字与 `-_.` 之外的字符
    一律换成 `_`），所以标识**归一化后不得重名**。两个标识若归一到同一个文件名，
    工具在**写盘前**就拒绝并点名是哪两个标识撞了（`checkMemberFileNames`）——
    共用一个包名等于有一个人拿不到自己的包（或拿到别人的），一人一票当场失效。
    现实名单里的中文标识（「张三-01」「李四-01」）正好会撞，所以**标识应当用
    只含字母数字与 `-_.` 的形式**（工号 / 选民编号 / 会员号通常如此）。
  - **落盘顺序**：成员包先写、`root.json` **最后**写 —— root 是要配到服务端的，
    落盘就意味着「这份名单可以上线」；写包中途失败时目录里没有 root，
    不会留下一份「声称 N 人、实际 N-1 个包」的名单被误上线。
- **两套密钥，互不相通**：`cmd/keygen -circuit ballot|eligibility`
  - 选票：`verifying.key`（服务端，`crypto.vk_path`）/ `proving.key`（客户端）
  - 资格：`eligibility-verifying.key`（服务端，`crypto.eligibility_vk_path`）/
    `eligibility-proving.key`（客户端）
  - Groth16 的 setup 是 **per-circuit** 的，两套密钥不能互换。
- **共享 MiMC**：`internal/infrastructure/crypto/mimc` —— 选票电路的 nullifier 约束
  与资格电路的 Merkle 承诺**共用同一份** MiMC 实现（`mimc.Hash`），并另导出
  `mimc.Constants()` 供跨语言 KAT 钉住轮常量。两处必须同一份：差一位，客户端就
  出不了证明（而且不会报错，只是验签失败）。
- **领域适配层**：`internal/infrastructure/crypto/eligibility_service.go` ——
  `Enabled` / `Root` / `NewChallenge` / `Verify`。
  **没有「跳过验签」的降级模式**：选票证明有（`vk_path` 为空时只查 proof 非空），
  资格证明没有 —— 验签密钥缺失等于服务端无法履行一人一票的承诺，那要在启动时
  失败，而不是悄悄放行。
- **与原设计稿 `docs/t22-eligibility-design.md` 的偏离**（该设计稿的 §8 也如实列了同一张表；
  实现口径以本文为准）：
  1. 设计稿原文说浏览器要**两个** WASM 模块；实现是**一个**模块四个 API
     （`init` / `prove` / `initEligibility` / `proveEligibility`），静态资源只新增
     `eligibility-proving.key`；
  2. 设计稿原文说 `qualification_id` 存 `BLOB(32)`；实现存 64 位十六进制的 **TEXT**；
  3. 设计稿原文的公开输入列了 7 个（含 Leaf / Path / Index）；实现是 **5 个**，
     路径与叶子全私有；
  4. 挑战**无状态**（服务端不记账）—— 这一条与设计稿一致；
  5. 设计稿的 D1–D6 六个决策点**已全部拍板并落地**（D1 完整方案 B / D2 离线工具生成、
     root 与成员包分开保管 / D3 MiMC / D4 claim 之前先取挑战 / D5 `depth=10` 且是编译期
     常量 / D6 名单可跨提案复用）。

## 7. 前端与 CLI

### 7.1 前端门户

- 单页 Vue 3 应用
- 包含两个工作区：
  - 用户端
  - 运营端
- 用户端能力：
  - 查看开放提案
  - 领取或导入凭证（开启资格校验时还要带资格证明，见 §6.11）
  - **在浏览器本地生成选票**（密文 + nullifier + 零知识证明，见 §6.10）
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
- 「按选项生成凭据」按钮**已删除**：它把明文选项发给服务端，与匿名投票的前提冲突。
  出票只有「在浏览器本地生成选票」这一条路。
- 门户与 CLI 都不再发送 `public_witness`；公开输入由服务端自拼。
- **凭证领取走真两段式盲签名**（`web/src/lib/credential.ts` + `web/src/lib/blind-rsa.ts`）：
  预留 → 本地盲化 → 盲签名 → 本地解盲 → attest。预留阶段的请求体里**没有公钥**
  （开启资格校验时只有那三件资格材料）—— 公钥只在 attest 那一步才交给服务端。
  槽位被抢走时门户换一个槽位重来（最多 5 次）。
- **浏览器端 prover 已落地**（`web/src/lib/wasm-prover.ts` + `web/src/workers/ballot-prover.worker.ts`）：
  门户在本地生成密文、nullifier 与证明再提交，服务端拿不到明文选项（见 §6.10）。
  前提是门户静态目录里备好 `wasm_exec.js`、`ballot-prover.wasm`、`proving.key` 三个文件
  （`make web-wasm` / `make web-proving-key`）；**开启资格校验时是四个** —— 还要一把
  `eligibility-proving.key`（见下）。
- **「一人一票」在门户里多一步**（见 §6.11）：「我的身份」面板多一个「导入资格文件」
  （`eligibility-<member_id>.json`）。领凭证时门户先向服务端要一次领取挑战，再把
  成员包与挑战交给浏览器端 prover（`web/src/lib/eligibility-prover.ts`，密码学全在
  WASM 里）出证明，最后带着证明去领凭证。
  - **资格包只留在内存**：不写 `localStorage`、不上传，刷新页面就要重新导入。
  - **发给 Worker 前要把成员包拷成普通对象**：门户里它存在 Vue 的 `ref` 里，读出来是
    响应式 Proxy，而 `postMessage` 的结构化克隆克隆不了 Proxy —— 浏览器会抛
    `DataCloneError: #<Object> could not be cloned.`，「匿名领取」必然失败。修法见
    `web/src/lib/eligibility-prover.ts` 的 `buildEligibilityProveRequest`（逐字段拷贝）。
  - 前提是静态目录里还备好 `eligibility-proving.key`（`make web-eligibility-key PK=<路径>`）。
- **nullifier 现在由 prover 派生**：出票时由 `ballot.ProveBallot` 在本地随机一个秘密
  `s` 并派生 `nullifier = MiMC(s, proposal_id)`，秘密不出本地。门户表单里的 nullifier
  字段仍可手工编辑（低频工具），但正式出票走 prover 这条路。

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
  `CREDENTIAL_EXISTS` / `CREDENTIAL_POOL_EMPTY` / `USER_ALREADY_VOTED` /
  `INVALID_CREDENTIAL` / `DECRYPTION_SESSION_EXISTS` / `RESULT_ALREADY_PUBLISHED` /
  `ELIGIBILITY_REQUIRED` / `INVALID_ELIGIBILITY_PROOF` / `ELIGIBILITY_ALREADY_USED` /
  `ELIGIBILITY_NOT_ENABLED` / `INTERNAL_ERROR`），
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
- `ballot-draft` 已删除：服务端没有任何接收明文选项的入口，也就没有「占位 proof」这回事。
- `ballot-encrypt` 在**客户端本地加密**选票：取公开的计票公钥 → 本地加密选项下标 →
  落盘选票文件。选票文件里的 `nullifier_secret` 只留本地，prover 需要它。
- `ballot-prove` 在**客户端本地出证明**：读选票文件 → 用本地 proving key 生成 Groth16
  证明 → 写回选票文件。证明不经过服务端，proving key 也不上服务端（服务端只拿
  verifying key）。缺材料（nonce / 选项下标 / 选项数 / 计票公钥）时报错退出。
- `credential-claim` 匿名领取凭证：走**真两段式盲签名**（预留 → 本地盲化 → 盲签名 →
  本地解盲 → attest），**不需要 admin token**（领取端点是公开的），服务端随机给出一个
  池槽位的凭证。服务端因此无法把凭证与投票人对应起来。槽位被抢走时客户端换一个槽位
  重来（最多 5 次）。
  - **带资格领取**：`-eligibility <成员包.json> -proving-key <资格 proving key>`。
    客户端先向服务端要一次领取挑战，再用成员包与资格 proving key 在本地出零知识证明，
    最后带证明去领。**缺 `-proving-key` 直接报错**，不静默降级成「不带资格领取」——
    那会让服务端把「没带资格」当成「没开资格校验」。
- `credential-issue` 管理端签发：同样走真两段式，只是走管理端端点、可以指定
  `user_id`。
- **份额文件的形状**：`decrypt-setup -out` 落盘的是**整个会话对象**
  （`{"proposal_id","public_key","shares":[...],"threshold","trustees"}`），
  `result-publish -shares-file` 必须能直接吃这个文件（也接受 JSON 数组与纯文本行）。
  两边对不上时，服务端会把整份 JSON 当成一个份额，回
  `400 INVALID_REQUEST: share does not belong to session`。
- 当前命令：
  - `health`
  - `version`
  - `proposals-open`
  - `proposal-create`
  - `credential-issue`
  - `credential-claim`
  - `credential-show`
  - `ballot-encrypt`
  - `ballot-prove`
  - `vote-submit`
  - `vote-status`
  - `credentials-list`
  - `decrypt-setup`
  - `result-publish`
  - `result-get`
  - `anchors-list`

### 7.5 离线工具：密钥生成与资格名单

- `cmd/keygen` 生成 Groth16 密钥，两套电路各一把：
  - `-circuit ballot`（默认）：`verifying.key`（0644，服务端 `crypto.vk_path`）/
    `proving.key`（0600，客户端）
  - `-circuit eligibility`：`eligibility-verifying.key`（服务端
    `crypto.eligibility_vk_path`）/ `eligibility-proving.key`（客户端）
  - 两套密钥**互不相通**（Groth16 的 setup 是 per-circuit 的）。
- `cmd/eligibility-list -members <名单文件> -out <目录>` —— 离线名单工具，
  不联网、不连服务端：
  - 输入：每行一个成员 ID（空行与 `#` 开头行忽略，重复行去重，成员按 ID 排序后建树）
  - 输出：`root.json`（给服务端配置）+ 每成员一份 `eligibility-<member_id>.json`
    （0600，成员本人保管）
  - **两份产物必须分开保管** —— 服务端只该拿到 root
  - **每次运行都会重新生成资格秘密**，因此树根每次都不同：名单与资格包必须**成套**
    保存，不能只留 root
  - **产物格式是接口，由 `cmd/eligibility-list` 的测试钉住**：工具落盘的 JSON
    （`root.json` 的 `root`/`depth`/`count`、成员包的 `secret`/`leaf`/`index`/`path`/
    `root`/`depth`）要被客户端与服务端读懂，任何字段编码或路径长度的偏差都会
    让「工具生成的名单」在真实投票时全线失败。测试覆盖**从文件读回**的完整往返：
    工具产物 → 客户端出证明 → 服务端用 root.json 验签通过。
- Makefile 目标：`make eligibility-list`（构建名单工具）、
  `make web-eligibility-key PK=<路径>`（把资格 proving key 放进门户静态目录）。

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
  - **资格密码学栈**：`internal/infrastructure/crypto/eligibility/eligibility_test.go`
    —— 名单树与路径、证明往返、篡改公开输入验证失败、约束数护栏（< 12000）；
    `internal/infrastructure/crypto/mimc/mimc_test.go` —— 共用 MiMC 的轮常量与哈希；
    `internal/infrastructure/crypto/eligibility_service_test.go` —— 领域适配层
    （根与验证密钥成对、挑战生成、验签）
  - **资格相关的 usecase 单元测试**：`internal/usecase/registration_eligibility_test.go`
    —— 挑战签发、未开启资格校验、带证明领取、资格使用记录、资格已被用过
  - **资格端点的请求校验分支测试**：`internal/infrastructure/http/eligibility_endpoints_test.go`
    —— 只给一半资格材料 → 400、非法 base64 → 400、未开启资格校验 → 409
    `ELIGIBILITY_NOT_ENABLED`、空请求体仍然合法（没开启资格校验时）
  - **资格使用记录的仓储测试**：`internal/infrastructure/persistence/eligibility_use_repository_test.go`
    —— 唯一索引 `(proposal_id, qualification_id)` 挡住第二次使用
  - **配置校验**：`internal/config/config_test.go` —— release 模式必须配资格根与
    资格验证密钥、两者必须成对出现
  - **资格证明的端到端测试**：`tests/e2e/eligibility_test.go` —— 路线图 T22 的验收 ①–⑤：
    没带资格证明被拒且**池槽位不被消耗**、有效资格证明能领到凭证、同一份资格第二次
    领取撞 409、跨提案复用被拒、领取请求体里**不许出现真人身份字段**
  - **浏览器端资格 prover 的端到端测试**：`tests/e2e/wasm_eligibility_test.go` ——
    node 驱动门户将要加载的那个 WASM 模块（`initEligibility` → `proveEligibility`），
    服务端用**真的资格验证密钥**验这份证明，并断言去重键与 Go 侧算出来的一致。
    缺 node 时跳过
  - **CLI 带资格领取的测试**：`cmd/cli/eligibility_claim_test.go` —— 带 `-eligibility`
    时先取挑战再出证明；缺 `-proving-key` 直接报错，不静默降级
  - **跨语言一致性测试**：`internal/infrastructure/crypto/ballot/kat_interop_test.go` 与
    `web/src/lib/ballot-cipher.test.ts` 共用一组已知答案向量 —— 浏览器端生成的密文
    必须能被 Go 侧解回同一个选项下标
  - **nullifier 派生的跨语言测试**：`internal/infrastructure/crypto/ballot/nullifier_interop_test.go`
    与 `web/src/lib/mimc.test.ts` / `web/src/lib/nullifier.test.ts` 共用一组向量
    （提案域元素、nullifier、MiMC 轮常量）；Go 侧另有测试用这组值真的出证明并验过
  - **盲签名 client 侧的跨语言测试**：`internal/infrastructure/crypto/blind_rsa_interop_test.go`
    与 `web/src/lib/blind-rsa.test.ts` 共用一组向量（固定密钥 + 固定盐 + 固定盲化因子）——
    浏览器端盲化出的消息与 CIRCL 逐字节一致，解盲出的签名也一样
  - **凭证消息的跨语言测试**：`internal/infrastructure/crypto/blind_rsa_interop_test.go`
    与 `web/src/lib/credential.test.ts` 共用同一组向量 —— `sha256(user_id ‖ 0x00 ‖
    user_pub_key)` 两边必须一致，否则服务端验签必然失败
  - **两段式盲签名的端到端测试**：`tests/e2e/blind_signature_test.go` —— 验收 ①
    （服务端在签名阶段只见盲化消息）与验收 ②（交叉攻击失败），外加「公开盲签名端点
    必须绑定池槽位」与「槽位被抢时客户端换槽位重来」
  - **客户端本地出证明的端到端测试**：`tests/e2e/client_prover_test.go` —— 服务端配
    **真的 verifying key**（不是开发模式的「proof 非空即通过」），证明由客户端用
    proving key 在本地生成，服务端全程没参与出证明也没见过明文选项。反向断言也
    补上了：改一个字节的证明必须 400 且不落库 —— 这条用例不会因为服务端没在验签
    而假通过。服务端在非 debug 模式下同样跑（服务端已没有 debug 专属能力），
    因此这张票只可能来自客户端。
  - **凭证端点的请求校验分支测试**：`internal/infrastructure/http/credential_endpoints_test.go`
    —— 公开端与管理端的盲签名、attest、备池、预留端点在「缺字段 / 非法 base64 /
    请求体不是 JSON」时一律 4xx（**不能 5xx**），管理端缺 admin token 是 401，
    且管理端可以为**池外**的 `user_id` 盲签（与公开端必须绑定池槽位的区别）
  - **客户端一站式出票的单元测试**：`internal/infrastructure/crypto/ballot/client_test.go`
    —— `ballot.ProveBallot` 生成的选票能被服务端侧自拼的公开输入验过，且计票能还原出
    客户端选的那个选项；非法选项下标 / 非法选项数 / 非法计票公钥一律报错
  - **WASM prover 的端到端测试**：`tests/e2e/wasm_prover_test.go` —— 用 node 加载
    **门户将要加载的那个 WASM 模块**（`GOOS=js GOARCH=wasm` 构建产物），走一遍
    「init（加载 proving key）→ prove → 提交 → 计票」。服务端配真的 verifying key：
    篡改一个字节的证明必须 400 且不落库，计票必须还原出客户端选的选项。
    缺 node 时跳过（CI 的 go job 装了 node，因此 CI 上会真跑）
  - `cmd/keygen` 单元测试
  - `tests/integration` 集成测试
  - `tests/e2e` 端到端流程测试（含「客户端本地加密 → 落库只有密文 → 计票还原」，
    以及「同一 nullifier 并发提交 → 恰好一张票入库」）
- PostgreSQL 集成测试使用 `testcontainers`
- 前端最小校验是：
  - `pnpm --dir web exec tsc --noEmit`
  - `pnpm --dir web run lint`
  - `pnpm --dir web run test`
- **门户侧的资格编排测试**：`web/src/composables/useVotePortal.test.ts` ——
  导入了资格包时「先取挑战 → 再把成员包与挑战交给 prover → 再把证明带进领取请求」，
  领取请求体里只有那三样资格材料；资格证明失败时**不发起领取**。

### 8.1 并发投票压测工具（`cmd/loadtest`）

只走 HTTP API 的压测工具 —— 不碰数据库，也不绕过服务端的任何校验。

**流程**（严格按服务端的时间约束排）：

1. 建一个**窗口稍后打开**的提案（解密会话必须在窗口打开之前初始化，这是服务端的硬性约束）；
2. 初始化门限解密会话 → 往凭证池投 `votes + duplicates` 个槽位 → 匿名领取凭证
   （**真两段式盲签名**：claim 预留 → 本地盲化 + 服务端盲签名 → 交回解盲后的签名由
   服务端验签落库，与门户、CLI 同一条链路）；
3. 取计票公钥，在本地加密出选票密文；
4. 等投票窗口打开，并发提交选票（每个请求一个**不同的凭证与不同的 nullifier**）；
5. 再用**同一个 nullifier**、不同凭证并发提交 `duplicates` 次 —— 唯一索引必须只放行一张；
6. 用 `GET /api/v1/proposals/:id/credentials` 核对「已投票凭证数」。

**输出**：吞吐（req/s）、延迟 p50/p90/p99/max、状态分布、防重投结论（放行了几张）、
落库核对。`-json` 输出机器可读报告。

> ⚠️ **计时有分辨率**：一轮如果比计时器还快（本机约 1ms），耗时与吞吐就测不准 ——
> 报告里 `timing_resolution_limited` 标记为 `true`，人类可读输出写「低于计时分辨率」。
> **不写「0 req/s」**：那会被误读成「没跑」。真实规模（几百个请求）不会触发。

**退出码**：

| 码 | 含义 |
|---|---|
| `0` | 防重投守住（同 nullifier 的并发请求恰好放行 1 张）**且**落库凭证数与放行数一致 |
| `1` | 防重投失守，或落库数与放行数对不上 |
| `2` | 用法错误（缺管理令牌、`votes`/`concurrency` 非法） |

> 结论能当门禁用 —— 这正是它以退出码而不是只打印报告的原因。

**已知边界**：

- **只能在 debug 模式跑。** **压测工具自己没有 prover**（门户与 CLI 都有），release 模式下
  所有投票请求都会因验签失败被拒
  （工具发的是 `ZGV2LXByb29m`，即 `"dev-proof"` 的 base64）。release 模式下报告的状态分布会
  如实反映这一点（全 400），那不是服务端的真实容量。
- **工具不支持资格证明**：它不会去取领取挑战、也不会出资格证明，因此在开启了
  资格校验的服务端上，领取环节会被 400 `ELIGIBILITY_REQUIRED` 拒绝。
- **压测数字反映的是存储层，不是投票逻辑。** SQLite 的单写者模型会把并发写串行化；按路线图
  §2.2，真正的**容量规划**（阈值、限流、PostgreSQL 基准）属 v0.3.0，这个工具提供的是
  **可复现脚本 + 一份实测输出**。

工具自身有单元测试（`cmd/loadtest/main_test.go`）：用 `httptest` 模拟最小 API 面，覆盖
「正常压测 → 防重投守住」「防重投失守 → 非 0 退出」「令牌不对」「参数非法」与报告计算。
**假服务端按 v0.2.0 的三段式盲签名协议实现**（claim 只预留、blind-sign 签盲化消息、
attest 验签落库），并断言**领取请求体里不许出现公钥** —— 压测工具哪天退回 v0.1.0 的
「一步式领凭证」，这条用例就会红。

### 8.2 门户面板的渲染测试

门户的 7 个 `.vue` 面板此前**既没有类型检查也没有任何测试** —— `tsc` 不认 `.vue`
文件，模板里的笔误（比如引用一个不存在的变量）只会在运行时炸。

现在用 Vue 的**服务端渲染**把面板渲染成 HTML 字符串：模板、setup、computed 都会
真的跑一遍。测试在 `web/src/components/panels/panels.test.ts`，助手在
`web/src/lib/render-panel.ts`。

- **这不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。抓的是
  「模板引用了不存在的变量」「setup 抛错」「条件分支写反」这一类问题。
- 为了能在 vitest 里跑 `.vue`，`web/vitest.config.ts` 里自写了一个 SFC 编译
  插件：离线环境装不了 `@vitejs/plugin-vue`，但 `vue` 自带 `vue/compiler-sfc`，
  `typescript` 又是已有的 devDependency。
- 覆盖了「服务端不得拿到明文选项」这条底线：面板**不渲染**任何「让服务端代生成凭据」的入口。

### 8.3 PostgreSQL 集成测试（本地 compose）

`tests/integration` 的 PostgreSQL 用例有**两条路径**，共用同一份测试代码：

| 路径 | 触发条件 | 数据库来源 |
|---|---|---|
| testcontainers | 没设 `ANONVOTE_TEST_PG_HOST` | 每次测试拉一个临时 `postgres:16-alpine` 容器 |
| 外部库 | 设了 `ANONVOTE_TEST_PG_HOST` | 连已有的 PostgreSQL |

**本地跑法**（仓库根有 `docker-compose.yml`）：

```bash
make pg-test        # 起库 → 跑集成测试 → 关库
```

`make pg-test` 依次做三件事：`docker compose up -d --wait postgres` → 带
`ANONVOTE_TEST_PG_*` 环境变量跑 `go test ./tests/integration/ -run Postgres` →
`docker compose down -v`。测试失败也会关库（退出码照样带出来）。

**每个测试一个独立 schema**（`itest_<纳秒时间戳>`）：外部库是共享的，同一个库上
反复跑测试不会撞上一次留下的表；跑完 `DROP SCHEMA ... CASCADE` 清理。

**环境变量**（默认值与 `docker-compose.yml` 一致）：

| 变量 | 默认值 |
|---|---|
| `ANONVOTE_TEST_PG_HOST` | 空（空 = 走 testcontainers） |
| `ANONVOTE_TEST_PG_PORT` | `5432` |
| `ANONVOTE_TEST_PG_USER` | `test` |
| `ANONVOTE_TEST_PG_PASSWORD` | `test` |
| `ANONVOTE_TEST_PG_DATABASE` | `anonvote` |

> 端口不是数字时按「没有外部库」处理 —— 宁可不跑，也不要连到错的地址。


## 9. 当前已知限制

> 上线前的逐项核对见 [`deployment-checklist.md`](./deployment-checklist.md) ——
> 那里把配置、密钥、迁移、TLS、门户部署与投票运营流程拆成了可勾的条目。

- **证明链已通，CLI 与浏览器都能在本地出证明。** 服务端能真实验签：真实
  Groth16 电路 + `cmd/keygen` + 加载 `crypto.vk_path` 后的 Groth16 验证都落地了；
  **CLI 侧**能自己出证明（`cmd/cli` 的 `ballot-prove`，用本地 proving key），端到端
  证据见 `tests/e2e/client_prover_test.go`；**浏览器侧**也能（`cmd/ballot-wasm` 编译成
  WASM，门户在 Worker 里跑，见 §6.10），端到端证据见 `tests/e2e/wasm_prover_test.go`。
  前提：门户静态目录里备好 `wasm_exec.js` / `ballot-prover.wasm` / `proving.key`
  三个文件，否则门户会报「下载 WASM 模块失败」；要用「一人一票」还需
  `eligibility-proving.key`。
- **明文选项对服务端不可知：CLI 与门户两侧都已闭合。** 密文、nullifier 与证明全部在
  客户端本地生成，服务端只提供公开的计票公钥（`GET .../election-key`），落库记录里
  只有密文。**服务端没有任何接收明文选项的入口** —— `ballot-draft`（服务端代加密）已删除。
  - **这条不变量由持续门禁把守**（不再是人工 grep）：`tests/architecture/plaintext_option_test.go`
    用 go/ast 扫 `internal/`，断言 HTTP 层请求体结构体没有明文选项 json tag、usecase 层没有
    接受明文选项的入口、服务端代加密那条链（`EncryptVoteOption` / `PrepareBallotDraft` /
    `BallotDraft`）不许复活、`ballot-draft` 端点不许复活。**不设豁免机制**。
- **凭证签发已经是真两段式盲签名**（见 §6.9）：客户端本地盲化，服务端在签名阶段只见
  盲化消息，attest 阶段才拿到公钥并验签。能力 F 在**签名阶段**达成。
  - 口径要说清楚：**attest 阶段服务端仍然看得到 `user_id` 与 `user_pub_key`** —— 验签
    必须用它们重算凭证消息。这是「签发方不知道被签内容」，不是「服务端全程不知道
    客户端是谁」。
  - 预留不占位 → 并发领取会抢同一个槽位，重试放在客户端（最多 5 次，只在 409
    `CREDENTIAL_EXISTS` 时重试）。
- **nullifier 的 MiMC 绑定：两侧都已用上。** 浏览器端（`web/src/lib/nullifier.ts` 的
  `deriveNullifier`）已能与 Go 侧逐位一致地算出电路认可的 nullifier，KAT 也把它钉死了
  （见 §6.4）；**出票时 nullifier 由 `ballot.ProveBallot` 在本地派生**（随机秘密 +
  MiMC），秘密不出本地。门户表单里的 nullifier 字段仍可手工编辑（低频工具）。
- 服务端的 `BallotNullifierGenerator` 仍在（供测试与工具用），但服务端不参与出票 ——
  正式出票的 nullifier 由 `ballot.ProveBallot` 在本地派生，不需要电路认可的 nullifier。
- **「一人一票」已落地，但它的边界要说清楚**（见 §6.11）：
  - **服务端强制的是「同一个资格只能领一次票」**，不是「同一个真人只能领一次票」。
    资格由权威名单分配，名单本身的真实性由运营方负责。
  - **「一人一票」只在公开领取端点上强制**：管理端签发（`POST /api/v1/credentials`）
    不校验资格证明，运营方可以为指定 `user_id` 签发凭证。
  - **资格包丢了就领不了了，没有找回路径**：资格秘密只存在于
    `eligibility-<member_id>.json` 里，服务端不存秘密、也不存叶子。补发只能由
    名单工具重新生成（那会换一套树根，等于换了一份名单）。
  - **名单上限 1024 人**：`eligibility.TreeDepth = 10` 是编译期常量，改它要重编
    电路与 WASM、重生成两套密钥，并让所有成员换新的资格包。
  - **服务端在未开启资格校验时不强制一人一票**（`GET /healthz` 的 `eligibility`
    字段会报出来）；release 模式强制开启，开发模式默认关闭。
  - **门户的浏览器端资格路径已在真浏览器里人工验过一遍**（2026-10-25，Chromium / Electron：
    导入资格文件 → 领取 201 → 重复领取 409 → 本地生成选票 → 提交 201 → 计票还原出所选
    选项）。仓库里**没有** Playwright，自动化只到 node 驱动 WASM
    （`tests/e2e/wasm_eligibility_test.go`）—— **每次上线仍要人工点一遍**。
  - **`cmd/loadtest` 不支持资格证明**（见 §8.1）：它不会取挑战、也不会出资格证明。
  - **资格表不许与凭证、票据关联**：`eligibility_uses` 只存去重键。一旦能把资格
    连到票，「分发不泄露身份」这条底线就破了。
- **服务端没有「替用户选票」的联调接口**：`ballot-draft` 已删除，不要把它写成生产能力。
- **压测工具只能在 debug 模式跑，且数字不是容量规划。** `cmd/loadtest` 提供的是
  「可复现脚本 + 实测输出」（见 §8.1）；SQLite 的单写者会把并发写串行化。真正的容量
  规划属 v0.3.0。
- **v3 密文与 v0.1.0 的 v2 密文不兼容**，旧的加密选票解不开（见 §6.1）。
- 解密会话的初始化时机是当前系统最容易被文档写错的点：
  - 服务端禁止在投票开放中 setup
  - 实际业务上应在开始前准备好 session
- `GET /api/v1/proposals/open` 只返回“状态 open 且当前处于时间窗口内”的提案：
  - 尚未开始的提案不会出现在大厅列表里
  - 管理端需要看到未开始/已关闭提案时，改用 `GET /api/v1/proposals`（全部提案，按创建时间倒序）
- 前端仍保留对历史 JSON 描述字段的兼容解析，仅用于读取旧数据；新写路径已统一为纯文本 `description` + 独立 `options`。
