# Repository Guidelines

## Project Structure & Module Organization

后端主代码位于 `cmd/` 与 `internal/`：`cmd/server` 是 HTTP 服务入口，`cmd/cli` 提供管理与联调用命令；`internal/domain` 放实体与仓储接口，`internal/usecase` 放业务编排，`internal/infrastructure` 放 HTTP、持久化与密码学实现，`internal/di` 维护 Wire 注入。测试位于 `tests/` 与各模块同级 `_test.go` 文件；前端位于 `web/src`，静态资源在 `web/public`。配置样例放在 `configs/`，设计与交接文档在 `docs/`。

## Build, Test, and Development Commands

- `go test ./...`：运行后端单元、集成与 E2E 测试。
- `go run ./cmd/server`：启动服务，默认使用根目录 `anonvote.db`。
- `go run ./cmd/cli help`：查看 CLI 工作流与参数。
- `./scripts/check-coverage.ps1`：检查重点包覆盖率，阈值默认 `80%`。
- `pnpm --dir web run dev`：启动前端开发服务器。
- `pnpm --dir web run build`：构建前端产物。
- `pnpm --dir web run test`：运行前端 Vitest。
- `pnpm --dir web exec tsc --noEmit`：执行前端类型检查。

## Coding Style & Naming Conventions

Go 代码保持 `gofmt` 默认风格，包名使用小写短名，导出符号使用 `PascalCase`，测试函数命名为 `TestXxx`。服务端运行日志统一使用注入的 `*zap.Logger`，不要在服务路径继续使用 `fmt.Println` 或 `log.Printf`。前端使用 TypeScript + Vue 3：组件文件采用 `PascalCase.vue`，组合式函数采用 `useXxx.ts`，工具模块使用小写语义名。提交前运行 `pnpm --dir web run lint` 与 `pnpm --dir web run format`。

## Testing Guidelines

后端以 Go 原生测试为主，断言库使用 `testify`；集成测试集中在 `tests/integration`，HTTP 与持久化测试靠近实现文件。前端测试使用 `vitest`，测试文件与源码同目录放置，如 `web/src/lib/datetime.test.ts`。合入前至少执行 `go test ./...`，并确保覆盖率脚本通过；涉及接口、密码学或持久化逻辑时，补充回归测试。

## Commit & Pull Request Guidelines

最近提交以简短祈使句和 Conventional Commits 为主，优先使用 `feat:`、`fix:`、`chore:` 开头，例如 `fix: tighten credential issuance validation`。每个 PR 应说明变更范围、测试结果与配置影响；修改 API 时附示例请求，修改 `web/` 时附截图；若变更 `configs/`、鉴权或密钥路径，请明确列出部署调整项。

## Security & Configuration Tips

不要提交真实密钥、令牌或生产数据库。开发环境优先使用 `configs/config.yaml` 或 `configs/production.env.example` 派生本地配置；生产模式必须显式设置 `CRYPTO_VK_PATH` 和管理员令牌。涉及 PostgreSQL 的测试依赖 Docker，不可用时相关用例可能自动跳过。
