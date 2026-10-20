# nullifier v1.0.0 实施计划（生产可用匿名投票）

> 起点：[`docs/release-plan-v0.1.0.md`](./release-plan-v0.1.0.md) 已全量落地（v0.1.0 内测版，分支 `release/v0.1.0-prep`）。
> 证据来源：[`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md) 的 6 CRITICAL / 29 REQUIRED / 20 NIT。
> 本文档列出 v1.0.0 需要做的项，每条都带 `file:line` 证据、具体改法与可验证的验收标准。
> **已落地的项保留在原文里**（问题描述与改法不改），实际实现与原始设想的出入写在各条目的
> 「落地备注」；整体状态见 §2 开头的落地状态表。

---

## 0. 一句话策略

**v0.1.0 是「骨架能跑」的版本，v1.0.0 是「匿名性成立」的版本。**
两者的差距不在业务代码，而在**一条完整的零知识证明链**：电路 → 密钥 → 客户端 prover → proof 绑定 → 客户端加密 → 真盲签名。
这条链是周级工作量，**不能和小时级的加固项捆在一起做** —— 捆在一起的结果是 v1.0.0 永远发不出去。

所以本计划分三层，**每层都能独立交付、独立验证**：

| 层 | 内容 | 依赖 | 预估 |
|---|---|---|---|
| **Phase 1** 加固层 | 与证明链无关的 12 项（模式开关、常数时间比较、自动迁移、连接池、KDF、覆盖率扩包…） | 无 | 天级 |
| **Phase 2** 证明链 | C1/C2/C3/C5根治/R2/R3 六项 | Phase 1 的 T1/T2 | 周级 |
| **Phase 3** 生产化 | 部署配置、端到端验收、发版 | Phase 2 | 天级 |

**Phase 1 的 T3–T12 已落地**（见 §2 状态表）；**Phase 2 需要先拍板 G6–G9**；Phase 3 需要真实环境。

---

## 1. 范围与非目标

**做**（v1.0.0 的定义）：服务端在**任何情况下都无法得知投票人选了哪个选项**，且选票可验证、可审计、可复算。

**不做**（明确排除，避免范围蔓延）：

- 端到端加密之外的传输层优化、限流、压测容量规划 —— 属运维，不属本计划
- 多租户、权限分级、投票人身份体系 —— 当前是「一个 admin 发凭证」的模型
- 移动端原生客户端 —— 只做浏览器 + CLI
- 密码学方案的替换（换 KEM、换曲线）—— 只做 KDF 修正（R16），不动主体方案

---

## 2. Phase 1 · 加固层（与证明链无关，可立即开工）

每项独立可交付、独立可 revert。

> **落地状态（2026-10-20）**：Phase 1 已拍板范围内的 **T3–T12 全部落地**（分支
> `release/v0.1.0-prep`）。下表保留原始的问题描述与改法，作为决策依据；
> 实际实现与原始设想有出入的地方在各条目的「落地备注」里写明。
>
> | 任务 | 状态 | 提交 |
> |---|---|---|
> | T3 admin token 常数时间比较 | ✅ | `4ed262d` |
> | T4 admin 鉴权补错误 token / 畸形头测试 | ✅ | `4ed262d` |
> | T5 生产模式关闭启动自动迁移 + 独立迁移入口 | ✅ | `0f0c0e9` |
> | T6 PostgreSQL 连接池配置 | ✅ | `1dcb4b9` |
> | T7 KDF 换 HKDF（协议变更） | ✅ | `a036a9b` |
> | T8 GuideOverlay 焦点陷阱 + Esc 关闭 | ✅ | `1eda25f` |
> | T9 凭证池 + 投票人匿名随机领取 | ✅ | `17937da` |
> | T10 覆盖率门槛扩包 + `cmd/server` 冒烟测试 | ✅ | 见下 |
> | T11 门户「查询投票状态」按钮 | ✅ | `2356638` |
> | T12 文档如实描述盲签名的真实性质 | ✅ | `a94b51e` |
>
> **仍未落地**：T1（`ballot-draft` 非 debug 404）、T2（门户隐藏按钮）—— 等 G2/G3 拍板；
> T13–T21 属 Phase 2/3。

#### T1 · `ballot-draft` 在非 debug 模式关闭（R8 / 决策门 G3）

| 项 | 内容 |
|---|---|
| 问题 | `POST /api/v1/proposals/:id/ballot-draft` 是**公开路由**（`internal/infrastructure/http/handler.go:65`），返回写死的假 proof `ZGV2LXByb29m`（`handler.go:314`）。任何人都能调用，且这条链路的密文由服务端生成。 |
| 证据 | `handler.go:65`（路由未挂在 admin 分组下）、`handler.go:314`（常量 proof） |
| 改法 | ① `config.Config` 增加 `DebugMode bool`（由 `server.mode` 推导：`debug`/`dev`/`development` 为 true）；② `handler.go:65` 改为在非 debug 模式返回 **404**（用 404 而不是 403，避免暴露端点存在性）；③ `server_test.go` 里现有的 `ballot-draft` 用例改为按模式分叉，debug 模式断言 201、release 模式断言 404 |
| 验收 | `go test ./...` 全绿；新增用例断言 release 模式 404；`grep -rn "ballot-draft" internal/` 确认只有一处路由定义 |
| 预估 | 1 小时 |
| 依赖 | 需先拍板 G3 |

#### T2 · 门户「按选项生成凭据」按钮在非 debug 模式隐藏（G2）

| 项 | 内容 |
|---|---|
| 问题 | 门户在运营侧提供「按选项生成凭据」的入口（`web/src/composables/useVotePortal.ts:977` `issueCredential`），这等于把「服务端替用户选」的能力做成一键操作。 |
| 证据 | `useVotePortal.ts:977-979`、`web/src/components/panels/IdentityPanel.vue:13` |
| 改法 | ① 服务端在 `/api/v1/healthz` 或一个新的 `/api/v1/meta` 暴露 `{ "debug_mode": bool }`；② 门户启动时拉取，非 debug 模式隐藏「按选项生成凭据」按钮并在界面上说明「凭据由运营方签发并线下分发」；③ 保留「导入凭据文件」路径 |
| 验收 | vitest 新增用例：debug=false 时按钮不存在；debug=true 时存在 |
| 预估 | 半天（依赖 T1 的 `DebugMode` 字段） |
| 依赖 | T1、需先拍板 G2 |

#### T3 · admin token 改常数时间比较（R13）

| 项 | 内容 |
|---|---|
| 问题 | admin bearer token 用普通字符串 `!=` 比较（`internal/infrastructure/http/middleware/admin_auth.go:27`），理论上可被计时侧信道逐字节爆破。 |
| 证据 | `middleware/admin_auth.go:27` |
| 改法 | 改用 `crypto/subtle.ConstantTimeCompare([]byte(provided), []byte(expected))`；长度不等时先比长度（`ConstantTimeCompare` 本身对长度不等返回 0，但要保证不因长度短路泄露 —— 用 `sha256.Sum256` 先摘要再比，天然等长） |
| 验收 | 现有 `middleware/admin_auth_test.go` 全绿；新增用例：长度不同、相同前缀不同后缀的 token 均返回 401 |
| 预估 | 30 分钟 |
| 依赖 | 无 |

#### T4 · admin 鉴权补「错误 token」与「畸形 Authorization 头」测试（R14）

| 项 | 内容 |
|---|---|
| 问题 | `middleware/admin_auth_test.go` 只有 `missing_token` 和 `valid_token` 两个子用例，**没有覆盖错误 token**和**畸形头**（`Basic xxx`、`Bearer`、`Bearer ` 带空格、多个 `Bearer` 段、非 ASCII）。 |
| 证据 | `middleware/admin_auth_test.go:15` / `:29`（仅两个 `t.Run`） |
| 改法 | 补表驱动用例：错误 token、无 `Bearer ` 前缀、`Bearer` 后无值、大小写不敏感前缀、多余空格、含非 ASCII 字符。全部断言 401 且响应体不泄露 token 内容 |
| 验收 | `go test ./internal/infrastructure/http/middleware/...` 全绿，用例数 ≥ 8 |
| 预估 | 1 小时 |
| 依赖 | 无 |

#### T5 · 生产模式关闭启动自动迁移（R12）

| 项 | 内容 |
|---|---|
| 问题 | `internal/di/providers.go:34` 无条件调用 `persistence.AutoMigrate(db)` —— 启动即改表结构。生产环境多个副本同时启动会争抢 DDL 锁，且没有独立的迁移步骤可回滚。 |
| 证据 | `internal/di/providers.go:34`、`internal/infrastructure/persistence/migration.go:9` |
| 改法 | ① `config.Config` 增加 `AutoMigrate bool`（默认 `true`，release 模式为 `false`）；② `di/providers.go` 按该开关决定是否调用；③ 新增 `cmd/migrate` 子命令（或 `server -migrate` 参数）作为**独立的迁移步骤**；④ 文档写明发布顺序：先跑迁移 → 再起服务 |
| 验收 | release 模式启动后 `SELECT` 表结构未变（新增用例断言）；`cmd/migrate` 能在空库上建表 |
| 预估 | 半天 |
| 依赖 | 无 |

#### T6 · PostgreSQL 连接池配置（R11）

| 项 | 内容 |
|---|---|
| 问题 | `internal/infrastructure/persistence/db.go:40` 的 `newPostgresDB` 只 `gorm.Open`，没有设置 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`，用 GORM 默认值（无限制）。高并发下会打爆 PG 连接数。 |
| 证据 | `internal/infrastructure/persistence/db.go:40-53` |
| 改法 | ① `DBConfig` 增加 `MaxOpenConns` / `MaxIdleConns` / `ConnMaxLifetime`；② 配置项 `db.max_open_conns` 等，带合理默认值（如 25 / 5 / 1h）；③ 启动日志打印实际生效值 |
| 验收 | 新增用例：设置非零值后 `sql.DB.Stats().MaxOpenConnections` 等于配置值 |
| 预估 | 2 小时 |
| 依赖 | 无 |

#### T7 · KDF 换 HKDF（R16）

| 项 | 内容 |
|---|---|
| 问题 | ElGamal+Kyber 的共享秘密用裸 `sha256.Sum256` 派生（`internal/infrastructure/crypto/kyber_elgamal.go:259-260`，`"enc:"` / `"auth:"` 前缀）。裸哈希不是合格的 KDF：无盐、无 info 绑定、无迭代。 |
| 证据 | `internal/infrastructure/crypto/kyber_elgamal.go:259-260` |
| 改法 | 改用 `golang.org/x/crypto/hkdf`，`salt` 与 `info` 纳入协议：salt 用随机 32 字节（或 proposal id 派生），info 绑定 `"nullifier/kyber-elgamal/v1"` + 用途标签 + 双方公钥摘要。**这是一次协议变更，需与已有密文不兼容 → v1.0.0 之后旧密文不可解**，须在 CHANGELOG 标注 |
| 验收 | 往返测试仍通过；新增用例：salt 不同则派生密钥不同；info 不同则派生密钥不同；跨版本密文解不开（显式断言失败） |
| 预估 | 1 天（含协议文档） |
| 依赖 | 无，但**会破坏与 v0.1.0 密文的兼容性** |

#### T8 · 引导浮层补焦点陷阱与 Esc 关闭（R20）

| 项 | 内容 |
|---|---|
| 问题 | `web/src/components/GuideOverlay.vue:13` 只有 `@click.self` 关闭，没有焦点管理：打开时焦点不进入浮层、Tab 会跑到背景内容、Esc 不关闭。对键盘与读屏用户不可用。 |
| 证据 | `web/src/components/GuideOverlay.vue:13` |
| 改法 | ① 打开时聚焦浮层内第一个可聚焦元素并记录 `previouslyFocused`；② Tab / Shift+Tab 在浮层内循环（focus trap）；③ Esc 关闭并把焦点还给触发元素；④ 浮层加 `role="dialog"` + `aria-modal="true"` + `aria-labelledby` |
| 验收 | vitest 新增用例：Esc 触发 `dismiss`；打开后焦点在浮层内；关闭后焦点回到原元素 |
| 预估 | 半天 |
| 依赖 | 无 |

#### T9 · 凭证池 + 投票人匿名随机领取（R21 / 决策门 G8）

> **G8 已拍板（2026-10-20）**：不用一次性领票码，改为**投票人匿名随机领取**。

| 项 | 内容 |
|---|---|
| 问题 | 凭证签发接口挂在 admin 分组下（`internal/infrastructure/http/server.go:47-48`），**投票人自己也必须持有 admin token 才能领凭证**。更致命的是：即便换成 admin 批量分发，**admin 手里有「谁领了哪个凭证」的台账** —— 而选票最终会绑定到凭证的 `user_id` / `user_pub_key`（`vote_participations` 存 `hash(user_id)`），于是 admin 可以把「这张票」关联到「这个人」。**这比 admin token 泄露更危险：它直接破坏选票与身份的不可关联性**，而这正是整个系统存在的理由。 |
| 证据 | `internal/infrastructure/http/server.go:47-48`、`internal/usecase/voting.go:60-64`（公开输入含 `nullifier`，由 `user_id` 派生）、`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181`（凭证绑定 `user_id` + `user_pub_key`） |
| 改法 | ① **凭证池**：admin 批量预生成 N 个凭证，**服务端不记录「谁对应哪个凭证」的台账**（凭证内容本身不含真人信息）；② **匿名领取**：新增**公开**端点 `POST /api/v1/credentials/claim`，请求体**不含任何身份信息**，服务端随机分配一个未领取的凭证并标记为已领取（一次性）；③ 客户端在领取时生成**全新的一次性 keypair** 并连同 pubkey 提交，服务端用 issuer key 对 `(领取随机 id, pubkey)` 签名；④ 领取端点**不写 IP、不写 User-Agent、不落任何可关联日志**，只记一条匿名计数；⑤ 投票时用该 keypair 签 nullifier，防重投靠 `nullifier` 唯一索引 |
| **必须同时接受的代价** | **随机领取 = 服务端无法强制「一人一票」。** 任何人可以反复领取并重复投票（每次领取换一个身份）。「一人一票」要靠**资格证明**（零知识证明「我符合资格」而不透露身份）来保证 —— 这属于 v1.0.0 证明链的**第二条电路**（与选票电路分离），见 §3 Phase 2 之后的备注。**在资格证明落地前，这个系统防的是「重复投票」，不是「一人多票」。** 文档必须如实写清。 |
| **残余风险** | ① **流量分析**：领取请求的 IP / 时间仍可能被用于弱关联 —— 因此要求服务端不记录 IP，并考虑前置代理；② **issuer key 在服务端**，服务端能签任意凭证，即服务端能自己造票 —— 属于信任假设，须写进文档与威胁模型；③ 凭证池耗尽后新投票人无票可领 —— 需要「按需补充池」的管理操作，且补充时机本身可能泄露信息（**建议在投票窗口开始前一次性备足**） |
| 验收 | ① 端到端：匿名领取 → 投票，**全程不需要 admin token**，且服务端**没有**任何「凭证→领取者」的记录；② 领取端点返回的凭证不含任何真人标识字段；③ 回归测试：领取端点**不写** `user_id` / IP 到任何表与日志；④ 池耗尽后返回 4xx（明确错误信息）；⑤ 同一凭证二次领取返回 4xx |
| 预估 | 2–3 天 |
| 依赖 | 无，但**这是新增功能**（新表 + 新端点 + 客户端改造） |

**与旧路径的关系**：admin 逐个签发（`POST /api/v1/credentials`）保留，但**只用于「一人一票」由资格证明兜底的场景**；在资格证明落地前，文档应引导使用匿名领取。

#### T10 · 覆盖率门槛扩到 crypto / persistence / cmd + `cmd/server` 冒烟测试（R9 / C6）

| 项 | 内容 |
|---|---|
| 问题 | 门槛脚本只跑 5 个包（`scripts/check-coverage.sh:9-15`）。`internal/infrastructure/crypto`、`internal/infrastructure/persistence`、`cmd/cli`（24.6%）、`cmd/server`（**0%**）**不在门槛内** —— 这些包的覆盖率回退不会让 CI 变红。`cmd/server` 0% 意味着启动流程完全没有测试。 |
| 证据 | `scripts/check-coverage.sh:9-15`、`cmd/server/` 无 `_test.go` |
| 改法 | ① 先给 `cmd/server` 补冒烟测试：起进程 → `/healthz` 200 → 关停（可直接用 `exec.Command` 起二进制）；② 给 `cmd/cli` 补命令级测试（已有部分）；③ 门槛脚本扩到全部包，阈值按各包当前水位分档设置，**分步抬高**（先设当前水位 -2%，再逐版本抬到 80%），避免一次性拉红 |
| 验收 | `bash ./scripts/check-coverage.sh <新阈值> out` 通过；`cmd/server` 覆盖率 > 0 且门槛脚本覆盖它 |
| 预估 | 1–2 天（补测试占大头） |
| 依赖 | 无 |
| 落地备注 | **已落地**。① `cmd/server`：把 `main()` 拆成可调用的 `run(args)` / `serve(migrateOnly)`（失败改为返回错误而不是 `logger.Fatal` 直接退出），给 `httpapi.Server` 加 `Serve(net.Listener)` 以便用临时端口驱动一次真实的启动 → `/healthz` 200 → 关停；`cmd/server` 覆盖率 0% → **77.4%**。② `cmd/cli`：补齐全部 14 个命令的命令级测试（路径、方法、admin 头、落盘行为、服务端报错映射），覆盖率 24.6% → **83.8%**。③ 门槛脚本：扩到 **9 个受控包**，改为**逐包分档判定** —— 门槛值写在 `scripts/coverage-thresholds.txt`（各包当前水位 -2%），`check-coverage.sh` / `check-coverage.ps1` 两版同步改造并共用同一张表；脚本第一个参数变成「表里没有的包」的兜底阈值。门槛口径 83.4% → **79.0%**（受控包变多，数字降低是覆盖范围变宽，不是倒退），全仓口径 62.7% → **74.1%**。 |

#### T11 · 门户补「查询投票状态」按钮（R18 剩余部分）

| 项 | 内容 |
|---|---|
| 问题 | `fetchVoteStatus` 已在 `web/src/composables/useVotePortal.ts:1122` 实现并导出（`:1711`），`VotePanel.vue:46-50` 也展示了 `hasVoted` 状态 —— 但**没有任何按钮能主动触发它**：全仓库只有 `submitVote` 内部调用（`:1118`）。投票人想在投票前确认「我投过没有」做不到，只能靠提交一次投票后的副作用。 |
| 证据 | `useVotePortal.ts:1122`（函数）、`:1711`（导出）、`:1118`（唯一调用点）、`web/src/components/panels/VotePanel.vue:46-50`（只读展示） |
| 改法 | ① `VotePanel.vue` 在凭证就绪 + 提案已选时显示「查询投票状态」按钮，调用 `portal.fetchVoteStatus()`；② 查询中禁用重复点击（复用已有的 `voteStatusLoading`）；③ 失败时给出行内错误而不是全局 notice |
| 验收 | vitest 新增用例：点击按钮 → 发出 `POST /proposals/:id/vote-status` 请求；`hasVoted` 更新为 true/false |
| 预估 | 1 小时 |
| 依赖 | 无 |
| 备注 | 这是 v0.1.0 计划 A8 的**剩余半项**（`hasVoted` 跨提案重置已由 `eb1d488` 修完），一并在这里收口 |

#### T12 · 文档如实描述盲签名的真实性质（决策门 G4）

| 项 | 内容 |
|---|---|
| 问题 | 盲签名的盲化在服务端内部完成：`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181` 在同一进程里依次 `Prepare` → `Blind` → `BlindSign` → `Finalize`，服务端全程持有明文消息 —— **不具备盲化性质**，等同普通签名。现有文档对此的表述不够明确。 |
| 证据 | `kryptology_credential_issuer.go:162-181` |
| 改法 | ① README「当前版本边界」与 `docs/techspec.md` 的密码学章节明确写出：**「签发方（服务端）在签发时就知道被签内容，当前不具备盲签名性质」**；② CHANGELOG 如实记录；③ 术语参考页同步；④ 明确写出「真正的两段式盲签名是 v1.0.0 证明链阶段的任务（T17）」，避免读者误以为已实现 |
| 验收 | 三处文档措辞一致；`grep -rn "盲签名" README.md docs/ web/` 逐条核对无夸大表述 |
| 预估 | 1 小时 |
| 依赖 | 无 |

---

## 3. Phase 2 · 证明链（周级）

**这是 v1.0.0 的主体，也是唯一的硬骨头。** 六项严格按顺序做，后一项依赖前一项。

```
T13 电路 + 密钥  →  T14 客户端 prover  →  T15 proof 绑定
                                              ↓
                              T16 加密移到客户端 ←─ T15
                                              ↓
                              T17 真盲签名（两段式）←─ T14
                                              ↓
                              T18 投票路径 VerifyCredential
```

#### T13 · 真实选票电路 + `verifying.key` 生成流程（C1）

| 项 | 内容 |
|---|---|
| 问题 | **选票的零知识电路根本不存在。** `internal/infrastructure/crypto/gnark_proof_verifier.go` 只有验证器骨架；`NewGnarkProofVerifier`（`:25`）在没有 `vk_path` 时构造出 `requireVerifyMode: false` 的实例，`Verify` 在 `:55-57` 对**任何非空 proof 直接返回 nil**（不验票）。生产配置的 `crypto.vk_path: "./keys/verifying.key"` 按本仓库**无法生成**。 |
| 证据 | `gnark_proof_verifier.go:25-31`、`:55-57`、`configs/config.production.yaml` 的 `vk_path` |
| 改法 | ① 电路定义「选票属于某个提案的某个选项」：`public = (proposal_id, option_index, nullifier)`，`private = (选中选项的承诺随机数)`，用 Groth16/BN254；② 新增 `cmd/keygen` 生成 `verifying.key` + `proving.key`（proving key **不入库**，只在客户端侧使用）；③ 密钥分发与轮换说明写进 `docs/techspec.md`；④ 仓库只提交 `verifying.key` 的**生成脚本与公钥指纹**，不提交 proving key |
| 验收 | ① `cmd/keygen` 产出的 vk 能被 `NewGnarkProofVerifier` 加载；② 电路测试：对正确 witness 生成 proof → 验证通过；篡改任一公开输入 → 验证失败 |
| 预估 | 3–5 天（含电路调参与性能评估） |
| 依赖 | 需先拍板 G7（曲线与电路形状） |

#### T14 · 客户端 prover（浏览器 WASM）（C2）

| 项 | 内容 |
|---|---|
| 问题 | 门户在生产模式下 **100% 无法完成投票**：没有 prover 就产不出 proof。现有 `ballot-draft` 返回写死的假 proof（`handler.go:314`）。 |
| 证据 | `handler.go:314`、review C2 |
| 改法 | ① 用 gnark 官方 wasm 模板产出 prover WASM，放进 `web/public/` 或独立静态目录；② 门户在浏览器内生成 proof **与 `public_witness`**（注意 `public_witness` 只能是公开输入部分，放整个 witness 会被 gnark 拒：报 `invalid witness size, got 2, expected 1`）；③ `ballot-draft` 降级为「仅返回选票选项与加密公钥」，**不再返回假 proof**；④ CLI 侧同步：CLI 目前已支持 `public_witness`（`cmd/cli/main.go:322`），但没有本地 prover —— 需明确 CLI 是否也要 prover（G9） |
| 验收 | ① 浏览器端生成的 proof 能通过服务端 `Verify`（e2e 用例）；② `public_witness` 长度为电路公开输入个数 |
| 预估 | 5–7 天 |
| 依赖 | T13 |

#### T15 · proof 与 `proposal_id` / `nullifier` / 密文绑定（C3）

| 项 | 内容 |
|---|---|
| 问题 | `SubmitVoteRequest.PublicInputs()`（`internal/usecase/voting.go:60-64`）**造了** `proposal_id` 和 `nullifier`，但 `Verify` 收到的 `inputs` 里只有 `public_witness` 是被真正读取的（`gnark_proof_verifier.go:64`）。也就是说：**服务端完全没有校验 proof 与请求里的 proposal_id / nullifier / 密文是否对应** —— 攻击者可以把 A 提案的合法 proof 拿去投 B 提案。 |
| 证据 | `internal/usecase/voting.go:60-64`、`:148`、`gnark_proof_verifier.go:64` |
| 改法 | ① 电路公开输入改为 `(proposal_id, option_index, nullifier, ciphertext_hash)`；② `PublicInputs()` 按序拼出**完整的**公开输入数组（含 `hash(ciphertext)`），不再走 `public_witness` 这条旁路；③ `gnark_proof_verifier.go` 改为读 `inputs["public_witness"]` 的同时，**校验其内容与 `proposal_id`/`nullifier`/`hash(ciphertext)` 一致**（不一致直接失败）；④ 移除 `public_witness` 的自由文本入口 —— 公开输入由服务端根据请求字段**自己拼**，客户端不再能指定 |
| 验收 | 回归测试：拿 A 提案的 proof 投 B 提案 → **400**；篡改 nullifier → 400；密文改了再投 → 400 |
| 预估 | 2–3 天（依赖 T13 的电路公开输入形状） |
| 依赖 | T13 |

#### T16 · 加密移到客户端（C5 根治）

| 项 | 内容 |
|---|---|
| 问题 | **当前版本最大的匿名性缺口**：密文由服务端生成（`ballot-draft` 拿明文 `option` 去加密），服务端在生成凭据时就知道选了哪个。README 曾因此作出与实现相反的承诺（已在 v0.1.0 改文案，但**实现本身没变**）。 |
| 证据 | `internal/infrastructure/http/handler.go:316-330`（`PrepareBallotDraft` 收明文 `option`）、review C5 |
| 改法 | ① 服务端只发**加密公钥 + 电路参数**，不再接受明文选项；② 浏览器端完成「选票 → 加密 → proof → 提交」全流程；③ 服务端接口改为接受 `{encrypted_vote, proof, nullifier, ...}`，**没有明文选项字段**；④ `ballot-draft` 端点随之删除（T1 已先把它关掉） |
| 验收 | ① 服务端代码里**不存在**接收明文选项的字段（grep 验证）；② e2e：浏览器投票 → 服务端落库记录里没有可还原的选项 |
| 预估 | 3–4 天 |
| 依赖 | T13、T14、T15 |

#### T17 · 盲签名改真两段式（R2）

| 项 | 内容 |
|---|---|
| 问题 | 盲签名的**盲化在服务端内部完成**：`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181` 在同一个进程里依次 `Prepare` → `Blind` → `BlindSign` → `Finalize`，服务端全程持有明文消息 —— **不具备盲化性质**，等同于普通签名。 |
| 证据 | `kryptology_credential_issuer.go:162-181` |
| 改法 | ① 拆成两个 HTTP 端点：客户端 → `POST /credentials/blind`（提交盲化后的消息）→ 服务端 `BlindSign` 返回盲签名 → 客户端 `Finalize` 得到最终签名 → `POST /credentials/attest`（提交最终签名 + 承诺）→ 服务端用 `Verify` 校验；② 客户端（浏览器 WASM）实现 `blindrsa` 的 client 侧（`Prepare`/`Blind`/`Finalize`）；③ 协议要绑定被盲化的消息承诺，防止「拿 A 的盲签名去签 B」 |
| 验收 | ① 端到端：客户端拿到的凭证，服务端**全程只见盲化消息**；② 交叉攻击测试：用 A 的盲签名 finalize 出 B 的凭证 → 校验失败；③ 服务端代码里不再出现「同一个函数里 Prepare+BlindSign+Finalize」 |
| 预估 | 5–7 天 |
| 依赖 | T14（需要客户端承载 crypto）、T9（领票码路径） |
| 备注 | 代价高、收益是「签发方看不到被签内容」。若投票规模小、运营方可信度可接受，**改文档如实描述**（G4）也是合理选择 |

#### T18 · 投票路径补 `VerifyCredential`（R3）

| 项 | 内容 |
|---|---|
| 问题 | 凭证签名在生产路径上**从不被密码学验证**：`VerifyCredential` 只在 `internal/infrastructure/crypto/` 及其测试里出现，投票提交路径不调用 —— 任何人都能伪造凭据投票。 |
| 证据 | `kryptology_credential_issuer.go:194` 定义了它，但 `internal/usecase/voting.go` 的 `Submit` 未调用 |
| 改法 | ① `Submit` 在落库前调用 `VerifyCredential(userID, userPubKey, credentialSignature)`，失败返回 401；② 需要在 `SubmitVoteRequest` 里带上签名与公钥（当前有 `CredentialSignature`，需确认公钥来源）；③ 补「伪造签名 → 401」回归测试 |
| 验收 | 伪造签名的投票请求返回 401；合法凭证正常落库 |
| 预估 | 1–2 天 |
| 依赖 | T16（否则明文选项仍在服务端） |

---

## 4. 决策门

**已拍板（2026-10-20）**

| # | 决策 | 结论 |
|---|---|---|
| **G4** | 盲签名：改实现还是改文档 | ✅ **先改文档如实描述**（任务 T21），两段式（T17）留到证明链阶段一起做 —— v1.0.0 承诺匿名性时必须补上 |
| **G8** | 投票人领凭证的形态 | ✅ **凭证池 + 投票人匿名随机领取**（任务 T9）—— 不再用一次性领票码。核心理由：admin 逐个分发就等于 admin 知道哪个票是谁的，直接破坏不可关联性 |
| **Phase 1 范围** | 这轮做哪些 | ✅ **T3–T11 这 9 项不依赖决策的**；T1（`ballot-draft` 非 debug 404）与 T2（门户隐藏按钮）**暂缓**，等另行拍板 G2/G3 |

**待拍板**

| # | 决策 | 选项 | 我的建议 | 卡住什么 |
|---|---|---|---|---|
| G1 | 版本定位 | 内测版 / 生产版 | **v1.0.0 走生产版**（本计划即为其执行计划）；v0.1.0 保持内测定位 | 全部 |
| G2 | 门户「按选项生成凭据」按钮 | 保留并加警示 / 非 debug 隐藏 | **非 debug 隐藏** | T2 |
| G3 | `ballot-draft` 生产可达性 | 保留 / 非 debug 404 | **非 debug 404** | T1、T16 |
| G5 | 首个版本号 | `0.1.0` / 其他 | 已落地 `0.1.0`，v1.0.0 用 `1.0.0` | 无关 |
| **G6** | **是否换纯 Go sqlite 驱动** | 保留 cgo（当前）/ 换 `modernc.org/sqlite` | **保留 cgo**：换驱动会波及持久化层方言差异测试（`entity/dialect_types_test.go`），收益只是「静态链接二进制」，不值 | 发布形态 |
| **G7** | **电路形状与曲线** | Groth16/BN254（与现有 verifier 一致）/ 其他 | **Groth16/BN254**：现有 `gnark_proof_verifier.go` 硬编码了 `ecc.BN254`，换曲线等于重写验证器 | T13、T15 |
| **G9** | **CLI 是否需要本地 prover** | CLI 也做本地 prover / CLI 只做「带 proof 提交」 | **CLI 只带 proof 提交**（`public_witness` 入口已有）；生成 proof 放浏览器端，CLI 面向脚本化/运维场景 | T14 |

---

## 5. Phase 3 · 生产化与发版

#### T19 · 生产配置落地（R6 / R7）

| 项 | 内容 |
|---|---|
| 问题 | `configs/config.production.yaml` 的 `cors_allowed_origins: []` 仍为空；且该文件**永远不会被自动加载**（viper `SetConfigName("config")`，`internal/config/config.go:27`）。 |
| 证据 | `configs/config.production.yaml`、`internal/config/config.go:27` |
| 改法 | ① 提供 `config.production.yaml` → `config.yaml` 的部署说明或 `CONFIG_FILE` 环境变量支持（**推荐**：给 viper 加 `CONFIG_FILE` 显式路径支持，比改名更不容易出错）；② CORS 填门户真实源 |
| 验收 | 用 `CONFIG_FILE=./configs/config.production.yaml` 启动，启动日志显示 `server.mode=release` 且 CORS 生效 |
| 预估 | 半天 |
| 依赖 | 无 |

#### T20 · 端到端验收（本机无 Docker 的三个盲区）

| 项 | 内容 |
|---|---|
| 问题 | review §5 列的三项**从未被验证**：① PostgreSQL testcontainers 集成测试（本机无 Docker，走的是跳过分支）；② 门户 7 个 `.vue` 面板的浏览器实际渲染（仓库里 `.vue` 零测试）；③ 并发投票下的防重投与性能容量。 |
| 证据 | `docs/release-review-2026-10-20.md` §5 |
| 改法 | ① Docker 环境跑通 `tests/integration`（CI 会实跑，本地补一份 compose）；② 引入 Playwright 覆盖关键路径（领凭证 → 投票 → 查询状态 → 计票）；③ 用 `hey`/`k6` 写并发投票脚本，验证防重投（唯一索引兜底）与容量 |
| 验收 | 三项都有**可复现的脚本 + 输出**，并把结果贴进 PR |
| 预估 | 2–3 天 |
| 依赖 | T14（浏览器端投票链路才完整） |

#### T21 · 发版（v1.0.0）

| 项 | 内容 |
|---|---|
| 动作 | ① `VERSION` 改 `1.0.0`；② `CHANGELOG.md` 写 1.0.0 条目（**必须**写明与 0.1.0 的密文不兼容，见 T7）；③ `git tag v1.0.0` → release workflow 自动构建；④ 按 v0.1.0 的生产部署清单逐项核对 |
| 验收 | `gh release view v1.0.0` 有产物；生产部署清单全勾 |
| 预估 | 半天 |
| 依赖 | Phase 1、2、3 全部完成 |

---

## 6. 提交分组

按 `incremental-implementation` 原则，每张任务卡一个提交，Conventional Commits：

```
chore(release): gate the ballot draft endpoint behind debug mode
fix(web): hide the option-scoped credential button outside debug
fix(auth): compare the admin bearer token in constant time
test(http): cover wrong and malformed admin credentials
feat(migrate): split schema migration out of server startup
feat(db): configure the PostgreSQL connection pool
fix(crypto): derive the shared secret with HKDF instead of a bare hash
fix(web): trap focus and close the guide overlay with Escape
feat(credential): issue credentials against one-time claim codes
feat(credential): issue credentials from an anonymous claim pool
docs: describe the blind signature for what it actually is
test(ci): extend the coverage gate to crypto, persistence and cmd
feat(web): add an explicit vote status lookup button
feat(circuit): add the ballot circuit and the key generation flow
feat(prover): prove ballots in the browser
fix(vote): bind the proof to the proposal, nullifier and ciphertext
feat(vote): encrypt ballots on the client
fix(credential): use a two-phase blind signature
fix(vote): verify the credential signature before storing the vote
feat(config): support an explicit config file path
test(e2e): cover the browser voting flow and concurrent voting
```

---

## 7. 验收标准（v1.0.0 的 go / no-go）

**No-Go（任一命中则不发 v1.0.0）**

- [ ] T13–T18 六项证明链任务有任一未完成
- [ ] 存在「服务端能拿到明文选项」的代码路径（grep 不到才过）
- [ ] 存在「proof 可以跨提案复用」的路径
- [ ] `go build` / `go vet` / `gofmt -l` / `go test ./...` 任一失败
- [ ] 覆盖率门槛退出码非 0
- [ ] 前端 lint / tsc / vitest 任一失败
- [ ] T20 的三项验收没有可复现输出
- [ ] README / techspec / 门户三处的匿名性声明不一致

**Go**

- [ ] 上述全过
- [ ] 决策门 G1–G9 全部拍板并落到代码/文档
- [ ] `VERSION` 与 tag 一致
- [ ] 生产部署清单全勾：`SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS` / `CRYPTO_VK_PATH` / `DB_DRIVER=postgres` / TLS / 门户独立部署 / 迁移步骤

---

## 8. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| T13 电路的证明生成太慢，浏览器端体验不可用 | 投票流程不可用 | T13 就带性能评估；必要时换电路形状（公开输入更少）而不是硬扛 |
| T17 真盲签名改动面大，拖垮整个 v1.0.0 | 发版无限延期 | T17 单独成提交链；G4 允许退化为「改文档」，v1.0.0 先不做 T17 但**必须在 CHANGELOG 如实说明** |
| T7 KDF 换协议使 v0.1.0 的密文永久不可解 | 旧数据丢失 | 明确标注密文格式版本；v1.0.0 前不要用旧数据做真实投票 |
| T16 把加密移到客户端后，前端成为新的攻击面 | 密钥泄露 | proving key 只在前端，**不入服务端**；前端构建产物做完整性校验 |
| T9 领票码引入新的状态机 | 新 bug 面 | 一次性 + 哈希存储 + 限流；先用最简单的同步实现，后续再优化 |
| Phase 1 拖太久，Phase 2 无时间 | 计划整体延期 | Phase 1 与 Phase 2 **可并行**（除 T1/T2 依赖 Phase 2 的字段） |

---

## 9. 与 v0.1.0 计划的关系

- v0.1.0 计划（[`release-plan-v0.1.0.md`](./release-plan-v0.1.0.md)）管的是**发得出去**：错误码、生产配置硬化、CLI 正确性、静默投错票、文案对齐、测试补强、CI 与发版流水线 —— **已全部落地**。
- 本计划管的是**匿名性成立**：证明链 + 加固 —— **Phase 1 的 T3–T12 已落地**，T1/T2 等 G2/G3 拍板，Phase 2 证明链与 Phase 3 生产化未开工。
- 两者唯一的重叠是决策门 G1–G5，已在本文件 §4 延续并补充 G6–G9。

> v0.1.0 已经在 `docs/release-plan-v0.1.0.md` §8 记录了落地状态与验证结果，
> 并标注了「v1.0.0 里程碑不在本计划内」。本文档即那部分内容的展开。
