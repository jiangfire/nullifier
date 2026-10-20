# Changelog

本文件记录 nullifier 的版本变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

版本号的唯一真相源是仓库根的 `VERSION` 文件；打 tag 时 release workflow 会校验两者一致。

---

## [0.1.0] — 2026-10-20

首个版本。**内测版**：把「选票内容与投票人身份分离」的存储与接口骨架跑通，
但**零知识证明链尚未实现**，不要用于真实投票。边界详见 README 的「当前版本边界」。

### 修复

- 证明校验失败改为返回 400 `INVALID_REQUEST`，之前误返 500 并被记进生产错误日志。
- 门户凭据绑定目标选项：改选项或换提案会作废旧凭据，提交前也会校验一致性 ——
  修掉「界面显示 B、实际记下 A」的静默错票。
- CLI 与门户都不再发送 `selected_option`：服务端不消费该字段，而明文选项上服务端
  会抵消「选票是加密的」这个属性。
- CLI 的 URL 路径不再被二次转义，含空格或 `/` 的 proposal id 不再必然 404。
- CLI 的 `decrypt-setup` 强制要求 `-out`，门限解密份额不再默认打到 stdout。
- 门户的管理令牌改存 `sessionStorage`，不再明文留在 `localStorage`。
- 门户切换提案时重置「是否已投票」，错误提示不再 4.2 秒自动消失。
- PowerShell 版覆盖率脚本此前把 `$Profile` 当字面量文件名写出，现在正确插值。

### 变更

- `release` 模式下 `server.admin_bearer_token` 为空将直接启动失败，不再使用每次重启
  都会变的临时 token。
- 启动日志打印实际生效的配置（`server.mode` / `db.driver` / `db.database` /
  admin token 来源 / 是否开启证明验证），并且不再把配置态的长期 admin token 写进日志。
- `crypto.vk_path` 为空时启动明确告警「证明校验已关闭」。
- 覆盖率门槛脚本不再覆盖 `GOMODCACHE` / `GOCACHE`，CI 的依赖缓存不再失效。
- `cmd/cli` 新增 `version` 命令，共 14 个命令。

### 文档

- README 补「当前版本边界」告示，匿名性描述与实现对齐（原描述与实际行为相反）。
- 覆盖率徽章改为全仓口径（60.1%），正文区分门槛口径与全仓口径。
- 新增 `docs/project-map.md`（分层上下文索引）、`docs/release-plan-v0.1.0.md`（发版计划）、
  `docs/release-review-2026-10-20.md`（发版前整体 review）。

### 测试

- 新增跑通真实 gnark 验证的 HTTP 层测试：有效 proof → 201、缺 `public_witness` → 400、
  被篡改的 `public_witness` → 400。之前所有 HTTP 测试用的都是空操作验证器。
- 新增门户 composable 测试（9 个用例）与投票凭据绑定规则单测（11 个用例）。

### CI 与发布

- CI 补上 `gofmt` 检查，并新增前端作业（lint / tsc / vitest）。
- 新增 tag `v*` 触发的发布流水线：校验 tag 与 `VERSION` 一致，在各平台 runner 上原生
  构建（保留 cgo，否则默认的 sqlite 驱动无法启动），对产物做冒烟启动测试，打包并
  生成 sha256 校验和，最后创建 GitHub Release。

### 已知限制

- 没有真实的选票零知识电路，`verifying.key` 无法由本仓库生成。
- 密文由服务端生成，服务端在生成凭据时就知道选了哪个。
- 盲签名的盲化在服务端内部完成，不具备真正的盲化性质。
- 门户 7 个 `.vue` 面板仍无自动化测试。

[0.1.0]: https://github.com/jiangfire/nullifier/releases/tag/v0.1.0
