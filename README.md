# nullifier

基于 Clean Architecture 的匿名投票后端实现（Go + Gin + GORM）。

## 项目状态

- 最新交接状态见：`docs/status.md`
- 截止 2026-03-08，后端主流程已可验收，覆盖率 gate（80%）已启用并通过。

## 快速开始

```bash
go mod tidy
go test ./...
go run ./cmd/server
```

默认使用 SQLite 文件数据库 `anonvote.db`，可通过环境变量切到 PostgreSQL：

```bash
DB_DRIVER=postgres
DB_HOST=localhost
DB_PORT=5432
DB_NAME=anonvote
DB_USER=postgres
DB_PASSWORD=postgres
DB_SSLMODE=disable
CRYPTO_ISSUER_KEY_PATH=issuer_keys.json
CRYPTO_ISSUER_ROTATE_DAYS=90
```

也可使用 `configs/config.yaml` 覆盖默认配置。
其中 `crypto.issuer_key_path` 和 `crypto.issuer_rotate_days` 用于凭证签发密钥持久化与自动轮换。

## API（v1）

- `POST /api/v1/credentials`：签发匿名凭证（RFC9474 RSA blind signature）
- `POST /api/v1/proposals`：创建提案
- `GET /api/v1/proposals/open`：查询开放提案
- `POST /api/v1/votes`：提交匿名投票（含 `user_id + user_pub_key + credential_signature`，服务端校验防冒充）
- `GET /api/v1/proposals/:proposal_id/vote-status?user_id=...`：查询该用户是否已投票
- `POST /api/v1/proposals/:proposal_id/decryption/setup`：初始化阈值解密会话（threshold/trustees）
- `POST /api/v1/proposals/:proposal_id/results/publish`：提交份额并发布计票结果
- `GET /api/v1/proposals/:proposal_id/results`：查询已发布计票结果
- `GET /api/v1/proposals/:proposal_id/audit-anchors`：导出审计锚点（含 Merkle Root）
- `GET /healthz`：健康检查

说明：
- 票据表仅存匿名投票数据（不含用户标识）。
- “已投票状态”单独存储为用户标识哈希，可用于“知道你是否投过票”，但不直接关联具体票内容。

## 测试

- 单元/集成/E2E：`go test ./...`
- 生成 gomock：`go generate ./internal/usecase/mocks`
- 覆盖率门槛（默认检查 `usecase + config + di + http`，总覆盖率需 >=80%）：
  - PowerShell：`./scripts/check-coverage.ps1`
  - Bash：`./scripts/check-coverage.sh`
- PostgreSQL 集成（testcontainers）在 Docker 不可用时会自动跳过

## 目录

```text
cmd/server               # 启动入口
internal/domain          # 实体与接口
internal/usecase         # 业务用例
internal/infrastructure  # DB/HTTP/crypto 实现
internal/di              # Wire 依赖注入
tests                    # 测试与测试工具
```
