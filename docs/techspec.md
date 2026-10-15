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

### 4.4 自动迁移

- 服务启动时自动执行 `persistence.AutoMigrate`
- 当前会迁移这些实体：
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

## 5. HTTP API

### 5.1 公共接口

- `GET /healthz`
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
  - 落票与参与记录要么在同一事务里完成，要么都不做；不存在“记了票但没记参与状态”的路径
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

- 后端负责验证 proof，不负责替用户端生成真实 proof。
- `ballot-draft` 只用于开发联调，返回的是便于串流程的草稿数据。
- 真实生产部署仍应由客户端自行生成密文和证明。

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

### 7.2 CLI

- CLI 直接复用 HTTP API，不绕过服务端校验。
- 当前命令：
  - `health`
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

## 9. 当前已知限制

- `ballot-draft` 仍是开发联调接口，不应被写成生产能力。
- 解密会话的初始化时机是当前系统最容易被文档写错的点：
  - 服务端禁止在投票开放中 setup
  - 实际业务上应在开始前准备好 session
- `GET /api/v1/proposals/open` 只返回“状态 open 且当前处于时间窗口内”的提案：
  - 尚未开始的提案不会出现在大厅列表里，管理端需要用创建返回的 `proposal_id` 继续操作
  - 目前没有“列出全部提案”的接口
- 前端仍保留对历史 JSON 描述字段的兼容解析，仅用于读取旧数据；新写路径已统一为纯文本 `description` + 独立 `options`。
