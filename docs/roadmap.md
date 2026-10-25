# nullifier 产品路线图

> **这是产品路线图，不是发版计划。**
> 它回答三个问题：v0.2.0 要达成什么产品状态、怎么算达成、现在走到哪。
> v0.2.0 之后的版本线是**推演**（[§1.2](#12-v020-之后还有几个-0x0)），不构成承诺。
> 逐条工程任务卡在[附录 A](#附录-a--工程细项)；
> 问题的完整证据与修复顺序见 [`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md)。

---

## 1. 版本格局

**已承诺** —— 有完成定义、有出口标准、可以照着验收：

| 版本 | 定位 | 状态 | 规模 |
|---|---|---|---|
| **v0.1.0** | 已发布的内测版。接口、存储、加密、门限解密、审计的骨架都在，但**零知识证明链尚未实现**，不要用于真实投票 | ✅ 已发布（2026-10-20） | — |
| **v0.2.0** | **路线图目标** —— 有真实零知识证明链的实验版。匿名性在密码学上**成立**，而不是被声称；但接口仍会变，**不承诺稳定** | M2/M3 未开工 | **27–39 人天 ≈ 6–8 周** |

**预期** —— 推演，**未承诺**，随 M2 进展重估（详见 [§1.2](#12-v020-之后还有几个-0x0)）：

| 版本 | 承诺命题 | 规模（推演） |
|---|---|---|
| **v0.3.0** | 「接口不再动」 | 15–20 人天（待 M2 后按任务卡重估） |
| **v0.4.0** | 「能用于有规模的真实投票」 | 20–30 人天（待 M2 后按任务卡重估） |
| **1.0.0** | 「生产可用 + API 稳定」 | 视范围而定 |

**v0.1.0 与 v0.2.0 的差距不在业务代码，在一条完整的零知识证明链**：
电路 → 密钥 → 客户端 prover → proof 绑定 → 客户端加密 → 真盲签名。
这条链是周级工作量，**不能和小时级的加固项捆在一起做** —— 捆在一起的结果是 v0.2.0 永远发不出去。

> **规模口径（可核对）**：v0.2.0 的 27–39 人天 = M2 的 19–28
> （T13 3–5 + T14 5–7 + T15 2–3 + T16 3–4 + T17 5–7 + T18 1–2）
> + M3 的 8–11（T19 0.5 + T20 2–3 + T21 0.5 + T22 5–7），按**单人**每周 5 人天、向上取整折算 ≈ 6–8 周。
> （T1 / T2 各 1 小时，也属 v0.2.0 的前置，未计入这 27–39。）
> **v0.3.0 往后的数字是推演**，不是从任务卡推导出来的 —— 等 M2 跑完一轮再按实际进展重估。

### 1.1 为什么目标版本是 0.2.0 而不是 1.0.0

1. **`1.0.0` 在语义化版本里意味着「API 稳定、生产可用」。** v0.2.0 发出去时证明链虽然通了，
   但凭证领取、投票提交、密文格式这几处接口大概率还要再动一轮。标成 1.0.0 是过度承诺。
2. **`0.x → 0.x` 的版本升级里改协议、改接口，阻力远小于 1.0.0 之后的破坏性变更。**
   这条路已经走过一次了：T7 把密钥派生换成 HKDF、密文格式升 v2，v0.1.0 之前的旧密文就再也解不开了（v0.1.0 自身即 v2 格式）。
3. **成熟度要说实话。** v0.1.0 连真实电路都没有，门户在生产模式下还投不了票。
4. **代价**：外界可能低估成熟度。对冲办法是把边界如实写进 README 的「当前版本边界」，
   而不是靠版本号暗示。

> **注意**：版本号表达的是**承诺强度**，不是工作量。
> 改成 0.2.0 **不降低**任何一条安全或匿名性的底线 —— §5 的 go/no-go 清单一条都不放松。
> 放松的是「接口不会变」这个承诺：v0.2.0 之后的破坏性变更走 minor 版本即可。

### 1.2 v0.2.0 之后还有几个 0.x.0

**答案分两层**：

- **文档后续承诺的只有 v0.2.0 这一个 0.x.0。** 后面几档是推演，不构成排期。
- **按现有能力缺口与 §2.2 明确不做的四类内容推演，v0.2.0 之后还有 2–3 档 0.x.0**（v0.3.0 / v0.4.0，必要时再插一档 v0.5.0）—— 连 v0.2.0 自身算上是 **3–4 档**。最后才是 1.0.0。

推演依据 —— 每档对应一个明确的承诺命题：

| 版本 | 承诺命题 | 主要内容 | 依据 |
|---|---|---|---|
| **v0.3.0** | 「接口不再动」 | 凭证领取 / 投票提交 / 密文格式三处接口收敛并冻结；密文格式定版；限流、传输层优化、**真正的**压测容量规划；审计**离线复算**工具（审计锚点 v0.1.0 已有，缺的是脱离服务端的复算脚本）、监控指标 | §1.1 第 1 条说 v0.2.0 之后这三处接口「大概率还要再动一轮」；§2.2 第 1 项把容量规划与监控排在 v0.2.0 之外；能力 I 只要求「有可复现脚本 + 输出」，那不是容量数字；能力 H 只覆盖配置与清单，监控指标属 §2.2 第 1 项的运维范畴 |
| **v0.4.0** | 「能用于有规模的真实投票」 | 多租户、权限分级、投票人身份体系；资格证明的**真实身份源**对接 | §2.2 第 2 项；T22 给的是「证明资格」的技术机制（一次性设备密钥 / 资格名单的 Merkle 承诺），把真实身份源（选区、组织名录）接进来是另一件事 |
| **1.0.0** | 「生产可用 + API 稳定」 | 内容取决于届时还剩什么；§2.2 第 3、4 项（移动端原生客户端、密码学方案替换）是**可选收尾**，不构成本档的必要条件 | 本档的门槛是**判据**而不是功能清单 —— 见 §1.3 |

**0.x.0 的个数不是固定的 —— 它取决于接口什么时候能冻结，而不是凑版本号。**

- 若 v0.3.0 之后接口真的稳住了，**v0.3.0 或 v0.4.0 之后就可以进入 1.0.0 的判定** —— 但仍要逐条满足 §1.3 的四条判据（判据 2、3 天然要求至少再有一个 0.x 档位）。「不必走满三档」说的是不必机械凑满，**不是可以跳判据**。
- 若 v0.2.0 之后接口反复动，还要再插 0.x.1 / 0.x.2。
- 换句话说：上表「预期」那几行是**方向**，不是**排期**。排期要等 M2 跑完一轮再定。

### 1.3 什么时候可以叫 1.0.0

晋级 1.0.0 的判据 —— **四条同时满足**：

> **这四条是必要条件，不是版本数量的上限。** 「档数不固定」与「判据不可跳过」是两件事：
> 档数由接口何时能冻结决定，能不能叫 1.0.0 由这四条决定。

1. **走过的每个 0.x 版本都经历过一次完整的接口变更窗口**（至少要有两个，否则无从判断稳定性）—— 证明破坏性变更是可控的，而不是从没发生过。
2. **密文格式定版后连续 ≥ 2 个 minor 版本不变** —— T7 已经用「v0.1.0 之前的 v1 密文永久不可解」演示过一次代价（v0.1.0 自身即 v2 格式）。
3. **有一次真实规模投票的完整复盘** —— 不是 demo。
4. **生产部署清单连续两次零改动** —— §5 的 Go 条件里那条「生产部署清单全勾」，要从「全勾一次」升级到「不用改就能再上一次」。

> 这四条比「功能做完了」更难达成，也更难假装达成 —— 这正是 1.0.0 该慢的理由。

---

## 2. v0.2.0 的产品定义

> **服务端在任何情况下都无法得知投票人选了哪个选项，且选票可验证、可审计、可复算。**

这句话是**可证伪**的：拿一个真实投票的服务端 + 完整数据库，
第三方应当无法回答「某人选了哪个选项」，同时应当能回答
「这张票是真的吗」「计票结果对吗」「谁投过票（仅是与否）」。

### 2.1 能力清单与完成定义

每项能力都有**可验证**的完成定义 —— 不是「做了」就算完，是「能拿出证据」才算完。

| # | 能力 | v0.1.0 现状 | v0.2.0 完成定义（证据） | 里程碑 |
|---|---|---|---|---|
| A | 选票内容对服务端不可知 | ❌ 密文由服务端生成，服务端在签发凭证时就知道选了哪个 | 服务端代码里**不存在**接收明文选项的字段（grep 可验）；落库记录无法还原选项 | M2 · T16 |
| B | 选票可验证 | ❌ 无真实电路；`vk_path` 为空时任何非空 proof 都通过 | 真实 Groth16 电路 + 密钥生成流程；篡改任一公开输入 → 验证失败 | M2 · T13 |
| C | proof 与票绑定 | ❌ proof 可跨提案复用（服务端只读 `public_witness`） | 公开输入含 `(proposal_id, nullifier, 计票公钥, 密文点, 选项数)`；A 提案的 proof 投 B 提案 → 400 | M2 · T15 |
| D | 客户端能投票 | ❌ 门户在生产模式下 100% 无法完成投票（没有 prover） | 浏览器内生成 proof 与密文，e2e 跑通「选票 → 加密 → 证明 → 提交」 | M2 · T14 |
| E | 凭证不可伪造 | ❌ 投票路径**从不**验凭证签名 | 落库前 `VerifyCredential`；伪造签名的投票 → 401 | M2 · T18 |
| F | 签发方看不到被签内容 | ❌ 盲化在服务端进程内完成，等同普通签名 | 两段式盲签名：服务端在**签名阶段**只见盲化消息；交叉攻击（A 的盲签名 finalize 出 B）→ 失败 | M2 · T17 |
| G | 凭证分发不泄露身份 | ✅ **v0.1.0 已达成**（T9）：凭证池 + 投票人匿名随机领取，运营方不再掌握「谁领了哪张」 | 保持「池里只存未签发槽位、领取时用领取者自带的一次性公钥当场签发、领取端点不落可关联日志」；补**资格证明**把一人一票补回来 | M3 · T22 |
| H | 生产可运维 | 🟡 **`CONFIG_FILE` 显式路径已支持**（生产配置模板仍不会被自动加载，但有了确定的加载方式，缺文件直接启动失败，启动日志打印实际配置文件路径）；CORS 真实源与部署清单仍靠人工核对 | 显式 `CONFIG_FILE` 支持 + 生产部署清单逐项可勾 | M3 · T19 |
| I | 端到端验收 | ❌ 三个盲区从未验证（见 §4.3） | PG 集成测试 / 浏览器真实渲染 / 并发投票与容量，各有**可复现脚本 + 输出** | M3 · T20 |

> **能力 C 的完成定义在 2026-10-21 被修正过。** 原文写的是「公开输入含
> `(proposal_id, option_index, nullifier, hash(ciphertext))`」—— 这条与**能力 A 直接冲突**：
> 选项下标一旦进公开输入，服务端拼公开输入时就等于拿到了明文选项。
> 两项完成定义打架时以能力 A 为准（它是整个 v0.2.0 的核心承诺），
> 因此选项下标改为**电路私有见证 + 范围检查**（`m < NumOptions`），
> 公开输入只保留服务端自己能拿到的材料。理由与代价见 §6 决策 **D1**。

### 2.2 v0.2.0 明确不做

避免范围蔓延，以下内容**不属于** v0.2.0：

- 端到端加密之外的传输层优化、限流、压测容量规划 —— 属运维，不属本路线图
- 多租户、权限分级、投票人身份体系 —— 当前是「一个 admin 管池」的模型
- 移动端原生客户端 —— 只做浏览器 + CLI
- 密码学方案的替换（换 KEM、换曲线）—— 只做 KDF 修正，不动主体方案

> **一条例外（2026-10-21）**：上一条的「不动主体方案」指**凭证签名方案**（仍是 RFC9474 盲签名）。
> 但**选票加密**必须换栈到电路同一条曲线（BN254 twisted Edwards）——
> 电路算术在 BN254 标量域里，ed25519 + HKDF 的群运算无法在电路中复算，
> 不换栈就无法证明「这份密文确实是本提案计票公钥加密的某个合法选项」。
> 这不是「顺手换个更好的方案」，是**证明链成立的前提条件**。详见 §6 决策 **D2**。

> 这四项的去向见 [§1.2](#12-v020-之后还有几个-0x0) 的 **v0.3.0 / v0.4.0 / 1.0.0 三档**推演（移动端原生客户端与密码学方案替换落在 1.0.0 那一档，且都是可选项）—— 排在「不做」里不等于永远不做。

### 2.3 能力 G 引入的新问题：一人一票没了

T9 把凭证改成**匿名随机领取**后（能力 G），服务端**无法强制一人一票**。

缺口的确切来源：池里的槽位带的是**随机假名 `user_id`**，领到它的人是谁服务端不知道；
而服务端唯一能用来做「一人一票」约束的字段正是 `user_id`
（`vote_participations` 存 `hash(user_id)`）。既然 `user_id` 是随机假名，
服务端就看不出同一个人领了几次、投了几次 —— 任何人都能反复领取并投票。

这不是可以「顺手修掉」的小事：它是「不可关联性」与「一人一票」之间的直接取舍。
**要同时拿到两者，只能靠资格证明** —— 让投票人证明「我有资格投这一票」，
但证明本身不暴露身份（零知识证明「我符合资格」而不透露是谁）。这条路依赖
M2 的证明链能力，因此它属于 M3 而不是 M2。

> 任务：**T22 · 资格证明**（见附录 A.3）。
> 另一个配套小项：CLI 缺 `credential-claim` 命令（门户已有按钮、HTTP 接口已有），
> 脚本化领取用不了 —— 一并归到 T22。

---

## 3. 里程碑

### M1 · 加固层 ✅ 已完成

**目标**：与证明链无关的工程加固 —— 让 v0.1.0 敢发内测。

**出口标准**：逐包覆盖率门槛生效、CI 全绿、已知缺陷要么修掉要么在文档里如实写明。

**状态**：**已完成**，随 v0.1.0 一起落地（见 §4.1）。

### M2 · 证明链 🟡 进行中

**目标**：能力 A–F —— 让「匿名性成立」从口号变成可验证的事实。

**出口标准**：能力 A–F 的完成定义逐条拿出证据；§5 的 No-Go 清单全过。

**状态**：**进行中**（2026-10-21 起）。已完成 T13 / T15 / T18 / T17 / T1 / T2，
剩余 T14 / T16（都卡在同一个前置件：浏览器端 prover）。逐项状态见 §4.1 的 M2 进展表。

**依赖与顺序**（后一项依赖前一项，不要并行）：

```
T13 电路 + 密钥  →  T15 proof 绑定 ──→ T14 客户端 prover
                          ↓                    ↓
              T16 加密移到客户端 ←───────────┘
                          ↓
              T17 真盲签名（两段式）←─ T14
                          ↓
              T18 投票路径 VerifyCredential
```

> **顺序调整（2026-10-21）**：原顺序是 `T13 → T14 → T15`。实际执行时发现
> **T15 不依赖 T14** —— 服务端自己拼公开输入、自己验签，这部分与客户端有没有
> prover 无关。先落 T15 的好处是：服务端的验证能力先到位，客户端 prover
> （T14）接上来时对接的是**已经定死**的公开输入格式，不用返工。
> T18 原本排在 T16 之后，实际可以提前：凭证验签与选票加密是否在客户端无关。
> **T17 原排在 T14 之后**，实际也提前了：两段式盲签名的客户端侧（纯 TS）不依赖
> prover，只依赖签发方公钥 —— 等 T14 会把 T17 一起堵死。T17 已于 2026-10-21 落地。

**预估**：周级。这是 v0.2.0 的主体，也是唯一的硬骨头。

### M3 · 生产化 ⬜ 未开工

**目标**：能力 G 的资格证明（T22）、H 的生产配置（T19）、I 的端到端验收（T20），以及发版本身。

**出口标准**：能力 G 的完成定义拿出证据（资格证明跑通、重复领取被拒）；生产部署清单全勾；
三项盲区都有可复现输出；`VERSION` 与 tag 一致。

**依赖**：M2 全部完成（资格证明要用 M2 的证明链能力）。

---

## 4. 当前进展

### 4.1 已落地（M1 全部完成）

随 v0.1.0 一起落地的加固项：

| 任务 | 内容 | 提交 |
|---|---|---|
| T3 | admin token 改常数时间比较（SHA-256 摘要 + `ConstantTimeCompare`） | `4ed262d` |
| T4 | admin 鉴权补「错误 token」与「畸形 `Authorization` 头」测试；拒绝含 CR/LF/NUL 的头 | `4ed262d` |
| T5 | `release` 模式关闭启动自动迁移；新增 `nullifier-server -migrate` 独立迁移步骤 | `0f0c0e9` |
| T6 | PostgreSQL 连接池参数显式配置（默认 25 / 5 / 3600 秒） | `1dcb4b9` |
| T7 | 密钥派生改用 HKDF-SHA256，salt 每次随机；**密文格式升 v2** | `a036a9b` |
| T8 | 引导浮层补焦点陷阱与 Esc 关闭 | `1eda25f` |
| T9 | 凭证改为「凭证池 + 投票人匿名随机领取」（见附录 A.4 的落地核对） | `17937da` |
| T10 | 覆盖率门槛扩到 9 个包并改为逐包分档；`cmd/server` 冒烟测试 | `f7ec274` |
| T11 | 门户补「查询投票状态」按钮 | `2356638` |
| T12 | 文档如实描述盲签名的真实性质 | `a94b51e` |

**T7 是协议变更**：v1 格式密文会被直接拒绝（v0.1.0 自身即 v2 格式），旧的加密选票无法解密。
升级前不要用旧数据做真实投票。

**T9 是能力取舍，不是纯增益**：它让「运营方掌握谁领了哪张」这条线索消失，
代价是**服务端不再能强制一人一票**。这个缺口没有在 v0.1.0 里绕过去，
而是作为 **T22** 留在路线图上（见 §2.3）。

#### M2 进行中（2026-10-21 起，本轮）

| 任务 | 内容 | 状态 |
|---|---|---|
| T13 | 真实选票电路 + `cmd/keygen` 密钥生成流程 | ✅ 已落地 |
| T15 | proof 与 `proposal_id` / `nullifier` / 密文绑定 | ✅ 已落地 |
| T18 | 投票路径落库前 `VerifyCredential` | ✅ 已落地 |
| T1 | `ballot-draft` 在非 debug 模式 404 | ✅ 已落地 |
| T2 | 门户「按选项生成凭据」按钮非 debug 隐藏 | ✅ 已落地 |
| T16 | 加密移到客户端 | 🟡 **客户端加密已落地**（浏览器端 TS 实现 + CLI 命令 + 跨语言互操作测试）；门户接线与客户端 prover 待 T14 |
| T14 | 客户端 prover（浏览器 WASM） | ⬜ 未开工（**离线不可行**：模块缓存里没有 `gnark-wasm`，也没有 JS 密码学库）。**前置件已就位**：浏览器端 nullifier 派生（MiMC + Keccak + SHA-256，纯 TS）与 Go 侧逐位一致并有跨语言 KAT 锁定 |
| T17 | 盲签名改真两段式 | ✅ **已落地**（2026-10-21）—— 真两段式协议（预留 → 本地盲化 → 盲签名 → 本地解盲 → attest）+ 门户 / CLI 接线 + 管理端同样走两段式；验收 ①（签名阶段只见盲化消息）与验收 ②（交叉攻击失败）均有 e2e 用例 |

**能力 A–F 的当前状态**：

| 能力 | 状态 | 说明 |
|---|---|---|
| A 选票内容对服务端不可知 | 🟡 客户端侧已通 | **选票密文已在客户端生成**：浏览器端 TS 实现（`web/src/lib/ballot-cipher.ts`）+ CLI `ballot-encrypt` 命令；服务端只提供公开的计票公钥，接口 `GET /api/v1/proposals/:id/election-key`。落库记录里只有密文（e2e 用例断言）。**未闭合**：门户还没接上（缺 prover），`ballot-draft` 这条服务端代加密的路径在 debug 模式仍存在 |
| B 选票可验证 | 🟡 服务端侧达成 | 真实 Groth16 电路 + `cmd/keygen` + 服务端真实验签已落地；**浏览器端 prover 未做**（T14），release 模式下门户仍无法完成投票 |
| C proof 与票绑定 | ✅ 已达成 | 公开输入由服务端按请求字段自拼，客户端无法自选；跨提案 / 跨 nullifier 复用均验签失败 |
| D 客户端能投票 | ❌ 未达成 | 缺浏览器端 prover（T14）；CLI 已有本地加密命令，但同样缺 prover |
| E 凭证不可伪造 | ✅ 已达成 | 落库前 `VerifyCredential`，伪造签名 → 401 |
| F 签发方看不到被签内容 | ✅ 已达成 | 真两段式盲签名：客户端本地盲化，服务端在**签名阶段**只见盲化消息；交叉攻击（A 的盲签名签 B）→ 验签失败。**口径**：attest 阶段服务端仍看得到 `user_id` / `user_pub_key`（验签必须重算凭证消息） |

**本轮的两个协议变更**（都要写进 CHANGELOG，详见 §6 决策 D2）：

- **选票密文 v2 → v3，与 v0.1.0 的密文不兼容** —— v0.1.0 的 ed25519 + HKDF
  群运算无法在 BN254 电路里复算，电路算术在标量域里，密文必须换到同一条曲线。
  旧的加密选票无法解密。
- **投票请求体不再接受 `public_witness`** —— 公开输入改由服务端自行拼装。

### 4.2 v0.1.0 的边界（v0.1.0 发布时的快照）

> 以下是 **v0.1.0 发布时**的边界，属历史快照。**当前实现见 §4.1 的 M2 进展表**
> —— 其中「没有真实电路」与「`ballot-draft` 生产可达」两条已在本轮改变。

**能**：

- 完整的投票生命周期接口：提案、领凭证、投票、查状态、门限解密、计票、审计锚点
- 选票内容与投票人身份在存储层分离；`vote_participations` 只存 `hash(user_id)`
- 门限解密份额只返回一次、不落库
- CLI 与门户两条完整的客户端工作流

**不能**（v0.1.0 的真实限制，不要误用）：

| 限制 | 后果 |
|---|---|
| **没有真实的选票零知识电路** | `crypto.vk_path` 为空时任何非空 proof 都通过（开发模式）；加载 vk 后 `public_witness` 变必填，而现有客户端拿不到它 |
| **密文由服务端生成** | 服务端在生成选票时就知道选了哪个 —— **当前版本最大的匿名性缺口** |
| **盲化在服务端内部完成** | 不具备真正的盲签名性质，签发方看得见被签内容 |
| **门户在生产模式下无法完成投票** | 没有 prover；`ballot-draft` 返回的是便于串通流程的草稿数据 |
| **服务端无法强制一人一票** | 凭证匿名随机领取的代价（见 §2.3） |
| **`ballot-draft` 是开发联调接口** | 生产环境也会暴露（返回的假 proof 会在验证阶段被拒，但接口本身不该在生产可达） |
| **`decryption/setup` 拒绝「投票开放中」的提案** | 运营上必须在 `starts_at` 之前完成初始化 |
| **门户 7 个 `.vue` 面板无自动化测试** | 浏览器渲染是未验证区 |

### 4.3 三个从未验证的盲区

以下三项在本机（无 Docker）从未实跑过，CI 会实跑其中第一项：

1. **PostgreSQL testcontainers 集成测试** —— 本机走的是跳过分支
2. **门户 7 个 `.vue` 面板的浏览器实际渲染** —— 仓库里 `.vue` 零测试
3. **并发投票下的防重投与性能容量** —— 唯一索引兜底，但没测过
   - 🟡 **防重投这一半已补**（本轮落地）：`tests/e2e/concurrent_nullifier_test.go` ——
     8 个请求**同时**用同一个 nullifier、各自带不同凭证提交，断言恰好 1 张票入库、
     其余 7 个被唯一索引拦成 409 `DOUBLE_VOTING`。
   - ✅ **容量这一半已补**（本轮落地）：新工具 `cmd/loadtest` —— 纯 HTTP API 的并发投票压测，
     输出吞吐 / 延迟分位 / 状态分布 / 防重投结论，**防重投没守住时以非 0 退出**（结论能当门禁用）。
     本机实测（SQLite + WAL + `_busy_timeout`，200 票 / 并发 20）：257 req/s，
     p50=2.1ms p90=42.8ms p99=648.7ms，200 张全 201；同 nullifier 并发 8 次恰好放行 1 张、
     其余 7 张 409 `DOUBLE_VOTING`。
     ⚠️ 这是**可复现脚本 + 一份实测输出**，不是容量规划：SQLite 的单写者把并发写串行化，
     且客户端 prover 还没做（只能在 debug 模式跑）—— 容量规划按 §2.2 属 v0.3.0。

---

## 5. go / no-go（v0.2.0）

**No-Go（任一命中则不发 v0.2.0）**

> 右列是 **2026-10-21 的当前状态** —— 不是勾选结果，是「这条现在过不过」。

| # | 检查项 | 当前状态 |
|---|---|---|
| 1 | 能力 A–F 任一未完成 | ❌ **未过** —— A / D 未完成（T16 / T14，都卡在浏览器端 prover）；B / C / E / F 已达成 |
| 2 | 能力 G 的完成定义未达成 —— 领取凭证时不需要任何资格证明（T22），意味着服务端无法强制一人一票 | ❌ **未过** —— T22 未开工 |
| 3 | 存在「服务端能拿到明文选项」的代码路径（grep 不到才过） | ❌ **未过** —— 客户端加密已可用（CLI `ballot-encrypt`、浏览器端 TS 实现），但 debug 模式的 `ballot-draft` 仍是那条路径（release 模式已 404）；门户接上客户端加密后，grep 才能干净 |
| 4 | 存在「proof 可以跨提案复用」的路径 | ✅ **已过** —— 公开输入由服务端自拼，跨提案 / 跨 nullifier 复用均验签失败 |
| 5 | `go build` / `go vet` / `gofmt -l` / `go test ./...` 任一失败 | ✅ **已过** —— 本轮全绿 |
| 6 | 覆盖率门槛退出码非 0 | ✅ **已过** —— 逐包分档，合计 80.2% |
| 7 | 前端 lint / tsc / vitest 任一失败 | ✅ **已过** —— 125 个用例全过 |
| 8 | §4.3 三项盲区没有可复现输出 | ❌ **未过** —— T20 未开工（盲区 3 的「防重投」与「容量」两半已补；盲区 1 的 PG 集成测试已有 compose + `make pg-test` 可复现脚本，⚠️ 本机无 Docker 未实跑；盲区 2 的浏览器渲染仍未做） |
| 9 | README / techspec / 门户三处的匿名性声明不一致 | 🟡 **本轮已同步，待复核** —— 三处与 §4.1 的能力表对齐 |

**Go**

- [ ] 上述全过
- [ ] 决策门 G1–G9 全部拍板并落到代码/文档 —— **G1 / G2 / G3 / G4 / G5 / G7 / G8 已拍板**；G6 / G9 仍待拍板
- [ ] `VERSION` 与 tag 一致
- [ ] 生产部署清单全勾：[`docs/deployment-checklist.md`](./deployment-checklist.md)（`SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS` / `CRYPTO_VK_PATH` / `DB_DRIVER=postgres` / TLS / 门户独立部署 / 迁移步骤 / 密钥就位 / 启动日志自检）

---

## 6. 决策记录

**已拍板（2026-10-20）**

| # | 决策 | 结论 |
|---|---|---|
| **G1** | 版本定位 | ✅ **v0.1.0 = 已发布的内测版**；**v0.2.0 = 产品路线图目标**，不是「生产可用版」的承诺 —— 接口仍会变 |
| **G2** | 门户「按选项生成凭据」按钮 | ✅ **非 debug 隐藏**（T2 已落地）—— debug 标志来自 `/healthz` 的 `debug` 字段 |
| **G3** | `ballot-draft` 生产可达性 | ✅ **非 debug 返回 404**（T1 已落地）—— 用 404 而不是 403，避免暴露端点存在性 |
| **G4** | 盲签名：改实现还是改文档 | ✅ **先改文档如实描述**（T12），两段式（T17）留到 M2 一起做 —— v0.2.0 承诺匿名性时必须补上 |
| **G5** | 版本号 | ✅ 当前发版 `0.1.0`；路线图目标用 **`0.2.0`**，**不用 `1.0.0`**（1.0.0 意味着 API 稳定 + 生产可用，届时做不到；见 §1.1） |
| **G7** | 电路形状与曲线 | ✅ **Groth16/BN254**（T13 已落地）—— 沿用 verifier 原本硬编码的 `ecc.BN254` |
| **G8** | 投票人领凭证的形态 | ✅ **凭证池 + 投票人匿名随机领取**（T9）—— 不再用一次性领票码。核心理由：admin 逐个分发就等于 admin 知道哪个票是谁的，直接破坏不可关联性。**代价已记下**：一人一票要靠资格证明补回（T22） |
| **范围** | 这轮做哪些 | ✅ **T3–T12 这 10 项**已全部落地；T1（`ballot-draft` 非 debug 404）与 T2（门户隐藏按钮）暂缓，等 G2/G3 |

**本轮新增的决策（2026-10-21）**

> 这几条不是 G 系列的「决策门」，而是**实现过程中发现路线图自相矛盾时的取舍**。
> 它们都改变了对外行为（协议、接口、文档口径），所以必须留痕。

| # | 决策 | 结论 |
|---|---|---|
| **D1** | 电路公开输入**不含选项下标** | ✅ 选项下标改为**私有见证 + 范围检查**（`m < NumOptions`）。**理由**：§2.1 能力 C 原文写的公开输入含 `option_index`，与能力 A（「服务端在任何情况下都无法得知选了哪个」）**直接冲突** —— 两项完成定义打架时以能力 A 为准。**代价**：服务端无法从公开输入反推选项，这正是我们要的；但电路对外暴露的信息也少了，审计侧要看「证明绑定了哪一份密文」得靠密文本身而不是公开输入 |
| **D2** | 选票密文**v2 → v3**，换到 BN254 twisted Edwards | ✅ 密文格式 = 1 字节版本 `0x03` + 两个压缩点（65 字节），加密**选项下标**，指数 ElGamal `(a,b)=(r·G, m·G+r·Y)`。**理由**：电路算术在 BN254 标量域里，v0.1.0 的 ed25519 + HKDF 群运算**无法在电路中复算** —— 不换栈就无法证明「这份密文确实是本提案计票公钥加密的某个合法选项」。**代价**：**与 v0.1.0 的密文不兼容**，旧密文无法解密；门限分享从 ed25519 改为 Fr 上的 Shamir |
| **D3** | 依赖顺序：**T15 提前到 T14 之前** | ✅ 服务端自拼公开输入并验签，不依赖客户端 prover。**收益**：客户端 prover 接上来时对接的是已定死的公开输入格式，不返工。T18 一并提前（凭证验签与选票加密在不在客户端无关） |
| **D4** | T17 盲签名的走向 | ✅ **走真两段式**（不是「改文档」退化路径）。**理由**：v0.2.0 的核心承诺就是匿名性，能力 F 不做等于这个版本名不副实。**代价**：改动面大，且**排在 T14 之后**（需要客户端承载 crypto） |
| **D5** | nullifier 的编码要求 | ✅ 必须是 **32 字节规范域元素**（固定 64 位小写 hex），非规范编码直接拒绝。**理由**：`fr.Element.SetBytes` 是**按模归约**而不是拒绝，同一个值能有多种字节表示 —— 防重投的唯一索引会漏判。同理，**不用**乘性构造（`nullifier = sk·H`）：那会让服务端能反推出 `sk`。**本轮补修**：`votes.nullifier` 的唯一索引建在**字符串**上，所以光有「解码后的字节唯一」不够 —— 客户端用大写或带空白的形式提交同一个 nullifier，原样落库就能占掉多行、绕开唯一索引。落库前统一归一成 64 位小写 hex（`usecase/voting.go` 的 `decodeNullifier`） |

**待拍板**

> G6 / G9 仍待拍板。G2 / G3 已在本轮拍板并落地，不再卡住 M2。

| # | 决策 | 选项 | 建议 | 卡住什么 |
|---|---|---|---|---|
| G6 | 是否换纯 Go sqlite 驱动 | 保留 cgo（当前）/ 换 `modernc.org/sqlite` | **保留 cgo**：换驱动会波及持久化层方言差异测试，收益只是「静态链接二进制」，不值 | 发布形态 |
| G9 | CLI 是否需要本地 prover | CLI 也做本地 prover / CLI 只做「带 proof 提交」 | **CLI 只带 proof 提交**；生成 proof 放浏览器端，CLI 面向脚本化/运维场景 | T14 |

> **[§1.2](#12-v020-之后还有几个-0x0) 的 v0.3.0 / v0.4.0 / 1.0.0 推演不属于决策门。**
> 它们是方向性推演：没有完成定义、没有出口标准、没有任务卡，也没经过 G1–G9 这样的拍板。
> M2 跑完一轮后按实际进展重估 —— 届时才谈是否把它们升格为正式里程碑。

---

## 7. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| T14 客户端 prover 未做，release 模式下门户无法投票 | v0.2.0 的能力 D 不达成，发版价值受限 | T13 的电路在服务端已可跑（证明约 20ms、验证约 1–2ms），WASM 化的可行性已验证；T14 单独成提交链，**CHANGELOG 必须如实写明 release 模式仍不可投票** |
| T13 电路的证明生成太慢，浏览器端体验不可用 | 投票流程不可用 | ✅ **已实测**：约 7917 条约束，证明约 20ms、验证约 1–2ms，性能风险基本解除。必要时仍可换电路形状（公开输入更少）而不是硬扛 |
| T17 真盲签名改动面大，拖垮整个 v0.2.0 | 发版无限延期 | ✅ **已解除** —— T17 已于 2026-10-21 落地（协议 + 门户 / CLI 接线 + e2e 验收），改动面比预估小：领域接口拆成 `BlindSign` / `IssuerPublicKey` / `VerifyCredential` 三个方法，没有新增 schema 列 |
| T7 KDF 换协议使 v0.1.0 之前的旧密文永久不可解 | 旧数据丢失 | 明确标注密文格式版本；v0.1.0 起格式即 v2，v0.2.0 前不要用旧数据做真实投票 |
| **D2 密文 v2 → v3 使 v0.1.0 的选票永久不可解** | 旧数据丢失 | v0.2.0 尚未发版，本地/内测数据可弃；CHANGELOG 必须写明「与 v0.1.0 不兼容」；发版前清库 |
| **debug 模式下服务端仍能拿到明文选项** | 能力 A 未达成 | T1 已把该端点在非 debug 模式关成 404；**release 模式必须显式设置 `server.mode`**，否则开发默认值会让能力 A 在生产失效 —— 部署清单里逐项核对 |
| T16 把加密移到客户端后，前端成为新的攻击面 | 密钥泄露 | proving key 只在前端，**不入服务端**；前端构建产物做完整性校验 |
| 匿名随机领取牺牲了一人一票 | 可重复投票 | M3 用资格证明补回（T22，见 §2.3） |
| M1 拖太久，M2 无时间 | 路线图整体延期 | M1 与 M2 **可并行**（除 T1/T2 依赖 M2 的字段） |

---

## 附录 A · 工程细项

路线图的判断以正文 §2–§4 为准；这里保留问题描述、证据与改法，供实现时直接取用。
**已落地的任务卡保留原始的问题与改法，并附「落地核对」行** —— 逐条说明实际怎么做的、
与原设想有什么偏差、哪些验收项没达成。当前已落地：**T1、T2、T13、T15、T18**（2026-10-21）
与 **T9**（见 A.4）。**其余任务卡尚未落地**（A.3 里的 T22 是 T9 留下的缺口）。

### A.1 挂在 M1 但等决策的

#### T1 · `ballot-draft` 在非 debug 模式关闭（决策门 G3）✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | `POST /api/v1/proposals/:id/ballot-draft` 是**公开路由**，返回写死的假 proof。任何人都能调用，且这条链路的密文由服务端生成。 |
| 证据 | `internal/infrastructure/http/handler.go:65`（路由未挂在 admin 分组下）、`:408`（常量 `developmentProofBase64`，由 `PrepareBallotDraft` 在 `:442` 返回） |
| 改法 | ① `config.Config` 增加 `DebugMode bool`（由 `server.mode` 推导：`debug`/`dev`/`development` 为 true）；② 路由在非 debug 模式返回 **404**（用 404 而不是 403，避免暴露端点存在性）；③ `server_test.go` 里现有的 `ballot-draft` 用例按模式分叉，debug 断言 201、release 断言 404 |
| 落地核对 | ① ✅ `config.Config.DebugMode`（`internal/config/config.go` 的 `isDebugMode`，`server.mode` 为 `debug`/`dev`/`development` 时为 true）；② ✅ `Handler` 新增 `NewHandlerWithTallyAndMode`，非 debug 时 `PrepareBallotDraft` **直接 404**；③ ✅ `GET /healthz` 响应新增 `debug` 布尔字段，门户据此决定要不要显示调试入口；④ ✅ 新增用例 `TestHandler_BallotDraftHiddenOutsideDebugMode` 断言非 debug → 404 |
| 验收 | `go test ./...` 全绿；新增用例断言 release 模式 404 —— ✅ 已达成 |
| 预估 | 1 小时 |
| 依赖 | G3 ✅ 已拍板（§6） |

#### T2 · 门户「按选项生成凭据」按钮在非 debug 模式隐藏（决策门 G2）✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | 门户在运营侧提供「按选项生成凭据」的入口，等于把「服务端替用户选」的能力做成一键操作。 |
| 证据 | `web/src/composables/useVotePortal.ts:977` `issueCredential`、`web/src/components/panels/IdentityPanel.vue:13` |
| 改法 | 按钮按 `debug` 模式条件渲染；非 debug 时给一句说明为什么没有这个入口 |
| 落地核对 | ① ✅ `useVotePortal.ts` 读 `/healthz` 的 `debug` 字段，暴露为 `debugMode` ref；② ✅ `VotePanel.vue` 的「按选项生成凭据」按钮加 `v-if="portal.debugMode"`，非 debug 时改为一句说明文案；③ ⚠️ **验收方式与任务卡原设想不同**：仓库无 `@vue/test-utils`（离线不可安装），`.vue` 面板零测试 —— 改为在 `web/src/lib/nullifier.ts` 落纯模块 vitest（4 个用例），**按钮的条件渲染本身未被自动化测试覆盖**（属 §4.3 的盲区） |
| 验收 | vitest：非 debug 模式渲染不出该按钮；debug 模式仍在 —— ⚠️ **部分达成**（见落地核对 ③） |
| 预估 | 1 小时 |
| 依赖 | G2 ✅ 已拍板（§6） |

### A.2 M2 · 证明链

#### T13 · 真实选票电路 + `verifying.key` 生成流程 ✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | **选票的零知识电路根本不存在。** `internal/infrastructure/crypto/gnark_proof_verifier.go` 只有验证器骨架；`NewGnarkProofVerifier`（`:25`）在没有 `vk_path` 时构造出 `requireVerifyMode: false` 的实例，`Verify` 在 `:55-57` 对**任何非空 proof 直接返回 nil**。生产配置的 `crypto.vk_path: "./keys/verifying.key"` 按本仓库**无法生成**。 |
| 证据 | `gnark_proof_verifier.go:25-31`、`:55-57`、`configs/config.production.yaml` 的 `vk_path` |
| 改法 | ① 电路定义「选票属于某个提案的某个选项」：Groth16/BN254；② 新增 `cmd/keygen` 生成 `verifying.key` + `proving.key`（proving key **不入库**，只在客户端侧使用）；③ 密钥分发与轮换说明写进 `docs/techspec.md`；④ 仓库只提交 `verifying.key` 的**生成脚本与公钥指纹**，不提交 proving key |
| 落地核对 | ① ✅ 新包 `internal/infrastructure/crypto/ballot/`：曲线 BN254 twisted Edwards（余因子 8 子群校验）、计票密钥对、Fr 上的 Shamir 拆份额 / 拉格朗日恢复、**v3 选票密文**（D2）、nullifier 派生（`DeriveProposalField` SHA-256→Fr；`DeriveNullifier` **MiMC**，与 gnark `std/hash/mimc` 逐位一致）、规范编码工具（`EncodeScalarHex` / `ParseScalarHex`）；② ✅ 真实电路 `circuit.go`：**公开输入 9 个**（提案 ID 域元素、nullifier、计票公钥 XY、密文 a 的 XY、密文 b 的 XY、选项数），**私有见证 3 个**（选项下标 m、nonce r、nullifier 秘密），约束 `a=r·G`、`b=m·G+r·Y`、`m<NumOptions`（8 位分解 + 最高位检查）、`nullifier=MiMC(secret, proposal_id)`；③ ✅ `cmd/keygen` 生成 `verifying.key`(0644) / `proving.key`(0600)，打印 sha256 指纹，默认不覆盖（`-force` 强制）；④ ✅ **实测：约 7917 条约束，证明约 20ms、验证约 1–2ms**；⑤ ✅ 电路测试覆盖：证明往返、篡改 nullifier / 提案 / 密文 / 选项数 / 公钥全部失败、选项越界无法出证、错 nullifier 秘密无法出证 |
| **偏差** | ⚠️ **公开输入不含选项下标** —— 任务卡原文写的是 `public = (proposal_id, option_index, nullifier)`，本轮改为**选项下标进私有见证 + 范围检查**。理由见 §6 决策 **D1**（与能力 A 冲突）。**服务端仍能验证「密文加密的是某个合法选项」，但不能验证是哪一个** |
| 验收 | ① `cmd/keygen` 产出的 vk 能被 `NewGnarkProofVerifier` 加载；② 电路测试：正确 witness 生成 proof → 验证通过；篡改任一公开输入 → 验证失败 —— ✅ 已达成 |
| 预估 | 3–5 天（含电路调参与性能评估） |
| 依赖 | G7 ✅ 已拍板（§6） |

#### T14 · 客户端 prover（浏览器 WASM）

| 项 | 内容 |
|---|---|
| 问题 | 门户在生产模式下 **100% 无法完成投票**：没有 prover 就产不出 proof。现有 `ballot-draft` 返回写死的假 proof。 |
| 证据 | `handler.go:410-445`（`PrepareBallotDraft`：`:408` 常量 proof、`:442` 返回）、review C2 |
| 改法 | ① 用 gnark 官方 wasm 模板产出 prover WASM，放进 `web/public/` 或独立静态目录；② 门户在浏览器内生成 proof；③ `ballot-draft` 降级为「仅返回选票选项与加密公钥」，**不再返回假 proof**；④ CLI 侧同步：CLI 只做「带 proof 提交」，本地 prover 归浏览器（G9 建议） |
| **状态修正** | ⚠️ 任务卡原文的「生成 proof **与 `public_witness`**」已失效 —— `public_witness` 自由文本入口在 T15 里删掉了，公开输入由服务端自拼。浏览器端 prover 现在只需产出 **proof**（外加密文与 nullifier），公开输入不用客户端给。**这也让 T14 比原设想简单**：不用再纠结「怎么把公开输入塞进客户端再传回来」 |
| **新增要求** | ⚠️ **nullifier 必须改成「秘密 + nullifier」成对生成**：电路约束是 `nullifier = MiMC(NullifierSecret, ProposalID)`，而当前服务端 `BallotNullifierGenerator` 与浏览器 `web/src/lib/nullifier.ts` 产出的都只是**随机域元素** —— 这样的 nullifier **出不了证明**（库里 `DeriveNullifier` 只在测试里被调用）。T14 必须：① 浏览器侧实现与 `ballot/nullifier.go` 逐位一致的 MiMC；② 生成随机秘密 s，公开 nullifier = MiMC(s, proposal_id)，**秘密 s 留在本地**作为电路的 `NullifierSecret` 见证；③ 门户不再用「随机 nullifier」填表 |
| **落地核对（部分）** | ✅ **①②已完成**（本轮补齐，路线图 T14 的前置件）：浏览器端新增 `web/src/lib/{field,sha256,keccak,mimc}.ts` 与扩展后的 `web/src/lib/nullifier.ts` —— SHA-256 映射提案 ID、legacy Keccak-256 链派生 91 个 MiMC 轮常量、Fr 上的 MiMC（轮函数 `(x+h+c)^5` + Miyaguchi–Preneel），与 Go 侧 `ballot/nullifier.go` **逐位一致**；跨语言向量由 `internal/infrastructure/crypto/ballot/nullifier_interop_test.go` 与 `web/src/lib/{mimc,nullifier}.test.ts` 共同锁定，Go 侧还真的用这组值出了一份证明并验过（**浏览器算出的 nullifier，电路认**）。⏳ **③未完成**：门户（`useVotePortal.ts`）仍在用 `generateNullifier` 生成随机域元素 —— 与门户 prover 一起改（T14）。SHA-256 没用 WebCrypto：`crypto.subtle` 要求安全上下文，门户部署在内网 http 地址下会失效 |
| 验收 | ① 浏览器端生成的 proof 能通过服务端 `Verify`（e2e 用例）；② proof 绑定的公开输入与服务端自拼的一致 |
| 预估 | 5–7 天 |
| 依赖 | T13 |

#### T15 · proof 与 `proposal_id` / `nullifier` / 密文绑定 ✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | `SubmitVoteRequest.PublicInputs()`（`internal/usecase/voting.go:60-64`）**造了** `proposal_id` 和 `nullifier`，但 `Verify` 收到的 `inputs` 里只有 `public_witness` 是被真正读取的。也就是说：**服务端完全没有校验 proof 与请求里的 proposal_id / nullifier / 密文是否对应** —— 攻击者可以把 A 提案的合法 proof 拿去投 B 提案。 |
| 证据 | `internal/usecase/voting.go:60-64`、`:148`、`gnark_proof_verifier.go:64` |
| 改法 | ① 电路公开输入按 §2.1 能力 C 的完成定义（**注意：不含选项下标**，见 §6 决策 D1）；② 服务端按序拼出**完整的**公开输入数组，不再走 `public_witness` 这条旁路；③ verifier 校验公开输入与 `proposal_id` / `nullifier` / 密文 / 计票公钥 / 选项数一致（不一致直接失败）；④ 移除 `public_witness` 的自由文本入口 —— 公开输入由服务端根据请求字段**自己拼** |
| 落地核对 | ① ✅ `entity.PublicInputs` 删除，改为 `entity.BallotStatement{ProposalID, Nullifier, ElectionPublicKey, Ciphertext, NumOptions}`；② ✅ `domaincrypto.ProofVerifier.Verify(proof, statement)` —— 验证器直接吃声明，不再吃自由文本字典；③ ✅ `GnarkProofVerifier` 改成**真实验签**（`vk_path` 为空时只校验 proof 非空 = 开发模式）；④ ✅ `usecase/voting.go` 的 `PublicInputs(proposal, session)` 由服务端按请求字段 + 提案 + 计票会话自行拼装；⑤ ✅ HTTP 层与 CLI 都**移除了 `public_witness` 字段**；⑥ ✅ nullifier 请求侧必须是 64 位 hex 规范域元素（`decodeNullifier` + `Validate()`），非规范编码直接拒（D5）；⑦ ✅ **nullifier 落库前归一成 64 位小写 hex** —— 唯一索引建在字符串上，不归一的话同一个 nullifier 的大小写变体就能绕开它（重投漏洞，见 D5）；⑧ ✅ HTTP 层新增 `entity.BallotStatement` 链路，跨提案 / 篡改 nullifier / 篡改密文均 400 |
| 验收 | 回归测试：拿 A 提案的 proof 投 B 提案 → **400**；篡改 nullifier → 400；密文改了再投 → 400 —— ✅ 已达成（`internal/infrastructure/http/proof_verification_test.go` 的 `TestServer_RealBallotProof`） |
| 预估 | 2–3 天 |
| 依赖 | T13 ✅（本轮实际顺序为 T13 → T15，理由见 §6 决策 D3） |

#### T16 · 加密移到客户端

| 项 | 内容 |
|---|---|
| 问题 | **当前版本最大的匿名性缺口**：密文由服务端生成（`ballot-draft` 拿明文 `option` 去加密），服务端在生成凭据时就知道选了哪个。 |
| 证据 | `internal/infrastructure/http/handler.go:432`（`EncryptVoteOption` —— 服务端拿明文选项加密）、`:405`（`Option string` 字段）、review C5 |
| 改法 | ① 服务端只发**加密公钥 + 电路参数**，不再接受明文选项；② 浏览器端完成「选票 → 加密 → proof → 提交」全流程；③ 服务端接口改为接受 `{encrypted_vote, proof, nullifier, ...}`，**没有明文选项字段**；④ `ballot-draft` 端点随之删除（T1 已先把它关掉） |
| **状态补充** | 加密栈已在本轮换到 **v3**（D2）：浏览器端要做的是 BN254 twisted Edwards 上的指数 ElGamal，加密的是**选项下标**。服务端计票侧已就绪（`BallotThresholdCrypto`），**T16 缺的是浏览器侧的加密实现**。另外：T1 已把 `ballot-draft` 在非 debug 模式关成 404，**服务端代加密这条路在 release 模式已经不可达** —— 但「服务端代码里不存在接收明文选项的字段」这条验收还没满足（`PrepareBallotDraft` 的请求体仍有 `option` 字段，debug 模式可达） |
| 验收 | ① 服务端代码里**不存在**接收明文选项的字段（grep 验证）；② e2e：浏览器投票 → 服务端落库记录里没有可还原的选项 |
| 落地核对 | ① ✅ **浏览器端选票加密**：`web/src/lib/ballot-cipher.ts` 实现 BN254 twisted Edwards 上的指数 ElGamal（加密选项下标），点编码与 Go 侧**逐字节一致**（互操作向量见 `internal/infrastructure/crypto/ballot/kat_interop_test.go` 与 `web/src/lib/ballot-cipher.test.ts`）；② ✅ **CLI 客户端加密**：`cmd/cli` 新增 `ballot-encrypt` 命令，客户端取公钥 → 本地加密 → 落盘选票文件（含 nullifier 秘密，只留本地）；③ ✅ **服务端公开计票公钥**：新接口 `GET /api/v1/proposals/:id/election-key`（未压缩形式，客户端不必实现点解压）；④ ✅ **e2e 证据**：`tests/e2e/client_side_encryption_test.go` —— 客户端本地加密 → 落库记录里只有密文 → 计票仍还原出客户端选的选项；⑤ ⚠️ **门户还没接上**（缺 prover，接了也投不出去），`ballot-draft` 这条服务端代加密路径在 debug 模式仍存在 —— **验收 ① 因此尚未完全达成** |
| 预估 | 3–4 天 |
| 依赖 | T13、T14、T15 |

#### T17 · 盲签名改真两段式 ✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | 盲签名的**盲化在服务端内部完成**：`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181` 在同一个进程里依次 `Prepare` → `Blind` → `BlindSign` → `Finalize`，服务端全程持有明文消息 —— **不具备盲化性质**，等同于普通签名。 |
| 证据 | `kryptology_credential_issuer.go:162-181` |
| 改法 | ① 拆成两个 HTTP 端点：客户端 → `POST /credentials/blind`（提交盲化后的消息）→ 服务端 `BlindSign` 返回盲签名 → 客户端 `Finalize` 得到最终签名 → `POST /credentials/attest`（提交最终签名 + 承诺）→ 服务端用 `Verify` 校验；② 客户端实现 `blindrsa` 的 client 侧 —— **纯 TypeScript 即可，不需要 WASM**（SHA-384 + EMSA-PSS 都是纯算术，BigInt 够用；Go 侧同库，零新依赖）；③ 协议要绑定被盲化的消息承诺，防止「拿 A 的盲签名去签 B」 |
| **实现要点（本轮查证）** | ⚠️ 读完 CIRCL `blindsign/blindrsa` 的源码后确认了三件事，直接决定改法：① **client 侧不需要私钥** —— `Client.Prepare` / `Blind` / `Finalize` 全是**客户端**操作，只要签发方的 **RSA 公钥**（`n`、`e`）就够了，服务端只做 `Signer.BlindSign(blindedMessage)`。因此 **CLI 可以立刻当真正的盲签名客户端**（Go 侧同库，零新依赖），而浏览器端需要自己实现 client 侧：SHA-384 + EMSA-PSS-ENCODE + RSA 盲化运算（BigInt 可做，但必须与 CIRCL **逐字节一致**，用 `ballot-cipher` 那套 KAT 纪律锁死）。② **签名端点必须绑定池槽位** —— 公开的盲签名端点会把签发方变成「任意消息签名预言机」：必须先给客户端预留一个槽位（拿到假名 `user_id`），盲签名只对这个槽位签，attest 时再用「最终签名验签 (user_id, user_pub_key)」把两边绑上 —— 客户端若拿 A 的盲签名去签 B，验签必然失败（验收 ② 因此可测）。③ **服务端在 attest 阶段仍会看到被签内容**（它必须把 `user_id` / `user_pub_key` 存下来），所以能力 F 的准确口径是「**签名阶段**服务端只见盲化消息」，不是「全程看不见」—— 文档口径要按这个写，别写成做不到的承诺 |
| **落地核对** | ✅ **全部完成**（2026-10-21）。① **领域接口拆分**：`domaincrypto.CredentialIssuer` 变成 `BlindSign(ctx, keyVersion, blindedMessage)` / `IssuerPublicKey(ctx)` / `VerifyCredential(userID, userPubKey, signature)` 三个方法，一次性的 `Issue` 已删除 —— 签发方不再有「同时看见消息并签名」的入口。② **协议三步**：`POST /proposals/:id/credentials/claim`（只预留，请求体为空，**不收公钥**）→ `POST /proposals/:id/credentials/blind-sign`（只对**池里还空着的槽位**签名）→ `POST /proposals/:id/credentials/attest`（先验签再落库）。③ **管理端同样走真两段式**：`POST /api/v1/credentials`（指定 `user_id` 预留）+ `POST /api/v1/credentials/blind-sign`（任意 `user_id`，走全局路径是因为 gin 的路由树不允许同路径挂两套中间件）。④ **客户端实现**：浏览器端 `web/src/lib/credential.ts`（+ 已有的 `blind-rsa.ts`），Go 端 `internal/infrastructure/crypto/credential_client.go`（CLI 与 e2e 共用）；跨语言向量除盲签名本身外，**凭证消息**（`sha256(user_id ‖ 0x00 ‖ user_pub_key)`）也由 `blind_rsa_interop_test.go` 与 `web/src/lib/credential.test.ts` 锁死。⑤ **CLI 与门户接线**：`credential-claim` / `credential-issue` / 门户的「领取凭证」「签发凭证」都走完整流程。⑥ **槽位竞争**：预留不占位 → 并发领取会抢同一槽位，重试放**客户端**（最多 5 次，只在 attest 返回 409 `CREDENTIAL_EXISTS` 时重试；409 的 `CREDENTIAL_POOL_EMPTY` 不重试）。**没有新增 schema 列** —— 槽位绑定用 `CredentialClaim` 既有字段 + 新增的 `GetAvailableByUserID` 查询实现 |
| 验收 | ① 端到端：客户端拿到的凭证，服务端**在签名阶段**只见盲化消息 —— ✅ `tests/e2e/blind_signature_test.go` 的 `TestAPI_TwoPhaseBlindSignature_E2E`（断言预留请求体里没有 `user_pub_key`，盲签名阶段收到的字节 ≠ 凭证消息原文）；② 交叉攻击测试：用 A 的盲签名 finalize 出 B 的凭证 → 校验失败 —— ✅ `TestAPI_BlindSignatureCrossAttack_E2E`（两种交叉都 401 `INVALID_CREDENTIAL`）；③ 服务端代码里不再出现「同一个函数里 Prepare+BlindSign+Finalize」 —— ✅ `Issue` 已删除，`kryptology_credential_issuer.go` 里服务端只做 `BlindSign(盲化消息)`；④ 外加「公开盲签名端点必须绑定池槽位」（`TestAPI_BlindSignRequiresPoolSlot_E2E`）与「槽位被抢时客户端换槽位重来」（`TestAPI_ClaimRetriesWhenSlotTaken_E2E`） |
| 预估 | 5–7 天（实际约 1 个工作日 —— 客户端侧早已就位，改动集中在协议与服务端） |
| 依赖 | T9（凭证池路径）✅；T14 **不是硬依赖** —— 两段式的客户端侧只需签发方公钥，不需 prover（实际执行顺序提前到 T14 之前，理由见 §6 决策 D3） |
| 备注 | **口径**：能力 F 达成的是「**签名阶段**服务端只见盲化消息」。attest 阶段服务端仍然看得到 `user_id` 与 `user_pub_key`（验签必须重算凭证消息）—— 文档不能写成「全程看不见」 |

#### T18 · 投票路径补 `VerifyCredential` ✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | 凭证签名在生产路径上**从不被密码学验证**：`VerifyCredential` 只在 `internal/infrastructure/crypto/` 及其测试里出现，投票提交路径不调用 —— 任何人都能伪造凭据投票。 |
| 证据 | `kryptology_credential_issuer.go:194` 定义了它，但 `internal/usecase/voting.go` 的 `Submit` 未调用 |
| 改法 | ① `Submit` 在落库前调用 `VerifyCredential(userID, userPubKey, credentialSignature)`，失败返回 401；② 需要在 `SubmitVoteRequest` 里带上签名与公钥（当前有 `CredentialSignature`，需确认公钥来源）；③ 补「伪造签名 → 401」回归测试 |
| 落地核对 | ① ✅ `domaincrypto.CredentialIssuer` 接口新增 `VerifyCredential`；② ✅ `usecase/voting.go` 的 `Submit` 在**落库前**调用签发方验签，失败 → `ErrInvalidCredential` → HTTP **401**；③ ✅ 公钥来源是请求里的 `user_pub_key`（凭证领取时由投票人自带的一次性公钥签发）；④ ✅ 回归测试：伪造凭证签名的投票请求 → 401（`TestServer_RealBallotProof` 内含该用例） |
| 验收 | 伪造签名的投票请求返回 401；合法凭证正常落库 —— ✅ 已达成 |
| 预估 | 1–2 天 |
| 依赖 | 原写「T16」，实际**提前到 T15 之后**即可做（凭证验签与选票加密在不在客户端无关，见 §6 决策 D3） |

### A.3 M3 · 生产化与发版

#### T19 · 生产配置落地

| 项 | 内容 |
|---|---|
| 问题 | `configs/config.production.yaml` 的 `cors_allowed_origins: []` 仍为空；且该文件**永远不会被自动加载**（viper `SetConfigName("config")`，`internal/config/config.go:27`）。 |
| 证据 | `configs/config.production.yaml`、`internal/config/config.go:42`（`v.SetConfigName("config")`） |
| 改法 | ① 给 viper 加 `CONFIG_FILE` 显式路径支持（**推荐**：比改名更不容易出错）；② CORS 填门户真实源 |
| 落地核对 | ① ✅ **`CONFIG_FILE` 显式路径**（`internal/config/config.go` 的 `ConfigFileEnvVar`）：`CONFIG_FILE=./configs/config.production.yaml` 启动即可加载生产配置，**显式文件不存在时直接启动失败**（静默回退到开发配置是最危险的上线事故）；② ✅ 启动日志的 `effective configuration` 新增 `config_file` 字段，打印**实际加载的配置文件路径**；③ ✅ release 模式下 `cors_allowed_origins` 为空时打**告警**（同源部署留空合法，故不阻止启动）；④ ✅ `configs/config.production.yaml` 的注释改为推荐 `CONFIG_FILE` 用法；⑤ ⚠️ **CORS 的真实源仍是部署项** —— 仓库里填不了，得按部署环境写进 `SERVER_CORS_ALLOWED_ORIGINS` 或配置文件 |
| 验收 | 用 `CONFIG_FILE=./configs/config.production.yaml` 启动，启动日志显示 `server_mode=release`、CORS 生效 → ⚠️ **部分达成**：加载路径、失败保护、日志可核对都已落地；CORS 生效取决于部署时填入真实源 |
| 预估 | 半天 |
| 依赖 | 无 |

#### T20 · 端到端验收（三个盲区）

| 项 | 内容 |
|---|---|
| 问题 | review §5 列的三项**从未被验证**：① PostgreSQL testcontainers 集成测试；② 门户 7 个 `.vue` 面板的浏览器实际渲染；③ 并发投票下的防重投与性能容量。 |
| 证据 | `docs/release-review-2026-10-20.md` §5 |
| 改法 | ① Docker 环境跑通 `tests/integration`（CI 会实跑，本地补一份 compose）；② 引入 Playwright 覆盖关键路径（领凭证 → 投票 → 查询状态 → 计票）；③ 用 `hey`/`k6` 写并发投票脚本，验证防重投（唯一索引兜底）与容量 |
| 落地核对 | ① ⏳ CI 已实跑 PG 集成测试，本机无 Docker 走跳过分支；✅ **compose 已补**（仓库根 `docker-compose.yml` + `make pg-test`：起库 → 跑 `go test ./tests/integration/ -run Postgres` → 关库），且集成测试新增**外部库路径**（设 `ANONVOTE_TEST_PG_HOST` 即连已有 PostgreSQL，每个测试开一个独立 schema 并在跑完时 `DROP SCHEMA ... CASCADE`）—— 两条路径共用同一份测试代码，本地与 CI 跑的是同一套断言；② ⏳ 离线环境装不了 Playwright，门户 `.vue` 仍零测试；③ 🟡 **防重投这一半已落地**：`tests/e2e/concurrent_nullifier_test.go` 用 8 个并发请求 + 同一 nullifier + 不同凭证，断言恰好 1 张票入库、其余 409 `DOUBLE_VOTING`（本机可复现，`go test ./tests/e2e/ -run TestAPI_ConcurrentSameNullifier_E2E`）；**容量这一半也已落地**：新工具 `cmd/loadtest`（纯 HTTP API，不碰数据库）输出吞吐 / 延迟分位 / 状态分布 / 防重投结论，`-json` 可机器读，**防重投没守住时退出码非 0**；本机实测 200 票 / 并发 20 → 257 req/s、200 张全 201、同 nullifier 并发 8 次恰好放行 1 张（⚠️ SQLite 单写者 + 只能在 debug 模式跑，**不是容量规划**） |
| 验收 | 三项都有**可复现的脚本 + 输出**，并把结果贴进 PR |
| 预估 | 2–3 天 |
| 依赖 | T14（浏览器端投票链路才完整） |

#### T21 · 发版（v0.2.0）

| 项 | 内容 |
|---|---|
| 动作 | ① `VERSION` 改 `0.2.0`；② `CHANGELOG.md` 写 0.2.0 条目（**若本轮改了密文格式，必须写明与 v0.1.0 的密文不兼容**；T7 的格式变更已含在 v0.1.0 之内，v0.2.0 默认不改格式）；③ `git tag v0.2.0` → release workflow 自动构建；④ 按 v0.1.0 的生产部署清单逐项核对；⑤ 同步 `web/package.json` 的 version（单一版本源是仓库根的 `VERSION`，发布流水线构建前自动注入，无需手改） |
| 验收 | `gh release view v0.2.0` 有产物；生产部署清单全勾 |
| 预估 | 半天 |
| 依赖 | M1、M2、M3 全部完成 |

#### T22 · 资格证明：把一人一票补回来（T9 留下的缺口）

| 项 | 内容 |
|---|---|
| 问题 | T9 把凭证改成匿名随机领取后，**服务端无法强制一人一票**。缺口的确切来源：池里槽位带的是随机假名 `user_id`，而服务端唯一能做「一人一票」约束的字段正是 `user_id`（`vote_participations` 存 `hash(user_id)`）—— 既然 `user_id` 是假名，服务端看不出同一个人领了几次、投了几次。**这等于用「可重复投票」换来了「不可关联性」。** |
| 证据 | `internal/usecase/registration.go:140`（`FillCredentialPool` 生成随机假名 `user_id`）、`:197`（`ClaimCredential` 用领取者自带公钥签发）、`internal/domain/entity/credential_claim.go` |
| 改法 | ① 定义**资格证明**：投票人在领取凭证时额外提交一份「我有资格投这一票」的零知识证明，证明内容不暴露身份（如基于一次性设备密钥的可验证资格 / 资格名单的 Merkle 承诺）；② 领取端点校验该证明，**不通过就不给凭证**（而不是给凭证后在投票时拒 —— 那时已经浪费了一个池槽位）；③ 电路的公开输入要绑定 `proposal_id` 与资格承诺，防跨提案复用；④ 服务端对「同一资格」只能领取一次（一次性资格标识，落库只存哈希）；⑤ ✅ **CLI `credential-claim` 命令已补**（本轮落地：公开端点、不需要 admin token、请求体只有本地生成的公钥） |
| 验收 | ① 无资格证明的领取请求 → 拒绝（4xx），池槽位不被消耗；② 有效资格证明 → 领取成功；③ 同一份资格证明第二次领取 → 拒绝；④ 资格证明在不同提案间不可复用；⑤ 领取请求体里不含任何真人身份字段（grep 可验） |
| 预估 | 5–7 天（依赖 M2 的电路与 prover） |
| 依赖 | T13、T14（资格证明要用真实电路与客户端 prover） |
| 备注 | **这是能力 G 的完成定义的最后一环**（§2.1）。不做的话，v0.2.0 就必须继续在 README 里写明「无法保证一人一票」—— 那与「有真实证明链的实验版」这个定位不符 |

### A.4 T9 · 凭证池 + 投票人匿名随机领取（✅ 已落地）

> 这张卡是**已落地**的任务，保留原始的问题与改法，并附**逐条落地核对**，
> 供以后回看「当时为什么这么改、哪些步没做」。

| 项 | 内容 |
|---|---|
| 问题 | 凭证签发接口挂在 admin 分组下，**投票人自己也必须持有 admin token 才能领凭证**。更致命的是：即便换成 admin 批量分发，**admin 手里有「谁领了哪个凭证」的台账** —— 而选票最终会绑定到凭证的 `user_id` / `user_pub_key`（`vote_participations` 存 `hash(user_id)`），于是 admin 可以把「这张票」关联到「这个人」。**这比 admin token 泄露更危险：它直接破坏选票与身份的不可关联性**，而这正是整个系统存在的理由。 |
| 证据 | `internal/infrastructure/http/server.go`（凭证签发挂在 admin 分组下）、`internal/usecase/voting.go:60-64`（公开输入只透传客户端给的 `nullifier`，尚未与 proof 绑定 —— 见 T15）、`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181`（凭证绑定 `user_id` + `user_pub_key`） |
| 改法 | ① **凭证池**：admin 批量预生成 N 个槽位，**服务端不记录「谁对应哪个凭证」的台账**；② **匿名领取**：新增**公开**端点，请求体**不含任何真人身份信息**，服务端随机分配一个未领取的槽位；③ 客户端在领取时生成**全新的一次性 keypair** 并连同 pubkey 提交，服务端用 issuer key 对 `(领取随机 id, pubkey)` 签名；④ 领取端点**不写 IP、不写 User-Agent、不落任何可关联日志**，只记一条匿名计数；⑤ 投票时用该 keypair 签 nullifier，防重投靠 `nullifier` 唯一索引 |
| 落地核对 | ① ✅ `CredentialClaim` 实体 + 仓储，池里存的是**未签发槽位**（随机假名 `user_id`，空 `UserPubKey` / `Signature`）；② ✅ `POST /api/v1/proposals/:id/credentials/claim`（公开）+ `POST /api/v1/proposals/:id/credentials/pool`（admin），池空 → 409 `CREDENTIAL_POOL_EMPTY`；③ ✅ 领取时用**领取者自带的一次性公钥**当场签发（`ClaimCredential` → `issuer.Issue(claim.UserID, userPubKey)`），签发不落池；④ ✅ 领取路由不记 `client_ip`（`middleware/request_logger.go` 的 `anonymousPathParts`），User-Agent 本就不记；⑤ ✅ 防重投靠两道数据库唯一索引 —— `votes.nullifier` 全局唯一 + `vote_participations` 的 `(proposal_id, user_hash)` 复合唯一。**但 nullifier 由客户端提供**，服务端还没把它与 proof 绑定校验（那是 T15）—— 这是「一人多领就能多投」之外的第二条路径；⑥ ⚠️ **一人一票没做到** —— 每次领到的 `user_id` 都是新的随机假名，同一个人领两次会拿到两张凭证、可以投两票，见 §2.3 与 T22；⑦ ✅ **CLI `credential-claim` 命令已补**（本轮落地）—— 此前门户有按钮、HTTP 接口有，脚本化领取用不了；现在 CLI 也能匿名领取（公开端点，不需要 admin token，请求体只有本地生成的公钥） |
| 验收 | ① admin 批量投池 → 池里有 N 个槽位；② 匿名领取拿到一张凭证，服务端日志里**没有领取者的 IP**；③ 池空 → 409；④ 并发领取不会把同一槽位发给两个人（条件更新保证）；⑤ 同一槽位不会被领两次（`credentials` 的 `(proposal_id, user_id)` 唯一索引兜底）；⑥ ⚠️ **同一个人领两次会拿到两张 `user_id` 不同的凭证、可以投两票** —— 这是一人一票的缺口，归 T22 |
| 提交 | `17937da` |
| 决策门 | G8 ✅ 已拍板（§6） |

**它留下的缺口**：能力 G 达成的同时丢掉了「一人一票」，且 CLI 领取路径缺失 ——
两项都归到 **T22**。

---

## 附录 B · 提交分组建议

按 `incremental-implementation` 原则，每张任务卡一个提交，Conventional Commits：

```
chore(release): gate the ballot draft endpoint behind debug mode
fix(web): hide the option-scoped credential button outside debug
feat(circuit): add the ballot circuit and the key generation flow
fix(vote): bind the proof to the proposal, nullifier and ciphertext
fix(vote): verify the credential signature before storing the vote
feat(prover): prove ballots in the browser
feat(vote): encrypt ballots on the client
fix(credential): use a two-phase blind signature
feat(config): support an explicit config file path
test(e2e): cover the browser voting flow and concurrent voting
feat(credential): require an eligibility proof when claiming a credential
feat(cli): add the credential-claim command
```

**本轮（2026-10-21）实际涉及的提交**：前 5 条 —— T1、T2、T13、T15、T18。
其中 T13 覆盖了「电路 + 密钥生成 + 密文换栈 v3」，T15 覆盖了「公开输入自拼 +
移除 `public_witness`」，两者改动面大但耦合紧密，**是否再拆细由实现者按可回滚性决定**。

**未做的三条（`feat(prover)` / `feat(vote)` / `fix(credential)`）分别是 T14 / T16 / T17** ——
它们是 M2 剩下的全部，也是 v0.2.0 能不能发的关键（能力 A / D / F 都在这三条上）。

---

## 相关文档

| 文档 | 内容 |
|---|---|
| [`docs/release-plan-v0.1.0.md`](./release-plan-v0.1.0.md) | v0.1.0 发版计划与落地状态（**当前发版**） |
| [`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md) | 发版前整体 review：6 个 CRITICAL + 29 个 REQUIRED + 20 个 NIT 的完整证据 |
| [`docs/errata.md`](./errata.md) | 勘误表：已发布版本里的错误陈述 |
| [`docs/techspec.md`](./techspec.md) | 当前实现的接口与流程口径 |
| [`docs/project-map.md`](./project-map.md) | 分层项目地图 / 上下文索引 |
