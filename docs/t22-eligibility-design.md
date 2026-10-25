# T22 · 资格证明设计方案（一人一票）

> 状态：**设计稿，待拍板**（2026-10-25）。
> 目标：把能力 G 的完成定义补齐 —— 在**不破坏不可关联性**的前提下，让服务端能强制「一人一票」。
> 对应路线图任务卡：[`docs/roadmap.md`](./roadmap.md) 附录 A.3 的 T22。

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

离线工具（拟新增 CLI `eligibility-list`）输入权威成员名单（每行一个成员 ID）：

```
成员 ID ──(工具)──> 资格秘密 secret（32 字节随机）
                      叶子 leaf = MiMC(secret)                 ← hiding：知道 leaf 推不出 secret
                      Merkle 树（深度 depth，叶子按成员 ID 排序）
                      root = 树根
```

输出两份，**必须分开保管**：

| 产物 | 去向 | 内容 |
|---|---|---|
| `root.json` | **服务端配置** | 只有 root（一个域元素，十六进制） |
| `eligibility-<member>.json` | 成员本人（安全渠道） | `secret` + `leaf` + Merkle path（兄弟节点）+ 成员 ID |

服务端**只拿到 root**：它知道「有一个 root」，知道每个来领票的人提交的 `leaf` 在树里，
但**无法把 leaf 对应到任何成员**（它没有成员包，也没有名单）。

> ⚠️ 运营纪律：名单工具的运行机器与成员包的保管方式，必须与投票服务分开。工具持有「成员 ↔ secret」映射，
> 谁拿到成员包谁就能投票 —— 这是资格发放的固有性质，不是缺陷。

### 3.2 电路（新增 `eligibilityCircuit`）

复用现有栈：Groth16 / BN254，域 `fr`，哈希用 **MiMC**（`gnark v0.7.1` 的 `std/hash` 只有 MiMC，
而本仓库的 MiMC 已经有 Go↔TS 双向 KAT，见 `ballot/nullifier_interop_test.go`）。

**公开输入**（服务端与客户端都能拿到）：

| 名字 | 含义 |
|---|---|
| `Root` | 名单根（域元素） |
| `Leaf` | 成员叶子（域元素，`MiMC(secret)`） |
| `Path` | Merkle path（兄弟节点数组，长度 = depth） |
| `Index` | 叶子位置（比特分解，用于选左右孩子） |
| `ProposalID` | 提案域元素（`ballot.DeriveProposalField`） |
| `Challenge` | 服务端本次领取会话的一次性挑战 |
| `QualificationID` | 去重键 = `MiMC(Leaf, ProposalID)` |

**私有见证**：`Secret`（资格秘密，域元素）。

**约束**：

1. `MiMC(Secret) == Leaf`
2. `QualificationID == MiMC(Leaf, ProposalID)`
3. Merkle 自底向上：`node = MiMC(left, right)`，左右由 `Index` 的比特选；末端 `node == Root`
4. `Index < 2^depth`（范围检查，复用 `ballot` 里的 `assertLessThan` 写法）

**约束数估算**：MiMC 91 轮 ≈ 550 约束/次；depth=10 时约 10×550 + 2×550 ≈ **6600 约束**，
与现有选票电路（7917）同量级 —— 浏览器端出证明仍在秒级。

**为什么 `QualificationID` 要按提案派生**：`Leaf` 是成员级的（一份名单可复用于多个提案），
但 `QualificationID` 含 `ProposalID` → **跨提案不可关联**（满足 T22 验收 ④），
且服务端按 `(proposal_id, qualification_id)` 去重。

### 3.3 协议（四个请求，全程服务端拿不到真人身份）

```
① POST /api/v1/proposals/:id/credentials/eligibility-challenge   （公开，无状态）
   → { proposal_id, root, depth, challenge }
      challenge 是服务端随机 nonce；服务端**不记账**（重放由去重键挡住，见 §3.5）

② POST /api/v1/proposals/:id/credentials/claim   （公开）
   请求体新增：{ eligibility_proof, qualification_id, challenge }
   服务端：验资格证明（资格电路 verifying key）→ 校验 challenge 与 root 一致
        → 校验 (proposal_id, qualification_id) 未被用过   ← 去重在这里发生
        → 预留池槽位，返回 { user_id, issuer_key, variant }（与现状一致）

③ POST /api/v1/proposals/:id/credentials/blind-sign  （公开，不变）
④ POST /api/v1/proposals/:id/credentials/attest      （公开，不变）
```

**关键点：资格校验在 claim 这一步**（路线图验收 ①「不通过就不给凭证」的要求）。
claim 失败**不消耗池槽位** —— 与「先发凭证、投票时再拒」相比不会浪费槽位。

**为什么不把资格材料塞进 blind-sign**：blind-sign 是公开端点，签发方在那里做盲签名计算。
资格不通过却先消耗签发方算力 = 一个现成的 CPU 消耗面。放在 claim 之前更干净。

**对现有协议的影响面**：
- `claim` 的请求体从「空」变成「带资格证明」—— 这**不破坏 T17 的盲签名性质**：
  资格材料里没有任何与凭证消息相关的东西（服务端仍然看不到凭证公钥，盲化依然有效）。
- `blind-sign` / `attest` **完全不变** → 现有客户端与 e2e 用例不受影响。

### 3.4 服务端存储

新增表（或在 `credential_claims` 上加列）：

```
eligibility_uses
  proposal_id      TEXT
  qualification_id BLOB(32)      -- 域元素的规范编码
  used_at          TIMESTAMP
  UNIQUE(proposal_id, qualification_id)
```

- 「落库只存哈希」：`qualification_id` 本身就是 `MiMC(Leaf, ProposalID)` —— hiding 的域元素，
  存它不等于存身份。**不存** leaf、**不存** secret、**不存** 成员 ID。
- 唯一索引就是「一人一票」的强制点：第二次用同一资格 → 唯一约束冲突 → 409。

### 3.5 重放与 DoS

| 攻击 | 结果 |
|---|---|
| 重放同一份资格证明 | 第二次 `claim` 撞唯一索引 → 409（`challenge` 无需服务端记账，正是为此） |
| 拿 A 的资格证明抢先领 | 能领走，但 A 就领不到了（DoS 存在，**不丢票**）。要根治需 challenge 绑定到具体槽位后再验 —— 代价是多一步交互，v0.2.0 不做 |
| 伪造资格证明 | 电路约束挡住（无法造出 `MiMC(secret) == leaf` 且 leaf 在 root 树里的证明） |
| 刷 claim 预留空槽 | **现状已存在**（claim 不收任何东西就能预留），不因 T22 恶化；但 T22 之后 claim 会验资格，攻击面反而变小 |

### 3.6 客户端

**CLI**（新增命令 `eligibility-prove` 或并入 `credential-claim`）：

```
go run ./cmd/cli eligibility-claim -proposal p-1 -eligibility eligibility-me.json
```

读成员包 → 出资格证明 → 带证明 claim。

**浏览器**：WASM prover 需要新增「资格证明」能力。
- `cmd/ballot-wasm` 增一个 API：`proveEligibility({root, leaf, path, index, proposal_id, challenge})`；
- 资格电路的 **proving key 是另一把**（Groth16 的 setup 是 per-circuit）→ 静态目录要**多两个文件**：
  `eligibility-prover.wasm` + `eligibility-proving.key`（构建顺序追加在 `web-wasm` 之后）；
- 门户要新增「导入资格文件 → 领凭证」的一步（成员包是 JSON，导入即可）。

### 3.7 不变量清单（实现时逐条对照）

| 能力 | 是否受影响 | 怎么保证 |
|---|---|---|
| A 服务端不得拿到明文选项 | 不受影响 | 资格证明里没有选项；`tests/architecture/` 的守卫继续有效 |
| B 选票可验证 | 不受影响 | 选票电路不动；资格电路是独立的第二套密钥 |
| C proof 与票绑定 | 不受影响 | 选票 proof 的公开输入构成不变 |
| G 分发不泄露身份 | **必须保持** | ① 服务端只见 root，不见成员包；② 资格材料里没有真人身份字段（grep 可验）；③ 投票提交的材料**不含** `qualification_id` |
| 防重投 | 不受影响 | `votes.nullifier` 唯一索引照旧 |

> ⚠️ 特别提醒：投票提交体里**不能**带 `qualification_id`。
> 一旦带上，服务端就能把「这张票」与「领取时用的资格」连起来 —— 虽然资格本身匿名，
> 但这会让「资格 ↔ 票」的关联成立，削弱能力 G。**去重只需要在 claim 时做一次。**

---

## 4. 验收口径（对齐路线图 T22 的五条）

| # | 验收 | 怎么测 |
|---|---|---|
| ① | 无资格证明的 claim → 拒绝（4xx），**池槽位不被消耗** | e2e：claim 前后池里空槽数不变 |
| ② | 有效资格证明 → 领取成功 | e2e：资格文件 → 证明 → claim → 拿到凭证 |
| ③ | 同一份资格证明第二次领取 → 拒绝 | e2e：第二次 claim → 409（唯一索引） |
| ④ | 资格证明在不同提案间不可复用 | e2e：提案 A 的证明拿去领提案 B → 验签失败（`ProposalID` 在公开输入里） |
| ⑤ | 领取请求体里不含任何真人身份字段 | **grep 可验** → 建议并入 `tests/architecture/`（字段名黑名单：`member_id` / `employee_id` / `name` / `id_card` / `phone` / `email`） |

外加两条本设计新增的：

| # | 验收 | 怎么测 |
|---|---|---|
| ⑥ | 服务端只见 root：把「成员包」拿掉，服务端仍能验资格、但无法把 leaf 对应到任何人 | 单元测试：服务端只有 root 时能验；反向断言「服务端不持有成员包」是结构性的（配置里只有 root） |
| ⑦ | 跨语言一致性：资格证明的公开输入拼装 Go↔TS 逐字节一致 | KAT：Go 侧 `eligibility_interop_test.go` + TS 侧断言同一组公开输入 |

---

## 5. 工作量拆解（5–7 天）

| # | 任务 | 估时 | 产出 |
|---|---|---|---|
| 1 | 资格电路（MiMC + Merkle + 范围检查）+ 编译 + **实测约束数** | 1.5 天 | `internal/infrastructure/crypto/eligibility/` |
| 2 | MiMC/Merkle 的 Go↔TS 双向 KAT | 0.5 天 | 两处断言 |
| 3 | 名单生成 CLI（`eligibility-list`） | 0.5 天 | root + 成员包 |
| 4 | 服务端：challenge 端点 + claim 扩展 + 资格验证器 + 去重表 + 迁移 | 1.5 天 | 新端点 + 新表 |
| 5 | 客户端：CLI 接线 | 0.5 天 | `credential-claim -eligibility` |
| 6 | 客户端：浏览器（WASM 增 API + 静态资源 + 面板） | 1 天 | `cmd/ballot-wasm` + `web/` |
| 7 | 测试（架构守卫 + e2e ①–④ + 回归） | 0.5 天 | `tests/` |
| 8 | 文档（techspec / project-map / roadmap / 部署清单 / CHANGELOG） | 0.5 天 | — |

**主要风险**：
- **约束数超预期**（MiMC 91 轮比 Poseidon 贵）→ 缓解：depth=10 时约 6600 约束，实测后再定；
  若超预算，退到「depth 8 + 多棵子树」的方案，或改用 `gnark-crypto` 的 `permutation` 自己搭 Poseidon。
- **浏览器端静态资源膨胀**（多一套 wasm + key）→ 缓解：资格证明只在领凭证时用一次，
  可以做成**按需下载**（不进门户首屏）。
- **接口变更**（claim 请求体）→ v0.3.0 承诺「接口不再动」，所以这次要一次改对：
  新增字段而不是改字段语义。

---

## 6. 需要拍板的决策点

| # | 决策 | 建议 |
|---|---|---|
| **D1** | v0.2.0 做完整方案 B（5–7 天，新电路），还是先落方案 A（1–2 天，一设备一票）？ | **B**。A 不满足能力 G 的完成定义，先做 A 只会把「一人一票」写成一个更弱的承诺 |
| **D2** | 名单的成员标识用什么（工号 / 选民编号 / 会员 ID）？名单由谁生成与保管？ | 需要一份**权威名单**；生成走离线工具，**服务端只见 root**，成员包与 root 分开保管 |
| **D3** | 资格证明的哈希原语 | **MiMC**（gnark v0.7.1 无 Poseidon，且 MiMC 已有双向 KAT）。若要 Poseidon 需自己搭 permutation |
| **D4** | 资格证明在哪个端点提交 | **claim 之前**（新增 `eligibility-challenge` 端点 + claim 请求体加字段）。这样「不通过就不消耗槽位」成立，且 blind-sign / attest 保持不变 |
| **D5** | 名单深度与规模 | depth=10 → 1024 人（约束 ~6600）；depth=16 → 65536 人（约束 ~9900）。先按 depth=10 做，配置化 |
| **D6** | 同一份名单能否复用于多个提案 | **能**。`Leaf` 是成员级的，`QualificationID` 按提案派生 → 跨提案不可关联，一次发放多次用 |

---

## 7. 降级路径（若 D1 选 A）

若 v0.2.0 时间不够，先做方案 A：

- 客户端首次使用时生成本地长期密钥，持久化在本地；
- `claim` 请求体带 `{ fingerprint_pub_key, challenge_sig }`（对服务端 challenge 的签名）；
- 服务端去重键 = `SHA-256(pub_key)`，唯一索引 `(proposal_id, fingerprint)`；
- 强度是**一设备一票**，不是一人一票 —— README 与路线图必须如实写明；
- 接口形状与 B 一致（都是「claim 带资格材料」），将来换 B 时**接口不用再改**。

这条路能把「完全可重复投票」改善到「一设备一票」，但**不满足能力 G 的完成定义**，
No-Go #2 仍然判未过。
