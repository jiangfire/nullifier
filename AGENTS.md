# AGENTS.md

基于 Clean Architecture 的匿名投票系统：Go 后端（Gin + GORM + Wire）+ Vue 3 门户前端 + 复用 HTTP API 的 CLI。

## Setup commands

推荐用仓库根目录的 `Makefile` 作为统一入口（Windows 下在 Git for Windows 的 bash 里跑 `mingw32-make <target>`，Linux/macOS 直接 `make <target>`；`make help` 列出全部目标）：

- `make deps` / `make build` / `make run` / `make test` / `make vet` / `make fmt-check` / `make cover`
- `make check` — 提交前快速门禁（vet + gofmt + 前端 lint + tsc）
- `make ci` — 复现 CI 流水线（`go test ./...` + 覆盖率门槛）
- `make web-install` / `web-dev` / `web-build` / `web-test` / `web-lint` / `web-typecheck` / `web-format`

等价的原始命令：

- 安装依赖：`go mod download`；前端 `pnpm --dir web install`
- 启动服务：`go run ./cmd/server`（默认 SQLite 文件库 `anonvote.db`）
- 构建：`go build ./...`；前端 `pnpm --dir web run build`
- 测试：`go test ./...`（**Windows 需 `CGO_ENABLED=1`**，`gorm.io/driver/sqlite` 依赖 cgo；Makefile 在 Windows 上已自动导出该变量）
- 静态检查：`go vet ./...` 与 `gofmt -l ./cmd ./internal ./tests`（应无输出）
- 覆盖率门槛：`./scripts/check-coverage.ps1`（PowerShell）/ `bash ./scripts/check-coverage.sh`（Bash），默认阈值 80%
- 前端：`pnpm --dir web run dev`（rsbuild，默认 :3000）、`run build`、`run test`（vitest）、`exec tsc --noEmit`
- CLI 工作流：`go run ./cmd/cli help` 查看全部 13 个命令

## Project layout

- `cmd/server` — HTTP 服务入口；`cmd/cli` — 命令行客户端（复用 HTTP API，不绕过服务端校验）
- `internal/domain` — 实体、仓储接口、密码学接口、领域错误（不依赖 HTTP/GORM/具体密码学库）
- `internal/usecase` — 业务编排（Proposal / Register / Voting / Participation / Tally），`usecase/mocks` 为 gomock 生成物
- `internal/infrastructure` — `http`（Gin 路由/中间件/错误映射）、`persistence`（GORM 仓储与自动迁移）、`crypto`（gnark / kyber / kryptology）
- `internal/di` — Wire 依赖注入；`internal/config` — viper 配置加载与运行期校验
- `tests/` — `unit` 同级 `_test.go`、`integration`（testcontainers + PG）、`e2e`
- `web/` — Vue 3 + TypeScript + Rsbuild 单页门户（`src/App.vue` 只做外壳与引导编排，页面拆在 `src/components/panels/`，设计令牌在 `src/styles/`，后端错误文案映射在 `src/lib/error-messages.ts`）
- `configs/` — `config.yaml`（开发）、`config.production.yaml`、`production.env.example`
- `docs/` — `techspec.md`（当前实现口径，改接口/流程优先更新）
- `scripts/` — 覆盖率门槛脚本；`.github/workflows/ci.yml` — CI 定义

## Code style

- Go 走 `gofmt` 默认风格；包名小写短名，导出符号 `PascalCase`，测试函数 `TestXxx`
- **服务端运行日志统一用注入的 `*zap.Logger`**，禁止在服务路径引入 `fmt.Println` / `log.Printf`；CLI 的纯文本输出属于命令结果，不受此限
- 前端：组件 `PascalCase.vue`，组合式函数 `useXxx.ts`，工具模块小写语义名（`lib/datetime.ts`）
- Prettier：`singleQuote: true`（`web/.prettierrc`）；ESLint 扁平配置 `web/eslint.config.mjs`
- 仓库有 `.gitattributes`（`* text=auto eol=lf`）：**提交前保持 LF**，不要引入 CRLF

## Testing instructions

- 后端用 Go 原生测试 + `testify`；集成测试集中在 `tests/integration`，HTTP 与持久化测试贴近实现文件
- 前端用 vitest，测试文件与源码同目录（如 `web/src/lib/datetime.test.ts`）
- 改动涉及接口、密码学或持久化逻辑时**必须补回归测试**；合入前至少 `go test ./...` 且覆盖率脚本通过（当前 81.8%，余量 1.8 个百分点，新增代码务必补测）
- CI 实际执行：`go test ./...` + `bash ./scripts/check-coverage.sh 80 coverage.ci.out`

## PR & commit conventions

- 从 `main` 切分支，不要直接推送默认分支
- Conventional Commits：`feat:` / `fix:` / `chore:` / `docs:` / `refactor:`，简短祈使句（例：`fix: tighten credential issuance validation`）
- PR 需说明变更范围、测试结果、配置影响；改 API 附示例请求，改 `web/` 附截图
- 若变更 `configs/`、鉴权或密钥路径，明确列出部署调整项

## Security

- 绝不提交真实密钥、令牌或生产数据库；`.env` 已在 `.gitignore`
- 生产模式（`server.mode=release/prod/production`）必须显式设置 `crypto.vk_path`（或 `CRYPTO_VK_PATH`），否则服务启动失败
- 管理面接口（凭证签发、创建提案、凭证状态、解密 setup、发布结果）需 `Authorization: Bearer <admin token>`；未配置时服务启动会生成仅当前进程有效的临时 token 并打印日志
- 票据表只存匿名投票数据；`vote_participations` 仅存 `hash(user_id)`，用于「知道你是否投过票」但不关联票内容
