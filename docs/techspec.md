# 技术说明（当前实现）

本文档描述的是仓库当前已经落地的实现，不再保留目标态伪代码。

## 1. 代码结构

```text
cmd/
├── cli/                    # 命令行客户端，复用 HTTP API
└── server/                 # 服务启动入口

internal/
├── config/                 # 配置加载与校验
├── di/                     # Wire/provider 组装
├── domain/
│   ├── crypto/             # 密码学接口
│   ├── entity/             # 领域实体
│   └── repository/         # 仓储接口与领域错误
├── infrastructure/
│   ├── crypto/             # gnark / kyber / kryptology 实现
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
  - `crypto`：gnark proof verifier、kyber ElGamal/threshold、kryptology credential issuer、Merkle root builder。
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
  - `nullifier`
  - `cipher`
- `vote_participations` 单独记录是否投过票：
  - 使用 `hash(user_id)` 记录参与状态
  - 不直接关联投票内容
- `VotingUseCase` 会同时检查：
  - `nullifier` 是否重复
  - 该用户在该提案下是否已投票
  - 提交的凭证材料是否与提案内凭证匹配

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
  - `configs/config.yaml`
  - 环境变量
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
- `POST /api/v1/proposals/:proposal_id/ballot-draft`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`

### 5.2 管理接口

以下接口统一走 Bearer Token 鉴权中间件：

- `POST /api/v1/credentials`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/:proposal_id/credentials`
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`

### 5.3 关键约束

- `POST /api/v1/votes`
  - 要求 `proposal_id`、`encrypted_vote`、`proof`、`nullifier`、`user_id`、`user_pub_key`、`credential_signature`，全部必填（HTTP 层与 usecase 层都强制，不存在匿名免凭证投票的路径）
  - 当前服务端会校验 proof、密文格式、凭证归属、防重投
  - **proof 校验失败返回 400 `INVALID_REQUEST`**（客户端输入问题，不归为服务端故障）
  - 落票与参与记录要么在同一事务里完成，要么都不做；不存在“记了票但没记参与状态”的路径
  - 请求体里的 `selected_option` 服务端**不消费**，也不应发送（明文选项上服务端会抵消
    “选票是加密的”这个属性）；门户与 CLI 均已移除该字段
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

- proof 验证：`gnark`
- 凭证签发：`kryptology`
- 选票加解密与门限恢复：`kyber` 封装
- 审计根：Merkle root builder

当前实现边界如下：

- **尚无真实的选票零知识电路。** 仓库里唯一的 gnark 电路是测试文件中的玩具电路
  （`tests/e2e/api_test.go` 的 `Y = X³ + X + 5`），生产配置的 `crypto.vk_path`
  指向的 `verifying.key` **无法由本仓库生成**。
- `crypto.vk_path` 为空时，proof 验证器接受任何非空 proof（`requireVerifyMode=false`）。
  载入 vk 后 `public_witness` 变为必填，而现有客户端都拿不到它。
- 服务端不校验 proof 与 `proposal_id` / `nullifier` / 密文的绑定：`PublicInputs()`
  构造了 `proposal_id` 与 `nullifier`，但验证器只读取 `public_witness`。
- 后端负责验证 proof，不负责替用户端生成真实 proof。
- `ballot-draft` 只用于开发联调，返回的是便于串流程的草稿数据。**密文由服务端生成，
  服务端在生成时就知道选了哪个** —— 这是当前版本最大的匿名性缺口。
- 真实生产部署仍应由客户端自行生成密文和证明。

> 补齐上述链路的顺序与工作量评估见 `docs/release-plan-v0.1.0.md` 的 v1.0.0 里程碑。

## 7. 前端与 CLI

### 7.1 前端门户

- 单页 Vue 3 应用
- 包含两个工作区：
  - 用户端
  - 运营端
- 用户端能力：
  - 查看开放提案
  - 领取或导入凭证
  - 按选项生成投票草稿
  - 提交投票
  - 查询是否已投票
- 运营端能力：
  - 创建提案
  - 初始化解密会话
  - 发布结果
  - 查询结果
  - 查看凭证状态
  - 查看审计锚点

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
- 当前命令：
  - `health`
  - `version`
  - `proposals-open`
  - `proposal-create`
  - `credential-issue`
  - `credential-show`
  - `ballot-draft`
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
  - `tests/integration` 集成测试
  - `tests/e2e` 端到端流程测试
- PostgreSQL 集成测试使用 `testcontainers`
- 前端最小校验是：
  - `pnpm --dir web exec tsc --noEmit`
  - `pnpm --dir web run lint`
  - `pnpm --dir web run test`

## 9. 当前已知限制

- `ballot-draft` 仍是开发联调接口，不应被写成生产能力。
- 解密会话的初始化时机是当前系统最容易被文档写错的点：
  - 服务端禁止在投票开放中 setup
  - 实际业务上应在开始前准备好 session
- `GET /api/v1/proposals/open` 只返回“状态 open 且当前处于时间窗口内”的提案：
  - 尚未开始的提案不会出现在大厅列表里
  - 管理端需要看到未开始/已关闭提案时，改用 `GET /api/v1/proposals`（全部提案，按创建时间倒序）
- 前端仍保留对历史 JSON 描述字段的兼容解析，仅用于读取旧数据；新写路径已统一为纯文本 `description` + 独立 `options`。
