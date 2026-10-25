# T25 · 限流设计方案

> 配套文档：[`docs/roadmap.md`](./roadmap.md)（任务卡）、
> [`docs/techspec.md`](./techspec.md)（当前实现口径）、
> [`docs/api-contract-v1.md`](./api-contract-v1.md)（**冻结的接口契约**）、
> [`docs/deployment-checklist.md`](./deployment-checklist.md)（生产部署清单）
>
> 制定日期：2026-10-25
> 状态：**设计已拍板，待实现**

---

## 1. 问题到底是什么

现在**没有任何限流**。所有公开端点都是裸奔的：

| 端点 | 攻击面 |
|---|---|
| `POST /api/v1/proposals/:id/credentials/claim` | 一个脚本可以把凭证池领空 |
| `POST /api/v1/proposals/:id/credentials/blind-sign` | 盲签名是 RSA 模幂运算，可被当计算资源用 |
| `POST /api/v1/proposals/:id/votes` | **每张票都要验一次 Groth16 proof** —— 最贵的操作，验签算力被打满 = 服务不可用 |
| `GET /api/v1/proposals/:id/results` 等读端点 | 公开信息被抓取 |

三条推论决定了后面所有取舍：

1. **最贵的不是「领票」，是「投票」。** 一张票的验签（Groth16 验签 + 密文合法性检查）
   比一次盲签名贵几个数量级。限流的第一目标是**保护验签算力**，第二目标才是防刷。
2. **「把池子领空」这个威胁，在 release 模式下已经被一人一票压住。**
   开启资格校验后，只有名单上的人能领 —— 池子最多被领到名单人数。
   所以限流**不是**一人一票的替代品，两者是叠加关系。
3. **限流是唯一能兜住「非名单内攻击者」的那一层。** 名单管的是「谁能领」，
   限流管的是「单个来源能打多快」。

### 1.1 一个必须先解决的坑：客户端 IP 是可以伪造的

gin v1.12.0 的**默认**行为是：

```go
// gin.go:225
trustedProxies: []string{"0.0.0.0/0", "::/0"},
// gin.go:215
RemoteIPHeaders: []string{"X-Forwarded-For", "X-Real-IP"},
```

也就是说 —— **信任所有代理**，且 `c.ClientIP()` 会取 `X-Forwarded-For` 的**第一个**值。
后果有两个：

- 朴素地拿 `c.ClientIP()` 做限流键，攻击者**每个请求换一个 `X-Forwarded-For` 就能完全绕过**；
- 这也是仓库里**既有的**一个弱点：`middleware/request_logger.go` 记的 `client_ip`
  现在也是可伪造的（记下来的不一定是真实来源）。

> ⚠️ 所以 T25 必须同时引入「可信代理」配置，否则限流只是**看起来**存在。
> 这一条是本设计里**最容易被跳过、也最致命**的部分。

---

## 2. 方案对比

### 2.1 计数键

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| A | **按 IP + 端点分组** | 简单、可运维；不同提案互不干扰 | NAT 后所有人共享一个桶 → 误伤 |
| B | IP + 凭证槽位双键 | NAT 友好（投票阶段可用槽位） | 领取阶段**还没有**槽位，仍只能靠 IP；复杂度翻倍 |
| C | 全局令牌桶 | 只保服务端总量 | 无法防定向刷；一个提案的流量能拖垮另一个 |

**决策：方案 A**（2026-10-25，用户确认）。理由：

- 领取阶段（claim / blind-sign / attest）**没有身份也没有槽位**，只能用来源标识 ——
  方案 B 在最需要保护的那一段并不比 A 强。
- 误伤问题**用「额度可配置 + 默认宽松」解决**，不用用复杂度换。
  真实部署规模决定最终值（§5.3）。

### 2.2 存储

| 方案 | 说明 | 结论 |
|---|---|---|
| 进程内内存 | 每个实例一份 | ✅ **采用**。多副本下额度按副本数放大 —— 写进文档，运维自己权衡 |
| Redis 等外部存储 | 全局统一额度 | ❌ 引入新基础设施依赖，超出本版范围 |

### 2.3 限流器实现

| 方案 | 说明 | 结论 |
|---|---|---|
| `golang.org/x/time/rate` | 成熟的令牌桶，并发安全 | ✅ **采用**（`go get golang.org/x/time@v0.16.0` 定点加，不跑 `go mod tidy`） |
| 自己手写令牌桶 | 约 60 行 | ❌ 手写并发限流器是典型的「看着对、边界错」 |

> ⚠️ `go mod tidy` 会顺手把 go 指令从 1.25.4 抬到 1.26.0 并升级无关依赖 ——
> 加依赖一律用 `go get <包>@<版本>`。

---

## 3. 完整设计

### 3.1 键的构成

```
键 = SHA-256( 每进程随机盐 ‖ 路由组 ‖ 提案ID ‖ 客户端IP )
```

四个部分各自的理由：

| 部分 | 为什么在里面 |
|---|---|
| **每进程随机盐** | 让内存里的键**不可还原成 IP**。限流器状态是常驻内存的（比日志更容易被 dump / 备份抓到），里面存着明文 IP 是不合适的。盐每进程随机 → 重启即失效、跨副本不可关联 |
| **路由组** | 领取 / 投票 / 读三种流量额度不同，必须分开计数 |
| **提案ID** | 多个提案同时开放投票时，A 提案的投票人不该吃掉 B 提案的额度。路径里没有提案的端点（如 `GET /proposals`）留空 |
| **客户端IP** | 唯一能区分来源的标识（见 §1.1 的可信代理约束） |

> **与匿名纪律的一致性**：仓库已经有一条硬规矩 —— 匿名领取凭证时**不记录来源 IP**
> （`middleware/request_logger.go` 的 `anonymousPathParts`）。限流器如果用明文 IP 做键，
> 等于在内存里另开了一份「领取者 ↔ 凭证」的关联线索。哈希键把这个口子堵上。

### 3.2 路由组与默认额度

| 组 | 覆盖的端点 | 默认 rps | 默认 burst | 依据 |
|---|---|---|---|---|
| `credentials` | `eligibility-challenge` / `claim` / `blind-sign` / `attest` | 10 | 40 | 一次完整领取走 **3～4** 个请求；burst 40 容得下同 IP 10 个并发投票人 |
| `votes` | `POST /proposals/:id/votes` | 10 | 40 | 每人 1 张票；**验签算力在这里**，宁可紧一点 |
| `read` | 提案列表 / `vote-status` / `election-key` / `results` / `audit-anchors` | 50 | 100 | 只读，攻击面小 |

**默认值的推导**（可复算，运维调额时照这个思路）：

一次完整投票的请求序列是
`GET election-key` → `POST eligibility-challenge` → `POST claim` → `POST blind-sign`
→ `POST attest` → `POST votes` → `POST vote-status`。

- `credentials` 组每人 3 次（不开资格校验）或 4 次（开）；
- `votes` 组每人 1 次；
- `read` 组每人 2 次。

**500 人 / 5 分钟 / 共用一个出口 IP**（机构集中投票的典型形态）：

- credentials 需要 2000 次 → `rps 10` 下约 200 秒 + burst 40 ≈ **3.5 分钟**，落在窗口内 ✓
- votes 需要 500 次 → `rps 10` 下约 **50 秒** ✓

**默认偏宽松是有意的**：宁可放过一次攻击，也不要在真实投票窗口里把投票人挡在门外。
额度是配置项，按真实部署规模调。

### 3.3 客户端 IP 的可信性

新增配置 `server.trusted_proxies`（CIDR 列表），启动时 `engine.SetTrustedProxies(cfg.TrustedProxies)`。

| 取值 | 行为 |
|---|---|
| `[]`（**默认**） | 不信任任何代理 → `c.ClientIP()` 取 `RemoteAddr` 的地址，**不可伪造** |
| `["10.0.0.0/8", ...]` | 只信任这些网段发来的 `X-Forwarded-For` |

**默认不信任任何代理**，理由：默认信任 = 默认可伪造 = 限流形同虚设。
部署在反向代理后面时**必须**显式配置，否则所有请求会被算到代理的同一个 IP 上
（fail-closed：偏严而不是偏松）。

> 这同时修掉了 §1.1 提到的既有问题：日志里的 `client_ip` 也不再可伪造。

### 3.4 响应

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 3
Content-Type: application/json

{"error":"rate limit exceeded","code":"RATE_LIMITED"}
```

- 状态码 `429`，响应体沿用仓库现有的错误格式（`error` + `code`）；
- `Retry-After` 是**秒数**（向上取整，最小 1），取「下一个令牌可用还需多久」；
- 错误码 `RATE_LIMITED` —— 这是**新的**机器可读码。

### 3.5 指标

新增 `anonvote_rate_limited_total{route}`。

- 标签 `route` **已在**允许出现的标签名白名单里（`method` / `route` / `status` / `result` / `code`）
  → **不改白名单**，`internal/infrastructure/metrics/metrics_test.go` 仍然绿。
- `route` 用**路由模板**（`/api/v1/proposals/:proposal_id/votes`）而不是原始路径 ——
  否则每个提案 ID 都会变成一个标签值，标签集会爆。
- 限流器**自己的内存状态**不落库、不进日志、不进任何指标。

### 3.6 开关与强制策略

| 模式 | 行为 |
|---|---|
| release / prod / production | `rate_limit.enabled` **默认 true**；显式设成 `false` → **启动失败** |
| debug / dev / development | 默认 false（方便联调），可显式打开 |

**为什么是「默认开 + 显式关才失败」，而不是「不配就失败」**：

「一人一票」用「必须成对出现」是因为漏配会让**匿名性**失效 —— 那种情况必须硬失败。
限流不同：漏配的后果是「没限流」，而硬失败会让所有存量 release 部署**升级即宕机**。
所以取「默认开」这个安全默认，只把**明确的关闭意图**当成错误。

> 额度值配错（rps ≤ 0、burst ≤ 0）一律**启动失败** —— 配错比不配更危险。

### 3.7 不变量清单（实现时逐条对照）

1. 限流器的内存状态里**不出现明文 IP**（键是加盐哈希）。
2. 限流状态**不落库、不进日志、不进指标**。
3. 客户端 IP **不可伪造**（`server.trusted_proxies` 默认空 = 不信任任何代理）。
4. `route` 标签用**路由模板**，不用原始路径。
5. 指标标签名**不超出**白名单。
6. 管理端端点**不**限流（已有 Bearer 令牌鉴权，128 位令牌暴力不可行）。
7. `/healthz` 与 `/metrics` **不**限流（运维端点被限会直接影响可观测性）。
8. 限流中间件**在指标中间件之内** —— 否则 429 不会进 `anonvote_http_requests_total{status="429"}`。
9. 限流中间件**不吞掉**已写出的响应（`c.Writer.Written()` 为真时不介入）。
10. 桶有**空闲清理** —— 无限增长的 map 是内存泄漏。

---

## 4. 验收口径

对齐路线图 T25 的验收：

- [ ] 同一 IP 超过阈值后返回 **429** + `Retry-After` 头 + `code: "RATE_LIMITED"`
- [ ] **不同 IP 互不影响**（一个 IP 被限，另一个照常）
- [ ] 限流触发能从 `/metrics` 观察到（`anonvote_rate_limited_total` 递增）
- [ ] 429 计入 `anonvote_http_requests_total{status="429"}`
- [ ] release 模式下 `rate_limit.enabled=false` → **启动失败**
- [ ] 额度 ≤ 0 → **启动失败**
- [ ] `X-Forwarded-For` 伪造**不能**绕过限流（默认不信任代理时）
- [ ] `go test ./...` 全绿；覆盖率门槛脚本通过
- [ ] `docs/api-contract-v1.md` / `docs/techspec.md` / `docs/deployment-checklist.md` 口径已同步

---

## 5. 工作量拆解

### 5.1 步骤

| # | 步骤 | 产出 | 验证 |
|---|---|---|---|
| 1 | 加依赖 | `go.mod` 加 `golang.org/x/time` | `go build ./...` |
| 2 | 限流器核心 | `internal/infrastructure/ratelimit/` | 包内单测 |
| 3 | 配置 | `internal/config/config.go` + `configs/*.yaml` | 配置单测（release 强制、额度校验） |
| 4 | 中间件 | `internal/infrastructure/http/middleware/rate_limit.go` | 中间件单测 |
| 5 | 指标 | `internal/infrastructure/metrics/metrics.go` | 现有白名单测试仍绿 |
| 6 | 可信代理 | `internal/infrastructure/http/server.go` | 伪造 XFF 不生效的用例 |
| 7 | 装配与路由分组 | `internal/di/` + `handler.go` | 端点级用例：超限 429、不同 IP 互不影响 |
| 8 | 文档 | 见 §5.4 | 人工核对口径 |

### 5.2 新增与改动清单

**新增**
- `internal/infrastructure/ratelimit/ratelimit.go` —— 限流器（加盐键 + 桶 + 空闲清理）
- `internal/infrastructure/ratelimit/ratelimit_test.go`
- `internal/infrastructure/http/middleware/rate_limit.go` —— gin 中间件
- `internal/infrastructure/http/middleware/rate_limit_test.go`
- `internal/infrastructure/http/rate_limit_test.go` —— 端点级（超限 / 不同 IP 互不影响）

**改动**
- `internal/config/config.go` —— `rate_limit.*` 与 `server.trusted_proxies`
- `internal/infrastructure/metrics/metrics.go` —— `RecordRateLimited`
- `internal/infrastructure/http/middleware/error_handler.go` —— `RATE_LIMITED` 的文案（**不进 MapError**，见下）
- `internal/infrastructure/http/server.go` —— `SetTrustedProxies` + 限流器装配
- `internal/infrastructure/http/handler.go` —— 路由挂限流中间件
- `internal/di/providers.go` + `wire.go` —— 装配
- `scripts/coverage-thresholds.txt` —— 加 `internal/infrastructure/ratelimit`

> **错误码为什么不进 `MapError`**：`MapError` 映射的是**领域错误**（`usecase.Err*`）。
> 限流是**传输层**的保护措施，不是业务失败 —— 把它塞进领域错误集会污染 usecase 的语义。
> 所以限流中间件**自己**写 429 响应（格式与 `MapError` 保持一致），
> `publicErrorMessage` 补一条 `RATE_LIMITED` 文案即可。

### 5.3 配置项

```yaml
server:
  # 可信代理网段（CIDR，空格分隔）。默认空 = 不信任任何代理，
  # 客户端 IP 取连接来源地址，伪造 X-Forwarded-For 无效。
  # 部署在反向代理后必须配置，否则所有请求算到代理同一个 IP。
  trusted_proxies: []

rate_limit:
  # release 模式默认 true；显式设成 false 会让服务启动失败。
  enabled: true
  credentials:
    rps: 10
    burst: 40
  votes:
    rps: 10
    burst: 40
  read:
    rps: 50
    burst: 100
```

环境变量（viper 的 `.` → `_` 替换规则）：
`SERVER_TRUSTED_PROXIES` / `RATE_LIMIT_ENABLED` / `RATE_LIMIT_CREDENTIALS_RPS` /
`RATE_LIMIT_CREDENTIALS_BURST` / `RATE_LIMIT_VOTES_RPS` / `RATE_LIMIT_VOTES_BURST` /
`RATE_LIMIT_READ_RPS` / `RATE_LIMIT_READ_BURST`

### 5.4 文档同步范围

| 文档 | 要写什么 |
|---|---|
| `docs/techspec.md` | 限流章节：分组、额度、429 格式、配置项 |
| `docs/api-contract-v1.md` | **429 属于「加法」，不构成破坏性变更**（决策 V3-D5）；客户端要处理 429 与 `Retry-After` |
| `docs/deployment-checklist.md` | 必配项（`trusted_proxies`）、自检项（超限返回 429） |
| `docs/project-map.md` | 新包、中间件顺序陷阱、路由分组表 |
| `README.md` | 配置表 + 环境变量 |
| `CHANGELOG.md` | 新增（限流）+ 变更（`trusted_proxies` 是新配置项） |
| `docs/roadmap.md` | T25 落地核对 + 决策 V3-D5 |
| `AGENTS.md` | 限流约定、指标白名单、覆盖率门槛 |
| `configs/*.yaml` | 配置项与注释 |

---

## 5.5 落地核对（2026-10-25）

| 步骤 | 状态 | 落地位置 |
|---|---|---|
| 1 加依赖 | ✅ | `go get golang.org/x/time@v0.15.0`（**不跑 `go mod tidy`** —— 它会把 go 指令抬到 1.26.0）。选 v0.15.0 而不是最新的 v0.16.0：后者要求 go 1.26.0，会**抬高使用者的最低 Go 版本** |
| 2 限流器核心 | ✅ | `internal/infrastructure/ratelimit/ratelimit.go` + 同包单测（11 条） |
| 3 配置 | ✅ | `internal/config/config.go`（`rate_limit.*` + `server.trusted_proxies`）+ `internal/config/rate_limit_test.go`（10 条） |
| 4 中间件 | ✅ | `internal/infrastructure/http/middleware/rate_limit.go` + 同包单测（8 条） |
| 5 指标 | ✅ | `internal/infrastructure/metrics/metrics.go`（`RecordRateLimited`） |
| 6 可信代理 | ✅ | `internal/infrastructure/http/server.go`（`SetTrustedProxies`）+ 端点级用例（伪造 XFF 不生效） |
| 7 装配与路由分组 | ✅ | `internal/di/providers.go`（`NewRateLimits`）+ `handler.go`（`RegisterRoutes` 分组挂限流） |
| 8 文档 | ✅ | 见 §5.4 |

### 与设计稿的偏离

1. **加了一条设计稿里没有的东西：跨语言错误码门禁**
   （`tests/architecture/error_code_test.go`）。
   原因是落地时发现：服务端加了 `RATE_LIMITED`，而门户的
   `error-messages.ts` 不认识它 —— **两边各自的自测都是绿的**，
   用户看到的是一句英文原文。这正是「两边各拿自己的常量自证是测不出
   这类问题」的典型，于是把它变成门禁：Go 的 `MapError` 码集合 ↔ TS 的
   `PortalErrorCode` 联合类型成员集合，双向比对。
   门禁已实证有效（删掉 TS 侧一个码 → 立刻变红并指出差集）。

2. **顺手补齐了门户本来就缺的一个码**：`CREDENTIAL_POOL_EMPTY`
   （凭证池领完）此前服务端有、门户没有 —— 同一类问题，一并修了。

3. **桶的清理挂在服务生命周期上**（`Server.Run` / `Server.Serve` 启停
   `StartCleanup` goroutine），而不是限流器自己起 —— 限流器不该管
   进程的生死。

4. **限流键的构成与设计稿的写法略有不同**：盐在限流器内部（每个限流器
   一份），组 / 提案 / 来源由中间件拼好后再交给限流器哈希。效果等价，
   但少了一次跨包传递。

5. **CLI 与门户都要能读懂 429**：CLI 的 `httpError.Error()` 单独给
   `RATE_LIMITED` 一句「等一等就好」的话（别让它看起来像服务端坏了），
   门户在 `error-messages.ts` 里补上中文文案。

### 新增的不变量

11. **错误码集合两边必须一致** —— 由 `tests/architecture/error_code_test.go`
    守着。
12. **每个错误码都要有中文文案** —— 两侧各钉一条（Go 侧在门禁里，
    TS 侧在 `web/src/lib/error-messages.test.ts`）。

---

## 6. 决策点（✅ 已拍板，2026-10-25）

| # | 决策 | 结论 |
|---|---|---|
| **V3-D5** | 限流的计数键 | ✅ **按 IP + 端点分组**。理由：领取阶段没有身份也没有槽位，双键方案在最需要保护的那一段并不更强；误伤问题用「额度可配置 + 默认宽松」解决 |
| **V3-D6** | release 模式要不要强制限流 | ✅ **强制**。默认开启，显式 `false` 启动失败。不用「不配就失败」是为了不让存量 release 部署升级即宕机 |
| **V3-D7** | 限流器实现 | ✅ **用 `golang.org/x/time/rate`**。手写并发令牌桶是典型的「看着对、边界错」 |
| **V3-D8** | 429 算不算破坏性变更 | ✅ **不算**。冻结的是线格式（字段名 / 语义 / 路径 / 密文格式），429 是**新增的一种状态码**，请求与响应的字段、路径全都不变 —— 属于加法。但**必须**写进契约文档与部署清单：客户端要处理 429 与 `Retry-After` |

---

## 7. 已知不做的事

| 不做 | 理由 |
|---|---|
| 分布式限流（Redis） | 引入新基础设施依赖；多副本额度放大写进文档，运维自行权衡 |
| 按用户 / 凭证维度限流 | 领取阶段没有身份；一人一票由资格证明兜底 |
| 管理端限流 | 已有 Bearer 令牌鉴权，128 位令牌暴力不可行 |
| 滑动窗口 / 精确计数 | 令牌桶够用；滑动窗口的内存与实现复杂度不划算 |
| 自适应额度 | 需要真实流量基线，属 T27 容量规划的产物 |
