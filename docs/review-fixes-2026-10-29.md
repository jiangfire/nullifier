# v0.4.0 进入测试前评审与修复清单（2026-10-29）

> 这份文档是 2026-10-29 对 `main`（v0.4.0 发布后，`93897e2`）整体评审的**修复跟踪文档**。
> 评审方法：全量门禁本地实测 + 三个独立深审（v0.4.0 新代码 / 服务端核心安全面 / 门户与跨语言契约），
> 所有 Critical / Required 发现都经过人工抽查核实（有代码证据，非臆测）。
> 用法：**P0 项进入测试前必须修完**；P1 项与测试并行；P2 项排期。
> 每项修完在「状态」列回填，并在 §4 修复记录里登记证据。

## 0. 门禁基线（2026-10-29 实测，全部绿）

| 门禁 | 结果 |
|---|---|
| `go test ./...`（29 个包，含 architecture 门禁 / e2e / integration） | ✅ 全绿 |
| 覆盖率门槛（`check-coverage.sh`，逐包分档） | ✅ 合计 81.42%；⚠️ `internal/usecase` 77.74% 距门槛 77 仅 0.74pp，**本批 usecase 改动不得降低其覆盖率** |
| `go build` / `go vet` / `gofmt -l` | ✅ 干净（⚠️ 本机裸 shell `CGO_ENABLED=0`，直接跑 go build/vet 会报 `db.go:182` 假错误；用 make 目标或显式导出 `CGO_ENABLED=1`） |
| 前端 eslint / tsc / vitest（160 用例） | ✅ 全绿 |

---

## 1. P0 —— 进入测试前必修（本轮实施）

### F1 · 分级令牌部署下，临时 admin 令牌顶掉配置的 admin 令牌并打进日志 【Critical】

- **证据**：`internal/config/admin_token.go:14-31` 只检查 `cfg.AdminBearerToken` 是否为空，不看 `cfg.AdminTokens`；`internal/infrastructure/http/server.go:127-129` 在 `AdminBearerToken` 非空时用它覆盖 admin 档；`cmd/server/main.go:96` 无条件调用，`:127-132` 把生成的明文令牌以 WARN 打进日志。
- **影响**：release 模式明确允许只配 `server.admin_tokens`（`config.go:336-341`「旧单值与分级令牌任一都算显式配置」）。这种**文档推荐的分权部署形态**下，每次启动生成随机令牌 → 配置好的 admin 令牌静默失效（重启即换）→ 活的 admin 凭证进日志。operator/auditor 两档正常，服务「看起来健康」，最坏要等重启后 admin 操作失联才暴露。`TestServer_AdminRoles_E2E` 直接构造 Server 绕过 `main.serve`，所以现有测试测不到。
- **修法**：`EnsureAdminBearerToken` 在 `AdminTokens` 任一档非空时直接返回不生成。admin 档只来自 `admin_tokens.admin`；刻意只配 operator/auditor（不配 admin）时，admin 路由对所有人 401（`admin_auth.go:62-66` 空候选集 fail-closed，已核实安全）。
- **验收**：
  - [ ] config 单测：tiered-only 配置 → 返回 `false`，`AdminBearerToken` 保持为空，不生成；
  - [ ] http 层测试：tiered-only 配置下 `adminTokens()` 的 admin 档 = `admin_tokens.admin` 原值；
  - [ ] 旧单值 / 全空（debug 生成临时令牌）两条既有路径行为不变（既有测试原样通过）。
- **状态**：✅ 已修复（2026-10-29）

### F2 · 资格去重键先落库、后取签发方公钥 —— 取钥失败烧掉一名投票人的资格

- **证据**：`internal/usecase/registration.go` 的 `ReserveCredential`：`RecordUse`（提交资格去重键，唯一索引立即生效）之后才调 `uc.issuer.IssuerPublicKey(ctx)`；后者可能触发密钥轮换并写盘（`kryptology_credential_issuer.go` 的 `rotateIfNeededLocked` → `persistStateLocked` 的 `os.WriteFile`），磁盘/权限/keystore 故障都会失败。
- **影响**：失败时资格已永久消耗、凭证未签发、无任何恢复路径（`eligibility_uses` 没有清除 API），预留槽位还要等 10 分钟 TTL 惰性回收。一次服务端故障 = 永久锁死一名投票人。
- **修法**：把 `IssuerPublicKey` 调用提前到资格校验之后、槽位预留与 `RecordUse` **之前**（纯读 + 可能的轮换写盘，提前无副作用）。失败时什么都还没消耗。
- **验收**：
  - [ ] usecase 单测：mock issuer 在 `IssuerPublicKey` 返回错误 → claim 失败，且 `RecordUse` **未被调用**（资格未烧）、槽位未被消耗；
  - [ ] 既有成功路径测试原样通过（返回的 reservation 仍带 IssuerKey）。
- **状态**：✅ 已修复（2026-10-29）

### F3 · 匿名领取链路的 IP 日志豁免不完整 —— 挑战值可把领取者 IP 重新连上

- **证据**：`internal/infrastructure/http/middleware/request_logger.go:16-18` 的 `anonymousPathParts` 只有 `/credentials/claim`。T22 之后领取链路是四步：`eligibility-challenge` → `claim` → `blind-sign` → `attest`。challenge 端点记录 `client_ip`，挑战值每次唯一且客户端原样放回 claim 请求体；blind-sign / attest 同样记录 IP 且链路上可关联到同一假名。
- **影响**：能读请求日志的运维（或攻陷日志的人）用「同一 IP + 时间相邻」即可把「领取者 ↔ 凭证假名」重新连上，击穿 T9/T22 的匿名不变量（该文件注释自己写明：记录来源 IP 会多出一条可关联线索）。
- **修法（决策 D1）**：整条凭证领取链路四个端点统一豁免 `client_ip`：`/credentials/claim`（既有）+ `/credentials/eligibility-challenge` + `/credentials/blind-sign` + `/credentials/attest`。请求日志仍记方法 / 路径 / 状态 / 耗时；限流不受影响（限流键独立于日志）。
- **验收**：
  - [ ] request_logger 测试：四个端点路径全部不落 `client_ip`，非领取路径照落；
  - [ ] 部署清单 / techspec 的口径同步为「整条领取链路不落 IP」。
- **状态**：✅ 已修复（2026-10-29）

### F4 · 计票读票排序无唯一次级键 —— created_at 并列时审计复算可能误报不一致

- **证据**：`internal/infrastructure/persistence/vote_repository.go:107` 只有 `Order("created_at asc")`。merkle 叶子顺序直接决定 merkle 根与锚点哈希；PostgreSQL 时间戳微秒精度 + 容量报告 2300 票/秒，并列可预期（万票级下 µs 生日碰撞）。`ORDER BY` 并列行的相对顺序不保证稳定（执行计划即可翻转）。服务端计票（`internal/usecase/tally.go:219`）与审计复算（`cmd/audit-recompute/main.go:183`）**走同一个** `GetByProposal` —— 修一处两侧同时生效。
- **影响**：一旦发布时与复算时并列票顺序不同，`merkle_root` / `anchor_hash` 逐字节比对在**合法选举**上误报不一致，且结果已发布无法重算 —— 审计机制的可信度被无谓消耗。
- **修法（决策 D2）**：`Order("created_at asc, id asc")`（`votes.id` 是 uuid 字符串，确定性 tiebreak）。锚点哈希公式（KAT 钉死）不变；仅数据库读取顺序更确定。
- **验收**：
  - [ ] 仓储测试：同 `created_at` 的多张票按 `id` 稳定排序，两次读取结果一致；
  - [ ] `internal/tally`（含 `TestAnchorHash_KAT`）与 `cmd/audit-recompute` 既有测试原样通过；
  - [ ] ⚠️ 已发布存量数据的口径见决策 D2 —— 当前没有真实选举数据，无迁移负担。
- **状态**：✅ 已修复（2026-10-29）

### F5 · 门户：领取重试白烧证明 + 「一人一票」文案在「领取未完成」场景误导

- **证据**：
  - `web/src/composables/useVotePortal.ts` 的 `claimCredential` 重试循环里，每次尝试都调 `claimCredentialOnce` → `buildEligibilityBody`（重新取挑战 + 重新出 7~10 秒的零知识证明），而资格材料只与提案绑定，与槽位无关 —— 撞槽重试白白重烧；
  - claim 成功（服务端 `RecordUse` 落库）但 attest 未完成（409 撞车 / 断网 / 刷新页面）后重领，服务端回 409 `ELIGIBILITY_ALREADY_USED`，门户显示「这份资格已经领过票了 —— 一人一票」—— 但该用户**没拿到凭证也没投过票**，真实含义是「领取没有完成，资格已作废」；
  - 服务端口径见决策 D3：claim 即烧资格是 T22 的**有意取舍**（资格校验必须先于槽位消耗），服务端无法区分「二次领取」与「上次没领完」，不改。
- **修法**：
  1. 资格材料（挑战 + 证明）在重试循环**外**生成一次，循环内复用；
  2. reserve 阶段收到 `ELIGIBILITY_ALREADY_USED` 时分状态：本会话已成功领取过（`wallet.signature` 非空）→ 维持「一人一票」原文案（真重复领取，浏览器 e2e 依赖此文案）；本会话没领到过 → 改用「领取没有完成，资格已被占用」的准确文案并给出运营恢复指引；
  3. 部署清单补运营口径（决策 D3）：claim 完成但 attest 未完成 = 该资格在本提案作废，恢复 = 换提案或重新生成名单叶。
- **验收**：
  - [ ] useVotePortal 单测：撞槽重试 N 次只取一次挑战、只出一次证明；
  - [ ] useVotePortal 单测：`ELIGIBILITY_ALREADY_USED` 在「本会话未领取过」时给新文案、不重试；「已领取过」时维持原文案；
  - [ ] `web/e2e/portal.spec.ts` 与 `panels.test.ts` 的既有断言不被破坏（重复领取场景文案不变）。
- **状态**：✅ 已修复（2026-10-29）

### Checkpoint（P0 完成后）

- [ ] `CGO_ENABLED=1 go test ./...` 全绿；`go vet` / `gofmt` 干净；
- [ ] 覆盖率门槛达标（usecase 不得回退到 77 以下）；
- [ ] 前端 lint / tsc / vitest 全绿；
- [ ] `make web-e2e` 的资格路径用例通过（F5 改了它覆盖的链路）。

---

## 2. P1 —— 与测试并行修（本批不动，逐项独立提交）

| # | 问题 | 证据 | 修法 |
|---|---|---|---|
| F6 | `SchemaReady` 不查 `eligibility_uses` 表与 `eligibility_scope` 列，v0.3.0 库忘跑 `-migrate` 时启动照常通过，运行期领取链路全断 | `internal/infrastructure/persistence/migration.go:37-51` | 对 `EligibilityUse` 加 `HasTable`、对 `Proposal` 加 `HasColumn("eligibility_scope")`；补「缺列启动失败」用例 |
| F7 | `crypto.eligibility_roots` 与显式 `crypto.eligibility_depth` 混配不报错，depth 被静默忽略 | `internal/config/config.go:277-283` 只查 root/vk_path | 用 `v.IsSet("crypto.eligibility_depth")` 识别显式配置，命中即启动失败 |
| F8 | 三档令牌不校验互不相同：operator 与 admin 同值 = 白送 admin 权 | `internal/config/config.go:336-349` | release 校验段对去空格后的四枚令牌两两去重断言 |
| F9 | 多名单且无 default 时，空 scope 提案创建成功但首个领取者吃 500 | `internal/domain/crypto/eligibility.go:42-44`（`ScopeKnown("")` 恒真）+ `error_handler.go:88-89` | `CreateProposal` 在资格开启、scope 为空且无 default 可解析时 400 拒绝（预检一次 `RosterFor("")`） |
| F10 | 签发方 keystore 加载失败推迟成运行期 initErr，与资格密钥 fail-fast 口径不一致；与 F2 叠加时烧资格 | `kryptology_credential_issuer.go:75-77` | 构造时返回错误（或启动时显式 `IssuerPublicKey` 预检一次） |
| F11 | `PublishResult` 落结果与落审计锚点不在同一事务，anchor 写失败即永久缺失 | `internal/usecase/tally.go:266-279` | 两笔写入包进一个事务（仓储层组合方法），或允许对已发布提案补偿写 anchor |
| F12 | 门户 blind-sign 409 `CREDENTIAL_EXISTS` 不重试（attest 分支有 retryable），与服务端/CLI 契约不一致 | `web/src/composables/useVotePortal.ts:1333-1343`；过时注释在 `:149` 与 `cmd/cli/main.go:274` | blind-sign 分支补 `retryable: isSlotTaken(...)`，补对称单测；修正两处「预留不占位」过时注释 |
| F13 | 运营端创建提案无 `eligibility_scope` 入口，多名单特性从 UI 不可达 | `useVotePortal.ts:1659-1671`（请求体只有五个字段） | OpsPanel 创建表单加可选「资格名单」下拉（数据源 `/healthz` 的 `eligibility_rosters`）；或至少文档注明走 API/CLI |

## 3. P2 —— Nit（排期，不阻塞）

- N1 启动日志 `eligibility_enforced` 只看 `EligibilityRoot`，多名单部署恒 false（`cmd/server/main.go:122`）—— 加 `|| len(cfg.EligibilityRosters) > 0`
- N2 `eligibility-list` 的 `.txt` 分支不剥 UTF-8 BOM，与注释宣称不符（`cmd/eligibility-list/main.go:186-197`）
- N3 `CreateProposal` 不拒绝过去的 `starts_at`（造出无法备解密会话的死提案，`proposal.go:53-61`）
- N4 并发双建解密会话 / 双发布结果回 500 而非 409（`tally.go:117-121` + 两个仓储 Save 未映射唯一索引错误）
- N5 roster 名 / `eligibility_scope` 无 64 长度上限，PostgreSQL 上超长建提案炸 500（`handler.go:664` vs `proposal.go:21` `size:64`）
- N6 Worker 内 fetch 无超时，网络停滞让按钮永久卡死（`ballot-prover.worker.ts:72-76, 83-87`）
- N7 门户丢弃挑战响应里的 `depth`，深度不匹配时白烧 7 秒出注定失败的证明（`useVotePortal.ts:1185-1188`，挑战响应里就有 depth，一行比对）
- N8 SQLite `_busy_timeout` 靠驱动默认值 5000ms，未显式写进 DSN（`persistence/db.go:104-125`）
- N9 `cmd/server` 每深度一套密钥但文件名固定，靠目录/指纹自行配对（fail-closed 已有测试兜底，文档写明即可）
- N10 每次出证明新开 Worker 全量重下 11.9MB WASM（设计取舍，`eligibility-prover.ts:93` / `wasm-prover.ts:80`，记录在案）

## 4. 决策记录

| # | 决策 | 结论与理由 |
|---|---|---|
| D1 | 匿名领取链路的 IP 日志豁免范围 | **整条四端点链路统一豁免**（challenge / claim / blind-sign / attest）。只豁免 challenge 堵不住：challenge 与 blind-sign 都落 IP，同 IP + 时间相邻即可跨步关联到同一假名。口径：凡是「凭证获取链路」的请求日志都不落 `client_ip`（方法/路径/状态/耗时照记）；限流不受影响（限流键独立于日志）。管理端 blind-sign 同路径片段一并豁免，口径统一。 |
| D2 | 计票排序次级键 | `votes.id`（uuid 字符串，确定性）。**对存量发布数据的影响**：旧口径在 created_at 并列时顺序本就不确定，属于缺陷而非契约；当前没有真实选举数据，无迁移负担。锚点哈希公式（KAT）不变，`internal/tally` 不动。 |
| D3 | 「claim 即烧资格」的服务端口径 | **保持不变**。资格校验必须先于槽位消耗（否则不合格的人能烧池子），而 attest 在槽位消耗之后 —— 把 RecordUse 挪到 attest 会重新打开「一份资格占多个槽位」的洞。服务端也**无法**区分「真二次领取」与「上次没领完」（领取是匿名的，第二次请求与第一次无关联）。因此：服务端不动，门户按会话状态给准确文案（F5），部署清单写明运营恢复口径（换提案 / 重新生成名单叶）。 |
| D4 | 刻意只配 operator/auditor（不配 admin 档） | 允许且安全：F1 修复后不再生成临时 admin 令牌，`RequireAdminRole` 对空候选集 fail-closed（全 401）。分权部署的意图得到尊重。 |

## 5. 修复记录（实施时回填）

| 项 | 改动文件 | 测试 | 结果 |
|---|---|---|---|
| F1 | `internal/config/admin_token.go`；`internal/config/admin_token_test.go`；`internal/infrastructure/http/server_test.go` | 新增 tiered-only 不生成用例 + adminTokens 档位用例；既有用例全过 | ✅ |
| F2 | `internal/usecase/registration.go`；`internal/usecase/registration_eligibility_test.go`；`internal/usecase/registration_test.go`（PoolEmpty 用例补取钥期望） | 新增「取钥失败不烧资格」用例 | ✅ |
| F3 | `internal/infrastructure/http/middleware/request_logger.go`；`request_logger_test.go` | 四端点豁免断言 | ✅ |
| F4 | `internal/infrastructure/persistence/vote_repository.go`；`vote_repository_test.go` | 并列 created_at 稳定排序用例；tally / audit-recompute 既有测试全过 | ✅ |
| F5 | `web/src/composables/useVotePortal.ts`；`useVotePortal.test.ts`；`docs/deployment-checklist.md` | 重试只出一次证明；ALREADY_USED 分状态文案 | ✅ |
| 文档 | `docs/review-fixes-2026-10-29.md`（本文件）、`AGENTS.md`、`CHANGELOG.md`、`docs/deployment-checklist.md` | — | ✅ |

## 6. 全量验证记录（2026-10-29 实测）

- [x] `CGO_ENABLED=1 go test ./...` 全绿（exit 0，29 个包）
- [x] `go vet ./...` / `gofmt -l` 干净（⚠️ 本机裸 shell `CGO_ENABLED=0`，vet 必须显式导出）
- [x] `bash ./scripts/check-coverage.sh` 达标 —— 合计 **81.46%**（修复前 81.42%，新用例为正贡献）
- [x] 前端 lint / tsc / vitest 全绿 —— **163/163**（修复前 160，+3 为本批新用例）
- [x] `make web-e2e` 资格路径通过 —— **2026-10-29 实测**：真浏览器整条链路（领凭证 → 本地出票 →
  提交 → 查状态 → 计票；资格重复领取被拒）1 passed（2.2m），F5 的文案分支与重试复用在真实路径上无回归
