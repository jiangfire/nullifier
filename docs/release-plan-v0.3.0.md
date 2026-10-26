# nullifier 发版计划 v0.3.0（接口不再动）

> 配套文档：[`docs/roadmap.md`](./roadmap.md)（能力与任务卡）、
> [`docs/deployment-checklist.md`](./deployment-checklist.md)（生产部署清单）、
> [`docs/project-map.md`](./project-map.md)（上下文索引）、
> [`docs/techspec.md`](./techspec.md)（当前实现口径）、
> [`docs/api-contract-v1.md`](./api-contract-v1.md)（**冻结的接口契约**）
>
> 分支：`feat/v0.3.0-contract-and-audit`（从 `main` 切出）
> 制定日期：2026-10-25
> 状态：**已发布**（2026-10-26）—— 六张任务卡全部落地，出口标准逐条勾上并附实测证据（§5）；
> 发版说明正文与发版步骤见 §8，**发布结果见 §8.4**。发版点由 tag `v0.3.0` 独立保留；
> 代码已合并回 `main` @ `4d3bac3`（merge commit），分支已删除。

---

## 0. 一句话策略

**v0.2.0 是「能真投」，v0.3.0 是「接口不再动」。**

这一版不改密码学、不改匿名性口径、不改「服务端拿不到明文选项」这条底线。
它做四件事：

1. **把三处接口冻结下来**（凭证领取 / 投票提交 / 密文格式），并**定版**密文格式 ——
   从此以后它们只增不改，改动要走 minor 版本。
2. **把审计做成能脱离服务端复算**（`cmd/audit-recompute`）—— 审计锚点 v0.1.0
   就有了，缺的是一把「拿着数据库 + 份额就能自己算一遍」的独立复算脚本。
3. **把监控指标补上**（Prometheus `/metrics`）—— 路线图 §2.2 把监控排在 v0.2.0 之外。
4. **把公开端点的限流补上**（T25）—— 凭证领取 / 投票提交 / 只读端点按来源限流，
   release 模式强制开启。

> ⚠️ **这一版不是「生产可用」的承诺。** v0.3.0 解决的是**接口稳定性与可运维性**，
> 不是「有规模的真实投票」—— 后者按路线图属 v0.4.0。

---

## 1. 版本口径

| 能力 | v0.2.0 | v0.3.0 |
|---|---|---|
| **A** 选票内容对服务端不可知 | ✅ 客户端加密 | ✅ **保持**，不变 |
| **B** 选票可验证 | ✅ 真实 Groth16 电路 | ✅ **保持**，不变 |
| **C** proof 与票绑定 | ✅ 公开输入服务端自拼 | ✅ **保持**，不变 |
| **D** 客户端能投票 | ✅ CLI + 门户两侧 | ✅ **保持**，不变 |
| **E** 凭证不可伪造 | ✅ 落库前验签 | ✅ **保持**，不变 |
| **F** 签发方看不到被签内容 | ✅ 真两段式盲签名 | ✅ **保持**，不变 |
| **G** 凭证分发不泄露身份 | ✅ 资格证明兑现一人一票 | ✅ **保持**，不变 |
| **H** 生产可运维 | 🟡 显式配置路径 + 部署清单 | 🟡 **+ 监控指标**（`/metrics`）**+ 公开端点限流** |
| **I** 端到端验收 | 🟡 三盲区各补一部分 | ✅ **保持并补齐**（Playwright 已落地：T28，真浏览器跑门户的两条路径） |
| **J** 接口可冻结 | ❌ 三处接口「大概率还要再动一轮」 | ✅ **三处接口冻结 + 密文格式定版** |
| **K** 审计可复算 | ❌ 审计锚点有，但缺脱离服务端的复算脚本 | ✅ **`cmd/audit-recompute` 离线复算** |
| **L** 容量可规划 | ❌ 只有压测实测，没有资源建议 | ✅ **容量报告 + 按规模的资源配置建议**（T27） |

**版本号来源**：`VERSION` 文件（单一真相源），release workflow 从 tag 读取并校验；
`web/package.json` 的 `version` 由流水线注入，不手改。

---

## 2. 本版范围

### 做

| # | 内容 | 任务卡 |
|---|---|---|
| 1 | 三处接口的线格式冻结（Go↔TS 双向锁定 + 契约文档） | T23 |
| 2 | 投票提交接口收敛：`POST /api/v1/votes` → `POST /api/v1/proposals/:proposal_id/votes` | T23 |
| 3 | 密文格式定版：v3 定为「不再变」的格式，格式号与长度双向锁死 | T23 |
| 4 | 审计离线复算工具 `cmd/audit-recompute` | T24 |
| 5 | 监控指标端点 `GET /metrics`（Prometheus） | T26 |
| 6 | 公开端点限流（按 IP + 路由组，release 模式强制开启） | T25 |
| 7 | 真正的压测容量规划（真 PostgreSQL + 真证明，产出容量报告与资源配置建议） | T27 |
| 8 | 门户 UI 自动化（真浏览器跑门户的两条路径） | T28 |

### 不做（明确排除）

- **密码学方案替换、多租户、权限分级、移动端原生客户端** —— 分别属 v0.4.0 与 1.0.0。

> **「不做」不等于永远不做。** 这一项仍在 v0.3.0 的名义范围内，只是分批交付。
>
> ⚠️ **Playwright 曾被排在本版之外**（2026-10-25：它是独立的一条，排在核心之后）。
> **2026-10-26 已补上**（T28）—— 当时记的「离线装不上浏览器」的前提**本来就不成立**
> （chromium 118.1 MiB 能正常下载）。

---

## 3. 任务卡

### T23 · 三处接口冻结 + 密文格式定版

| 项 | 内容 |
|---|---|
| 问题 | 路线图 §1.1 第 1 条说 v0.2.0 之后「凭证领取、投票提交、密文格式这几处接口**大概率还要再动一轮**」。v0.2.0 的接口确实有两处不一致：投票提交把 `proposal_id` 放在**请求体**里（别的端点都在**路径**里），密文格式只有「版本号 3」这个事实、没有「**定版**」的承诺与守卫。 |
| 证据 | `internal/infrastructure/http/handler.go:62`（`r.POST("/votes", …)`）、`:83`（`submitVotePayload` 含 `proposal_id`）、`internal/infrastructure/crypto/ballot/cipher.go:16`（`CipherVersion = 3`）、`web/src/lib/ballot-cipher.ts:43`（`ballotCipherVersion = 3`） |
| 改法 | ① **投票提交接口收敛**：路径改成 `POST /api/v1/proposals/:proposal_id/votes`，`proposal_id` **只从路径来**（请求体里那个字段删掉）—— 与其余端点的形状一致，客户端不可能把两个提案 ID 搞混；② **契约文档** `docs/api-contract-v1.md` 逐字段写死三处接口的请求/响应；③ **门禁测试**（`tests/architecture/`）：Go 侧的 struct tag、TS 侧的接口字段、密文的版本号与长度**双向锁死**，改任一边就红；④ 密文格式**定版**：v3 是「不再变」的格式 —— 格式号 `0x03` 与长度 65 字节写进契约并锁死。 |
| 验收 | ① Go↔TS 字段名双向锁定测试通过（改了任一边红）；② 密文版本号与长度两侧一致且被门禁锁住；③ `go test ./...` 全绿；④ 旧路径 `POST /api/v1/votes` 返回 **404**（不是 400 —— 端点不存在就该是 404） |
| 预估 | 2–3 天 |
| 依赖 | 无 |

> ⚠️ **破坏性变更**：投票提交的 URL 变了。旧的 `POST /api/v1/votes` 不再可达。

### T24 · 审计离线复算工具 `cmd/audit-recompute`

| 项 | 内容 |
|---|---|
| 问题 | 审计锚点（`merkle_root` + `anchor_hash`）v0.1.0 就有了，但**只有服务端能算**。第三方拿到「完整数据库 + 门限份额」后无法独立复算 —— 路线图 §1.2 明说「审计锚点 v0.1.0 已有，缺的是**脱离服务端的复算脚本**」。 |
| 证据 | `internal/usecase/tally.go:243`（`merkleBuilder.BuildRoot`）、`:252`（锚点哈希的拼法）、`internal/domain/entity/audit_anchor.go`（锚点只存根与哈希） |
| 改法 | ① 新增 `cmd/audit-recompute`：**只读数据库**，用**门限份额**还原计票私钥 → 逐票解密 → 重新计票 → 重建 Merkle 根 → 重算锚点哈希 → 与库里的锚点**逐字节比对**；② 走**真两段式**的路径拿份额（份额是管理端发放的，服务端只存份额的哈希）；③ **退出码非 0 表示复算不一致** —— 于是它能直接当门禁用（与 `cmd/loadtest` 的防重投结论同一口径）；④ **复算逻辑与服务端共用同一份代码**（`usecase` 的计票链路）—— 两边算法漂移会让复算结论失去意义。 |
| 验收 | ① 正常数据复算 → 锚点逐字节一致，退出码 0；② **篡改库里的一张票**（改一个字节）→ 复算不一致，退出码非 0；③ 缺份额 / 份额不足 → 明确报错，不是 500；④ 有单元测试与 e2e 证据 |
| 预估 | 2–3 天 |
| 依赖 | T23（接口冻结后端点形状才定） |

### T26 · 监控指标 `GET /metrics`

| 项 | 内容 |
|---|---|
| 问题 | 路线图 §2.2 把「监控指标」明确排在 v0.2.0 之外，属运维范畴。服务端现在只有结构化日志（zap）与 `/healthz` 的三个布尔字段 —— **没有任何可被采集的指标**。 |
| 证据 | `internal/infrastructure/http/server.go:40`（`/healthz` 只有 status/debug/eligibility）、全仓无 `prometheus` / `metrics` 命中 |
| 改法 | ① 引入 `github.com/prometheus/client_golang`；② 新增 `GET /metrics` 端点（Prometheus 文本格式）；③ 一组**与投票语义相关的指标**：投票提交数 / 被拒数（按错误码分）、凭证领取与盲签名数、资格证明校验数、计票发布数、HTTP 请求数与耗时直方图；④ 指标端点**只暴露计数与耗时，不暴露任何选票内容或身份**。 |
| 验收 | ① `GET /metrics` 返回 200 与 Prometheus 文本格式；② 投票提交后对应计数器 +1；③ **指标里不出现任何选票内容、nullifier、user_id**（有测试断言）；④ `go test ./...` 全绿 |
| 预估 | 1–2 天 |
| 依赖 | 无 |

### T25 · 公开端点限流

| 项 | 内容 |
|---|---|
| 问题 | 凭证领取与投票提交是**公开端点**（无鉴权），此前没有任何请求速率约束 —— 单个客户端可以把凭证池抽干或刷爆投票写入。路线图 §1.2 把「限流」排进 v0.3.0。 |
| 证据 | `internal/infrastructure/http/handler.go`（凭证领取 / 投票提交路由无任何前置约束）、全仓无 `ratelimit` / `golang.org/x/time` 命中 |
| 改法 | ① 新增 `internal/infrastructure/ratelimit`：**令牌桶**限流器，键 = `SHA-256(每进程随机盐 ‖ 路由组 ‖ 提案ID ‖ 客户端IP)`（加盐 → 键不可反推 IP）；② **路由组**分三档（credentials / votes / read），管理端、`/healthz`、`/metrics` **不限流**；③ 超限返回 **429** + `Retry-After` + `{"code":"RATE_LIMITED"}`；④ **release 模式强制开启**（显式 `rate_limit.enabled=false` 直接启动失败）；⑤ **可信代理**：`server.trusted_proxies` 默认空（gin 默认信任全部代理，是既有的 `client_ip` 可伪造问题），不配反代网段时伪造 `X-Forwarded-For` 能绕过限流；⑥ 空闲桶定时清理，避免内存无界增长。 |
| 验收 | ① 超限请求返回 429 且带 `Retry-After`；② **伪造 `X-Forwarded-For` 不能绕过限流**（有 HTTP 级用例）；③ 不同路由组额度独立；④ release 模式下显式关限流 → 启动失败；⑤ `go test ./...` 全绿 |
| 预估 | 2 天 |
| 依赖 | T26（限流计数要落在同一套指标里） |

> ⚠️ **限流是进程内的**：键里含**每进程随机盐**，多副本部署下额度不共享 ——
> **实际额度 = 配额的副本数倍**。要全局精确限流得换共享存储方案，那超出本版范围。
> ⚠️ **`cmd/loadtest` 会被限流挡住**：测容量前必须调高额度或临时关限流（仅 debug 模式可关）。

> **设计稿**：[`docs/t25-ratelimit-design.md`](./t25-ratelimit-design.md)（含额度推导、10+2 条不变量、与设计稿的偏离说明）。

---

## 4. 破坏性变更（升级前必须知道）

1. **投票提交端点换路径**：`POST /api/v1/votes` → **`POST /api/v1/proposals/:proposal_id/votes`**。
   请求体的 `proposal_id` 字段**删除**（它只从路径来）。CLI、门户、压测工具已同步。
   自建客户端请改路径并删掉请求体里的 `proposal_id`。
2. **密文格式 v3 定版**：格式号 `0x03`、长度 65 字节，从此**不再变**。
   （**不是**新的不兼容 —— v0.2.0 的密文本来就能用；这一条是**承诺**，不是变更。）

### 非破坏性但需要部署动作

- **限流新增 429**（T25）—— 线格式没变，**不构成破坏性变更**；但升级后公开端点
  可能返回 429，**客户端要能处理「稍后重试」**（CLI 与门户已加文案）。
- **新增配置项**：`server.trusted_proxies` 与 `rate_limit.*`（见
  [`docs/deployment-checklist.md`](./deployment-checklist.md) §3）。release 模式
  **默认开启限流** —— 额度按「500 人 / 5 分钟 / 共用一个出口 IP」推导，
  **部署方应按真实规模复核**（这正是 T27 要做的事）。

---

## 5. 出口标准

v0.3.0 可以发版的条件 —— 逐条都要有证据：

- [x] T23 的门禁测试全绿（Go↔TS 字段双向锁死、密文格式定版）
- [x] T24 的复算工具：正常数据一致（退出码 0）、篡改数据不一致（退出码非 0）
- [x] T26 的 `/metrics`：返回 200、投票提交后计数 +1、**不含任何选票内容**
- [x] T25 的限流：超限 429 + `Retry-After`、**伪造 `X-Forwarded-For` 不能绕过**、release 模式强制开启
- [x] T27 的容量报告：真 PostgreSQL + release 模式 + 真证明下跑出吞吐 / 延迟分位 / 错误率，
      并据此在部署清单里写出按投票规模的资源配置建议
- [x] T28 的门户 UI 自动化：`make web-e2e` 在**真浏览器**里跑完两条路径
      （领凭证 → 本地出票 → 提交 → 查状态 → 计票；资格重复领取被拒），CI 有独立 job
- [x] `go build ./...` / `go vet ./...` / `gofmt -l` 无输出
- [x] `go test ./...` 全绿；覆盖率门槛脚本通过（逐包分档）
- [x] 前端 lint / tsc / vitest 全绿
- [x] `docs/api-contract-v1.md` 与实现**逐字段一致**（门禁测试保证）
- [x] `docs/techspec.md` / `README.md` / `docs/project-map.md` 的接口口径已同步
- [x] `CHANGELOG.md` 写明 §4 的破坏性变更

> 全部勾选的实测日期：**2026-10-26**。覆盖率：门槛口径 **80.86%**（合计门槛 77，17 个受控包）、
> 全仓口径 **81.8%**（含跨包覆盖）。前端：19 个测试文件 / **159** 条用例全绿；
> 门户 e2e：1 条用例 / **2.2~2.5 分钟**通过（真浏览器 + 真服务端 + 真资格证明 + 真计票）。

---

## 6. 落地状态

| 任务 | 状态 | 证据 |
|---|---|---|
| T23 三处接口冻结 + 密文格式定版 | ✅ 已落地 | 提交 `d410b5b`（接口收敛）+ `c148d25`（契约文档 + 门禁）；门禁 **7 条**用例在 `tests/architecture/api_contract_test.go`；旧路径 404 的 HTTP 级断言在 `internal/infrastructure/http/vote_endpoint_test.go` |
| T24 审计离线复算 | ✅ 已落地 | 提交 `d418b7e`；`cmd/audit-recompute` + 共用的 `internal/tally` / `internal/shares`；真密码学端到端用例 `cmd/audit-recompute/main_test.go` 覆盖五种情形（一致 / 篡改计票 / 篡改选票 / 外来份额 / 份额不足） |
| T26 监控指标 | ✅ 已落地 | 提交 `11951a3`；`GET /metrics`（要 admin 令牌）+ `internal/infrastructure/metrics`；HTTP 级用例 `internal/infrastructure/http/metrics_endpoint_test.go`（401 / 200 / 计数 +1 / 不含选票内容）与 `internal/infrastructure/http/metrics_vote_accepted_test.go` |
| T25 公开端点限流 | ✅ 已落地 | 提交 `c60068b`；`internal/infrastructure/ratelimit`（令牌桶 + 盐化键）+ `middleware/rate_limit.go`（429 / `Retry-After` / `RATE_LIMITED`）；HTTP 级用例 `internal/infrastructure/http/rate_limit_test.go`（伪造 `X-Forwarded-For` 不能绕过 / 可信代理生效 / 路由组额度独立）；跨语言错误码门禁 `tests/architecture/error_code_test.go` |
| T27 容量规划 | ✅ 已落地 | 压测工具支持真 prover 与资格领取（`cmd/loadtest`）；脚本 `scripts/capacity-run.sh`（真 PostgreSQL + release 模式）；报告 `docs/capacity-report-v0.3.0.md`（吞吐 / 延迟分位 / 错误率 / 资源配置建议） |
| T28 Playwright 门户 UI 自动化 | ✅ 已落地 | `web/e2e/portal.spec.ts`（Playwright，**真浏览器**）跑两条路径：领凭证 → 本地出票 → 提交 → 查状态 → 计票；资格路径（导入资格文件 → 匿名领取 → 重复领取被拒）；环境脚本 `scripts/web-e2e.sh`（`make web-e2e`，自己备好两套密钥 / 名单 / release 模式服务端 / 凭证池 / 解密会话 / 门户 dev server）；CI 独立 job `web-e2e` |

---

## 7. 决策记录

| # | 决策 | 结论 |
|---|---|---|
| **V3-D1** | v0.3.0 的范围 | ✅ **接口冻结 + 审计离线复算 + 监控指标 + 公开端点限流 + 容量规划 + 门户 UI 自动化**。**2026-10-25**（限流于同日补上），**2026-10-26**（容量规划 T27 与 Playwright T28 一并补齐） |
| **V3-D2** | 投票提交接口往哪个形状收敛 | ✅ **路径带 `proposal_id`**（与 `proposals/:id/...` 其余端点一致），请求体不再带它 —— 同一个提案 ID 只有**一个**来源，客户端不可能搞混。**2026-10-25** |
| **V3-D3** | 监控指标用什么 | ✅ **Prometheus**（`client_golang`）—— 生态标准、部署方已有采集器；零依赖手写格式没有收益。**2026-10-25** |
| **V3-D4** | 审计复算工具的代码来源 | ✅ **与服务端共用同一份计票链路** —— 两边算法漂移会让复算结论失去意义（复算工具要证明的是「服务端没算错」，不是「服务端和工具犯同一个错」）。**2026-10-25** |
| **V3-D5** | 限流的计数键 | ✅ **按 IP + 端点分组** —— 键 = `SHA-256(每进程随机盐 ‖ 路由组 ‖ 提案ID ‖ 客户端IP)`。加盐 → 键不可反推 IP；盐每进程随机 → 状态不跨进程共享。**2026-10-25** |
| **V3-D6** | release 模式要不要强制开限流 | ✅ **强制开启**；显式 `rate_limit.enabled=false` **启动失败**（不用「不配就失败」是为了不让存量 release 部署升级即宕机）。**2026-10-25** |
| **V3-D7** | 限流器用什么实现 | ✅ **`golang.org/x/time/rate`** v0.15.0 —— 标准库的替代品、生态标准。**2026-10-25**（选 v0.15.0 而非 v0.16.0：后者要求 go 1.26.0，会抬高使用者的最低 Go 版本） |
| **V3-D8** | 429 算不算破坏性变更 | ✅ **不算** —— 冻结的是**线格式**，429 是新增状态码（加法）。但**必须**写进契约文档与部署清单。**2026-10-25** |

---

## 8. 发版说明正文与发版步骤

### 8.1 发版说明正文（tag 注解，可直接用）

release workflow 把**注解 tag 的说明**原样当作 GitHub Release 正文（不是提交列表）。
下面这段就是可直接用的正文 —— 关键是三件事说清：**这版能干什么、破坏性变更、部署方必须做什么**。

```text
v0.3.0 —— 接口不再动

这一版不新增投票能力，而是把「已经能投」这件事变成**可运维、可审计、可冻结**：
接口契约定版（从此只增不改）、审计能脱离服务端复算、公开端点有限流、有监控指标、
有带数字的容量报告，门户的浏览器路径第一次有了自动化回归。

⚠️ 破坏性变更（升级前必读）
- 投票提交端点换路径：POST /api/v1/votes → POST /api/v1/proposals/:proposal_id/votes，
  请求体里的 proposal_id 字段删除（它只从路径来）。CLI、门户、压测工具已同步；
  自建客户端请改路径并删掉该字段。
- 密文格式 v3 定版：0x03 + 两个压缩点（65 字节），从此不再变。
  （不是新的不兼容 —— v0.2.0 的密文本就能用；这一条是承诺。）

部署必做项
- 升级前先跑迁移：新增 credential_claims.reserved_at 列（凭证槽位的预留时间，TTL 10 分钟）。
  不迁移的话槽位占位机制不工作，并发领取会互相抢同一个槽位。
- release 模式强制开限流：rate_limit.enabled=false 会让服务启动失败。
  额度按「来源 + 路由组」分三档，部署方应按真实规模复核。
- 部署在反向代理后面必须配 server.trusted_proxies：gin 默认信任所有代理，
  不配的话伪造 X-Forwarded-For 就能绕过限流。
- 数据库连接池建议 16/16：database/sql 的默认值只保留 2 条空闲连接，
  高并发下每个请求都要重开连接（实测把吞吐打掉一个量级）。
- 详见 docs/deployment-checklist.md（§3 配置、§4.1 资源配置、§8 上线后自检）。

新增能力
- GET /metrics：Prometheus 指标（要 admin Bearer 令牌），只含计数与耗时，不含任何选票内容或身份。
- cmd/audit-recompute：拿计票份额脱离服务端重算计票与锚点，与库内已发布的结果逐字节比对，
  不一致时退出码非 0（可当门禁用）。
- 公开端点限流：超限返回 429 + Retry-After。
- 容量报告（docs/capacity-report-v0.3.0.md）：真 PostgreSQL 上的吞吐/延迟分位/错误率实测，
  以及按投票规模的资源配置建议。

本版不承诺的事
- 不承诺「能用于有规模的真实投票」—— 面向真实规模投票是 v0.4.0 的命题。
- 限流是进程内的：多副本部署时实际额度是配额的副本数倍。
- 部署形态（真域名、反向代理）仍要人工点一遍浏览器验收；
  自动化验的是「接线没断」，不是「部署形态没问题」。
```

打 tag（注解 tag，正文从文件读，避免 shell 引号地狱）：

```bash
git tag -a v0.3.0 -F release-notes-0.3.0.txt   # 文件内容就是上面那段
git push origin feat/v0.3.0-contract-and-audit v0.3.0
```

> ⚠️ `release-notes-0.3.0.txt` 是**临时文件**，别提交进仓库（已在 `.gitignore` 里）。

### 8.2 发版步骤（照着走）

| # | 步骤 | 命令 / 说明 |
|---|---|---|
| 1 | 版本号 | `VERSION` → `0.3.0`（唯一真相源；`web/package.json` 由流水线注入，不手改） |
| 2 | 变更日志 | `CHANGELOG.md` 的 `[未发布]` → `## [0.3.0] — 2026-10-26`（正文沿用 §4 的破坏性变更） |
| 3 | 门禁 | `go build ./...` / `gofmt -l` / `go vet ./...` / `go test ./...` / 覆盖率脚本 / 前端 lint+tsc+vitest / `make web-e2e` |
| 4 | 写发版说明 | 把 §8.1 那段存成 `release-notes-0.3.0.txt`（临时文件，gitignore 掉了） |
| 5 | 打注解 tag | `git tag -a v0.3.0 -F release-notes-0.3.0.txt`（**本地**，不推） |
| 6 | 推分支 + 推 tag | `git push origin feat/v0.3.0-contract-and-audit v0.3.0` —— **推 tag 就发版了** |
| 7 | 看流水线 | release workflow 的 8 个 job（verify / build×5 / web / release）全绿才算发成功 |
| 8 | 核验产物 | `gh release download` 一次，核 sha256 与门户包里的 prover 资源 |
| 9 | 合并回 main | 开 PR 合回 `main`（merge commit 保留全部提交），删分支 |

> ⚠️ **第 6 步是不可逆的对外动作**：推 tag 会立刻触发发布流水线并创建公开的
> GitHub Release。动手前确认第 1~5 步都已完成、且正文已定稿。

### 8.3 本版相对 v0.2.0 的流水线差异

**`release.yml` 一行没改** —— v0.2.0 已经在 CI 里真跑通过（run `40185592513`，8 个 job
全绿：verify / build×5 平台 / web / release）。本版只改了 `ci.yml`（新增 `web-e2e` job），
它**不参与发布流水线**。

发布产物与 v0.2.0 同构：**5 个平台的 `nullifier-<os>-<arch>.zip` + 各自的 `.sha256`，
外加 `nullifier-web-0.3.0.zip` + `.sha256`**（门户包里带 `ballot-prover.wasm` 与
`wasm_exec.js`；两把 proving key 是秘密材料，默认不进包，部署方自备或用 secret 注入）。

### 8.4 发布结果（2026-10-26）

**v0.3.0 已公开发布**：[GitHub Release v0.3.0](https://github.com/jiangfire/nullifier/releases/tag/v0.3.0)

| 项 | 值 |
|---|---|
| tag | `v0.3.0`（**注解 tag**）@ `6dd3ed4`（tag 对象）/ `47a3925`（指向的提交） |
| 分支 | `feat/v0.3.0-contract-and-audit` → `origin`（发版时本地 HEAD = 远端 = tag 指向的提交，三者一致） |
| 合并 | PR [#5](https://github.com/jiangfire/nullifier/pull/5) → `main` @ `4d3bac3`（**merge commit**，15 笔提交全部保留）；合并后分支已删除（本地与远端都删了） |
| 流水线 | release run **40321361924**，**8 个 job 全绿**（`verify` / `build` ×5 平台 / `web` / `release`），12:17:11 → 12:21:24 UTC |
| 产物 | **12 个**：5 个平台的 `nullifier-<os>-<arch>.zip` + 各自的 `.sha256`，外加 `nullifier-web-0.3.0.zip` + `.sha256` |

**发布出去的产物已下载核验**（`gh release download`）：

- `nullifier-web-0.3.0.zip` —— 2,762,232 字节，**sha256 与随附的 `.sha256` 逐字节一致**
  （`184a72b0ca16ba311b6f708533b33a986615fa73b660efeedeccb1f05df2d07d`）
- 包内含 `ballot-prover.wasm`（11,651,679 字节，magic `0061736d`，合法 WASM 模块）与
  `wasm_exec.js`（16,992 字节）—— 门户**开箱能出证明**
- ⚠️ **两把 proving key 不在包里**（秘密材料，流水线只发警告）—— 部署方必须自己补
  （部署清单 §1/§6）
- **发布页正文与 §8.1 逐字一致**（已下载比对）

**`web-e2e` job 第一次在 CI 里真跑**（PR #5 的 run `40321461256`）：真浏览器里门户的
两条路径全过 —— 领凭证 → 本地出票 → 提交 → 查状态 → **计票还原出所选的选项**，
以及「同一份资格重复领取被拒」。用例本身 **2.2 分钟**通过，服务端日志里有
`vote submitted`。
⚠️ **但这个 job 很慢**：浏览器下载（120 MiB）+ 系统依赖安装在 GitHub runner 上花了
**12 分钟** —— 整轮 job 约 16 分钟。已给浏览器本体加缓存（`~/.cache/ms-playwright/`，
key 按 lockfile 哈希），后续运行命中缓存就能省掉这段下载；⚠️ **命中与否尚未实测**
（加缓存后的第一次运行必然是 cache miss，要等它把浏览器写进缓存，下一次才谈得上命中）。
缓存没命中也只是重新下载，**不会让 job 失败**。

---

## 相关文档

| 文档 | 内容 |
|---|---|
| [`docs/roadmap.md`](./roadmap.md) | 产品路线图与任务卡（§1.2 的 v0.3.0 推演已升格为正式里程碑） |
| [`docs/api-contract-v1.md`](./api-contract-v1.md) | **冻结的接口契约**（三处接口逐字段） |
| [`docs/release-plan-v0.2.0.md`](./release-plan-v0.2.0.md) | v0.2.0 发版计划（上一版，已发布） |
| [`docs/techspec.md`](./techspec.md) | 当前实现的接口与流程口径 |
| [`docs/deployment-checklist.md`](./deployment-checklist.md) | 生产部署清单 |
