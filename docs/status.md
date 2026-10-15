# 项目状态（交接）

更新时间：2026-10-15 20:56:00 +08:00  
当前分支：`main`

## 总体结论

- 当前主流程是可用的，问题主要不在“程序没做完”，而在于少数边界分支会以 500 或 panic 收场，以及个别文档和 UI 字段没有跟上实现。
- 本轮做了一次整体复查（功能缺口 + 缺陷），修掉了下面列出的问题，并把新行为写回文档。
- `docs/techspec.md`、根目录 `README.md`、`web/README.md` 已统一到当前实现口径。

## 本轮修复的问题

1. 严重：`KyberElGamal` 的 keystream 生成在明文超过 32 字节时数组越界 panic
   - `xorKeystream` 把整段剩余数据一次性拷贝后又按 32 字节 block 取下标，明文/密文一超过一个 block 就 panic。
   - 影响面：`ballot-draft`（选项文本超过 32 字节即 500）和 `results/publish`（真实长度的选票密文一律解密失败）。
   - 已改为按 block 推进计数器，并补了 1/31/32/33/64/65/200/4096 字节的往返测试。
2. 高：`entity.Proposal` 有两个字段共用 `json:"options"`，`go vet` 直接失败，序列化会出现重复 key
   - `OptionsJSON` 改为 `json:"-"`，只保留 `Options` 对外暴露，并补了回归测试。
3. 高：同一提案重复 `decryption/setup` 或 `results/publish` 返回 500
   - 原因是 `decryption_sessions.proposal_id`、`tally_results.proposal_id` 有唯一索引，而仓储用新 UUID 再 `Save` 一次。
   - 现在在生成密钥/计票之前先检查，返回 `409 DECRYPTION_SESSION_EXISTS` / `409 RESULT_ALREADY_PUBLISHED`，并保证不会覆盖已有会话或重复追加审计锚点。
   - 选择“拒绝并明确报错”而不是“覆盖会话”：`setup` 只拒绝“投票开放中”，提案关闭后仍可调用，覆盖会直接毁掉已投选票的解密能力。
4. 中：前端凭证没有和提案绑定
   - `wallet` 现在记录 `proposal_id`，导出使用凭证自身的提案，导入会带上提案并同步当前选中提案。
   - 凭证与所选提案不一致时，提交/状态查询会直接拦下并给出原因，不再让用户提交后收到 401。
5. 中：前端把数字字段当字符串读，`issuer_key_version` 永远显示为空
   - 服务端返回的是数字，`textOf` 只接受字符串；现在会同时接受数字。
   - 受影响的还有从页面导出的凭证文件（`issuer_key_version` 之前会写成空字符串）。
6. 中：`SubmitVoteRequest` 里 `user_id` 可为空的分支已删除
   - 原来 `user_id` 为空就跳过凭证校验、跳过防重投记录、直接落票；HTTP 层虽然强制必填，但这个分支留在 usecase 里是实打实的隐患。
   - 现在 `user_id`、`user_pub_key`、`credential_signature` 在 usecase 层也是必填，`credentialRepo` / `participationRepo` 变为必需依赖，"不记录参与状态就落票"的路径整体删除。
   - 顺带修正了 4 个原本依赖该分支的用例，并修掉 2 个"因为缺 `user_id` 而碰巧通过"的假绿用例。
7. 低：`results/publish` 提交重复 share 会绕过 threshold 计数，并最终以 500 收场
   - `ensureSharesBelongToSession` 现在会拒绝重复 share，返回 `400 INVALID_REQUEST`。
8. 低：覆盖率门槛在 `main` 上本来就是红的（77.7% < 80%），CI 的 coverage gate 实际处于失败状态
   - 补齐了零覆盖的核心路径：`ParticipationUseCase.HasVotedWithCredential`、`GET /proposals/:id/credentials`、`Recovery`。
   - 现在总覆盖率为 81.4%，门槛重新变绿（余量不大，见"仍需注意"）。

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

## 上一轮（2026-03-18）确认并修正的偏差

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
- `POST /api/v1/votes` 的 `user_id`、`user_pub_key`、`credential_signature` 均为必填（HTTP 层强制）。
- `decryption/setup` 当前实现会拒绝“投票开放中”的提案。
  - 实际运营上应在 `starts_at` 之前完成初始化。
  - 同一提案重复 setup 返回 `409 DECRYPTION_SESSION_EXISTS`，不会覆盖已有会话。
  - 结果发布仍只允许在提案关闭后执行。
  - 同一提案重复发布返回 `409 RESULT_ALREADY_PUBLISHED`，不会重复写审计锚点。
- `ballot-draft` 仅用于开发联调；生产闭环应由真实客户端/钱包生成 proof 与密文。
- 投票状态查询不接受裸 `user_id` 枚举，必须提交当前凭证材料完成校验式查询。
- 凭证按提案绑定：前端 `wallet` 会记录凭证所属 `proposal_id`，与所选提案不一致时会直接拦截提交。

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

- 本轮（2026-10-15）：
  - `go vet ./...` 通过（基线是失败的）
  - `go test ./...` 通过（Windows 下需 `CGO_ENABLED=1`，`gorm.io/driver/sqlite` 依赖 cgo）
  - `./scripts/check-coverage.ps1` 通过：81.4% >= 80%（基线 77.7% 失败）
  - `pnpm --dir web exec tsc --noEmit` 通过
  - `pnpm --dir web exec prettier --check src/App.vue src/composables/useVotePortal.ts` 通过
  - `rsbuild build` 通过
  - `pnpm --dir web run lint` **失败**：`App.vue` 里 `workflowHeadline` 是历史遗留的未使用变量，属于本轮刻意未动的无关代码，见"仍需注意"
  - `vitest` 未能在本次环境运行（esbuild 子进程 `spawn EPERM`，属于沙箱限制）；受影响的测试只覆盖 `lib/datetime`、`lib/navigation`，本轮未改动这两个模块
  - 新增回归测试：keystream 长明文、Proposal JSON 单 `options`、重复 setup/publish 冲突、重复 share、投票凭证材料必填、管理令牌边界
- 上一轮（2026-03-18）：
  - `go test ./...` 通过
  - `pnpm --dir web exec tsc --noEmit` 通过
  - 新增 CLI 回归测试，确认 `proposal-create` 不再发送旧式 JSON 描述

## 仍需注意

- 兼容层仍保留了前端对历史脏数据描述字段的解析能力，用于读取旧提案数据；新写入路径已修正。
- 若后续要把系统推进到更接近生产环境，优先补的是“客户端侧真实 proof/密文生成链路”，不是后端 CRUD。
- `docs/status.md` 应继续作为交接入口；后续改接口或流程时，优先先改这里，再改 README。
- 本轮复查发现但**未修**的问题（都不是阻断项，按需要再排期）：
  - `ParticipationUseCase.ListCredentialStatuses` 是 N+1 查询：每个凭证查一次 `HasVoted`，凭证量大时管理面板会明显变慢。
  - `GET /api/v1/proposals/open` 只返回窗口内的提案，没有“列出全部提案”的接口，管理端只能靠手工填/留存 `proposal_id`。
  - `web/src/components/layout/AppShell.vue` 没有任何引用，是死代码。
  - `web/src/App.vue` 的 `workflowHeadline` 是未使用变量，`pnpm run lint` 因此长期失败；本轮按“无关死代码只提不删”的约定没有动它。
  - 工作区没有 `.gitattributes`，而全局 `core.autocrlf=true`：部分文件是 CRLF 检出，导致 `gofmt -l` 与 `prettier --check` 在 Windows 上大面积误报。注意仓库里的 blob 本身是 LF，所以这纯粹是检出产物，修正方式是补 `.gitattributes` 并把工作区换成 LF，不会产生内容 diff。
  - 覆盖率余量只有 1.4 个百分点（81.4% / 80%），后续新增代码要注意补测，否则门槛会再次变红。
