# 项目状态（交接）

更新时间：2026-03-09 22:39:46 +08:00  
当前分支：`feat/concurrency-hardening-and-prod-config`

## 总体结论

- 后端主流程可用：凭证签发、投票、防重投、防冒充、参与状态查询、阈值解密计票、结果发布、审计锚点。
- 前端已从“接口调试台”重构为“用户门户”，支持业务流程操作。
- 本地跨域访问已打通（服务端 CORS 中间件已启用）。

## 本轮关键新增

1. 前端用户化能力
   - 匿名凭证支持导入/导出 JSON 文件
   - 创建提案支持“多选项”输入
   - 参与投票支持按提案选项选择
2. 选项投票闭环补齐
   - 新增后端接口：`POST /api/v1/proposals/:proposal_id/ballot-draft`
   - 前端可一键按选项生成投票草稿并提交
3. 本地开发联调优化
   - 开发模式 proof 验证放宽（便于前端联调）
   - CORS 中间件支持浏览器 `OPTIONS` 预检

## 当前接口清单（v1）

- `POST /api/v1/credentials`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/open`
- `POST /api/v1/votes`
- `GET /api/v1/proposals/:proposal_id/vote-status?user_id=...`
- `POST /api/v1/proposals/:proposal_id/ballot-draft`
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`
- `GET /healthz`

## 已执行验证

- 后端
  - `go test ./internal/infrastructure/http/... ./internal/usecase/...` 通过
  - `go test ./internal/infrastructure/crypto -run GnarkProofVerifier -count=1` 通过
- 前端
  - `pnpm --dir web run lint` 通过
  - `pnpm --dir web exec tsc --noEmit` 通过
  - `pnpm --dir web run build` 通过

## 备注

- `ballot-draft` 为开发联调接口，生产建议使用真实钱包/客户端生成 proof 与密文。
- 工作区存在未跟踪文件 `anonvote.db`，如不需要建议加入 `.gitignore` 或删除。
