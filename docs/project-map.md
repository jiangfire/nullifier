# nullifier 项目地图（上下文索引）

> 本文档是 **Level 3 上下文**（分层摘要）。目的是让 agent / 人在进入某个子系统前，
> 只加载该子系统需要的那一节，而不是把整个仓库塞进上下文。
>
> 上下文层级：
> - **Level 1 规则文件**：`AGENTS.md`（仓库根）、`web/AGENTS.md`
> - **Level 2 规格文档**：`docs/techspec.md`（当前实现口径）、`README.md`（对外说明）
> - **Level 3 项目地图**：本文档 —— 分层索引 + 模式 + 陷阱
> - **Level 4 具体源码**：按需读取，本文每节都给出精确路径
>
> 改接口或流程时，**先更新 `docs/techspec.md`**，再更新本文档。

---

## 0. 一句话架构

匿名投票系统：**发资格的人看不到选谁**（凭证与选票内容完全无关）→ **记选票的人看不到是谁**（votes 表无用户标识）→
**验票真伪但不验身份**（零知识证明）。Go 后端（Gin + GORM + Wire）+ Vue 3 门户 + 复用同一套 HTTP API 的 CLI。

技术栈：Go 1.25.4 · Gin 1.12 · GORM 1.31 · Wire · gnark 0.7 · circl blindrsa（kryptology 1.8）· kyber v3 · SQLite/PostgreSQL · Vue 3.5 + TypeScript 5.9 + Rsbuild

---

## 1. 分层地图

### 1.1 启动与装配 —— `cmd/` + `internal/di/`

| 位置 | 职责 |
|---|---|
| `cmd/server/main.go` | 服务入口。加载配置 → 生成/校验 admin token → 建 logger → `di.InitializeServer` → `server.Run()` |
| `cmd/cli/main.go` | 13 个子命令的 CLI 客户端，**只走 HTTP API**，不绕过服务端校验 |
| `internal/di/providers.go` | 全部 Wire provider 的实现体（logger/db/各仓储/各 usecase/handler） |
| `internal/di/wire.go` + `wire_gen.go` | Wire 声明与生成产物。**改依赖注入后需重跑 wire** |
| `internal/config/config.go` | viper 加载 + 运行期校验 |
| `internal/config/admin_token.go` | admin bearer token 兜底生成（32 字节随机，base64url） |

**模式**：usecase 构造函数是纯 DI 传参，无全局变量；logger 允许为 nil（自动降级 `zap.NewNop()`）。

**陷阱**：
- `cmd/server/main.go:44-48` —— **配置的 admin token 也会以 Info 级别写进日志**，不只是临时生成的 token。生产日志会长期持有这个密钥。
- `internal/config/config.go:35` —— `server.mode` 默认值是 `release`，而 `release` 会**强制要求** `crypto.vk_path`（`config.go:90-92`）。配置文件缺失时服务直接启动失败，这是有意的兜底。
- `internal/config/config.go:47-52` —— viper 只找名为 `config.*` 的文件（`SetConfigName("config")`）。**`configs/config.production.yaml` 永远不会被自动加载**，部署必须靠环境变量或把它复制成 `config.yaml`。

### 1.2 领域层 —— `internal/domain/`

只放实体、接口、领域错误。**不依赖 HTTP / GORM / 具体密码学库**。

| 子包 | 内容 |
|---|---|
| `domain/entity/` | `Proposal` `Vote` `Credential` `VoteParticipation` `DecryptionSession` `TallyResult` `AuditAnchor` `Proof` |
| `domain/repository/` | 7 个仓储接口 + `errors.go`（`ErrNotFound` `ErrDuplicateVote` `ErrDuplicateParticipation`） |
| `domain/crypto/` | 3 个接口：`ProofVerifier` `Encrypter` `CredentialIssuer` |

**模式**：GORM tag 直接写在实体上（`entity.Vote` 的 `Nullifier` 是 `uniqueIndex`，`entity.VoteParticipation` 的 `(proposal_id, user_hash)` 是复合 `uniqueIndex:idx_vote_participation`）。数据库唯一约束是防重投的**最终防线**，不是 usecase 里的先查后写。

**陷阱**：`entity.Proposal.Options` 是 `gorm:"-"`（不落库），真正落库的是 `OptionsJSON`（`type:text`）。读出来需要仓储层反序列化。

### 1.3 用例层 —— `internal/usecase/`

5 个用例，依赖方向只朝内（只依赖 `domain` 接口）：

| 用例 | 文件 | 负责 |
|---|---|---|
| `ProposalUseCase` | `proposal.go` | 创建/列表；创建时去重、去空、校验 ≥2 选项、`ends_at > starts_at`；新提案 `Status` 固定为 `open` |
| `RegisterUseCase` | `registration.go` | 签发凭证；先查 `(proposal_id, user_id)` 是否已发过 |
| `VotingUseCase` | `voting.go` | **核心**。见 §2.1 投票链路 |
| `ParticipationUseCase` | `participation.go` | 「你投过票吗」；`HasVotedWithCredential` 必须带凭证材料 |
| `TallyUseCase` | `tally.go` | 门限会话初始化、发布结果、审计锚点。见 §2.2 计票链路 |

**模式**：错误用 `fmt.Errorf("%w: ...", ErrXxx)` 包装 `internal/usecase/errors.go` 里的哨兵错误；HTTP 层靠 `errors.Is` 映射状态码。

**陷阱**：`voting.go:205-219` 有**两条保存路径**——仓储实现了 `atomicVoteParticipationSaver` 就走事务（`SaveWithParticipation`），否则退化成先存票再存参与状态的非原子路径。目前 GORM 仓储实现了事务路径，mock 仓储没有。

### 1.4 HTTP 层 —— `internal/infrastructure/http/`

| 文件 | 职责 |
|---|---|
| `server.go` | 建 engine、挂 4 个中间件、注册公共组与 admin 组 |
| `handler.go` | 全部 handler；DTO 定义与绑定都在这里 |
| `middleware/admin_auth.go` | Bearer Token 鉴权 |
| `middleware/cors.go` | 白名单 Origin 精确匹配 |
| `middleware/error_handler.go` | 错误 → `{error, code}` 统一响应 |
| `middleware/recovery.go` | panic 兜底 |
| `middleware/request_logger.go` | 请求日志（只记 method/path/status/latency/ip，**不记 header 和 body**） |

**路由表**（`server.go:39-52`）：

| 方法 | 路径 | 鉴权 |
|---|---|---|
| GET | `/healthz` | 无 |
| POST | `/api/v1/votes` | 无 |
| GET | `/api/v1/proposals` | 无 |
| GET | `/api/v1/proposals/open` | 无 |
| POST | `/api/v1/proposals/:id/vote-status` | 无（需凭证材料） |
| POST | `/api/v1/proposals/:id/ballot-draft` | 无（**开发联调专用**） |
| GET | `/api/v1/proposals/:id/results` | 无 |
| GET | `/api/v1/proposals/:id/audit-anchors` | 无 |
| POST | `/api/v1/credentials` | **admin** |
| POST | `/api/v1/proposals` | **admin** |
| GET | `/api/v1/proposals/:id/credentials` | **admin** |
| POST | `/api/v1/proposals/:id/decryption/setup` | **admin** |
| POST | `/api/v1/proposals/:id/results/publish` | **admin** |

**模式**：handler 里 payload 结构体用 `validate:"..."` tag 校验，绑定失败统一 `c.Error(fmt.Errorf("%w: %v", usecase.ErrInvalidRequest, err))`，由 error_handler 统一出参。

**陷阱**：
- `middleware/admin_auth.go:27` 用 `!=` 做字符串比较，**不是常数时间比较**。
- `middleware/error_handler.go:68-69` —— `INVALID_REQUEST` 直接回 `err.Error()`，会把内部错误细节透出；其他 code 都做了归一化。改文案时注意前后端 `error-messages.ts` 的映射是按英文短语正则匹配的。
- 错误码枚举是**前后端契约**：`INVALID_REQUEST` `NOT_FOUND` `UNAUTHORIZED` `DOUBLE_VOTING` `CREDENTIAL_EXISTS` `USER_ALREADY_VOTED` `INVALID_CREDENTIAL` `DECRYPTION_SESSION_EXISTS` `RESULT_ALREADY_PUBLISHED` `INTERNAL_ERROR`。

### 1.5 持久化层 —— `internal/infrastructure/persistence/`

| 文件 | 职责 |
|---|---|
| `db.go` | SQLite / PostgreSQL 连接 |
| `db_env.go` | 从环境变量读 DB 配置（`DB_DRIVER` 等） |
| `base.go` | 泛型仓储基类（`BaseRepository[T]`） |
| `migration.go` | `AutoMigrate` + 历史 schema 兼容迁移 |
| `*_repository.go` | 7 个仓储实现 |

**模式**：仓储接口方法签名都带 `ctx`；`SaveWithParticipation` 用 `clause.OnConflict{DoNothing}` + `RowsAffected == 0` 检测重复参与，冲突时返回 `ErrDuplicateParticipation`。

**陷阱**：
- `db.go:40-53` —— PostgreSQL 的**连接池参数**由 `db.max_open_conns` / `db.max_idle_conns` / `db.conn_max_lifetime_seconds` 配置（默认 25 / 5 / 3600 秒），为 0 表示不改动 `database/sql` 默认值。
- `db.go:56-63` —— SQLite 文件库**没开 WAL**，也没设 busy timeout。
- `migration.go:26-28` —— 每次迁移都会回填 `options_json`（`WHERE options_json IS NULL OR = ''` → `'[]'`）。**release 模式启动不再自动迁移**，改表要走独立的 `nullifier-server -migrate` 步骤（`config.AutoMigrate`）。
- `db_env.go:23-25` —— `GO_ENV=test` 时 SQLite 强制切 `:memory:`。

### 1.6 密码学层 —— `internal/infrastructure/crypto/`

| 文件 | 实现 | 依赖 |
|---|---|---|
| `gnark_proof_verifier.go` | Groth16 验证 | gnark / gnark-crypto |
| `kyber_elgamal.go` | 自实现 ElGamal 封装（edwards25519 + HMAC-SHA256 + SHA256-CTR keystream）+ Shamir 门限 | kyber v3 的曲线与 `share` 包 |
| `kryptology_credential_issuer.go` | RFC9474 RSABSSA 签发、密钥版本轮换、落盘 | circl `blindsign/blindrsa` |
| `merkle_root.go` | Merkle root | gnark-crypto `merkletree` |

**模式**：密钥材料统一 `append([]byte(nil), ...)` 拷贝后返回，不外泄内部 slice；issuer 落盘权限 `0o600`（`kryptology_credential_issuer.go:358`）。

**陷阱**：
- `gnark_proof_verifier.go:25-31` —— **`vk_path` 为空时 `Verify` 直接 `return nil`**，任何非空 proof 都通过。`requireVerifyMode` 只由 vk_path 是否为空决定。
- `gnark_proof_verifier.go:64-67` —— 加载 vk 后，`public_witness` 是**必填**，缺失即验证失败。
- `kyber_elgamal.go` —— 密钥派生用 **HKDF-SHA256**（`crypto/hkdf`），salt 每次加密随机生成并随密文传输，info 绑定协议版本、用途与双方公钥。密文格式 **v2**，与 v0.1.0 的 v1 密文不兼容。
- `kryptology_credential_issuer.go:163-179` —— 盲签名全流程在**服务端内部**完成（服务端自己 Blind 再 Finalize），客户端不参与。
- `kryptology_credential_issuer.go:194` `VerifyCredential` —— **生产路径从不调用**，只有单测调用。投票时是拿提交的签名和库里存的签名做 `bytes.Equal` 比对（`voting.go:166`）。

### 1.7 前端门户 —— `web/`

| 位置 | 职责 |
|---|---|
| `src/App.vue` | 只做外壳、路由与新手引导编排 |
| `src/components/panels/` | 7 个页面：`OverviewPanel` `SetupPanel` `ReferencePanel` `HallPanel` `IdentityPanel` `VotePanel` `OpsPanel` |
| `src/components/` | `AppSidebar` `AppTopbar` `GuideOverlay` `JourneyBar` `NoticeStack` |
| `src/components/ui/` | `EmptyState` `NoticeBanner` `StatTile` |
| `src/composables/useVotePortal.ts` | **全部状态与 API 编排**（单文件约 1600 行，是前端的事实核心） |
| `src/lib/` | `api.ts` `error-messages.ts` `portal-view.ts` `navigation.ts` `options.ts` `datetime.ts` —— **只有这 6 个模块有单测** |
| `src/styles/` | `tokens.css` `base.css` `components.css` `shell.css` |
| `src/types/` | `portal.ts` `portal-api.ts` |

**模式**：设计令牌在 `styles/tokens.css`，组件只引用变量不写裸色值；视图推导（工作区归属/标题/引导步进）集中在 `lib/portal-view.ts`。

**陷阱**：
- `lib/api.ts` 是唯一的 HTTP 出口，返回 `{ok, status, data, error, code, rawText}`；错误文案统一走 `lib/error-messages.ts` 的 `describeApiError(message, code, fallback)`，优先级 **业务短语 > 错误码 > 调用方兜底**。
- `useVotePortal.ts:757-807` 的 `prepareBallotByOption` 调的是后端 `ballot-draft`，拿到的是**开发用假 proof**。生产模式下这条路径拿不到可验证的 proof。
- **凭据与当前选项不绑定**：切换选项没有任何 watcher 去清空 `encryptedVote`/`proof`（`grep "watch(selectedVoteOption"` 无命中），而提交时后端**不读** `selected_option`（`submitVotePayload` 无此字段）。→ 改了选项直接提交，计的是旧选项。
- **admin token 明文存 `localStorage`**（`:32` 键名、`:259-261` 读、`:1491-1493` 每次按键写）。
- **`hasVoted` 跨提案不重置**：`watch(selectedProposalId)`（`:1495-1501`）没清它 → 换个提案面板还显示「你已投过」。
- **门户的匿名性文案与服务端行为相反**：`VotePanel.vue:250-252` 说「计票前不会知道你选了谁」，但 `ballot-draft` 是服务端拿明文选项去加密的（`handler.go:338` → `tally.go:358`）—— 服务端在生成凭据时就看到了选项。
- `.vue` 面板**零测试**。改面板只能靠 `tsc` + lint + 手工验证。
- `pnpm run format`（prettier）会重写 `.pnpm-store`，必须先确认 `.prettierignore` 覆盖（当前已覆盖 `.pnpm-store/`）。

### 1.8 CLI —— `cmd/cli/main.go`

14 个命令，全部走 HTTP API：
`health` `version` `proposals-open` `proposal-create` `credential-issue` `credential-show` `ballot-draft` `vote-submit` `vote-status` `credentials-list` `decrypt-setup` `result-publish` `result-get` `anchors-list`

**模式**：`-admin-token` flag 或 `NULLIFIER_ADMIN_TOKEN` 环境变量；`-base-url` 或 `NULLIFIER_BASE_URL`。

**陷阱**：
- CLI 输出是纯文本命令结果，**不做中文本地化**（与前端相反，是有意为之）。
- **URL 路径二次转义**：`main.go:563` 把已 `url.PathEscape` 过的 path 塞进 `base.Path`，`url.URL.String()` 会再转义一次（`%` → `%25`）。含空格 / `/` / `%` 的 proposal id 必然 404。实测：`result-get -proposal "a b/c"` → `/api/v1/proposals/a%2520b%252Fc/results`。
- **`vote-submit` 会多发一个 `selected_option` 字段**（`main.go:319-321`），后端 `submitVotePayload` 没有这个字段 → 静默忽略，但**明文选项已经上了网**。
- **`decrypt-setup` 不带 `-out` 时会把门限份额打印到 stdout**（`main.go:398-414`）。
- CLI **没有** `-public-witness` 入口，而 `ballot-draft` 也不返回该字段 → 生产模式下无法提交投票。

---

## 2. 关键流程

### 2.1 投票链路（`voting.go:111-227`）

```
HTTP 绑定 + validate
  → proposal 必须处于 open 且在时间窗口内
  → 必须已存在 decryption session（否则「decryption session is not ready」）
  → verifier.Verify(proof, publicInputs)      ← vk_path 为空时此步是空操作
  → encrypter.ValidateCiphertext(encrypted_vote)
  → 查 (proposal_id, user_id) 的凭证，比对 user_pub_key 与 signature
  → hash(user_id) = sha256
  → 查 nullifier 是否已存在（全局唯一）
  → 查该 user_hash 是否已参与
  → 事务内：先插 vote_participations（OnConflict DoNothing），再插 votes
```

**防重投依赖两件事**：usecase 里的先查后写 + 数据库唯一索引兜底。真正的并发安全靠后者。

### 2.2 计票链路（`tally.go`）

```
decrypt-setup  → 生成密钥对 → Shamir 拆成 threshold/trustees 份额
                → 库里只存份额的 sha256 哈希（SharesJSON）
                → 响应体里把明文份额一次性返回给 admin
publish        → 提案必须已关闭
                → 重复发布返回 409
                → 份额必须属于本 session，且不能重复计数
                → RecoverSecret → 逐票解密 → 统计 → 校验选项在提案白名单内
                → Merkle root → anchor_hash = sha256(proposal_id|root|tally)
                → 写 tally_results + audit_anchors
```

**陷阱**：`decrypt-setup` 在**投票开放中会被拒绝**（`tally.go:101-111`）。运营上必须在 `starts_at` 之前完成初始化。同理 `publish` 只能在提案关闭后执行。

### 2.3 门限解密会话的生命周期

- 一个提案一个 session，重复 setup → `409 DECRYPTION_SESSION_EXISTS`（**不会覆盖**）。
- 覆盖会导致已投选票无法解密，所以这是硬约束。
- 份额明文只在 setup 响应里出现一次，库里只有哈希。

---

## 3. 已知陷阱（改动时优先检查）

> 本清单随代码更新。**已修掉的问题不再列在这里**，但历史结论保留在
> `docs/release-review-2026-10-20.md`。

1. **`vk_path` 为空 = 关闭证明验证。** `gnark_proof_verifier.go:55`。开发配置 `configs/config.yaml` 就是 `vk_path: ""`。
2. **`configs/config.production.yaml` 不会被自动加载。** viper 只认 `config.*`。
3. **生产配置 `cors_allowed_origins: []`** → 门户跨域会被拒。这是部署项，不是代码项。
4. **覆盖率门槛只覆盖 5 个包**（usecase / config / di / http / http-middleware），`internal/infrastructure/crypto`（78.1%）、`internal/infrastructure/persistence`（58.6%）、`cmd/cli`（24.6%）、`cmd/server`（**0%**）**都不在门槛内**。门槛口径 83.40%，**全仓口径 62.7%**（2026-10-20 实测），余量 3.4 个百分点；其中 usecase 单包 79.8%（已低于 80%）。
5. **窗口期语义在两个用例里各写了一份**：`isProposalOpenForVoting`（`voting.go:234`）和 `isProposalClosed`（`tally.go:386`）。改时间规则要同时改两处。
6. **`ballot-draft` 是公共路由且无模式开关** —— 生产环境也会暴露（此时返回的假 proof 会在验证阶段被拒，但接口本身不该在生产可达）。修复任务见 v1.0.0 计划 T1。
7. **proof 没有绑定到请求字段**：`PublicInputs()`（`voting.go:60-69`）造了 `proposal_id` / `nullifier`，但 `Verify`（`gnark_proof_verifier.go:48-80`）**只读 `public_witness`**。`encrypted_vote` 也不在任何公开输入里。修复任务见 v1.0.0 计划 T15。
8. **release 模式不再启动即改表** —— 必须先跑 `nullifier-server -migrate`，忘了会直接启动失败（`persistence.SchemaReady` 检查）。开发模式仍然自动迁移。
9. **盲签名全流程在服务端内部完成**（`kryptology_credential_issuer.go:163-179`）—— 服务端自己 Blind 再 Finalize，客户端不参与，**不是真盲签名**。v1.0.0 计划 T17 改成两段式。
10. **凭证签发端点在 admin 分组下**（`server.go:47-48`）—— 每个投票人都得持有 admin token，且运营方天然掌握「哪个凭证给了谁」。v1.0.0 计划 T9 改为凭证池 + 匿名随机领取。
11. **密文格式 v2 与 v0.1.0 不兼容** —— v1 密文会被直接拒绝（`unsupported ciphertext version`），旧的加密选票解不开。密钥派生用 HKDF-SHA256（`crypto/hkdf`）。
12. **admin token 的存放位置**：CLI 优先读 `NULLIFIER_ADMIN_TOKEN` 环境变量（`cmd/cli/main.go:60`），`-admin-token` 只是兜底；门户存 `sessionStorage`，不落 `localStorage`。

---

## 4. 验证手册

```bash
# 后端
go build ./...
go vet ./...
gofmt -l ./cmd ./internal ./tests      # 应无输出
go test ./...                          # Windows 需 CGO_ENABLED=1
bash ./scripts/check-coverage.sh 80 coverage.ci.out   # 用 Git Bash，不是 WSL

# 前端
pnpm --dir web run lint
pnpm --dir web exec tsc --noEmit
pnpm --dir web run test
pnpm --dir web run build

# 一次性门禁
make check      # vet + gofmt + web-lint + web-typecheck
make ci         # go test ./... + 覆盖率门槛
```

Windows 注意：仓库根的 `Makefile` 需要在 **Git for Windows 的 bash** 里用 `mingw32-make` 跑；`bash` 命令在 Windows 上默认可能指向未安装的 WSL。

---

## 5. 尚未完成的部分（不要写成生产能力）

- **没有真实的选票零知识电路。** 仓库里唯一的 gnark 电路是测试文件里的玩具电路（`Y = X³ + X + 5`，见 `tests/e2e/api_test.go:30-39`）。生产配置的 `crypto.vk_path: "./keys/verifying.key"` 指向一个**本仓库无法生成**的密钥。
- **没有客户端侧证明生成。** 服务端只验不生成。门户与 CLI 都拿不到可用的 `public_witness`。
- **没有任何测试跑通过真实 gnark 验证。** 所有 HTTP 层测试都用 `NewGnarkProofVerifier("")`（空操作验证器），`tests/e2e/api_test.go:62`、`internal/infrastructure/http/server_test.go:571`。
- **盲签名的盲化在服务端内部完成**，客户端不参与（见 §1.6 陷阱）—— 不具备真正的盲化性质。
- **`ballot-draft` 是开发联调接口。** 门户的「按选项生成投票凭据」依赖它；而且它由**服务端拿明文选项去加密**，所以当前实现下服务端知道每一票的明文选项。
- **门户与 CLI 的匿名性文案与实现不符**（见 §1.7 陷阱）。

> 完整的问题清单、证据与修复顺序见 `docs/release-review-2026-10-20.md`。
