# 项目状态（交接）

更新时间：2026-03-08 22:37:13 +08:00  
当前分支：`main`

## 总体结论

- 后端主流程已可验收，包含匿名凭证签发、投票、去重、防冒充、参与状态查询、阈值解密计票、结果发布、审计锚点导出。
- `techspec` 中先前标记的 6 个主要缺口已补齐并有测试覆盖。

## Techspec 对照状态

1. 盲签名：已实现 RFC9474 RSA blind signature（`circl/blindrsa`）。
2. 阈值解密：已实现私钥拆分与恢复，支持多份额解密发布结果。
3. Merkle + 审计锚点：已实现根构建、锚点持久化与查询导出接口。
4. mockgen/gomock：已落 `go:generate` 自动生成 mock，并有 gomock 用例。
5. 覆盖率门槛：已落强制脚本，默认检查 `usecase + config + di + http`，门槛 80%。
6. tally 闭环：已实现 `setup -> publish -> result -> audit-anchors` 全链路。

## 当前验证结果

- 全量测试：`go test ./...` 通过。
- 覆盖率门槛：`./scripts/check-coverage.ps1` 通过。
  - 总覆盖率：`81.2%`（门槛 `80%`）。
  - 分包：`usecase 81.6%`、`config 100%`、`di 92.6%`、`http 72.7%`、`middleware 95%`。

## 本次额外补充（收工前）

- 新增配置层测试：`internal/config/config_test.go`
- 新增 DI 层测试：`internal/di/providers_test.go`
- 新增 HTTP 层测试：`internal/infrastructure/http/server_test.go`
- 新增中间件测试：`internal/infrastructure/http/middleware/error_handler_test.go`
- 覆盖率脚本默认检查范围已扩展到：
  - `./internal/usecase`
  - `./internal/config`
  - `./internal/di`
  - `./internal/infrastructure/http`
  - `./internal/infrastructure/http/middleware`

## 关键接口（后端）

- `POST /api/v1/credentials`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/open`
- `POST /api/v1/votes`
- `GET /api/v1/proposals/:proposal_id/vote-status?user_id=...`
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`

## 下次开工建议

- 若要继续拉高总体覆盖率，优先补 `internal/infrastructure/http/handler.go` 的失败分支细粒度用例（目前该包覆盖率最低）。
- 若要对外发布，建议补充 API 文档示例请求/响应（含错误码表）和 CI 中的覆盖率 gate 步骤。
