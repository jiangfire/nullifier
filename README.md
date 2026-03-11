# nullifier

基于 Clean Architecture 的匿名投票后端实现（Go + Gin + GORM）。

## 项目状态

- 最新交接状态见：`docs/status.md`
- 截止 2026-03-10，后端主流程、前端运营面板、CLI 工作流已打通，覆盖率 gate（80%）已启用并通过。

## 快速开始

```bash
go mod tidy
go test ./...
go run ./cmd/server
```

也可以直接使用命令行客户端操作流程：

```bash
go run ./cmd/cli health
go run ./cmd/cli -admin-token dev-admin-token proposal-create -title "Board" -options "A,B" -start 2026-03-10T10:00:00Z -end 2026-03-10T12:00:00Z
go run ./cmd/cli credential-issue -proposal p-1 -user u-1001 -out cred-u1001.json
go run ./cmd/cli ballot-draft -proposal p-1 -option A -out ballot.json
go run ./cmd/cli vote-submit -credential-file cred-u1001.json -ballot-file ballot.json
go run ./cmd/cli -admin-token dev-admin-token credentials-list -proposal p-1
go run ./cmd/cli -admin-token dev-admin-token decrypt-setup -proposal p-1 -threshold 2 -trustees 3 -out shares.json
go run ./cmd/cli -admin-token dev-admin-token result-publish -proposal p-1 -shares-file shares.json
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
CRYPTO_VK_PATH=./keys/verifying.key
```

也可使用 `configs/config.yaml` 覆盖默认配置。
其中 `crypto.issuer_key_path` 和 `crypto.issuer_rotate_days` 用于凭证签发密钥持久化与自动轮换。
当 `server.mode` 为 `release/prod/production` 时，必须设置 `crypto.vk_path`（或环境变量 `CRYPTO_VK_PATH`），否则服务会启动失败。
管理面接口使用 `server.admin_bearer_token` 鉴权；在 `release/prod/production` 下该值不能为空，否则服务会启动失败。
生产部署可参考：
- `configs/config.production.yaml`（`server.mode=release`）
- `configs/production.env.example`（显式设置 `CRYPTO_VK_PATH` 等环境变量）

## API（v1）

- `POST /api/v1/credentials`：管理员签发匿名凭证（RFC9474 RSA blind signature）
- `POST /api/v1/proposals`：创建提案
- `GET /api/v1/proposals/open`：查询开放提案
- `POST /api/v1/votes`：提交匿名投票（含 `user_id + user_pub_key + credential_signature`，服务端校验防冒充）
- `POST /api/v1/proposals/:proposal_id/vote-status`：持凭证材料查询该用户是否已投票
- `GET /api/v1/proposals/:proposal_id/credentials`：管理员查询该提案下所有凭证及 `has_voted`
- `POST /api/v1/proposals/:proposal_id/ballot-draft`：按选项生成投票草稿（开发联调）
- `POST /api/v1/proposals/:proposal_id/decryption/setup`：初始化阈值解密会话（threshold/trustees）
- `POST /api/v1/proposals/:proposal_id/results/publish`：提交份额并发布计票结果
- `GET /api/v1/proposals/:proposal_id/results`：查询已发布计票结果
- `GET /api/v1/proposals/:proposal_id/audit-anchors`：导出审计锚点（含 Merkle Root）
- `GET /healthz`：健康检查

说明：
- 票据表仅存匿名投票数据（不含用户标识）。
- “已投票状态”单独存储为用户标识哈希，可用于“知道你是否投过票”，但不直接关联具体票内容。
- 匿名凭证按 `proposal_id` 绑定，管理员可看到某提案下哪些凭证已投票，但看不到凭证对应的投票内容。
- 提案创建时必须显式提供 `options`，后端会把它作为正式计票口径保存并在草稿生成/计票时校验。
- 管理面接口（凭证签发、创建提案、凭证状态、解密 setup、发布结果）都需要 `Authorization: Bearer <admin token>`。
- 开发模式内置 CORS（支持 `http://localhost:3000` 本地前端跨域访问）。
- `ballot-draft` 用于开发联调；生产模式应使用真实钱包/客户端生成 proof 与密文。

## 前端门户（web）

- 目录：`web/`
- 定位：面向用户与运营的业务页面，不是 API 调试台
- 已支持：
  - 匿名凭证文件导入/导出
  - 创建多选项提案
  - 参与投票时按提案选项选择并一键生成投票草稿
  - 管理员凭证状态面板（只看 `has_voted`，不看投票内容）
  - 顶部 `Admin Token` 输入，直连已鉴权的管理接口
- 详细说明见：`web/README.md`

## 测试

- 单元/集成/E2E：`go test ./...`
- CLI 帮助：`go run ./cmd/cli help`
- 前端类型检查：`pnpm --dir web exec tsc --noEmit`
- 生成 gomock：`go generate ./internal/usecase/mocks`
- 覆盖率门槛（默认检查 `usecase + config + di + http`，总覆盖率需 >=80%）：
  - PowerShell：`./scripts/check-coverage.ps1`
  - Bash：`./scripts/check-coverage.sh`
- PostgreSQL 集成（testcontainers）在 Docker 不可用时会自动跳过

## 日志规范

- 服务端运行期日志统一使用 `zap`，包括启动日志、HTTP 请求日志、错误日志和 panic recovery。
- 不要在服务端代码路径继续引入 `log.Printf`、`fmt.Println` 这类运行期日志输出；需要记录运行状态时统一走注入的 `*zap.Logger`。
- Gin 默认的 debug/route/listen 输出已关闭，避免和业务日志混杂。
- CLI 保留纯文本或 JSON 结果输出，这些属于命令结果，不属于服务端运行日志；不要把 CLI 用户输出和服务端日志规则混为一谈。

## 目录

```text
cmd/server               # 启动入口
internal/domain          # 实体与接口
internal/usecase         # 业务用例
internal/infrastructure  # DB/HTTP/crypto 实现
internal/di              # Wire 依赖注入
tests                    # 测试与测试工具
```
