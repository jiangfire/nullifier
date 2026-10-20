# nullifier 发版前整体 Review（2026-10-20）

审查对象：`main` @ `2398b00`（工作区干净，无未提交改动）
审查范围：Go 后端（cmd/internal/tests）、Vue 3 门户（web/）、CI 与部署配置、文档一致性
审查基线：本次实际执行了全部门禁命令（结果见 §0），其余结论均来自逐文件读取并附 `file:line` 证据。

> **后续状态**：本文档是 **2026-10-20 审查当时的快照**，数字与结论保留原样不改写。
> 修复落地情况见 [`docs/release-plan-v0.1.0.md`](./release-plan-v0.1.0.md) 的里程碑与
> [`CHANGELOG.md`](../CHANGELOG.md) 的 0.1.0 条目。已完成的关键项：
> C4（静默投错票）、C5（文案与实现对齐）、C6（覆盖率口径）、R4（token 进日志）、
> R5（生产 token）、R7（production.yaml 不生效）、R9/R10（覆盖率门槛与 CI）、
> R11–R15、R17–R19、R23–R27、R29。
> **未完成（需要 v0.2.0 里程碑）**：C1、C2、C3、R2、R3、R8、R12、R16、R20、R21。
>
> **复核更正（2026-10-20 落地时核对代码）**：原快照把 R1、R28 列为未完成，实际不成立 ——
> R1 已由 `cb78f32` 的启动告警覆盖（`vk_path` 为空时明确提示「证明校验已关闭」）；
> R28 本就已有 `NULLIFIER_ADMIN_TOKEN` 环境变量入口（`cmd/cli/main.go:60`），
> `-admin-token` 只是兜底。R23（CLI `public_witness`，`cmd/cli/main.go:322`）同理，代码里本就具备。
>
> **R18 只修了一半**：`hasVoted` 跨提案重置已由 `eb1d488` 修完，但「查询投票状态」的
> **UI 入口仍缺** —— `fetchVoteStatus` 已实现并导出（`useVotePortal.ts:1122`/`:1711`），
> 唯一调用点在 `submitVote` 内部（`:1118`），没有按钮能主动触发。列为路线图的 T11。
>
> 剩余项的完整计划见 [`docs/roadmap.md`](./roadmap.md)（产品路线图）。

---

## 结论（先说结果）

> **当前代码不能按「生产可用的匿名投票系统」发版。**
>
> 5 个 CRITICAL 分两组：**一组是零知识证明链路（C1–C3）** —— 这条链路只有验证器的骨架，**没有电路、没有密钥生成、没有可用的证明产物**，C4/C5 是这个缺口的直接后果在用户界面的显形；**非密码学部分**（HTTP、持久化、计票、审计锚点）质量是好的，全部门禁通过，没有发现数据竞争、事务漏洞或明显的逻辑错误。**问题集中在「证明链」和「门户对这条链路的依赖与表述」上。**
>
> 另有 **C6（覆盖率徽章虚高 21.7 个百分点）** 属于对外表述失真，与密码学无关但同样影响发版可信度。
> 存在两条可走的发版路径，请你拍板：
>
> | 路径 | 前提 | 适合 |
> |---|---|---|
> | **A. 发「内测版 v0.x」** | 在 README/techspec/门户三处明确标注「服务端可见明文选项、证明链未实现」，并把 C1–C5 列为下一里程碑 | 内测、演示、继续开发 |
> | **B. 发「生产版」** | 先修完 C1–C5（工作量是「补一个真实的选票电路 + 客户端 prover + 密钥分发流程」，不是小改） | 真实投票 |

> 无论走哪条路，**C4（静默投错票）都必须先修** —— 它是独立于密码学的纯前端逻辑 bug，改动量小，且会让用户在不知情的情况下投出错误选票。
>
> 我没有替你选。这取决于这份代码近期是否要接待真实投票者。

---

## 0. 已实际执行并通过的门禁

| 检查 | 命令 | 结果 |
|---|---|---|
| 编译 | `go build ./...` | ✅ exit 0 |
| 静态检查 | `go vet ./...` | ✅ exit 0 |
| 格式 | `gofmt -l ./cmd ./internal ./tests` | ✅ 无输出 |
| 单元/集成/E2E | `go test ./...` | ✅ 全部 ok（12 个包） |
| 覆盖率门槛 | `bash ./scripts/check-coverage.sh 80 coverage.ci.out` | ✅ 81.80% ≥ 80% |
| 前端 lint | `pnpm --dir web run lint` | ✅ exit 0 |
| 前端类型 | `pnpm --dir web exec tsc --noEmit` | ✅ exit 0 |
| 前端单测 | `pnpm --dir web run test` | ✅ 5 文件 / 37 用例全过 |
| 密钥泄漏扫描 | `git ls-files` 全量过滤 | ✅ 无 key/env/db/coverage 被跟踪 |

覆盖率明细：usecase 79.8% · config 94.6% · di 91.5% · http 82.1% · http-middleware 77.5%。

---

## 1. CRITICAL

### C1 · 选票的零知识电路根本不存在，生产 verifying key 无法生成

**证据**：全仓库搜索 `groth16.Compile|NewCircuit|groth16.Setup|Circuit`，命中全部在 `_test.go` 里，且都是与选票无关的玩具电路：

- `tests/e2e/api_test.go:30-39` — `type voteCircuit struct { X, Y }`，约束是 `Y = X³ + X + 5`
- `tests/integration/crypto_test.go:20-40` — 同款 `integrationCircuit`
- `internal/infrastructure/http/server_test.go:28-33` — `serverTestCircuit`
- `internal/infrastructure/crypto/gnark_proof_verifier_test.go:18-23` — `cubicCircuit`

**没有任何**非测试文件定义选票电路，**也没有任何**生成 `verifying.key` 的工具或命令。

而生产配置强制要求这个密钥：

- `configs/config.production.yaml:8` — `vk_path: "./keys/verifying.key"`
- `configs/production.env.example:2` — `CRYPTO_VK_PATH=./keys/verifying.key`
- `internal/config/config.go:90-92` — `server.mode` 为 `release/prod/production` 时 `vk_path` 为空直接启动失败

**影响**：`release` 模式启动需要 `verifying.key`，但**按当前仓库无法产出这个文件**。生产实例要么起不来，要么运营会随便放一个文件上去。

---

### C2 · Web 门户在生产模式下 100% 无法完成投票

**证据链**：

1. 门户唯一能自动获得 proof 的路径是 `prepareBallotByOption`：
   `web/src/composables/useVotePortal.ts:775-780` → `POST /api/v1/proposals/:id/ballot-draft`
2. 后端该接口返回的是**写死的假 proof**：
   `internal/infrastructure/http/handler.go:314` — `const developmentProofBase64 = "ZGV2LXByb29m"`（即 `"dev-proof"`）
   `internal/infrastructure/http/handler.go:344-350` — 响应体只有 `encrypted_vote` / `proof` / `nullifier`，**没有 `public_witness`**
3. 生产模式下验证器进入严格分支：
   `internal/infrastructure/crypto/gnark_proof_verifier.go:41-45` — 载入 vk 后 `requireVerifyMode = true`
   `internal/infrastructure/crypto/gnark_proof_verifier.go:64-67` — `public_witness` 缺失即 `errors.New("public_witness is required")`

**影响**：release 模式下，用户点「按选项生成投票凭据」→ 拿到 dev-proof → 点提交 → 服务端报 `public_witness is required`。**门户的投票主流程在生产环境下完全不可用。**

更糟的是**错误码也错了**：

- `internal/usecase/voting.go:148-150` — `fmt.Errorf("invalid proof: %w", err)`，**没有**包装 `ErrInvalidRequest`
- `internal/infrastructure/http/middleware/error_handler.go:41-63` — `mapError` 匹配不上 → 落到 `default` → **HTTP 500 / `INTERNAL_ERROR`**
- `web/src/lib/error-messages.ts` — 前端只能显示「服务端出错，请查看后端日志」

也就是说，**一个纯粹由客户端输入造成的失败被当成服务端故障返回 500**，还会被 `error_handler.go:30` 以 `Error` 级别记进生产日志。proof 校验失败应当是 **400**。

文档已经承认了这个缺口（`README.md:220`、`web/README.md:55-58`：「生产模式请使用真实钱包/客户端生成 proof 与密文」），但**那个钱包/客户端不存在，也没有接入点** —— 门户里只有「粘贴 JSON 导入」和「服务端生成草稿」两条路，两条都拿不到真 proof。

---

### C3 · 服务端不校验 proof 与 `proposal_id` / `nullifier` / 密文的绑定

**证据**：

- `internal/usecase/voting.go:60-69` — `PublicInputs()` 构造了三个公开输入：
  ```go
  inputs := entity.PublicInputs{
      "proposal_id": []byte(r.ProposalID),
      "nullifier":   []byte(r.Nullifier),
  }
  if len(r.PublicWitness) > 0 { inputs["public_witness"] = ... }
  ```
- `internal/infrastructure/crypto/gnark_proof_verifier.go:48-80` — `Verify` **只读取 `inputs["public_witness"]`**：
  ```go
  publicWitnessBytes, ok := inputs["public_witness"]   // proposal_id / nullifier 从头到尾没被使用
  ...
  if err := groth16.Verify(gnarkProof, v.verifyingKey, publicWitness); err != nil { ... }
  ```
- `internal/usecase/voting.go:151-155` — `encrypted_vote` 只做格式校验（`ValidateCiphertext`），**不在任何公开输入里**。

**影响**：proof 的约束对象是「客户端自报的那段 witness 字节」，而不是「这条请求的 proposal_id / nullifier / 密文」。也就是说，proof 目前**没有绑定到选票内容**。

这不构成一个现在就能利用的漏洞 —— 因为真实电路还不存在（C1），没有可伪造的 proof。但**它是一颗定时炸弹**：将来补上电路时，如果公开输入里没有把 `proposal_id`、`nullifier` 和 `hash(ciphertext)` 放进去，那么
- 同一份 proof 可以配不同的 `nullifier` 提交（防重投将退化为只靠 DB 唯一索引 + 参与记录）；
- 同一份 proof 可以配不同的 `encrypted_vote` 提交（选票内容与证明脱钩）。

**建议**：补电路时**一次性**把 `proposal_id`、`nullifier`、`ciphertext` 的承诺写进公开输入，并改 `Verify` 的实现 —— 让它真正校验 `proposal_id` / `nullifier` 与 witness 中的公开输入一致（现在 `PublicInputs()` 造了却不用，是个明确的未完成接线）。

---

### C4 · 界面显示的选项与实际计票的选项可能不一致 —— 用户会静默投错票

**证据链**：

1. 前端提交时把当前选中的选项一起发过去：
   `web/src/composables/useVotePortal.ts:1023-1025`
   ```ts
   if (selectedVoteOption.value.trim() !== '') {
     body.selected_option = selectedVoteOption.value.trim();
   }
   ```
2. 后端**根本不读这个字段**：
   `internal/infrastructure/http/handler.go:70-79` 的 `submitVotePayload` 结构体里**没有 `selected_option`**；Gin 的 `ShouldBindJSON` 静默忽略未知字段。`grep selected_option internal/` 无任何命中。
3. **切换选项不会作废已生成的凭据**：`grep "watch(selectedVoteOption"` 在 `useVotePortal.ts` 中**无命中** —— 没有任何 watcher 监听选项变化来清空 `encryptedVote` / `proof`。
4. 提交前的校验（`useVotePortal.ts:1000-1012`）只检查 `encryptedVote` / `proof` / `nullifier` **非空**，不检查密文对应的是哪个选项。
5. `prepareBallotByOption` 失败时提前 return（`useVotePortal.ts:783-794`），**不清空**上一次成功的 `encryptedVote` / `proof`。

**复现路径**：选 A → 点「按选项生成凭据」（成功）→ 改成 B → 直接点「提交匿名选票」（不重新生成）→ **服务端记下的是 A，界面显示的是 B**，用户完全无感。

（服务端本身是安全的：`tally.go:231-233` 会校验解密出的选项在提案白名单内，非法会拒绝计票。但**选错合法选项无法被发现**。）

**这是投票产品最不能接受的一类缺陷** —— 用户的真实意图与被记录的内容不一致，且没有任何提示。

**建议**：
- 前端在凭据里记下 `ballotProposalId` / `ballotOption`，选项或提案变化时立即清空 `encryptedVote` / `proof`；
- 提交前校验「凭据的目标选项 === 当前选中选项」，不一致就禁用提交按钮并提示重新生成；
- 后端接收并校验 `selected_option`，与密文解密结果不一致时拒绝（这需要后端能看到选项，只能在开发模式做；生产模式靠客户端自证 + proof 绑定）。

---

### C5 · 门户对用户承诺的匿名性，与服务端实际行为相反

**证据**：

- `web/src/components/panels/VotePanel.vue:250-252`：
  > 提交后服务端只保存加密后的选票，计票前不会知道你选了谁。
- `web/src/components/panels/VotePanel.vue:152-154`（**同一页面的上一步**）：
  > 凭据由服务端按你选中的选项生成，不需要手写 proof 或密文。

这两句话互相矛盾，而**第二句才是当前实现的真相**：

- `internal/infrastructure/http/handler.go:338` → `internal/usecase/tally.go:325-362` — `ballot-draft` 接收**明文选项**，由服务端用会话公钥加密。
- 即：**服务端在生成凭据的那一刻就看到了每一张选票的明文选项**。`votes` 表里存的确实是密文，但服务端**已经观测到了**选项。

这是本产品最核心的承诺（「服务端数得清结果，数不出是谁投的」）。当前门户的默认路径让这个承诺**在用户界面层面就是假的**。

**建议**（二选一）：
- **改实现**：把加密和 proof 生成移到客户端（浏览器端 WebCrypto / WASM gnark），服务端只验不生成 —— 这也顺带解决了 C1/C2；
- **改文案**：在 C1/C2 未修复前，门户与 README 必须明确写「当前是开发模式：服务端会看到明文选项，仅用于联调」。

注意 `web/README.md:55-58` 已经诚实地写了「该草稿凭据用于本地联调（开发模式），生产模式请使用真实钱包/客户端生成 proof 与密文」—— **是门户 UI 没说**。以 README 的口径为准去修 UI。

### C6 · README 的覆盖率徽章虚高 21.7 个百分点

`README.md:12` 的徽章写 `coverage-81.8%`。**81.8% 只是覆盖率门槛选中的 5 个包的加权值**（`scripts/check-coverage.sh:9-15`）。我在本机实测的**全仓真实覆盖率**：

| 包 | 覆盖率 | 在门槛内？ |
|---|---|---|
| `internal/config` | 94.6% | ✅ |
| `internal/di` | 91.5% | ✅ |
| `internal/infrastructure/http` | 82.1% | ✅ |
| `internal/usecase` | **79.8%** | ✅（单包已低于 80%） |
| `internal/infrastructure/http/middleware` | 77.5% | ✅ |
| `internal/infrastructure/crypto` | **77.7%** | ❌ |
| `internal/infrastructure/persistence` | **59.3%** | ❌ |
| **`cmd/cli`** | **14.2%** | ❌ |
| **`cmd/server`** | **0.0%** | ❌ |
| **全仓合计** | **60.1%** | — |

> 表里的 60.1% 是**审查当时的全仓合计**。今天同一个数字的含义已经变了 ——
> 现在的全仓口径是 74.1%，而 60.1% 是 `internal/infrastructure/persistence`
> 单包的最低档（见 README 与 `AGENTS.md` 的覆盖率两口径）。最新数值以 README 为准。

复现命令：`go test -count=1 -coverprofile=all.out ./... && go tool cover -func=all.out | tail -1`

**影响**：
- README 对外宣称 81.8%，真实是 60.1% —— **这是一处对外的事实性错误陈述**；
- 密码学（77.7%）与持久化（59.3%）这两个最关键的包**不在门槛内**，覆盖率回退不会让 CI 失败；
- `cmd/cli` 只有 1 个测试文件 1 个用例（`cmd/cli/main_test.go:16`），13 个命令里 12 个零测试；
- `cmd/server` **零覆盖** —— 启动、配置加载、admin token 生成这些生产关键路径完全没被测到。

**建议**：
1. 徽章改为全仓口径（60.1%），或明确标注「仅核心 5 包」；
2. 门槛纳入 `crypto` / `persistence` / `cmd/cli` / `cmd/server`；
3. 至少给 `cmd/server` 补一个「配置加载 + token 生成 + 路由注册」的冒烟测试（能一次性覆盖 R4/R5/R7 这类启动期问题）。

---

## 2. REQUIRED（发版前建议处理）

### R1 · `vk_path` 为空时，证明验证是彻底的空操作，且无任何警告

`internal/infrastructure/crypto/gnark_proof_verifier.go:25-31` 与 `:55-57`：

```go
if vkPath == "" {
    return &GnarkProofVerifier{requireVerifyMode: false}, nil
}
...
if !v.requireVerifyMode { return nil }   // 任何非空 proof 都通过
```

开发配置 `configs/config.yaml:12` 就是 `vk_path: ""`。所以**开发模式下任何人构造一个合法 body 就能投票**。这是有意的开发便利，但没有任何运行期提示。

**建议**：`mode` 为 release/prod/production 之外的、且 `vk_path` 为空时，启动日志打一条 `Warn`；或引入显式的 `server.allow_unverified_proofs` 开关，让「不验票」成为一个必须被主动声明的决定，而不是默认行为。

### R2 · 盲签名的盲化在服务端内部完成，不具备盲化性质

`internal/infrastructure/crypto/kryptology_credential_issuer.go:163-179`：

```go
preparedMsg, err := material.client.Prepare(crand.Reader, message)
blindedMsg, state, err := material.client.Blind(crand.Reader, preparedMsg)   // 服务端自己盲化
blindSignature, err := material.signer.BlindSign(blindedMsg)
signature, err := material.client.Finalize(state, blindSignature)            // 服务端自己解盲
```

客户端（门户 / CLI）**完全不参与**盲化。服务端同时持有私钥和盲化状态，因此这个签名对服务端而言等价于一次普通 RSA-PSS 签名 —— RFC9474 的「签发方对被签消息不可见」这一核心性质没有成立。

而 README 把这条列为第一条匿名性能力（`README.md:23`：「凭证用 RSA 盲签名签发（RFC9474）。签发方看到的是被盲化的随机串，选票内容留在用户手里」）。

**必须二选一**：
- 改实现：把 `domaincrypto.CredentialIssuer` 接口改成真正的两段式（客户端提交 blinded message → 服务端签 → 客户端解盲），工作量大但符合 README 的承诺；
- 改文档：把「盲签名」降级为「RSA 签名凭证」，并删掉「签发方看到的是被盲化的随机串」这句。

对一个以匿名性为核心卖点的产品，**「文档承诺 vs 实现不符」比功能缺失更伤**。

### R3 · 凭证签名在生产路径上从不被密码学验证

`internal/infrastructure/crypto/kryptology_credential_issuer.go:194` 的 `VerifyCredential` —— 全仓库只有单测在调用（`kryptology_credential_issuer_test.go:20/32/43/57/74/76`）。

投票时的实际校验是 `internal/usecase/voting.go:166`：
```go
if !bytes.Equal(credential.Signature, req.CredentialSignature) { return ErrInvalidCredential }
```
即「出示库里存的那串原文」。防冒充效果上等价，但签名的**可验证性**没有落地（换句话说：拿一个伪造的签名去比对也能过，因为比对的是库里的值）。

**建议**：在投票路径上改为「先 `bytes.Equal` 确认是本凭证的签名，再 `VerifyCredential` 确认签名确实由签发方对 (user_id, user_pub_key) 签出」。

### R4 · 配置好的 admin token 也会写进生产日志

`cmd/server/main.go:38-48`：

```go
adminTokenSource := "configured"
adminTokenLogLevel := logger.Info
if generatedAdminToken { adminTokenSource = "generated"; adminTokenLogLevel = logger.Warn }
adminTokenLogLevel("admin bearer token ready", zap.String("source", adminTokenSource), zap.String("admin_token", cfg.AdminBearerToken))
```

**配置的**（即长期有效的）token 以 `Info` 级别打印；只有临时生成的才降级成 `Warn`。这与 `AGENTS.md` 的描述（「未配置时…打印日志」）不一致。

**影响**：生产日志系统（ELK/Loki/CloudWatch…）会长期持有管理密钥。**建议**：只在 `generated == true` 时打印 token；`configured` 时只打印 `source=configured`，不打印值。

### R5 · 生产配置 admin token 为空，env 示例里也没有这一项 → 每次重启 token 都会变

- `configs/config.production.yaml:4` — `admin_bearer_token: ""`
- `configs/production.env.example` —— **没有** `SERVER_ADMIN_BEARER_TOKEN` 这一行
- `internal/config/admin_token.go:14-31` —— 为空则生成**仅本次进程有效**的临时 token

**后果**：按 `production.env.example` 部署 → 每次重启/扩容，admin token 都变 → 运营门户和 CLI 的 Bearer Token 全部失效，多副本之间 token 还不一样。

**建议**：① 往 `production.env.example` 补 `SERVER_ADMIN_BEARER_TOKEN=`（留空但写明必填）；② 在启动时若 `mode` 为生产且 token 是临时生成的，直接 `Fatal` 而不是 `Warn`。

### R6 · 生产配置 CORS 白名单是空的

`configs/config.production.yaml:5` — `cors_allowed_origins: []`

门户与后端不同源时，浏览器会被 CORS 直接拒绝。**部署时必须显式设置**（`SERVER_CORS_ALLOWED_ORIGINS`）。

### R7 · `configs/config.production.yaml` 永远不会被自动加载 —— 最容易误上线的坑

`internal/config/config.go:28-30`：
```go
v.SetConfigName("config")
v.SetConfigType("yaml")
v.AddConfigPath(".")
v.AddConfigPath("./configs")
```

viper 只会找 **`config.yaml`**。`config.production.yaml` 永远匹配不上。

**后果链**：运营以为「把 production.yaml 拷到 configs/ 就行」→ 实际生效的仍是 `configs/config.yaml`（`mode: debug`、`vk_path: ""`、`admin_bearer_token: "dev-admin-token"`）→ **带着公开的开发 token 上线，且证明验证处于关闭状态**。

**建议**（任选或都做）：
- 支持 `CONFIG_NAME` 环境变量指定配置文件名；
- 启动日志打印**实际生效**的关键配置（`server.mode` / `db.driver` / `db.database` / admin token 来源），让误配一眼可见；
- 在 `configs/config.yaml` 的 `dev-admin-token` 旁加注释「生产环境必须覆盖」。

### R8 · `ballot-draft` 是公共路由，且没有模式开关

`internal/infrastructure/http/handler.go:65` 把 `POST /proposals/:proposal_id/ballot-draft` 注册在**公共组**；`handler.go:316-351` 的实现里没有任何 `mode` 判断。

生产可达。严格说它不泄露选票内容（只能加密不能解密），但它把「服务端代持密码学操作」这个开发态能力暴露在公网。**建议**：非 debug 模式下返回 404，或改成 admin 路由。

### R9 · 覆盖率门槛漏掉了两个最关键的包

`scripts/check-coverage.sh:9-15`：
```bash
PACKAGES=(
  "./internal/usecase"
  "./internal/config"
  "./internal/di"
  "./internal/infrastructure/http"
  "./internal/infrastructure/http/middleware"
)
```

`internal/infrastructure/crypto` 和 `internal/infrastructure/persistence` **不在门槛内** —— 密码学与持久化的覆盖率回退**不会让 CI 失败**。

而且余量很薄：整体 81.80% vs 门槛 80%（余 1.8pp），其中 `internal/usecase` 单包已经只有 79.8%（低于 80%）。任何一次新增不带测试的改动都会破线。

**建议**：把 `crypto` / `persistence` / `cmd/cli` 纳入门槛，并同步补测后适度提高阈值（例如 85%）。

### R10 · CI 不跑前端，也不跑 gofmt

`.github/workflows/ci.yml` 只有两步：`go test ./...` + 覆盖率门槛。

前端 lint / tsc / vitest、`gofmt -l` 检查**都不在 CI 里**。本地 `make check` 有这些，但 CI 没有 —— 意味着「本地过了」不等于「CI 会过」，反之亦然。

**建议**：CI 补上 `gofmt` 检查与前端三件套（lint / tsc / test）。

### R11 · PostgreSQL 没有连接池配置

`internal/infrastructure/persistence/db.go:40-53`：
```go
return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: ...})
```
没有 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`，用的是 GORM 默认（idle 2，open 无上限）。

**影响**：多副本 + 并发投票时可能打爆 PG 的 `max_connections`。**建议**：显式设置连接池上限与空闲超时。

### R12 · 服务启动即自动改表

`internal/infrastructure/persistence/migration.go:13-28`：每次启动都 `AutoMigrate` 7 张表，并执行
```go
db.Model(&entity.Proposal{}).Where("options_json IS NULL OR options_json = ''").Update("options_json", "[]")
```

生产环境启动即 DDL，多副本同时启动有竞争风险；`AutoMigrate` 在生产上是公认危险动作。**建议**：生产模式跳过自动迁移，改为独立的迁移步骤（golang-migrate / goose）。

### R13 · admin 鉴权用非常数时间比较

`internal/infrastructure/http/middleware/admin_auth.go:27`：
```go
if strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) != expected {
```

**建议**：`crypto/subtle.ConstantTimeCompare`（需先比长度）。风险不高但改起来是几行。

### R14 · admin 鉴权缺「错误 token」与「畸形 Authorization 头」的测试

`internal/infrastructure/http/middleware/admin_auth_test.go` 只有 `missing_token` 和 `valid_token` 两个用例。缺少：错误的 token、`Basic xxx`、`Bearer`（无值）、`bearer` 小写、空 header。

**建议**补齐 —— 鉴权是唯一的权限边界，值得单独测全。

### R15 · CLI help 里的示例时间已过期

`cmd/cli/main.go:705`：
```
go run ./cmd/cli ... -start 2026-03-10T10:00:00Z -end 2026-03-10T12:00:00Z
```
README 已在 2026-10-18 改成 `2026-12-01`（`README.md:79`），**CLI help 被漏掉了**。照抄 help 的用户会创建一个已经结束的投票窗口。

**建议**：与 README 对齐。

### R16 · 自实现的 ElGamal 封装 + 裸 SHA256 做 KDF

`internal/infrastructure/crypto/kyber_elgamal.go:258-262`：
```go
func deriveKeys(shared []byte) (encKey []byte, authKey []byte) {
	hash1 := sha256.Sum256(append([]byte("enc:"), shared...))
	hash2 := sha256.Sum256(append([]byte("auth:"), shared...))
	return hash1[:], hash2[:]
}
```

没有 HKDF、没有 salt、没有显式的域分离参数绑定。整体是手写的密码学方案（用了 kyber 库的曲线原语 + Shamir，但封装/KDF 都是自己写的），不是标准或经审计的 KEM。

另外命名有误导：README 写「kyber ElGamal 封装」，但实现用的是 kyber 库里的 **edwards25519** 群，不是 Kyber KEM。

**建议**（可分两步）：
- 短期：KDF 换成 HKDF-SHA256，并显式绑定 `ephemeral_pub || recipient_pub || nonce`；
- 中期：换成标准方案（X25519 + HKDF，或 ML-KEM）。

---

## 2b. REQUIRED — 门户（web/）

> 以下 7 条来自门户专项审查，每条都有 `file:line` 证据。

### R17 · admin bearer token 明文存进 `localStorage`

`web/src/composables/useVotePortal.ts:32`（键名 `anonvote.front.admin_token`）、`:259-261`（启动时读）、`:1491-1493`（**每次按键都写**）；输入框在 `web/src/components/panels/SetupPanel.vue:41-49`。

这个 token 能创建提案、签发凭证、发布计票结果。同源下的任意脚本、共享浏览器、浏览器插件都能直接读到。

**建议**：改成 `sessionStorage` 或纯内存（刷新即失效，需重新输入）。这是本次审查里**改动最小、收益最高**的一条。

### R18 · `hasVoted` 跨提案不重置 + 「查询是否已投票」没有 UI 入口

- `hasVoted` 只在 `fetchVoteStatus`（`:1084`）里被赋值，领取/导入凭证时置空（`:646`、`:970`），**但 `watch(selectedProposalId)`（`:1495-1501`）不重置它**。
- `web/src/components/panels/VotePanel.vue:45-54` 用它渲染「你已投过这一票」→ 在 A 提案投完后切到 B，面板会**谎称 B 也投过了**。
- `fetchVoteStatus` 虽被导出（`:1623`），但 `grep fetchVoteStatus web/src` 显示**没有任何组件调用它**（只有 `submitVote` 成功后内部自调用 `:1049`）—— 文档里承诺的「查询是否已投票」能力在界面上不存在。

**建议**：`watch(selectedProposalId)` 里重置 `hasVoted`；在投票面板补一个显式的「查询投票状态」按钮。

### R19 · 错误提示 4.2 秒后自动消失，投票面板没有常驻错误态

`web/src/composables/useVotePortal.ts:435-437` — 所有 notice（含 `error`）统一 `setTimeout(..., 4200)` 自动关闭；`VotePanel` 没有内联错误横幅。

叠加 C2（生产环境每次提交都失败）→ **面板看起来和「就绪」状态一模一样**，用户根本不会知道自己投失败了。

**建议**：`error` 类 notice 不自动消失；投票面板补内联错误态。

### R20 · 引导浮层是无焦点管理的模态框

`web/src/components/GuideOverlay.vue:13-19` —— 有 `role="dialog" aria-modal="true"`，但**没有焦点陷阱、没有初始焦点、没有 Esc 关闭、背景没有 inert**。而它会在首次访问时自动打开（`useVotePortal.ts:1547-1551`）。

键盘/读屏用户一进页面就在模态框外，Tab 会跑到背后的页面里。

**建议**：加焦点陷阱 + 初始聚焦 + Esc 关闭。

### R21 · 投票人自己也要 admin token 才能领凭证

`web/src/components/panels/IdentityPanel.vue:28-40` + `useVotePortal.ts:920-923, 941-951` —— 凭证签发是 admin 接口，投票人工作区在流程上就断在这里。

**建议**：要么在文档里明确「凭证由运营方线下签发后交给投票人」，要么提供非 admin 的领取路径。**不要**为了让流程走通而降低凭证签发的鉴权。

### R22 · 门户其他位置对匿名性的描述也需要一起核对

`web/src/components/panels/ReferencePanel.vue:20-21`（「术语参考」页）对 `proof` 的说明同样暗示了当前不具备的性质。与 C5 同源，一并处理。

---

## 2c. REQUIRED — CLI 与测试体系

> 13 个命令的**方法与路径全部正确**（逐条比对 `server.go:39-52` 与 `handler.go:60-68`），响应解析与退出码（0/1/2）也都正常。以下是具体问题。

### R23 · CLI 与门户一样拿不到 `public_witness`，且没有开关可以传

`cmd/cli/main.go:316-318` 只在凭据文件里有该字段时才发送：
```go
if strings.TrimSpace(ballot.PublicWitness) != "" { body["public_witness"] = ballot.PublicWitness }
```
但唯一的生产者是 `ballot-draft`，而服务端草稿响应（`handler.go:344-350`）**不返回 `public_witness`** → `main.go:271` 的 `stringify(raw["public_witness"])` 恒为 `""`。

CLI **没有** `-public-witness` / `-proof` 之类的 flag，也**没有**能产出该字段的命令。而 `cmd/cli/main.go:696` 的用法只写了 `vote-submit -credential-file -ballot-file`，没有任何提示。

**影响**：与 C2 同源 —— 生产模式下 CLI 提交的每一票都会被拒。

**建议**：给 `vote-submit` 加 `-public-witness`（或新增 `proof-gen` 命令），并在 help 里说明「开发模式下 proof 来自 ballot-draft」。

### R24 · 没有任何测试跑通过真实 gnark 验证

所有 HTTP 层测试用的都是**空操作验证器**：

- `tests/e2e/api_test.go:62` — `crypto.NewGnarkProofVerifier("")`
- `internal/infrastructure/http/server_test.go:571` — `cryptoinfra.NewGnarkProofVerifier("")`

`vkPath == ""` → `requireVerifyMode=false` → `gnark_proof_verifier.go:55-57` 直接 `return nil`。

更刺眼的是：e2e 测试**真的构造了一个有效的 Groth16 proof**（`api_test.go:170` `mustBuildProofBytes`），但验证器把它忽略了；而且 `voteReq`（`:171-179`）**根本没带 `public_witness`**。真实验证只在验证器单元测试层面被覆盖（`gnark_proof_verifier_test.go:38`、`tests/integration/crypto_test.go:64`），**从未经过 handler → usecase → 验证器这条链**。

**影响**：整条 proof 链路（`public_witness` 字段、`PublicInputs()`、错误到 HTTP 状态码的映射）都是零覆盖。**如果有人改坏 handler 丢掉 `public_witness`，CI 依然是绿的。**

**建议**：把 vk 写到 `t.TempDir()`，用真实 vk 构造 engine，断言 201（有效 proof+witness）/ 400（缺 witness）/ 400（篡改 witness）。这条测试同时能挡住 C2/C3 的回归。

### R25 · CLI 把明文选项发给服务端（与 C4 同源，但性质更差）

`cmd/cli/main.go:319-321`：
```go
if strings.TrimSpace(ballot.SelectedOption) != "" { body["selected_option"] = ballot.SelectedOption }
```
后端 `submitVotePayload`（`handler.go:70-79`）没有这个字段，gin 也没开 `EnableDecoderDisallowUnknownFields` → **服务端静默忽略**。

但注意：**这个字段在网络上传输的是明文选项**。也就是说 CLI 每投一票，都在把「你选了哪个」明文送到服务端 —— **直接抵消了「选票是加密的」这个属性**，而服务端根本不消费这个字段。

门户侧是同一个 bug（`useVotePortal.ts:1023-1025`），但浏览器到服务端这一段也走的是明文 HTTP 请求体 —— 除非有 TLS，否则同样明文可见。

**建议**：CLI 与门户都删掉 `selected_option`；并在文档里明确「加密发生在密文里，不是传输里，生产必须走 HTTPS」。

### R26 · CLI 的 `decrypt-setup` 默认把门限份额打印到 stdout

`cmd/cli/main.go:398-414` —— 不带 `-out` 时直接 `printJSON(out)`，而服务端响应里包含 `"shares": setup.Shares`（`handler.go:391`）。

**影响**：**门限解密份额**（能解密全部选票的密钥材料）落到终端回滚缓冲 / CI 日志 / 截屏里。

**建议**：份额必须显式 `-out` 落盘（并 `0o600`），或打印时把 `shares` 字段抹掉。

### R27 · CLI 的 URL 路径被二次转义，含空格或 `/` 的 proposal id 必然 404

`cmd/cli/main.go:563` — `base.Path = strings.TrimRight(base.Path, "/") + path`，而 path 在 `:258/:347/:374/:400/:436/:459/:482` 已被 `url.PathEscape` 转义过一次。`base.Path` 是**解码后**的路径，`url.URL.String()` 会再转义一次（`%` → `%25`）。

**实测复现**（本机）：
```
go run ./cmd/cli -base-url http://127.0.0.1:9 result-get -proposal "a b/c"
→ Get "http://127.0.0.1:9/api/v1/proposals/a%2520b%252Fc/results"
```

**影响**：任何含空格、`/` 或 `%` 的 proposal id 都会 404。路径穿越本身被挡住了（`/` 被转义），所以这是**正确性 bug，不是安全漏洞**。

**建议**：构造 `url.URL{Path: ...}` 时不要预先 `PathEscape`，或同时设置 `RawPath`。

### R28 · admin token 作为 CLI flag，会出现在进程列表和 shell 历史里

`cmd/cli/main.go:62` 的 `-admin-token`，且 help 文本（`:705-709`）直接把 `dev-admin-token` 写在示例里。

共享主机上 `ps` / `/proc/<pid>/cmdline` 可见。**好消息**：已核实 token **不会**出现在错误信息里（`main.go:522-556` 返回的 `*url.Error` 只带 URL 不带 header），所以这是唯一的泄漏路径。

**建议**：优先读 `NULLIFIER_ADMIN_TOKEN`；flag 只作为兜底，且不要在 help 示例里展示。

### R29 · 覆盖率脚本覆盖了 Go 的模块/构建缓存，反噬 CI 的 `cache: true`

`scripts/check-coverage.sh:18-19`：
```bash
export GOMODCACHE="${GOMODCACHE:-$(pwd)/.gomodcache}"
export GOCACHE="${GOCACHE:-$(pwd)/.gocache}"
```
在干净的 CI runner 上这两个变量默认都为空（已实测本机为空），于是整个模块集被重新下载进 `./.gomodcache`，构建缓存也被丢弃 —— 与 `.github/workflows/ci.yml:22-23` 的 `cache: true` 相抵触。

**建议**：从脚本里删掉这两个覆盖，交给 CI 控制。

---

## 3. NIT

| # | 位置 | 说明 |
|---|---|---|
| N1 | `internal/usecase/voting.go:234` / `internal/usecase/tally.go:386` | 时间窗口语义在两个用例各写了一份（`isProposalOpenForVoting` / `isProposalClosed`），改规则要同步改两处。建议抽到共享 helper。 |
| N2 | `internal/domain/entity/vote.go:9` | `Nullifier` 是**全局** `uniqueIndex`，不是按提案唯一 —— 同一 nullifier 不能跨提案复用。若后续要做「一票一提案」的复用语义需注意。 |
| N3 | `internal/infrastructure/http/middleware/error_handler.go:68-69` | `INVALID_REQUEST` 直接回 `err.Error()`，会透出内部细节；其他 code 都做了归一化。前端 `error-messages.ts` 是按英文短语正则匹配的，改后端文案必须同步改前端映射。 |
| N4 | `scripts/check-coverage.sh:20` | `export GOSUMDB="off"` 削弱了依赖校验，CI 里同样生效。建议只在本地脚本保留，CI 用默认值。 |
| N5 | 仓库根 | 无 `CHANGELOG.md`、无 `VERSION` 文件、`.github/workflows/` 只有 `ci.yml`（无 release workflow）。打 tag 后没有自动化发版/产物流程。 |
| N6 | `web/package.json:3` | 前端 `version` 原本写死 `"1.0.0"`，与后端无版本概念。**已修**：`web/package.json` 的 version 改为跟随仓库根的 `VERSION`（单一版本源），发布流水线在构建前注入。 |
| N7 | 仓库根 | `coverage.out` / `coverage.gitbash.out` / `probe.out` / `internal_http.cover.out` 散落（已 gitignore，`make clean` 可清）。 |
| N8 | `configs/config.yaml:4` | `admin_bearer_token: "dev-admin-token"` 明文在会被自动加载的默认配置里。配合 R7，这是「用开发 token 上生产」的完整路径，建议加注释警示。 |
| N9 | `web/src/composables/useVotePortal.ts` | 死代码导出（grep 确认无消费方）：`showAdvancedVoteFields`（`:311`,`:1616`）、`startGuideJourney`（`:464`,`:1671`，与 `App.vue:166-171` 重复）、`openGuideMode`（`:454`,`:1669`）、`visibleSections`（`:402`,`:1575`）、`sections`（`:41`,`:1574`）、`workspaces`（`:36`,`:1573`）。 |
| N10 | `web/src/composables/useVotePortal.ts:41-84` / `AppSidebar.vue:20-36` / `lib/portal-view.ts:4-25` | **导航的三个真相源**，标题文案在 `portal-view.ts` 与侧边栏硬编码之间重复。改一处要同步两处。 |
| N11 | `web/src/lib/api.ts:12` | `ApiCallOptions.signal` 从未被任何调用方传入（死选项）；`apiCall` **没有超时**，只有调用方能传 `signal` 才会中断。 |
| N12 | `web/src/lib/error-messages.ts:160-161` | 未命中已知短语的 `INVALID_REQUEST` 会**透出后端英文原文**（如 `invalid request: load credential: …`）。与 N3 是同一个问题的两端。 |
| N13 | `web/src/composables/useVotePortal.ts:604`、`:654` | 导出的凭证文件名与 toast 里都带 `user_id`。凭证材料本身只存内存（这点是对的），但标识符外泄。 |
| N14 | `web/src/index.css:1` | 从 CDN 引 Google Fonts。离线/内网部署会静默回退到系统字体（功能不受影响，但字重会变）。 |
| N15 | `tests/testutil/db.go:11` | `SetupTestDB` 是**死代码**，全仓库零调用方。 |
| N16 | `tests/e2e/README.md:3` | 仍写着「端到端测试目录占位」，但该目录已有 536 行的 e2e 套件。 |
| N17 | `cmd/cli/main.go:36-39` | `credentialFile` 没有 `id` 字段，服务端返回的 `id`（`handler.go:187`）在 `-out` 文件里被丢掉；`main.go:213` 还自己编了一个服务端从未返回的 `"version": 1`。 |
| N18 | `cmd/cli/main.go` | 退出码不统一：根用法错误退 2（无参/未知命令），子命令用法错误退 1（缺 `-title`、服务不可达）。多传的裸位置参数被静默忽略。 |
| N19 | `internal/infrastructure/http/middleware/cors.go:41-48` | 只对 `OPTIONS` 预检请求做 403 拒绝；**简单跨域请求会照常执行**，只是浏览器拿不到响应。`cors_test.go:50` 也只覆盖了预检这一种。 |
| N20 | `go.mod:3` | `go 1.25.4` 锁的是**补丁版本**，CI 不会自动跟进 1.25.x 的安全补丁。 |

**已核实干净的部分**（避免下次重复排查）：`web/src` 内**无** `v-html` / `innerHTML` / `console.*` / `any` / `@ts-ignore`；表单 label 关联正确（`label[for]` / `fieldset`+`legend` / `aria-label`，见 `OpsPanel.vue:180,188`）；选项行编辑器键盘可操作且增删后焦点自动落到相邻输入框（`OpsPanel.vue:34-54`）；全局 `:focus-visible` 焦点环存在（`base.css:97-100`）；`.grid-2` 在 560px 断点折叠（`shell.css:297-301`），无响应式断裂；`PortalApi` 类型由 composable 派生（`types/portal-api.ts:11`），面板与 composable 的类型不会漂移。

---

## 4. 发版前动作清单

### 4.1 决策项（需要你拍板）

- [ ] **D1**：选路径 A（内测版 v0.x，标注证明链未实现）还是路径 B（生产版，先补 C1–C6）
- [ ] **D2**：R2 盲签名 —— 改实现（两周量级）还是改文档（一小时）
- [ ] **D3**：是否接受在生产保留 `ballot-draft` 公共端点（我建议不留）
- [ ] **D4**：版本号口径与首个版本号（仓库目前没有任何版本号来源）
- [ ] **D5**：C5 / R22 门户匿名性文案 —— 改文案（立刻可做）还是等客户端加密（B2 一起做）

### 4.2 阻断项（路径 A 也建议做）

- [ ] **A1**（C4）**前端：凭据绑定目标选项** —— 记录 `ballotProposalId`/`ballotOption`，选项或提案变化即清空 `encryptedVote`/`proof`，提交前校验一致，不一致禁用提交
- [ ] **A2**（C5）**修正门户匿名性文案** —— `VotePanel.vue:250-252` 与 `ReferencePanel.vue:20-21` 改为「当前开发模式下服务端可见明文选项」
- [ ] **A3**（C2 错误码）proof 校验失败改为 **400 INVALID_REQUEST**（`voting.go:148` 包装 `ErrInvalidRequest`），不再返回 500
- [ ] **A4**（R7）启动日志打印实际生效的 `server.mode` / `db.driver` / `db.database` / admin token 来源
- [ ] **A5**（R5）`production.env.example` 补 `SERVER_ADMIN_BEARER_TOKEN` 与 `SERVER_CORS_ALLOWED_ORIGINS`，并在生产模式下「临时 token」直接启动失败
- [ ] **A6**（R4）配置态 admin token 不再进日志
- [ ] **A7**（R17）admin token 改存 `sessionStorage` 或纯内存
- [ ] **A8**（R18）`watch(selectedProposalId)` 重置 `hasVoted`；补「查询投票状态」按钮
- [ ] **A9**（R19）error 类 notice 不自动消失；投票面板补内联错误态
- [ ] **A10**（R15）CLI help 示例时间改为 `2026-12-01`
- [ ] **A11**（R1）`vk_path` 为空时启动打 Warn（明确提示「当前不验证选票证明」）
- [ ] **A12**（C6）**README 覆盖率徽章改为全仓口径（60.1%）**，或明确标注「仅核心 5 包」
- [ ] **A13**（R26）**CLI `decrypt-setup` 不再默认把门限份额打印到 stdout**
- [ ] **A14**（R25）CLI 与门户都删掉 `selected_option`（它在明文传输明文选项，服务端还不消费）
- [ ] **A15**（R24）补一个跑通真实 gnark 验证的 HTTP 层测试（vk 写 `t.TempDir()`，断言 201/400/400）
- [ ] **A16**（R29）覆盖率脚本删掉 `GOMODCACHE` / `GOCACHE` 覆盖

### 4.3 生产化必须做（路径 B）

- [ ] **B1**（C1）产出真实的选票电路 + `verifying.key` 生成流程（含密钥分发与轮换说明）
- [ ] **B2**（C2）客户端 prover：真实 proof + `public_witness` 的生成，并把门户「按选项生成凭据」接到它
- [ ] **B3**（C3）把 `proposal_id` / `nullifier` / `hash(ciphertext)` 写进电路公开输入，并让 `Verify` 真正校验它们与请求字段一致
- [ ] **B4**（C5 根治）把加密移到客户端 —— 服务端只验不生成，从根上解决「服务端看到明文选项」
- [ ] **B5**（R2）盲签名改真实两段式，或按 D2 改文档
- [ ] **B6**（R3）投票路径上补 `VerifyCredential`
- [ ] **B7**（R8）`ballot-draft` 在非 debug 模式下关闭
- [ ] **B8**（R9）覆盖率门槛纳入 crypto / persistence / cmd，并补测
- [ ] **B9**（R10）CI 补 gofmt + 前端三件套
- [ ] **B10**（R11）PostgreSQL 连接池配置
- [ ] **B11**（R12）生产模式关闭启动自动迁移，改独立迁移步骤
- [ ] **B12**（R13）admin token 常数时间比较
- [ ] **B13**（R14）admin 鉴权补齐错误 token / 畸形头测试
- [ ] **B14**（R16）KDF 换 HKDF（或换标准 KEM）
- [ ] **B15**（R6）生产 CORS 白名单填入门户真实源
- [ ] **B16**（R20）引导浮层补焦点陷阱 + Esc 关闭
- [ ] **B17**（R21）明确投票人领凭证的路径（文档或非 admin 接口），**不要**为图顺降低凭证签发鉴权
- [ ] **B18**（R23）CLI `vote-submit` 增加 `-public-witness` / `-proof` 入口
- [ ] **B19**（R27）CLI URL 构造修掉二次转义（`url.URL{Path:...}` 不预转义，或设置 `RawPath`）
- [ ] **B20**（R28）CLI 优先读 `NULLIFIER_ADMIN_TOKEN`，help 示例不展示 token
- [ ] **B21**（C6）覆盖率门槛纳入 `crypto` / `persistence` / `cmd/cli` / `cmd/server`，并给 `cmd/server` 补冒烟测试

### 4.4 发版流程（**审查当时完全缺失，现已全部落地**）

> 本节用本节自己的编号（**P1–P4**）。§2 的 CRITICAL 发现也用 C 编号（C1–C6），两套不是一回事，别串。

- [x] **P1** 约定版本号来源（`VERSION` 文件 or tag 推导），`web/package.json` 与之对齐
- [x] **P2** 建 `CHANGELOG.md`
- [x] **P3** 加 release workflow（打 tag → 构建前后端产物 → 建 Release）
- [x] **P4** 打 tag —— **v0.1.0 已于 2026-10-20 发布**（原快照此处写的是「仓库目前 `git tag -l` 为空，从未发过版」）

### 4.5 文档

- [ ] **D1**（若选路径 A）在 `README.md`、`docs/techspec.md`、门户「术语参考」页三处明确标注「生产模式需外部钱包生成 proof」+「当前开发模式下服务端可见明文选项」
- [ ] **D2** 更新 `docs/techspec.md` 记录 C1–C6 的真实边界（当前 §6「密码学实现边界」写得偏乐观）
- [ ] **D3** 新增 `docs/project-map.md`（本次已产出）并在 `AGENTS.md` 中挂上引用
- [ ] **D4** README 补一条「生产部署必须设置 `SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS`，且 `config.production.yaml` 需改名或用环境变量」的说明（R5/R6/R7）

---

## 5. 我没有验证到的（请勿当作已通过）

- **真实 gnark 电路的正确性** —— 电路不存在（C1），无从验证
- **PostgreSQL 集成测试**（`tests/integration/persistence_postgres_test.go`）—— 本机无 Docker，未实跑；`go test ./...` 中该包是 ok（走的是跳过分支）。已核实该文件在 CI 环境下**会**因缺 Docker 而 `require.NoErrorf` 失败（`:139-142`），但这条分支是读出来的，没实跑
- **前端 7 个 `.vue` 面板的浏览器实际渲染与交互** —— 仓库里 `.vue` **零测试**，只有 6 个 `lib/` 模块有 vitest（37 用例）。面板的问题只能靠人工验证
- **CLI 打到真实运行中服务端的行为** —— CLI 的 13 个命令的**方法与路径已逐条核对无误**，URL 二次转义是**实测复现**的；但「提交投票被服务端拒绝」的完整链路是静态推导，没有对着运行中的 release 模式服务端跑通
- **并发投票下的防重投** —— 事务路径（`SaveWithParticipation`）有 DB 唯一索引兜底，`persistence/vote_repository_test.go:73/134/170` 三条回滚路径都有测试，逻辑上成立，但没做过并发压测
- **性能与容量** —— 未测
- **`gh` release / tag 推送流程** —— 仓库无 tag 历史，无 release workflow 可验
- **`actions/setup-go@v5` 是否导出 `GOMODCACHE`/`GOCACHE`** —— 决定 R23 的实际影响，action 源码不在本仓库内
- **生产是否走 HTTPS** —— 决定 R19（`selected_option` 明文传输）的实际严重程度；仓库内无 TLS 配置

---

## 6. 已核实「没问题」的部分（不必重复排查）

- **CLI 的 13 个命令，方法与路径全部正确**，与 `server.go:39-52` / `handler.go:60-68` 逐条一致；响应解析正常；退出码 0/1/2 行为可预期
- **CLI 的错误处理完整**：服务不可达 / 非 2xx / 非法 JSON / 缺参数 / 文件读写错误都有明确中文错误；**admin token 不会出现在错误信息里**
- **凭证文件以 `0o600` 落盘**（`cmd/cli/main.go:591`）
- **事务回滚有三条测试覆盖**：重复参与 → 票未落库；重复 nullifier → 参与记录回滚；参与写入失败 → 全回滚（`persistence/vote_repository_test.go:73/134/170`）
- **CORS 预检拒绝有测试**（`cors_test.go:50`）
- **请求日志不记 header 和 body**（`middleware/request_logger.go:19-26`），只记 method/path/status/latency/ip
- **`web/src` 无 `v-html` / `innerHTML` / `console.*` / `any` / `@ts-ignore`**
- **门户表单 label 关联正确，选项行编辑器键盘可操作且增删后焦点自动落位**
- **仓库无密钥入库**（`git ls-files` 全量扫描，161 个跟踪文件全部干净）
- **覆盖率脚本本身的解析与门禁逻辑是正确的**（Go 1.25/1.26 的 padding 差异已处理，阈值 90 时确实退出 1）

---

## 附：本次审查新增的上下文工程文档

`docs/project-map.md` —— 分层索引（入口 / 领域 / 用例 / HTTP / 持久化 / 密码学 / 前端 / CLI）+ 5 条主链路 + 8 个 Top 陷阱 + 验证手册 + 「未完成的部分」清单。目的是让后续 agent 在进入某个子系统前只加载那一节，而不是通读仓库。
