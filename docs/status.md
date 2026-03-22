# 项目状态（交接）

更新时间：2026-03-18 21:05:00 +08:00  
当前分支：`main`

## 总体结论

- 当前主流程是可用的，问题主要不在“程序没做完”，而在于部分文档和少量 CLI 残留没有跟上实现。
- 本次已把 CLI 提案创建的旧描述编码残留移除，避免继续写入历史兼容格式的脏数据。
- `docs/techspec.md`、根目录 `README.md`、`web/README.md` 已统一到当前实现口径。

## 当前可用能力

- 后端主流程可用：
  - 创建提案
  - 按提案签发匿名凭证
  - 初始化门限解密会话
  - 匿名投票
  - 防重放（`nullifier`）
  - 防冒充（`proposal_id + user_id + user_pub_key + credential_signature` 校验）
  - 个人投票状态查询
  - 提案内凭证状态查询
  - 阈值解密计票
  - 结果发布
  - 审计锚点导出
- 前端门户覆盖用户端和运营端流程。
- CLI 已覆盖健康检查、提案创建、凭证签发、投票、状态查询、计票和审计相关命令。

## 本次确认并修正的偏差

1. 文档超前/漂移
   - 旧 `docs/techspec.md` 仍混杂大量“目标态/示意代码”，与当前目录结构、接口和约束不一致。
   - `docs/status.md` 之前把 `decryption/setup` 写成“仅允许在提案关闭后执行”，这与当前实现不符。
   - `README.md` 和 `web/README.md` 中有个别示例和流程顺序没有跟上当前接口约束。
2. 程序残留
   - `cmd/cli` 的 `proposal-create` 之前会把 `description` 包成旧 JSON 结构再发给后端。
   - 该行为已修复，CLI 现在直接发送纯文本 `description` 和独立的 `options` 字段。
3. 启动体验
   - `server.admin_bearer_token` 现在支持启动时自动生成。
   - 若配置缺失，服务会生成当前进程有效的临时 token，并打印到控制台日志。

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

## 关键实现约束

- 管理面接口都要求 `Authorization: Bearer <admin token>`：
  - `POST /api/v1/credentials`
  - `POST /api/v1/proposals`
  - `GET /api/v1/proposals/:proposal_id/credentials`
  - `POST /api/v1/proposals/:proposal_id/decryption/setup`
  - `POST /api/v1/proposals/:proposal_id/results/publish`
- 若未显式配置 `server.admin_bearer_token`，服务会在启动时自动生成一个临时 token。
- 提案创建必须显式提供 `options`，并作为正式计票口径落库。
- 投票前必须先有对应提案的解密会话，否则投票会被拒绝。
- `decryption/setup` 当前实现会拒绝“投票开放中”的提案。
  - 实际运营上应在 `starts_at` 之前完成初始化。
  - 结果发布仍只允许在提案关闭后执行。
- `ballot-draft` 仅用于开发联调；生产闭环应由真实客户端/钱包生成 proof 与密文。
- 投票状态查询不接受裸 `user_id` 枚举，必须提交当前凭证材料完成校验式查询。

## 当前前端/CLI 覆盖

- 前端用户端：
  - 浏览开放提案
  - 选择提案后领取或导入凭证
  - 按选项生成 ballot draft
  - 提交投票
  - 查询个人是否已投票
- 前端运营端：
  - 创建提案
  - 初始化解密会话
  - 发布结果
  - 查询结果
  - 查看审计锚点
  - 查看提案内凭证状态
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
- 新增 CLI 回归测试，确认 `proposal-create` 不再发送旧式 JSON 描述

## 仍需注意

- 兼容层仍保留了前端对历史脏数据描述字段的解析能力，用于读取旧提案数据；新写入路径已修正。
- 若后续要把系统推进到更接近生产环境，优先补的是“客户端侧真实 proof/密文生成链路”，不是后端 CRUD。
- `docs/status.md` 应继续作为交接入口；后续改接口或流程时，优先先改这里，再改 README。
