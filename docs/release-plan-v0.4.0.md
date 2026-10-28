# nullifier 发版计划 v0.4.0（能用于有规模的真实投票）

> 配套文档：[`docs/roadmap.md`](./roadmap.md)（能力与任务卡，里程碑 M5）、
> [`docs/deployment-checklist.md`](./deployment-checklist.md)（生产部署清单）、
> [`docs/project-map.md`](./project-map.md)（上下文索引）、
> [`docs/techspec.md`](./techspec.md)（当前实现口径）、
> [`docs/api-contract-v1.md`](./api-contract-v1.md)（**冻结的接口契约** —— 本版不动它）
>
> 分支：`feat/v0.4.0-scale-and-tenancy`（从 `main` 切出）
> 制定日期：2026-10-28
> 状态：**已实现，待发版**（2026-10-28 升格为里程碑 M5 并当轮落地 —— 四张任务卡
> T29 / T30 / T31 / T32 全部完成，出口标准 §5 逐条有证据，落地记录见 §8）

---

## 0. 一句话策略

**v0.3.0 是「接口不再动」，v0.4.0 是「能用于有规模的真实投票」。**

这一版不改密码学方案、不动冻结契约、不放松任何匿名性底线。它解决的是
「真办一场几百到几十万人的投票」时撞到的四堵墙：

1. **名单规模**：资格名单的 Merkle 树深是编译期常量 `TreeDepth = 10` ——
   **一份名单最多 1024 人**，真实选区/组织名录根本放不下（T29）。
2. **一份服务只能办一场投票**：`crypto.eligibility_root` 是全局单值 ——
   一台服务端只能配**一个**名单根，两个互不相干的组织没法共用一套部署（T30）。
3. **管理面一把钥匙管所有事**：单一 `admin_bearer_token`，把令牌交给第三方审计
   就等于交出全部运营权（T31）。
4. **身份源进不来**：名单工具只吃「一行一个 ID」的纯文本，重复成员被**静默丢弃**，
   HR/选区系统导出的名单没有安全的摄入路径（T32）。

> ⚠️ **「有规模」的口径**：本版交付的是「名单放得下、多场投票并存、管理面可分权、
> 名单摄入安全」。投票吞吐的容量数字见 T27 的容量报告（单实例约 2300 票/秒），
> 本版不重测吞吐 —— 它不是本版的墙。

---

## 1. 版本口径

| 能力 | v0.3.0 | v0.4.0 |
|---|---|---|
| **A–G** 匿名性七项 | ✅ 全部达成 | ✅ **保持**，一行不改 |
| **J** 接口冻结 | ✅ 三处冻结 | ✅ **保持** —— 本版对冻结接口**只增不改** |
| **K** 审计可复算 | ✅ `cmd/audit-recompute` | ✅ 保持，不变 |
| **L** 容量可规划 | ✅ 容量报告 | ✅ 保持（吞吐不重测） |
| **M** 名单可规模 | ❌ 编译期 1024 人上限 | ✅ **树深可配（4–24），默认 10 不变**（T29） |
| **N** 多场投票并存 | ❌ 全局单一名单根 | ✅ **多套资格名单，提案绑定名单域**（T30） |
| **O** 管理面分权 | ❌ 单一 admin 令牌 | ✅ **admin / operator / auditor 三级**（T31） |
| **P** 身份源可对接 | 🟡 只有「一行一个 ID」文本，重复静默丢弃 | ✅ **CSV 导入 + 重复/溢出拒绝 + 离线校验**（T32） |

**版本号来源**：`VERSION` 文件（单一真相源），release workflow 从 tag 读取并校验；
`web/package.json` 的 `version` 由流水线注入，不手改。

---

## 2. 本版范围

### 做

| # | 内容 | 任务卡 |
|---|---|---|
| 1 | 资格名单树深可配（4–24），默认 10 向后兼容；keygen / 服务端 / CLI / WASM / 门户全链路按深度接线 | T29 |
| 2 | 多套资格名单并存：配置为命名列表，提案绑定名单域，challenge / claim 按提案解析 | T30 |
| 3 | 管理面三级令牌：admin / operator / auditor，包含关系，旧单令牌 = admin | T31 |
| 4 | 身份源摄入：CSV（含 BOM 容错）、重复与溢出默认拒绝、`-verify` 离线校验产出目录 | T32 |

### 不做（明确排除）

- **独立租户账户体系 / 按租户分库** —— 多租户以「多套名单并存 + 提案绑定名单域」的
  形态落地（V4-D2）；租户级计费/配额/隔离部署属 1.0.0 档。
- **投票吞吐重测** —— T27 的容量报告仍然有效；本版不重跑压测。
- **密文格式 / 冻结端点的任何变更** —— 冻结契约一动就是破坏性变更，本版没有这个需求。
- **移动端原生客户端、密码学方案替换** —— 属 1.0.0 档（可选项）。

---

## 3. 任务卡

### T29 · 资格名单规模化：树深可配

| 项 | 内容 |
|---|---|
| 问题 | `eligibility.TreeDepth = 10` 是**编译期常量**（`internal/infrastructure/crypto/eligibility/circuit.go:35`）：电路按它做 index 位分解，名单工具按它建树，服务端按它报 depth。一份名单**上限 2^10 = 1024 人** —— 一个真实选区/大组织根本放不下。路线图 T22 落地核对里明确记着这条未达成项：「单份名单上限 1024 人（TreeDepth 是编译期常量）」。 |
| 证据 | `internal/infrastructure/crypto/eligibility/circuit.go:35`（`const TreeDepth = 10`）、`:108`（`api.ToBinary(c.Index, TreeDepth)`）、`prover.go:183-191`（`compileCircuit` 写死常量）、`cmd/keygen/main.go:135`（`eligibility.GenerateKeys()` 无深度参数）、`cmd/eligibility-list/main.go:154`（`BuildTree(leaves, eligibility.TreeDepth)`） |
| 改法 | ① 电路**参数化**：`Define` 里的位分解宽度与 Merkle 上溯层数改用 `len(c.Path)` —— 深度由编译模板的 Path 切片长度决定，不再读常量（不需要给电路结构体加额外字段）；② 深度合法性：`ValidateTreeDepth` 允许 **4–24**（2^4=16 人下限防呆，2^24 ≈ 1678 万上限由约束数与 setup 时长兜住），`DefaultTreeDepth = 10` 向后兼容；③ **每深度一套密钥**：`GenerateKeys(depth)`；`cmd/keygen` 新增 `-depth`（仅 eligibility 电路接受）；④ 服务端配置 `crypto.eligibility_depth`（默认 10），`EligibilityService` 的 depth 来自配置而不是常量；⑤ 客户端全链路：CLI `credential-claim` 从成员包的 `Depth` 字段取深度；WASM `initEligibility` 增加可选 depth 参数（缺省 10）；门户把成员包里的 depth 传给 WASM；名单工具 `eligibility-list` 新增 `-depth`，成员超限时明确报错。 |
| 验收 | ① 深度 4 的密钥对走完整 prove/verify 往返（快），篡改任一公开输入失败；② 深度 10 的既有全部测试**原样通过**（默认值不变 = 密钥不换 = 向后兼容）；③ `keygen -circuit eligibility -depth 4` 产出的密钥能被对应深度的 prover/verifier 使用，深度不匹配时证明失败；④ `eligibility-list -depth 4` 装第 17 人时报错并提示换深度；⑤ node 驱动的 WASM 用例以非默认深度跑通一条完整领取链路（`tests/e2e/wasm_eligibility_test.go` 扩展）；⑥ `go test ./...` 全绿 |
| 预估 | 2–3 天 |
| 依赖 | 无（本卡的起点） |

> ⚠️ **跨语言锁**：深度不是常量之后，两侧的「默认深度 = 10」要各钉一条断言
> （Go 侧 `DefaultTreeDepth` + TS 侧 `web/src/lib/eligibility-prover.ts` 的缺省值），
> 防止单边改动。成员包的 `Depth` 字段**早已存在**（v0.2.0 落地时就有），本版开始真正使用它。

### T30 · 多套资格名单并存（多租户）

| 项 | 内容 |
|---|---|
| 问题 | `crypto.eligibility_root` + `crypto.eligibility_vk_path` 是**全局单值** —— 一台服务端只能配一个名单根。两个互不相干的组织（各自有各自的名单、各自的证明密钥）没法共用一套部署，只能各起一套服务。计票密钥**已经是每提案独立**（解密 setup 时生成），凭证池也已经是每提案独立 —— 唯独「谁能投票」的名单是全局的。 |
| 证据 | `internal/config/config.go:77-82`（单一 `EligibilityRoot` / `EligibilityVerifyingKeyPath`）、`internal/infrastructure/crypto/eligibility_service.go`（服务端适配层只持有一个 root + 一个 verifier）、`internal/usecase/registration.go:214`（`uc.eligibility.Root()` 无提案维度）、`internal/domain/entity/proposal.go`（提案没有名单域字段） |
| 改法 | ① 配置：`crypto.eligibility_roots` 变成**命名列表**（`name` / `root` / `vk_path` / `depth`）；旧的单值写法仍然合法，等价于一个名为 `default` 的名单（向后兼容）；两种写法同时出现 → 启动失败（消除歧义）；② 提案绑定名单域：`proposals` 新增 `eligibility_scope` 列（可空）；创建提案（admin）可带 `eligibility_scope`，填了不存在的名单名 → 400（创建时就拦住，不留到领取时才炸）；③ 解析规则：提案的 scope 按名字解析名单；**scope 为空**时 → 名为 `default` 的名单；若只配了一个名单就用它；两者都不满足 → 该提案的资格校验失败（fail-closed）；④ `eligibility-challenge` 与 claim 的校验都按提案解析到对应名单的 root / depth / verifier —— 冻结契约的响应字段一个不变（`root` / `depth` 的**值**随名单走）；⑤ `/healthz` 增加名单数（加法）；⑥ release 模式升级需先跑 `-migrate`（新列）。 |
| 验收 | ① 两个名单、两个提案（各绑一个）的完整领取链路各自通过，跨名单的成员包在对方提案上**证明失败**（`INVALID_ELIGIBILITY_PROOF`）；② 旧配置（单值写法）行为与 v0.3.0 完全一致（回归：既有资格 e2e 原样通过）；③ 创建提案时 scope 不存在 → 400；④ 名单名重复 / 旧新写法混配 → 启动失败；⑤ 迁移在 SQLite 与 PostgreSQL 上都可用（集成测试）；⑥ `go test ./...` 全绿 |
| 预估 | 3–4 天 |
| 依赖 | T29（名单条目要带 depth） |

> ⚠️ **服务端仍然只见 root 不见名单**：多名单不改变这条底线 —— 每个名单条目还是
> 只有 root + vk + depth，`eligibility_uses` 只存去重键。服务端能验证「这个资格在
> 这个名单的树里」，仍然无法把资格对应到任何成员。
> ⚠️ **边界**：盲签名的签发方密钥与选票电路验证密钥仍是**服务端全局**的（它们是
> 「这台服务器的密钥」，不是租户数据；凭证池与去重本来就按提案隔离，跨名单不可复用）。

### T31 · 管理面权限分级

| 项 | 内容 |
|---|---|
| 问题 | 管理面是**一把钥匙**：`server.admin_bearer_token` 一枚令牌通吃「创建提案、灌凭证池、解密 setup、发布结果、看凭证状态、读 /metrics」。把只读观察权交给第三方审计、或把日常运营权交给选举工作人员时，只能交出全部权限。 |
| 证据 | `internal/config/config.go:66`（单一 `AdminBearerToken`）、`internal/infrastructure/http/server.go:60-90`（所有管理路由挂同一个 `RequireAdminBearerToken`）、`internal/infrastructure/http/middleware/`（鉴权中间件只有「是否 admin」一档） |
| 改法 | ① 三级角色：**admin**（全部）⊃ **operator**（运营：创建提案、灌池、解密 setup、发布结果、管理端凭证签发）⊃ **auditor**（只读：凭证状态查询、`/metrics`）；包含关系是刻意的 —— 上一级令牌能做下一级的一切；② 配置：`server.admin_tokens: {admin, operator, auditor}` 三个可选条目；旧的单值 `admin_bearer_token` 继续合法且等价于 admin（存量部署零改动升级）；release 模式要求**至少一枚**令牌；③ 中间件按角色放行：令牌对候选集做常数时间比较（沿用 T3 的摘要比较法）；④ 路由映射落到 `server.go` 的各条管理路由上（`/healthz` 与公开端点不受影响）；⑤ 部署清单写明「哪枚令牌发给谁」。 |
| 验收 | ① auditor 令牌能读 `/metrics` 与凭证状态，**不能**灌池 / 创建提案；② operator 能做全部运营操作，auditor 面可达；③ admin 令牌全通；④ 旧配置（单令牌）行为与 v0.3.0 完全一致（回归：既有 admin 用例原样通过）；⑤ 畸形 / 空白 / 带 CR-LF 的 Authorization 头在三个角色上一律拒绝；⑥ `go test ./...` 全绿。⚠️ **落地偏离**：权限不足**与令牌错误同形地返回 401**，不是 403 —— 回 403 会告诉持低级令牌的人「这个路由存在且需要更高权限」；401 什么都不泄露，且不用为它新增一个要进跨语言错误码门禁的错误码 |
| 预估 | 1–2 天 |
| 依赖 | 无（与 T29 / T30 无耦合，排在其后只为减少 server.go 的合并冲突） |

> ⚠️ **诚实边界**：operator 与 admin 今天覆盖同一条路由面（当前没有「只有 admin 能做」
> 的运行期操作）—— 第三级是给未来的密钥级操作预留的位置，本版的价值在 **auditor**
> 这一级的真实分权与整套机制的守卫测试。分级不改变「管理面接口一律要 Bearer 令牌」的口径。

### T32 · 身份源对接：名单的安全摄入

| 项 | 内容 |
|---|---|
| 问题 | `cmd/eligibility-list` 只吃「一行一个 ID」的纯文本，且**重复成员被静默丢弃**（`main.go:101-104` 直接 `continue`）—— HR / 选区系统导出的名单里若混入重复行，有一个成员会**静默失去资格**，没有任何报错；名单超出 `2^depth` 时报错信息不指向解决办法；生成完的目录没有离线校验手段（root 与成员包是否成套、路径是否真的通到 root，全靠运气）。 |
| 证据 | `cmd/eligibility-list/main.go:86-115`（`readMembers`：重复静默去重、无格式支持）、`:154`（超限报错只有 `BuildTree` 的底层消息）、全工具无 `-verify` 类子命令 |
| 改法 | ① **CSV 输入**：`.csv` 扩展名自动走 CSV 解析（首行表头；成员 ID 列默认取第一列，`-id-column <列名>` 覆盖；UTF-8 BOM 容错 —— Excel 导出常态）；`.txt` 保持原样；② **重复默认拒绝**：重复成员列出清单并以非 0 退出（`readMembers` 的静默去重删除）—— 名单少一个人比生成失败严重得多；③ **溢出明确报错**：人数 > 2^depth 时提示「用 `-depth` 换更深的树」；④ **`-verify <目录>`**：离线校验一份产出 —— 逐个成员包从秘密重导叶子、沿路径上溯到根，与 `root.json` 的 root / depth / count 比对，缺包 / 多包 / 改动一例即报，退出码 0 / 1 / 2 可当门禁用；⑤ `root.json` 增加 `label`（`-label`，运营标注名单来源）与 `generated_at`（加法字段，服务端忽略不认识的字段）。 |
| 验收 | ① 含 BOM 的 CSV 正确解析；② 重复成员 → 退出码非 0 且列出重复 ID；③ 超限 → 明确提示换深度；④ `-verify` 对完整目录通过（0），对「改一个字节的成员包 / 缺一个包 / 包数与 count 不符 / 换了 root」各情形失败（非 0）；⑤ 纯文本路径与 v0.3.0 行为一致（除重复从静默变拒绝）；⑥ `go test ./...` 全绿（该命令的覆盖率门槛已在 `coverage-thresholds.txt` 里） |
| 预估 | 1–2 天 |
| 依赖 | T29（`-depth` 旗标） |

---

## 4. 破坏性变更（升级前必须知道）

**冻结契约：无变更。** 字段名、端点路径与密文格式 v3 一概不动
（1.0.0 判据第 2 条「密文格式定版后连续 ≥ 2 个 minor 不变」从本版起开始计满两格：
v0.3.0、v0.4.0）。

### 非破坏性但需要部署动作

1. **多名单 / 新树深**：`crypto.eligibility_roots`（列表）与 `crypto.eligibility_depth`
   是**新增**配置，旧单值写法继续有效。换更深名单时：`keygen -depth N` 出新密钥对 →
   `eligibility-list -depth N` 重新生成名单 → root / vk / depth **三项成套**配进服务端
   → 门户静态目录换对应的 eligibility proving key。三项不成套 = 证明必败。
2. **迁移**：`proposals.eligibility_scope` 新列 —— release 模式升级前先跑
   `nullifier-server -migrate`（与 v0.3.0 的 `reserved_at` 同一条规矩）。
3. **权限分级**：要分权才需要动配置 —— 什么都不配 = 旧单令牌照常当 admin 用。
4. **名单工具重复行为收紧**：重复成员从「静默丢弃」改为「拒绝生成」——
   名单里有重复行时 v0.4.0 的工具会拒绝，需要运营方先清洗名单。

---

## 5. 出口标准（go / no-go）

| # | 检查项 | 状态 |
|---|---|---|
| 1 | 冻结契约零变更（`tests/architecture/api_contract_test.go` 原样通过） | ✅ **已过**（2026-10-28：架构门禁全绿；§4 的加法记录已写进契约文档） |
| 2 | T29：深度 4 / 深度 10 双深度的证明往返 + WASM 非默认深度链路 + 默认兼容回归 | ✅ **已过**（`internal/infrastructure/crypto/eligibility/depth_test.go` 六条；`tests/e2e/wasm_eligibility_test.go` 新增深度 4 的 node 驱动链路，实测 init 2649ms / prove 998ms，服务端真实验签通过；深度 10 既有用例原样通过） |
| 3 | T30：双名单双提案 e2e、跨名单失败、旧配置回归、迁移可用 | ✅ **已过**（`tests/e2e/multi_roster_eligibility_test.go`：acme 成员在自己提案 201 / 跨名单 403 / default 兜底 201 / 未知名单 400；旧单值配置的既有资格 e2e 原样通过；`proposals.eligibility_scope` 落库回读有持久化测试） |
| 4 | T31：三级令牌的放行 / 拒绝矩阵 + 旧单令牌回归 + 恶意头拒绝 | ✅ **已过**（中间件 9 格包含矩阵 + 恶意头 + 无候选令牌；HTTP 级真实服务端矩阵 `admin_roles_test.go`；旧单令牌回归 `TestServer_AdminRoles_LegacyTokenIsAdmin`） |
| 5 | T32：CSV / BOM / 重复 / 溢出 / `-verify` 五情形各有测试 | ✅ **已过**（BOM / `-id-column` / 缺列 / 重复拒绝（txt 与 csv）/ 超限且不留半份名单 / `-verify` 的通过与「改包 / 丢包 / 无 root」三种失败） |
| 6 | `go build` / `go vet` / `gofmt -l` / `go test ./...` 全绿；覆盖率门槛退出码 0 | ✅ **已过**（2026-10-28：24 个包全绿；覆盖率门槛 **81.42%** 达标，`internal/di` 因新增装配分支补了 provider 测试后回绿） |
| 7 | 前端 lint / tsc / vitest 全绿；`make web-e2e`（真浏览器两条路径）通过 | ✅ **已过**（eslint / tsc 零输出；vitest 19 文件 160 用例全过；`web-e2e` 真浏览器 1 passed（2.2m），release 模式服务端 + 计票还原 + 一人一票拒绝，healthz 已报 `eligibility_rosters`） |
| 8 | 文档闭合：techspec / deployment-checklist / project-map / roadmap / CHANGELOG 全部更新 | ✅ **已过**（techspec §3.5 / §5.2 / §5.3 / §6.11；部署清单 §2 / §2.1 / §2.2 / §3；project-map 陷阱 46–48；契约文档 §4 加法记录；配置模板与 env 样例；CHANGELOG 0.4.0 段） |
| 9 | `VERSION` = `0.4.0`，tag `v0.4.0` 已推，release 流水线全绿，产物已核验 | ⬜ 发布步骤见 §7 |

---

## 6. 决策记录（V4-D1 ~ D6，2026-10-28 拍板）

| # | 决策 | 结论 |
|---|---|---|
| **V4-D1** | 本版做哪些 | ✅ 四张卡：T29 名单规模化（树深可配）、T30 多名单并存、T31 权限分级、T32 身份源摄入。**理由**：路线图 §1.2 给 v0.4.0 的四项内容（多租户、权限分级、身份体系、真实身份源）里，「有规模」的第一堵墙是 1024 人上限，「多租户」的真实缺口是「一台服务只能配一个名单」（计票密钥与凭证池早已按提案隔离），「身份体系」的缺口在摄入端而不是证明端（T22 的证明机制已落地）。**代价**：不做独立租户账户体系与吞吐重测（分别归 1.0.0 与「不需要」）。 |
| **V4-D2** | 多租户的形态 | ✅ **多套命名名单 + 提案绑定名单域**，不是租户账户/分库。**理由**：投票系统的「租」发生在名单层（谁能投这场票），计票密钥与凭证池天然按提案隔离；账户体系解决的是 SaaS 化运营，那不是「能办有规模的投票」的前提。**代价**：同一服务上的多场投票共享服务端密钥（签发方密钥、选票 vk）—— 它们是「服务器的密钥」而非租户数据，边界写入部署清单。 |
| **V4-D3** | 树深怎么配 | ✅ **运行期可配 4–24、默认 10、每深度一套密钥**，不是「改成更大的编译期常量」。**理由**：改常量 = 全世界所有部署同时换密钥、旧名单全部作废；参数化后默认值不变 = 存量部署零动作，需要规模的部署自己选深度。上限 24（约 1678 万）远超任何单选区，约束数与 setup 时长自然兜底。**代价**：服务端、keygen、CLI、WASM、门户、名单工具六处都要接深度参数 —— 一次性成本，换来「深度是部署决策」而非「深度是代码决策」。 |
| **V4-D4** | 权限分级的形状 | ✅ **三级命名令牌 + 包含关系**（auditor ⊂ operator ⊂ admin），旧单令牌 = admin。**理由**：真实分权需求是「把只读观察权交给第三方审计」（/metrics 与凭证状态）；operator 与 admin 今天路由面相同，admin 是给未来密钥级操作预留的位置 —— 不发明今天没有的对象。**代价**：三级里有两级暂时等价，文档里如实写明。 |
| **V4-D5** | 身份源的摄入策略 | ✅ **重复默认拒绝**（从静默丢弃收紧）、CSV + BOM 容错、`-verify` 离线校验。**理由**：静默丢人是不可接受的失败模式 —— 名单少一个人，那个人**永远投不了票且无人知道**；「生成失败、人工清洗、重跑」是唯一安全的路径。`-verify` 让「root 与成员包成套」从口头纪律变成一条可当门禁的命令。 |
| **V4-D6** | 冻结契约在本版的地位 | ✅ **零变更**。资格相关的新能力全部走「已有字段的值随名单走 + 配置层扩展」：`eligibility-challenge` 响应的 `root` / `depth` 字段名不变、值按提案解析；创建提案是管理端端点不在冻结范围，新增可选 `eligibility_scope` 属加法。**收益**：1.0.0 判据第 2 条（密文格式 ≥ 2 个 minor 不变）从本版开始计满。 |

---

## 7. 发布步骤

1. `VERSION` → `0.4.0`；`CHANGELOG.md` 写 0.4.0 条目（含部署动作清单）。✅
2. 分支 `feat/v0.4.0-scale-and-tenancy` 推送 → PR → 合并回 `main`。
3. 打**注解 tag** `v0.4.0`（release workflow 用 tag 说明当 release 正文）并推送。
4. 监控 release 流水线（verify / build×5 平台 / web / release 八个 job）全绿。
5. `gh release view v0.4.0` 核验 12 个产物（5 平台二进制 + 门户包，各带 sha256），
   门户包含 `ballot-prover.wasm` 与 `wasm_exec.js`（两把 proving key 按设计不在包里）。
6. 回填本文件 §8 落地记录与路线图 M5 进展表。

---

## 8. 落地记录（2026-10-28）

四张任务卡当轮落地，按依赖序 T29 → T30 → T31 → T32，每张一个提交：

| 卡 | 提交 | 实测要点 |
|---|---|---|
| T29 树深可配 | `feat(crypto): make the eligibility tree depth a deployment parameter` | 深度 4 = 3566 条约束（每层约 549 条，与深度 10 的 6860 吻合）；WASM 深度 4 链路 init 2649ms / prove 998ms；跨语言锁两侧各一条 |
| T30 多名单并存 | `feat(api): let multiple eligibility rosters coexist on one server` | 双名单双提案 e2e 四场景全过；wire 生成器重跑（`NewProposalUseCase` 注入资格服务做创建时校验）；`/healthz` 带 `eligibility_rosters` |
| T31 权限分级 | `feat(http): split admin bearer auth into admin/operator/auditor roles` | 9 格包含矩阵 + HTTP 级真实服务端矩阵；401 同形决策见任务卡的偏离注 |
| T32 身份源摄入 | `feat(offline): ingest real identity sources safely in the eligibility list tool` | BOM CSV / 重复拒绝 / 超限拒绝且不留半份名单 / `-verify` 四情形；`eligibility.WalkPath` 与建树一致性有单测 |

**验证汇总（2026-10-28）**：`go test ./...` 24 个包全绿；覆盖率门槛 81.42% 达标；
`go vet` / `gofmt -l` 干净；前端 eslint / tsc 零输出、vitest 160 用例全过；
`scripts/web-e2e.sh run` 真浏览器两条路径通过（release 模式 + 真 proving key +
强资格校验，计票还原出所选选项）。

**过程中的两个发现**：

- `BuildTree` 原来自带一个 `depth > 20` 的上限，与新支持范围（4–24）不一致 ——
  已改为走 `ValidateTreeDepth`，消灭了「深度 21–24 能过配置校验、建树必失败」的暗坑。
- 电路的深度参数化**不需要**给电路结构体加额外字段：`Define` 里的位分解宽度与
  上溯层数直接取 `len(c.Path)`，深度由编译模板的切片长度决定 —— 比给结构体加
  `gnark:"-"` 字段的方案少一个反射面。
