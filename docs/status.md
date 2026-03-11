# 项目状态（交接）

更新时间：2026-03-11 21:57:00 +08:00  
当前分支：`main`

## 总体结论

- 后端主流程可用：提案创建、按提案签发匿名凭证、匿名投票、防重投、防冒充、参与状态查询、阈值解密计票、结果发布、审计锚点。
- 前端同时覆盖用户端与运营端流程；CLI 已补齐主要管理与投票命令，可脱离网页完成联调。
- 之前 review 指出的三个安全问题已修复：管理面已鉴权、解密份额不再明文持久化/整包泄露、投票开放期间禁止 setup。
- 服务端运行期日志已统一到 `zap`，Gin 默认 debug 输出已静音。

## 本轮关键变更

1. 安全收敛
   - 新增管理面 Bearer Token 鉴权中间件
   - `release/prod/production` 模式强制要求 `server.admin_bearer_token`
   - 解密 session 仅持久化 share hash，不再落库明文份额
   - `decryption/setup` 恢复为仅允许在提案关闭后执行
   - 凭证签发改为管理员接口，投票状态查询改为凭证校验式接口
2. 提案内凭证台账
   - `Credential` 绑定 `proposal_id`
   - 签发凭证必须显式指定提案
   - 投票校验改为按 `proposal_id + user_id` 查凭证
   - 新增管理员接口：列出某提案下所有凭证及 `has_voted`
   - 提案 `options` 升级为正式字段，草稿生成和计票都会校验候选项合法性
3. 前端运营补齐
   - 顶部支持输入 `Admin Token`
   - 新增“凭证状态”面板
   - 运营端可查询提案内凭证投票状态，但不展示投票内容
4. CLI 第一版
   - 新增 `cmd/cli`
   - 复用 HTTP API，覆盖健康检查、提案创建、凭证签发、草稿生成、投票提交、状态查询、凭证台账、解密 setup、结果发布、结果查询、审计锚点查询

## 当前接口清单（v1）

- `POST /api/v1/credentials`
- `POST /api/v1/proposals`
- `GET /api/v1/proposals/open`
- `POST /api/v1/votes`
- `POST /api/v1/proposals/:proposal_id/vote-status`
- `GET /api/v1/proposals/:proposal_id/credentials`
- `POST /api/v1/proposals/:proposal_id/ballot-draft`
- `POST /api/v1/proposals/:proposal_id/decryption/setup`
- `POST /api/v1/proposals/:proposal_id/results/publish`
- `GET /api/v1/proposals/:proposal_id/results`
- `GET /api/v1/proposals/:proposal_id/audit-anchors`
- `GET /healthz`

## 当前前端/CLI 覆盖

- 前端用户端：浏览开放提案、领取或导入凭证、按选项生成 ballot draft、提交投票、查询个人是否已投票
- 前端运营端：创建提案、初始化解密、发布结果、查看结果、查看审计锚点、查看提案内凭证状态
- CLI：
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

## 已执行验证

- `go test ./...` 通过
- `pnpm --dir web exec tsc --noEmit` 通过
- `go run ./cmd/server` 启动后仅输出 `zap` 风格服务日志

## 仍需注意

- `ballot-draft` 仍是开发联调接口，生产应由真实钱包/客户端生成 proof 与密文。
- CLI 目前是标准库版，适合联调和脚本化；若后续追求更顺手的终端交互，可继续补交互式向导、表格输出和批量命令。
- 服务端日志规范已固定为 `zap`；后续若新增运行期日志，默认要求复用注入的 `*zap.Logger`，不要重新引入 `log.Printf`。
