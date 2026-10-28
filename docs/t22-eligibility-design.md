# T22 · 资格证明设计方案（一人一票）

> 状态：**已实现（v0.2.0）** —— 设计稿成文于 2026-10-25，实现落地于 2026-10-25。
> 目标：把能力 G 的完成定义补齐 —— 在**不破坏不可关联性**的前提下，让服务端能强制「一人一票」。
> 对应路线图任务卡：[`docs/roadmap.md`](./roadmap.md) 附录 A.3 的 T22（✅ 已落地）。
>
> **落地范围**：资格电路（`internal/infrastructure/crypto/eligibility/`）、离线名单工具
> （`cmd/eligibility-list`）、服务端（challenge 端点 + claim 扩展 + 去重表 + 运行期配置）、
> CLI（`credential-claim -eligibility`）、浏览器端（同一个 WASM 模块的资格 API + 门户「身份」面板）、
> 端到端验收（`tests/e2e/eligibility_test.go` 与 `tests/e2e/wasm_eligibility_test.go`）。
> 本文档的分析与论证保留原样（那是决策依据）；**实现与本文的偏离见 §8**。

---

## 1. 问题到底是什么

T9 把凭证改成**匿名随机领取**之后，服务端拿到的是池里槽位的随机假名 `user_id`：

- 服务端能知道「槽位 S 被领了」，但不知道「谁领的」；
- 于是服务端也看不出同一个人领了几次、投了几次 —— **任何人都能反复领取并投票**。

这不是 bug，是一次明确的取舍：**用「可重复投票」换来了「不可关联性」**。T22 要把这笔账补平。

**要补平它，服务端必须能做一件事**：对某个东西去重，并且要求「同一个人只能有那一个」。
所以设计的第一性问题不是「用什么密码学」，而是：

> **服务端拿什么当去重键？那个键由谁分配？**

### 1.1 三条推论（决定了后面所有取舍）

**推论 1：客户端自由生成的标识，服务端去重没有意义。**
如果去重键是客户端自己随机生成的（设备密钥、浏览器指纹……），客户端生成 N 个就能领 N 张票。
服务端的唯一索引拦不住「同一个人」。**去重只有在键由权威方唯一分配时才有意义。**

**推论 2：权威方一旦掌握「谁领了哪个资格」，就可能把它与票关联起来。**
服务端必须记住「资格 X 已经用过」（否则拦不住第二次领取），而投票时提交的凭证材料里如果也带着资格标识，
服务端就能把「票」连到「资格 X」。若权威方知道 X 是谁，**可关联性就破了**。

**推论 3：解开这个矛盾的唯一办法是「零知识地把资格与身份隔开」。**
服务端要能验证「这个资格是真的、且没被用过」，但**不能知道这个资格背后是谁**。
这需要匿名凭证（anonymous credential）而不是普通的签名或哈希。

> **一句话**：T22 = 「权威分配资格」+「零知识地出示资格」。缺任何一个，一人一票都只是「一设备一票」。

---

## 2. 方案对比

| | **方案 A：设备指纹去重** | **方案 B：匿名名单 + ZK 成员资格证明**（推荐） | **方案 C：实名领取资格** |
|---|---|---|---|
| 资格从哪来 | 客户端本地生成的长期密钥 | **离线工具**按权威名单生成，成员各自持有 | 运营方按实名发放 |
| 去重键 | `H(设备公钥)`，客户端自由生成 | `QualificationID = MiMC(Leaf, proposal_id)`，由名单派生 | 成员 ID 的哈希 |
| 持有证明 | 对挑战的 Schnorr 签名 | **零知识证明**（新电路） | 无需证明（运营方自己知道） |
| 一人一票强度 | ❌ 一**设备**一票（换浏览器/清缓存即可绕过） | ✅ 一**人**一票（只要名单权威且一人一份） | ✅ 一人一票 |
| 不可关联性 | ✅（指纹与真人无关联） | ✅（名单叶子是 hiding 的，**服务端只见 root**） | ❌ 运营方知道「谁领的资格」→「资格 ↔ 票」可关联 |
| 名单由谁掌握 | 无 | 离线工具 + 成员本人（**服务端只见 root**） | 运营方 |
| 工作量 | 1–2 天 | **5–7 天**（新电路 + KAT + 名单工具 + 客户端接线） | 2–3 天 |
| 真实身份源依赖 | 无 | 需要一份权威名单（v0.4.0 的「身份源对接」就是喂这份名单） | 需要运营方有实名名录 |
| 结论 | **降级方案**，不满足能力 G 的完成定义 | ✅ **主线** | ❌ 破坏能力 G |

**为什么推荐 B 而不是 A**：A 看起来便宜，但它把「一人一票」永远留在「一人一设备」的水平上 ——
而能力 G 的完成定义要的是「一人一票」。B 的额外成本主要在**新电路**，而电路栈（MiMC / Groth16 / WASM prover /
KAT 纪律）本仓库已经齐备，不需要引入任何新的密码学依赖（离线环境装不了新库）。

**为什么 B 不需要 C**：C 的问题不是「需要实名」，而是「实名的那一方同时是投票服务方」。
B 把两者分开：**名单由离线工具生成，工具的输出（成员包）走安全渠道发给成员，投票服务方只拿到 root**。
服务方因此既不知道谁在名单里，也没法把票连到人。

---

## 3. 推荐方案（B）的完整设计

### 3.1 名单与叶子

离线工具（CLI `cmd/eligibility-list`）输入权威成员名单（每行一个成员 ID）：

```
成员 ID ──(工具)──> 资格秘密 secret（32 字节随机）
                      叶子 leaf = MiMC(secret)                 ← hiding：知道 leaf 推不出 secret
                      Merkle 树（深度 depth，叶子按成员 ID 排序）
                      root = 树根
```

输出两份，**必须分开保管**：

| 产物 | 去向 | 内容 |
|---|---|---|
| `root.json` | **服务端配置** | `root`（一个域元素，十六进制）+ `depth` + `count` |
| `eligibility-<member_id>.json` | 成员本人（安全渠道） | `member_id` + `secret` + `leaf` + `index` + Merkle path（兄弟节点）+ `root` + `depth` |

服务端**只拿到 root**：它知道「有一个 root」，知道每个来领票的人提交的 `leaf` 在树里，
但**无法把 leaf 对应到任何成员**（它没有成员包，也没有名单）。

> ⚠️ 运营纪律：名单工具的运行机器与成员包的保管方式，必须与投票服务分开。工具持有「成员 ↔ secret」映射，
> 谁拿到成员包谁就能投票 —— 这是资格发放的固有性质，不是缺陷。

### 3.2 电路（新增 `eligibilityCircuit`）

复用现有栈：Groth16 / BN254，域 `fr`，哈希用 **MiMC**（`gnark v0.7.1` 的 `std/hash` 只有 MiMC，
而本仓库的 MiMC 已经有 Go↔TS 双向 KAT，见 `ballot/nullifier_interop_test.go`）。

**公开输入**（服务端与客户端都能拿到，**定长 5 个**）：

| 名字 | 含义 |
|---|---|
| `Root` | 名单根（域元素） |
| `ProposalID` | 提案域元素（`ballot.DeriveProposalField`） |
| `Challenge` | 服务端本次领取会话的一次性挑战 |
| `QualificationID` | 去重键 = `MiMC(Leaf, ProposalID)` |
| `SessionBinding` | 会话绑定 = `MiMC(QualificationID, Challenge)` —— 由**服务端自算** |

**私有见证**：`Secret`（资格秘密）、`Index`（叶子位置）、`Path`（Merkle path，兄弟节点数组）。

> ⚠️ **实现与本节原文的一处偏离**：设计稿原文把 `Leaf` / `Path` / `Index` 也列进公开输入
> （7 个）。**实现把它们全部改成私有见证**，公开输入只留上表 5 个 —— 公开输入的个数因此
> 是**定长**的，不随树深变化，服务端也不需要知道树的结构。详见 §8。

**约束**：

1. `Leaf = MiMC(Secret)`（叶子由秘密单向决定）
2. `QualificationID == MiMC(Leaf, ProposalID)`
3. `SessionBinding == MiMC(QualificationID, Challenge)` —— 证明因此被钉在本次领取会话上
4. `Index` 的比特分解必须能复原回 `Index` 本身（`ToBinary` 的宽度是编译期常量 `TreeDepth`）
5. Merkle 自底向上：`node = MiMC(left, right)`，左右由 `Index` 的比特选；末端 `node == Root`

**约束数**：**实测 6860 条**（`internal/infrastructure/crypto/eligibility/eligibility_test.go`
的 `TestConstraintsAreReasonable`，护栏是 < 12000），与现有选票电路（7917）同量级 ——
浏览器端出证明仍在秒级。

**`TreeDepth = 10` 是编译期常量**（`eligibility/circuit.go`）：电路里 `ToBinary` 的宽度必须是
常量，因此电路、名单工具与客户端对树深的认知必须一致。单份名单上限 **1024 人**；
需要更大时改这个常量并重新生成密钥（**不是配置项**）。

**为什么 `QualificationID` 要按提案派生**：`Leaf` 是成员级的（一份名单可复用于多个提案），
但 `QualificationID` 含 `ProposalID` → **跨提案不可关联**（满足 T22 验收 ④），
且服务端按 `(proposal_id, qualification_id)` 去重。

### 3.3 协议（四个请求，全程服务端拿不到真人身份）

```
① POST /api/v1/proposals/:proposal_id/credentials/eligibility-challenge   （公开，无状态）
   → { proposal_id, root, depth, challenge }
      challenge 是服务端随机 nonce；服务端**不记账**（重放由去重键挡住，见 §3.5）

② POST /api/v1/proposals/:proposal_id/credentials/claim   （公开）
   请求体新增：{ eligibility_proof, qualification_id, challenge }
      —— 三件要么都给要么都不给（只给一部分 → 400 INVALID_REQUEST；
         只给一半不能当成「没带资格」放行）
      —— 没开启资格校验时，整个请求体可以为空（既有客户端就是这么发的）
   服务端：用**自己配置的** root + 提案 ID + 客户端给的 challenge 与去重键，
        自拼出五个公开输入 → 验资格证明（资格电路 verifying key）
        → 校验 (proposal_id, qualification_id) 未被用过   ← 去重在这里发生
        → 预留池槽位，返回 { proposal_id, user_id, issuer_key_version,
                            variant, modulus, exponent }

③ POST /api/v1/proposals/:proposal_id/credentials/blind-sign  （公开，不变）
④ POST /api/v1/proposals/:proposal_id/credentials/attest      （公开，不变）
```

**关键点：资格校验在 claim 这一步**（路线图验收 ①「不通过就不给凭证」的要求）。
claim 失败**不消耗池槽位** —— 与「先发凭证、投票时再拒」相比不会浪费槽位。

**新错误码**：

| 错误码 | HTTP | 含义 |
|---|---|---|
| `ELIGIBILITY_REQUIRED` | 400 | 开了资格校验但请求体没带资格材料 |
| `INVALID_ELIGIBILITY_PROOF` | 403 | 资格证明验签失败 |
| `ELIGIBILITY_ALREADY_USED` | 409 | 同一份资格在这个提案里已经领过 |
| `ELIGIBILITY_NOT_ENABLED` | 409 | 服务端没开资格校验，却收到了资格材料（也用于请求 challenge） |

> 三件材料**只给一部分**时返回的是 `INVALID_REQUEST`（400），不是 `ELIGIBILITY_REQUIRED` ——
> 那是客户端的错，不是「没带资格」。

**为什么不把资格材料塞进 blind-sign**：blind-sign 是公开端点，签发方在那里做盲签名计算。
资格不通过却先消耗签发方算力 = 一个现成的 CPU 消耗面。放在 claim 之前更干净。

**对现有协议的影响面**：
- `claim` 的请求体从「空」变成「带资格证明」—— 这**不破坏 T17 的盲签名性质**：
  资格材料里没有任何与凭证消息相关的东西（服务端仍然看不到凭证公钥，盲化依然有效）。
- `blind-sign` / `attest` **完全不变** → 现有客户端与 e2e 用例不受影响。
- **没开资格校验的部署完全不受影响**：旧的 claim 请求体（空）照常工作。

### 3.4 服务端存储

新增表 `eligibility_uses`（GORM 实体 `entity.EligibilityUse`）：

```
eligibility_uses
  proposal_id      TEXT     -- 提案 id
  qualification_id TEXT     -- 去重键，64 位小写 hex（域元素的规范编码）
  used_at          TIMESTAMP
  UNIQUE(proposal_id, qualification_id)
```

- 「落库只存哈希」：`qualification_id` 本身就是 `MiMC(Leaf, ProposalID)` —— hiding 的域元素，
  存它不等于存身份。**不存** leaf、**不存** secret、**不存** 成员 ID。
- 唯一索引就是「一人一票」的强制点：第二次用同一资格 → 唯一约束冲突 → 409
  `ELIGIBILITY_ALREADY_USED`。
- ⚠️ 这张表**不能**与凭证、票据关联：一旦能把资格连到票，能力 G 就破了。
  去重只需要在领取时做一次。

### 3.5 重放与 DoS

| 攻击 | 结果 |
|---|---|
| 重放同一份资格证明 | 第二次 `claim` 撞唯一索引 → 409（`challenge` 无需服务端记账，正是为此） |
| 拿 A 的资格证明抢先领 | 能领走，但 A 就领不到了（DoS 存在，**不丢票**）。要根治需 challenge 绑定到具体槽位后再验 —— 代价是多一步交互，v0.2.0 不做 |
| 伪造资格证明 | 电路约束挡住（无法造出 `MiMC(secret) == leaf` 且 leaf 在 root 树里的证明） |
| 刷 claim 预留空槽 | **现状已存在**（claim 不收任何东西就能预留），不因 T22 恶化；但 T22 之后 claim 会验资格，攻击面反而变小 |

### 3.6 客户端

**离线名单工具**（新命令 `cmd/eligibility-list`）：

```
go run ./cmd/eligibility-list -members members.txt -out ./eligibility-out
```

输入权威成员名单（每行一个成员 ID，`#` 开头与空行忽略，按 ID 排序保证同一份名单两次生成的
树根一致），输出两份**必须分开保管**的产物：

| 产物 | 权限 | 去向 |
|---|---|---|
| `root.json` | 0644 | **服务端配置**（`crypto.eligibility_root` 取其中的 `root`） |
| `eligibility-<member_id>.json` | 0600 | 成员本人（安全渠道发放） |

**CLI**（`cmd/cli` 的 `credential-claim` 命令加两个参数）：

```
go run ./cmd/cli credential-claim -proposal p-1 \
  -eligibility eligibility-me.json -proving-key keys/eligibility-proving.key
```

读成员包 → 取 challenge → 用**本地**资格 proving key 出证明 → 带证明 claim。
`-eligibility` 必须与 `-proving-key` 同时给（只给一个直接报错）。

**密钥**（`cmd/keygen -circuit ballot|eligibility`）：两套电路的密钥**互不相通**
（Groth16 的 setup 是 per-circuit）。默认文件名：

| 电路 | verifying key | proving key |
|---|---|---|
| `ballot` | `verifying.key`（0644） | `proving.key`（0600） |
| `eligibility` | `eligibility-verifying.key` | `eligibility-proving.key` |

**浏览器**：WASM prover 需要新增「资格证明」能力。
- ⚠️ **实现与本节原文的偏离**：设计稿原文设想的是**两个** WASM 模块
  （`eligibility-prover.wasm` + `eligibility-proving.key`）。**实现是同一个模块四个 API**
  —— `cmd/ballot-wasm` 在全局对象 `nullifierProver` 上注册 `init` / `prove` /
  `initEligibility` / `proveEligibility`，两套电路编进同一个 `.wasm`。
  新增的静态资源**只有** `web/public/eligibility-proving.key`（`make web-eligibility-key PK=<路径>`）。
  详见 §8。
- 门户「身份」面板新增「导入资格文件 → 领凭证时本地出证明」；
  资格包**只留内存**，不写 localStorage、不上传，刷新页面就要重新导入。

### 3.7 不变量清单（实现时逐条对照）

| 能力 | 是否受影响 | 怎么保证 |
|---|---|---|
| A 服务端不得拿到明文选项 | 不受影响 | 资格证明里没有选项；`tests/architecture/` 的守卫继续有效 |
| B 选票可验证 | 不受影响 | 选票电路不动；资格电路是独立的第二套密钥 |
| C proof 与票绑定 | 不受影响 | 选票 proof 的公开输入构成不变 |
| G 分发不泄露身份 | **必须保持** | ① 服务端只见 root，不见成员包；② 资格材料里没有真人身份字段（e2e 断言请求体字段集合）；③ 投票提交的材料**不含** `qualification_id` |
| 防重投 | 不受影响 | `votes.nullifier` 唯一索引照旧 |

> ⚠️ 特别提醒：投票提交体里**不能**带 `qualification_id`。
> 一旦带上，服务端就能把「这张票」与「领取时用的资格」连起来 —— 虽然资格本身匿名，
> 但这会让「资格 ↔ 票」的关联成立，削弱能力 G。**去重只需要在 claim 时做一次。**

---

## 4. 验收口径（对齐路线图 T22 的五条）

| # | 验收 | 怎么测 | 证据 |
|---|---|---|---|
| ① | 无资格证明的 claim → 拒绝（4xx），**池槽位不被消耗** | e2e：claim 前后池里空槽数不变 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityRequiredForClaim_E2E`（400 `ELIGIBILITY_REQUIRED`，随后合格者仍能领到槽位） |
| ② | 有效资格证明 → 领取成功 | e2e：资格文件 → 证明 → claim → 走完盲签名两步拿到凭证 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityClaimSucceeds_E2E` |
| ③ | 同一份资格证明第二次领取 → 拒绝 | e2e：第二次 claim → 409（唯一索引） | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityCannotBeReused_E2E`（409 `ELIGIBILITY_ALREADY_USED`） |
| ④ | 资格证明在不同提案间不可复用 | e2e：提案 A 的证明拿去领提案 B → 验签失败（`ProposalID` 在公开输入里） | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityNotReusableAcrossProposals_E2E` |
| ⑤ | 领取请求体里不含任何真人身份字段 | e2e 断言请求体字段集合 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityClaimBodyHasNoIdentityFields_E2E` |

外加两条本设计新增的：

| # | 验收 | 怎么测 | 证据 |
|---|---|---|---|
| ⑥ | 服务端只见 root：把「成员包」拿掉，服务端仍能验资格、但无法把 leaf 对应到任何人 | 结构性的：服务端只持有 root 与 verifying key（`crypto/eligibility_service.go`），没有名单、没有成员包 | `internal/infrastructure/crypto/eligibility_service.go` + `internal/infrastructure/crypto/eligibility/merkle.go`（`ListPackage` 只有 root / depth / count） |
| ⑦ | 跨语言一致性：浏览器端出的资格证明能被服务端的真验证密钥验过 | e2e：node 驱动**真的 WASM 模块**出资格证明，服务端用**真的资格验证密钥**验过，去重键与 Go 侧自算一致 | `tests/e2e/wasm_eligibility_test.go` 的 `TestAPI_WASMEligibilityProver_E2E` |
| ⑧ | 门户的资格领取在**真浏览器**里走通 | 人工：导入资格文件 → 领取 → 重复领取 → 本地生成选票 → 提交 → 计票 | 2026-10-25（Chromium / Electron）：claim **201** / 重复领取 **409** / 提交 **201** / 计票 `{"beta":1}` |

**单测与守卫**（另有，不在 T22 的五条之列）：
`internal/usecase/registration_eligibility_test.go`（challenge / 必需性 / 验签失败 / 落库去重 /
重复领取 / 未开启时拒绝带材料）、`internal/infrastructure/http/eligibility_endpoints_test.go`
（材料必须三件齐、没开时接受空体）、`internal/infrastructure/persistence/eligibility_use_repository_test.go`
（唯一索引）、`internal/infrastructure/crypto/eligibility/eligibility_test.go`（电路往返与约束数）、
`cmd/cli/eligibility_claim_test.go`（CLI 接线）、`web/src/composables/useVotePortal.test.ts`（门户接线）。

**跨语言线路的守卫**（新包 `tests/architecture/` 的 `eligibility_wire_test.go`）：
门户这一侧只做 JSON 解析与转发，**最容易出错的不是数学而是字符串** —— 字段名、静态资源名、
WASM API 名一旦两边错开，表现往往不是报错，而是「资格包无效」或浏览器里一个 404。
因此两边各钉一条断言：

| 守卫 | Go 侧 | TS 侧 |
|---|---|---|
| `TestEligibilityMemberPackageFieldsMatch` | `eligibility/merkle.go` 的 `MemberPackage` 的 json tag | `wasm-prover-protocol.ts` 的 `EligibilityMemberPackage` |
| `TestEligibilityProveRequestFieldsMatch` | `cmd/ballot-wasm/main.go` 的 `eligibilityProveRequest` | `wasm-prover-protocol.ts` 的 `EligibilityProveRequest` |
| `TestEligibilityWasmApiNamesMatch` | `cmd/ballot-wasm/main.go` 的 `api.Set` 的键 | 门户调用的 API 名 |
| `TestEligibilityProvingKeyAssetNameMatches` | 静态资源名 | 门户请求的静态资源名 |

---

## 5. 工作量拆解（5–7 天）✅ 已落地

> 下表是实现时的拆解。**实际落地时第 2 项（Go↔TS 双向 KAT）没有单独做** ——
> 浏览器路径改由 `tests/e2e/wasm_eligibility_test.go` 端到端覆盖（node 驱动真的 WASM 模块
> 出证明、服务端用真验证密钥验过、去重键与 Go 侧自算一致），比纯 KAT 更能证明「两边拼的
> 公开输入逐字段相同」。

| # | 任务 | 估时 | 产出 | 落地 |
|---|---|---|---|---|
| 1 | 资格电路（MiMC + Merkle + 范围检查）+ 编译 + **实测约束数** | 1.5 天 | `internal/infrastructure/crypto/eligibility/` | ✅ 实测 **6860** 条约束 |
| 2 | MiMC/Merkle 的 Go↔TS 双向 KAT | 0.5 天 | 两处断言 | ⚠️ 改为 e2e 端到端覆盖（见上） |
| 3 | 名单生成 CLI（`eligibility-list`） | 0.5 天 | root + 成员包 | ✅ `cmd/eligibility-list` |
| 4 | 服务端：challenge 端点 + claim 扩展 + 资格验证器 + 去重表 + 迁移 | 1.5 天 | 新端点 + 新表 | ✅ |
| 5 | 客户端：CLI 接线 | 0.5 天 | `credential-claim -eligibility` | ✅ |
| 6 | 客户端：浏览器（WASM 增 API + 静态资源 + 面板） | 1 天 | `cmd/ballot-wasm` + `web/` | ✅ **同一个 WASM 模块四个 API**（见 §8） |
| 7 | 测试（e2e ①–⑤ + 回归） | 0.5 天 | `tests/` | ✅ |
| 8 | 文档（techspec / project-map / roadmap / 部署清单 / CHANGELOG） | 0.5 天 | — | ✅ |

**主要风险的落地结果**：
- **约束数超预期**（MiMC 91 轮比 Poseidon 贵）→ ✅ **未发生**：实测 6860，与选票电路（7917）
  同量级，「退到 depth 8 + 多棵子树」的备选方案没有启用。
- **浏览器端静态资源膨胀**（多一套 wasm + key）→ ✅ **比预估更好**：只新增了一个静态资源
  （`eligibility-proving.key`），WASM 模块没有第二份 —— 详见 §8。
- **接口变更**（claim 请求体）→ ✅ **按「新增字段而不是改字段语义」做**：三件材料要么都给
  要么都不给；没开资格校验的部署，旧的空请求体照常工作。

---

## 6. 决策点（✅ 已拍板，2026-10-25）

> 这六条在设计稿成文时是「待拍板」，**实现前全部拍板并落地**。下表的「落地口径」列是
> 代码里的实际形态。

| # | 决策 | 拍板结论 | 落地口径 |
|---|---|---|---|
| **D1** | v0.2.0 做完整方案 B（5–7 天，新电路），还是先落方案 A（1–2 天，一设备一票）？ | **B**。A 不满足能力 G 的完成定义，先做 A 只会把「一人一票」写成一个更弱的承诺 | ✅ **已落地完整方案 B**：`internal/infrastructure/crypto/eligibility/`（资格电路 + Merkle + 名单工具），服务端强制一人一票。方案 A **没有实现** |
| **D2** | 名单的成员标识用什么（工号 / 选民编号 / 会员 ID）？名单由谁生成与保管？ | 需要一份**权威名单**；生成走离线工具，**服务端只见 root**，成员包与 root 分开保管 | ✅ `cmd/eligibility-list` 生成 `root.json`（给服务端）与 `eligibility-<member_id>.json`（0600，成员本人保管），两者分开保管。⚠️ **成员标识的具体形式仍由运营方决定** —— 工具接受任意非空行，代码不解释它的含义 |
| **D3** | 资格证明的哈希原语 | **MiMC**（gnark v0.7.1 无 Poseidon，且 MiMC 已有双向 KAT）。若要 Poseidon 需自己搭 permutation | ✅ **MiMC**，零新依赖 |
| **D4** | 资格证明在哪个端点提交 | **claim 之前**（新增 `eligibility-challenge` 端点 + claim 请求体加字段）。这样「不通过就不消耗槽位」成立，且 blind-sign / attest 保持不变 | ✅ 新增公开端点 `POST /api/v1/proposals/:id/credentials/eligibility-challenge`；claim 请求体加 `{eligibility_proof, qualification_id, challenge}`；blind-sign / attest **完全不变** |
| **D5** | 名单深度与规模 | depth=10 → 1024 人（约束 ~6600）；depth=16 → 65536 人（约束 ~9900）。先按 depth=10 做，配置化 | ✅ **depth=10，且是编译期常量**（`eligibility.TreeDepth`），**不是配置项** —— 电路里 `ToBinary` 的宽度必须是常量。单份名单上限 1024 人；实测约束数 **6860** |
| **D6** | 同一份名单能否复用于多个提案 | **能**。`Leaf` 是成员级的，`QualificationID` 按提案派生 → 跨提案不可关联，一次发放多次用 | ✅ **能复用**。`QualificationID = MiMC(Leaf, ProposalID)` 随提案变化，服务端按 `(proposal_id, qualification_id)` 去重 → 跨提案不可关联（验收 ④） |

**拍板之外新增的两条实现决定**：

| # | 决定 | 结论 |
|---|---|---|
| — | **release 模式必须开资格校验** | `crypto.eligibility_root` 与 `crypto.eligibility_vk_path` **必须成对出现**，且 **release 模式下缺一即启动失败**（`internal/config/config.go`）。理由：资格校验缺失等于「能发凭证但管不住一人一票」—— 那时它只是个发凭证的服务器，不是投票系统 |
| — | **服务端不设「跳过验证」的降级模式** | 与选票证明不同（`vk_path` 为空时开发模式只校验 proof 非空），资格验证密钥缺失**一律启动失败**，不悄悄放行（`crypto/eligibility_service.go`）。同理，**去重仓储缺失也不放行** —— 少了它等于一人一票没生效，宁可报错（`usecase/registration.go`） |

---

## 7. 降级路径（若 D1 选 A）—— **未启用**

若 v0.2.0 时间不够，先做方案 A：

- 客户端首次使用时生成本地长期密钥，持久化在本地；
- `claim` 请求体带 `{ fingerprint_pub_key, challenge_sig }`（对服务端 challenge 的签名）；
- 服务端去重键 = `SHA-256(pub_key)`，唯一索引 `(proposal_id, fingerprint)`；
- 强度是**一设备一票**，不是一人一票 —— README 与路线图必须如实写明；
- 接口形状与 B 一致（都是「claim 带资格材料」），将来换 B 时**接口不用再改**。

这条路能把「完全可重复投票」改善到「一设备一票」，但**不满足能力 G 的完成定义**，
No-Go #2 仍然判未过。

> **实际走的是方案 B**，因此这一节没有实现，**只是留作「当时考虑过什么」的记录**。

---

## 8. 实现与设计稿的偏离

> 这一节如实列出实现与本文原文（§3.2 / §3.4 / §3.6 / §5）不一致的地方。
> **分析、论证与方案对比（第 1、2 节）保留原样** —— 那是决策依据，不是待办清单。

| # | 设计稿原文 | 实际实现 | 理由 |
|---|---|---|---|
| 1 | **两个 WASM 模块**：`eligibility-prover.wasm` + `eligibility-proving.key`，「构建顺序追加在 `web-wasm` 之后」 | **一个 WASM 模块，四个 API**：`cmd/ballot-wasm` 在 `nullifierProver` 上注册 `init` / `prove` / `initEligibility` / `proveEligibility`。静态资源**只新增一个**：`web/public/eligibility-proving.key`（`make web-eligibility-key PK=<路径>`） | 两套电路编进同一个 `.wasm`，省掉第二份模块的下载与一次加载，门户的 Worker 也能复用同一个运行时。**代价**：模块体积略增（多了一套 6860 条约束的资格电路） |
| 2 | `qualification_id` 存 **BLOB(32)** —— 「域元素的规范编码」 | 存 **64 位小写 hex 的 TEXT**（`entity.EligibilityUse.QualificationID`，`size:64`） | 与 `votes.nullifier` 的口径一致（落库前归一成 64 位小写 hex 的字符串，唯一索引建在字符串上）。规范编码的域元素字节与 hex 是一一对应的，规范性由 `eligibility.DecodeField` 在入口处保证（非规范编码直接拒） |
| 3 | 公开输入 **7 个**（含 `Leaf` / `Path` / `Index`） | 公开输入 **5 个**：`Root` / `ProposalID` / `Challenge` / `QualificationID` / `SessionBinding`；**叶子、路径、位置全部私有** | 公开输入的个数因此是**定长**的，不随树深变化；服务端不需要知道树的结构，也不必知道叶子 —— 少暴露的正是「哪个成员」这条线。多出来的 `SessionBinding = MiMC(QualificationID, Challenge)` 由**服务端自算**，作用是把证明钉在本次领取会话的挑战上（截获的证明挪不到别的挑战上用） |
| 4 | 挑战**无状态**（服务端不记账），重放由去重键挡住 | ✅ **一致** —— 实现与设计稿一致：`IssueEligibilityChallenge` 只生成一个随机 nonce，不落库 | 无变化 |
| 5 | 决策点 **D1–D6** 是「待拍板」 | ✅ **全部拍板并落地**，口径见 §6。D1 = 完整方案 B；D2 = 离线工具生成、root 与成员包分开保管（成员标识形式仍由运营方决定）；D3 = MiMC；D4 = claim 之前提交（新增 challenge 端点）；D5 = depth=10 且是**编译期常量**（非配置项）；D6 = 名单可跨提案复用 | 与 §6 表逐条对应 |

### 8.1 仍然没有闭合的（写文档时必须说清）

- ~~**浏览器端没有自动化**：仓库里没有 Playwright。~~ —— ✅ **已闭合**（2026-10-26，T28）：
  门户的「导入资格文件 → 领凭证」这条路现在有 Playwright 自动化
  （`web/e2e/portal.spec.ts`，CI 有独立 job `web-e2e`），断言「导入资格文件 → 匿名领取
  **201** → **重复领取 409「这份资格已经领过票了 —— 一人一票」** → 浏览器本地生成选票 →
  提交 **201** → 计票还原出所选选项」。
  ⚠️ **没闭合的是「部署形态」**：真域名、反向代理、真实浏览器版本这些**每次上线仍要人工点
  一遍**（步骤见 [`docs/deployment-checklist.md`](./deployment-checklist.md) §8.1）——
  自动化验的是「接线没断」，不是「部署形态验过了」。
  > 那次 2026-10-25 的人工验收还挖出一个**只有真浏览器能发现的 bug**：资格包在门户里是 Vue 的
  > 响应式 Proxy，`postMessage` 克隆不了它 —— 「匿名领取」必然失败。已修并补了回归用例。
- **PostgreSQL 集成测试本机无 Docker**：与 T22 无关，但整体验收仍缺这一项
  —— ✅ 2026-10-25 已用本机便携版 PostgreSQL 17.7 起临时集群、按外部库路径实跑通过
  （含本设计稿相关的「一人一票」复合唯一索引用例）；✅ **testcontainers 路径也已在 CI 实跑**
  （2026-10-26，run `40324941645`，`ok anonvote/tests/integration 24.725s`）。
- **`cmd/loadtest` 原先不支持资格证明**（只在**未开启资格校验**的服务上可用，开了之后 claim
  会返回 `ELIGIBILITY_REQUIRED`）—— ✅ **T27 已补齐**：给 `-eligibility-dir` +
  `-eligibility-proving-key`，它会取挑战、出资格证明，走的是与门户 / CLI 同一条链路。
- **资格包丢失无法找回**：成员包是秘密材料（拿到它就能领这一票），发放必须走安全渠道。
- **真实名单的内容与成员标识形式仍需运营方提供**：仓库只提供工具与协议。
- **名单规模上限 1024 人**（`TreeDepth = 10` 是编译期常量），更大要改常量并重新生成密钥。
