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
| **v0.2.0** | **路线图目标** —— 有真实零知识证明链的实验版。匿名性在密码学上**成立**，而不是被声称；但接口仍会变，**不承诺稳定** | M2 已完成 · M3 进行中（T22 已落地） | **27–39 人天 ≈ 6–8 周** |

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
| A | 选票内容对服务端不可知 | ❌ 密文由服务端生成，服务端在签发凭证时就知道选了哪个 | 服务端代码里**不存在**接收明文选项的字段（已从人工 grep 升级为持续门禁 `tests/architecture/`）；落库记录无法还原选项 | M2 · T16 |
| B | 选票可验证 | ❌ 无真实电路；`vk_path` 为空时任何非空 proof 都通过 | 真实 Groth16 电路 + 密钥生成流程；篡改任一公开输入 → 验证失败 | M2 · T13 |
| C | proof 与票绑定 | ❌ proof 可跨提案复用（服务端只读 `public_witness`） | 公开输入含 `(proposal_id, nullifier, 计票公钥, 密文点, 选项数)`；A 提案的 proof 投 B 提案 → 400 | M2 · T15 |
| D | 客户端能投票 | ❌ 门户在生产模式下 100% 无法完成投票（没有 prover） | 浏览器内生成 proof 与密文，e2e 跑通「选票 → 加密 → 证明 → 提交」 | M2 · T14 |
| E | 凭证不可伪造 | ❌ 投票路径**从不**验凭证签名 | 落库前 `VerifyCredential`；伪造签名的投票 → 401 | M2 · T18 |
| F | 签发方看不到被签内容 | ❌ 盲化在服务端进程内完成，等同普通签名 | 两段式盲签名：服务端在**签名阶段**只见盲化消息；交叉攻击（A 的盲签名 finalize 出 B）→ 失败 | M2 · T17 |
| G | 凭证分发不泄露身份 | ✅ **v0.1.0 已达成**（T9）：凭证池 + 投票人匿名随机领取，运营方不再掌握「谁领了哪张」 | 保持「池里只存未签发槽位、领取时用领取者自带的一次性公钥当场签发、领取端点不落可关联日志」；补**资格证明**把一人一票补回来 | M3 · T22 ✅ **已达成**（见 §4.1 的 M3 进展表） |
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

### 2.3 能力 G 引入的新问题：一人一票没了 ✅ 已补回（T22，2026-10-25）

T9 把凭证改成**匿名随机领取**后（能力 G），服务端**无法强制一人一票**。

缺口的确切来源：池里的槽位带的是**随机假名 `user_id`**，领到它的人是谁服务端不知道；
而服务端唯一能用来做「一人一票」约束的字段正是 `user_id`
（`vote_participations` 存 `hash(user_id)`）。既然 `user_id` 是随机假名，
服务端就看不出同一个人领了几次、投了几次 —— 任何人都能反复领取并投票。

这不是可以「顺手修掉」的小事：它是「不可关联性」与「一人一票」之间的直接取舍。
**要同时拿到两者，只能靠资格证明** —— 让投票人证明「我有资格投这一票」，
但证明本身不暴露身份（零知识证明「我符合资格」而不透露是谁）。这条路依赖
M2 的证明链能力，因此它属于 M3 而不是 M2。

> ✅ **T22 · 资格证明已落地（2026-10-25）**：权威名单 → 离线工具生成 Merkle 承诺 →
> **服务端只拿到 root** → 成员用零知识证明「我在名单里」→ 服务端按
> `(proposal_id, qualification_id)` 去重。**服务端能验证「这个资格在树里」，却无法把资格
> 对应到任何成员。** 设计与落地的口径见
> [`docs/t22-eligibility-design.md`](./t22-eligibility-design.md)（状态：已实现）。
>
> 配套小项也已落地：CLI 的 `credential-claim` 命令（公开端点、不需要 admin token）。

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

### M3 · 生产化 🟡 进行中

**目标**：能力 G 的资格证明（T22）、H 的生产配置（T19）、I 的端到端验收（T20），以及发版本身。

**出口标准**：能力 G 的完成定义拿出证据（资格证明跑通、重复领取被拒）；生产部署清单全勾；
三项盲区都有可复现输出；`VERSION` 与 tag 一致。

**状态**：**进行中**（2026-10-25 起）。已完成 T22（资格证明 —— 能力 G 的完成定义已达成）；
T19 已完成其可自动化的部分（CORS 的真实源仍是部署项）；T20 三项盲区各补了一部分，
**没有一项完全闭合**（见 §4.3）。T21（发版）**准备就绪**（发版计划已成文、流水线已带 prover、
`VERSION` 与 `CHANGELOG` 已改为 0.2.0），**推送分支 + 打 tag 等拍板**。

**依赖**：M2 全部完成（资格证明要用 M2 的证明链能力）。✅ M2 已全部完成。

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
| T1 | `ballot-draft` 在非 debug 模式 404 | ✅ 已落地 → **后续升级为「端点整个删除」**（2026-10-25） |
| T2 | 门户「按选项生成凭据」按钮非 debug 隐藏 | ✅ 已落地 → **后续升级为「按钮整个删除」**（2026-10-25） |
| T16 | 加密移到客户端 | ✅ **已落地**（浏览器端 TS 实现 + CLI 命令 + 跨语言互操作测试；门户已接上：`generateBallotLocally` 走 WASM prover，密文在浏览器本地生成） |
| T14 | 客户端 prover | ✅ **已落地（CLI + 浏览器两侧）** —— CLI 是 `ballot-prove`；浏览器侧是**自建的 Go → js/wasm 路线**：`cmd/ballot-wasm` 编译成 WASM，门户在 Worker 里加载（`web/src/lib/wasm-prover.ts` + `web/src/workers/ballot-prover.worker.ts`）。加密、nullifier 派生、公开输入拼装与 Groth16 证明全部在 Go 侧完成，客户端与服务端共用同一套代码。**实测**：初始化 7.6 秒、出证明 2.25 秒、证明 128 字节（`tests/e2e/wasm_prover_test.go` 用 node 驱动同一个 WASM 模块端到端验收） |
| T17 | 盲签名改真两段式 | ✅ **已落地**（2026-10-21）—— 真两段式协议（预留 → 本地盲化 → 盲签名 → 本地解盲 → attest）+ 门户 / CLI 接线 + 管理端同样走两段式；验收 ①（签名阶段只见盲化消息）与验收 ②（交叉攻击失败）均有 e2e 用例 |

**能力 A–G 的当前状态**：

| 能力 | 状态 | 说明 |
|---|---|---|
| A 选票内容对服务端不可知 | ✅ **已达成** | **选票密文在客户端生成**：CLI（`ballot-encrypt`）+ 浏览器（门户的「在浏览器本地生成选票」）。服务端只提供公开的计票公钥，接口 `GET /api/v1/proposals/:id/election-key`。落库记录里只有密文（e2e 用例断言）。**服务端没有任何接收明文选项的入口** —— `ballot-draft`（服务端代加密）已删除，这条不变量由 `tests/architecture/` 持续守卫（请求体字段 / usecase 入口 / 代加密链不许复活 / 端点不许复活，**不设豁免**） |
| B 选票可验证 | ✅ **已达成** | 真实 Groth16 电路 + `cmd/keygen` + 服务端真实验签；浏览器端 prover 已落地（`cmd/ballot-wasm` → WASM，门户 Worker 里跑），CLI 侧 `ballot-prove` 也已落地 |
| C proof 与票绑定 | ✅ 已达成 | 公开输入由服务端按请求字段自拼，客户端无法自选；跨提案 / 跨 nullifier 复用均验签失败 |
| D 客户端能投票 | ✅ **已达成（CLI 与门户两侧）** | CLI 用本地 proving key 出证明；门户在浏览器 Worker 里用 WASM prover 出证明并提交。两条路径都有端到端证据（`tests/e2e/client_prover_test.go` / `tests/e2e/wasm_prover_test.go`） |
| E 凭证不可伪造 | ✅ 已达成 | 落库前 `VerifyCredential`，伪造签名 → 401 |
| F 签发方看不到被签内容 | ✅ 已达成 | 真两段式盲签名：客户端本地盲化，服务端在**签名阶段**只见盲化消息；交叉攻击（A 的盲签名签 B）→ 验签失败。**口径**：attest 阶段服务端仍看得到 `user_id` / `user_pub_key`（验签必须重算凭证消息） |
| G 凭证分发不泄露身份 | ✅ **已达成（T22，2026-10-25）** | **服务端只见 root，不见成员包** —— 它能验证「这个资格在树里」，却无法把资格对应到任何成员。领取请求体里**不含任何真人身份字段**（e2e 断言）；投票提交的材料**不含** `qualification_id`（去重只在领取时做一次）。一人一票由 `eligibility_uses` 的唯一索引 `(proposal_id, qualification_id)` 强制。**未闭合**：门户的资格领取路径仍需真浏览器人工点一遍（仓库无 Playwright） |

**本轮的两个协议变更**（都要写进 CHANGELOG，详见 §6 决策 D2）：

- **选票密文 v2 → v3，与 v0.1.0 的密文不兼容** —— v0.1.0 的 ed25519 + HKDF
  群运算无法在 BN254 电路里复算，电路算术在标量域里，密文必须换到同一条曲线。
  旧的加密选票无法解密。
- **投票请求体不再接受 `public_witness`** —— 公开输入改由服务端自行拼装。

#### M3 进行中（2026-10-25 起，本轮）

| 任务 | 内容 | 状态 |
|---|---|---|
| T22 | 资格证明（一人一票） | ✅ **已落地** —— 见下面的「T22 落地核对」 |
| T19 | 生产配置落地 | 🟡 **部分落地** —— `CONFIG_FILE` 显式路径、缺文件启动失败、启动日志打印实际路径都已完成；CORS 的真实源仍是部署项 |
| T20 | 端到端验收（三个盲区） | 🟡 **三盲区各补了一部分，没有一项完全闭合** —— 见 §4.3（盲区 2 的手工流程已固化成 `scripts/browser-acceptance.sh`） |
| T21 | 发版（v0.2.0） | 🟡 **发版准备就绪** —— 发版计划已成文、流水线已带 prover、`VERSION` 与 `CHANGELOG` 已改为 0.2.0；⬜ **推送分支 + 打 tag + 发版等拍板**（见任务卡） |

**T22 落地核对**（一人一票）—— 落地了什么：

- **权威名单 → 离线工具 → 服务端只拿到 root**：`cmd/eligibility-list -members <名单文件> -out <目录>`
  生成 `root.json`（给服务端）与每成员一份 `eligibility-<member_id>.json`（0600，成员本人保管）。
  服务端**只拿到 root**：它能验证「这个资格在树里」，却无法把资格对应到任何成员。
- **资格电路**：Groth16 / BN254 / MiMC，**实测 6860 条约束**，公开输入**定长 5 个**
  （Root / ProposalID / Challenge / QualificationID / SessionBinding），叶子、Merkle 路径、位置**全私有**。
  `TreeDepth = 10` 是**编译期常量** → 单份名单上限 1024 人。
- **去重键** `QualificationID = MiMC(Leaf, ProposalID)`：同一成员同一提案稳定（第二次领撞唯一索引 → 409
  `ELIGIBILITY_ALREADY_USED`），跨提案不同（跨提案不可关联）。
- **协议三步**：`POST /api/v1/proposals/:id/credentials/eligibility-challenge` →
  `POST /api/v1/proposals/:id/credentials/claim`（请求体多了
  `{eligibility_proof, qualification_id, challenge}`，三件要么都给要么都不给）→
  `blind-sign` / `attest` **完全不变**。**资格不过不消耗池槽位。**
- **新错误码**：`ELIGIBILITY_REQUIRED`(400) / `INVALID_ELIGIBILITY_PROOF`(403) /
  `ELIGIBILITY_ALREADY_USED`(409) / `ELIGIBILITY_NOT_ENABLED`(409)。
- **数据库**：新表 `eligibility_uses`（`proposal_id` + `qualification_id` + `used_at`），
  唯一索引 `(proposal_id, qualification_id)` —— 这就是一人一票的强制点。
  **只存去重键**，不存叶子 / 秘密 / 成员 ID。
- **运行期要求**：`crypto.eligibility_root` + `crypto.eligibility_vk_path`
  （环境变量 `CRYPTO_ELIGIBILITY_ROOT` / `CRYPTO_ELIGIBILITY_VK_PATH`）**必须成对出现**；
  **release 模式必须配这两个，否则服务启动失败**。`GET /healthz` 多了 `eligibility` 布尔字段，
  启动日志的 `effective configuration` 多了 `eligibility_enforced` 字段。
- **命令行**：`cmd/eligibility-list`（离线名单工具）；`cmd/keygen -circuit ballot|eligibility`
  （两套电路的密钥互不相通，默认文件名 `verifying.key`/`proving.key` 与
  `eligibility-verifying.key`/`eligibility-proving.key`）；
  `cmd/cli credential-claim -eligibility <成员包.json> -proving-key <资格 proving key>`。
- **浏览器端**：`cmd/ballot-wasm` 暴露**四个 API**：`init` / `prove` / `initEligibility` /
  `proveEligibility`（**同一个模块**）；静态资源新增 `web/public/eligibility-proving.key`
  （`make web-eligibility-key PK=<路径>`）；门户「身份」面板：导入资格文件 → 领凭证时本地出证明，
  资格包**只留内存**，不写 localStorage、不上传。

**T22 的验收 ①–⑤ 各自的证据位置**：

| # | 验收 | 证据 |
|---|---|---|
| ① | 无资格证明 → 拒绝（4xx），**池槽位不被消耗** | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityRequiredForClaim_E2E` |
| ② | 有效资格证明 → 领取成功 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityClaimSucceeds_E2E` |
| ③ | 同一份资格第二次领取 → 拒绝 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityCannotBeReused_E2E` |
| ④ | 资格证明跨提案不可复用 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityNotReusableAcrossProposals_E2E` |
| ⑤ | 领取请求体里不含任何真人身份字段 | `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityClaimBodyHasNoIdentityFields_E2E` |

**浏览器路径的跨语言证据**：`tests/e2e/wasm_eligibility_test.go` 的
`TestAPI_WASMEligibilityProver_E2E` —— 用 node 驱动**真的 WASM 模块**出资格证明，
服务端用**真的资格验证密钥**验过，去重键与 Go 侧自算的一致。

**真浏览器里的资格路径验收（2026-10-25，Chromium / Electron）** —— 走完了整条链路：

| 步 | 期望 | 实测 |
|---|---|---|
| 导入资格文件 | 提示「资格文件已导入」，只留在内存 | ✅ `eligibility-m-0001.json` |
| 匿名领取 | 取挑战 → 浏览器本地出资格证明 → claim / blind-sign / attest 全过 | ✅ claim **201**、attest **201**（服务端日志 `credential attested`） |
| **重复领取（同一资格）** | **409 `ELIGIBILITY_ALREADY_USED`** | ✅ 门户提示「**这份资格已经领过票了 —— 一人一票**」 |
| 换另一个提案再领 | 能领（去重键按提案派生） | ✅ claim **201** |
| 浏览器本地生成选票 | 密文 + nullifier + 证明都在本地生成 | ✅ `selected_option: beta` |
| 提交选票 | 服务端只拿到密文 | ✅ **201**（服务端日志 `vote submitted`） |
| 计票 | 门限解密还原出浏览器端选的选项 | ✅ `{"tally":{"beta":1},"total_votes":1}` |

> ⚠️ **这次验收挖出一个只有真浏览器能发现的 bug**：资格包存在 Vue 的 `ref` 里，读出来是
> 响应式 Proxy，而 `postMessage` 的结构化克隆克隆不了 Proxy —— 浏览器抛
> `DataCloneError: #<Object> could not be cloned.`，**「匿名领取」在真浏览器里必然失败**。
> node 驱动的 WASM 用例不走 Worker，所以完全测不出来。已修（`fix(web): send a plain
> member package to the prover worker`），并补了一条回归用例钉住它。

> ✅ **同一套流程已固化成脚本，并在真浏览器里再跑了一遍**（2026-10-25，
> `scripts/browser-acceptance.sh`）：`setup` 备好环境并逐条打印人工步骤 → 在浏览器里点完 →
> `verify` 自动核对服务端日志里的 `credential attested` / `vote submitted` /
> `eligibility already used`，等窗口关闭后发布结果，核对**计票还原出的就是浏览器端选的
> 那个选项**（`{"tally":{"beta":1},"total_votes":1}`）。脚本见 §4.3 盲区 2。

**另有单测**：usecase（`internal/usecase/registration_eligibility_test.go`）、
persistence（`internal/infrastructure/persistence/eligibility_use_repository_test.go`）、
http（`internal/infrastructure/http/eligibility_endpoints_test.go`）、
crypto（`internal/infrastructure/crypto/eligibility/`）、CLI（`cmd/cli/eligibility_claim_test.go`）、
前端 composable（`web/src/composables/useVotePortal.test.ts`）。

**跨语言线路的守卫**（`tests/architecture/eligibility_wire_test.go`）：门户这一侧只做 JSON 解析与
转发，**最容易出错的不是数学而是字符串** —— 成员包字段名、出证明请求的字段名、WASM API 名、
静态资源名，两边各钉一条断言（Go 侧读 struct tag / `api.Set` 的键，TS 侧读接口字段 / 调用名）。

**T22 仍然没有闭合的**：

- **浏览器端没有自动化**：仓库里没有 Playwright。门户的资格领取路径**已在真浏览器里人工
  验过一遍**（2026-10-25，见上面的验收表），但**每次上线仍要人工点一遍**
  （步骤见 `docs/deployment-checklist.md` §8.1）—— 自动化证据只到 node 驱动的 WASM 模块。
- **PG 集成测试本机无 Docker**（与 T22 无关，但整体验收仍缺）—— ✅ 2026-10-25 已用本机的
  便携版 PostgreSQL 17.7 起临时集群实跑通过（外部库路径，见 §4.3 盲区 1）；**testcontainers
  那条路径仍要 CI 实跑**。
- **`cmd/loadtest` 不支持资格证明**：它只在**未开启资格校验**的服务上可用。
- **资格包丢失无法找回**（成员包是秘密材料，发放靠安全渠道）。
- **真实名单的内容与成员标识形式仍需运营方提供**。

### 4.2 v0.1.0 的边界（v0.1.0 发布时的快照）

> 以下是 **v0.1.0 发布时**的边界，属历史快照。**当前实现见 §4.1 的 M2 / M3 进展表**
> —— 其中「没有真实电路」已改变；「`ballot-draft` 生产可达」这条**已彻底消失**：
> 该端点连同服务端的代加密链一起删掉了（2026-10-25），现在服务端没有任何
> 接收明文选项的入口；「服务端无法强制一人一票」这条也**已改变** ——
> T22 的资格证明已落地（2026-10-25，见 §2.3 与 §4.1 的 M3 进展表）。

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
   - ✅ **已在本机真 PostgreSQL 上实跑**（2026-10-25）：本机有一份便携版 PostgreSQL
     （`C:\Users\yimo\Tools\pgsql`，PG 17.7），用它的二进制 `initdb` 起了一个**临时独立集群**
     （trust 认证、独立端口、独立数据目录，不碰机器上原有的实例），然后走集成测试的
     **外部库路径**（`ANONVOTE_TEST_PG_HOST`）跑：`go test ./tests/integration/ -run Postgres`
     **全过** —— 含 `TestEligibilityUseRepository_Postgres`（复合唯一索引的行为证明）、
     `TestEligibilityUseRepository_Postgres_ConcurrentSameQualification`（并发记同一份资格
     恰好一个成功）、`TestVoteRepository_Postgres_ConcurrentSameUser`。
     ⚠️ **Docker 仍然没有**，所以「testcontainers 路径」仍要 CI 实跑 —— 但两者是**同一份测试
     代码**，断言一致（`setupPostgresDB` 按环境变量二选一）。
   - ✅ **连整条浏览器验收都在 PG 上跑通了一遍**（2026-10-25）：`scripts/browser-acceptance.sh`
     新增 `DB_DRIVER=postgres` 后，服务端连的是**生产库形态**（PG + 真 verifying key +
     资格校验强制），真浏览器里导入资格文件 → 匿名领取（claim 201）→ 重复领取
     409「这份资格已经领过票了 —— 一人一票」→ 浏览器本地生成选票 → 提交 201 →
     计票还原出所选选项（`{"tally":{"beta":1},"total_votes":1}`，并带 anchor_hash 与
     merkle_root）。**这是「生产用的就是 PostgreSQL」这条最硬的证据** ——
     此前所有浏览器证据都跑在 SQLite 上。
     ⚠️ 复现前提：机器上要有 PostgreSQL 二进制（本次用的是 `C:\Users\yimo\Tools\pgsql`
     的便携版 17.7，`initdb` 起临时集群 + 外部库路径）。
   - 🟡 **已补可复现脚本**（本轮落地）：仓库根 `docker-compose.yml` + `make pg-test`；
     集成测试也支持外部库模式（`ANONVOTE_TEST_PG_HOST`），每个测试开独立 schema。
     CI 仍会实跑（CI 的 go job 跑在 ubuntu-latest，自带 Docker；缺 Docker 时集成测试
     **失败而不是跳过**，免得「跳过」被误读成「通过」）。
   - ✅ **「一人一票」的强制点也纳入 PG 覆盖**（`tests/integration/eligibility_postgres_test.go`）：
     去重靠的是 AutoMigrate 建出来的**复合唯一索引** `(proposal_id, qualification_id)`，
     而 SQLite 与 PG 的 DDL 行为不同 —— 索引没建出来或建成单列，生产上就会变成
     「同一个人能领两次票」且**没有任何报错**。测试用行为证明索引存在：同键第二次写必须
     失败（409 的落地点）、换提案必须仍能写（约束是复合的）、同一提案下另一份资格也必须
     能写（约束不是单列 `proposal_id`），并发同时记同一份资格必须**恰好一个**成功。
     ⚠️ 本机无 Docker，这两条要等 CI 实跑才算证据。
2. **门户 7 个 `.vue` 面板的浏览器实际渲染** —— 仓库里 `.vue` 曾零测试
   - 🟡 **已用服务端渲染补上一半**（本轮落地）：`web/src/components/panels/panels.test.ts`
     （16 个用例）把面板渲染成 HTML 字符串，模板 / setup / computed 都真跑一遍。
     ⚠️ **不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。
   - ✅ **浏览器端的关键路径已在真实浏览器里跑通**（2026-10-25，Chromium / Electron）：
     门户的静态资源三件套（`wasm_exec.js` / `ballot-prover.wasm` / `proving.key`）从浏览器
     真实加载 → WASM 出证明（init 11.8~14.3 秒、prove 2.9 秒）→ 凭证匿名领取（claim →
     blind-sign → attest 三步全过）→ 服务端以真 verifying key 验签通过（201）→ 提案关闭后
     解密计票，还原出浏览器端选的那个选项（`{"tally":{"candidate-B":1}}`）。
     ⚠️ **闭合的是「WASM 路径在真实浏览器里可用」，不是「门户有 UI 自动化回归」** ——
     仓库仍然没有 Playwright 之类的浏览器自动化，面板交互仍靠人工核对。
   - 这次真实浏览器验收还挖出一个**只靠 node 测不出来的跨语言 bug**：门户把盲签名变体名
     写成了 Go 的枚举标识符，服务端发的是 CIRCL 的线路名 —— 领取流程会停在 claim 之后
     （blind-sign 从未发出）。已修（`fix(web): use the CIRCL wire name for the blind
     signature variant`），并加了 Go/TS 两侧的跨语言锁定。
   - ✅ **T22 的资格领取路径已在真浏览器里验过**（2026-10-25，Chromium / Electron）：导入资格
     文件 → 匿名领取（claim 201）→ **重复领取 409「这份资格已经领过票了 —— 一人一票」** →
     浏览器本地生成选票 → 提交 201 → 计票还原出所选选项。⚠️ 仓库仍**没有**浏览器自动化
     （证据只到 node 驱动的 WASM 模块），**每次上线仍要人工点一遍**
     （步骤见 `docs/deployment-checklist.md` §8.1）。
   - 这次验收还挖出**第二个只靠 node 测不出来的 bug**：资格包在门户里是 Vue 的响应式 Proxy，
     `postMessage` 克隆不了它 —— 「匿名领取」在真浏览器里必然失败。已修
     （`fix(web): send a plain member package to the prover worker`），并补了回归用例。
   - ✅ **整套手工流程已固化成脚本**（本轮落地）：`scripts/browser-acceptance.sh` 分三段 ——
     `setup` 备好环境（两套密钥 → 资格名单 → 提案 → 凭证池 → 门限解密 → 门户 dev server）
     并**逐条打印人工步骤**；`verify` 核对服务端日志里的三件事（`credential attested` /
     `vote submitted` / `eligibility already used`），等窗口关闭后发布结果并核对
     **计票还原出的就是浏览器端选的那个选项**；`stop` 收尾。
     ⚠️ **闭合的是「每次上线不用从零手搓」，不是「浏览器自动化」** —— 中间那一下
     导入资格文件与点击仍然要人来做，仓库里依然没有 Playwright。
3. **并发投票下的防重投与性能容量** —— 唯一索引兜底，但没测过
   - 🟡 **防重投这一半已补**（本轮落地）：`tests/e2e/concurrent_nullifier_test.go` ——
     8 个请求**同时**用同一个 nullifier、各自带不同凭证提交，断言恰好 1 张票入库、
     其余 7 个被唯一索引拦成 409 `DOUBLE_VOTING`。
   - ✅ **容量这一半已补**（本轮落地）：新工具 `cmd/loadtest` —— 纯 HTTP API 的并发投票压测，
     输出吞吐 / 延迟分位 / 状态分布 / 防重投结论，**防重投没守住时以非 0 退出**（结论能当门禁用）。
     本机实测（SQLite + WAL + `_busy_timeout`，200 票 / 并发 20）：**164.1 req/s**，
     p50=3.56ms p90=325.6ms p99=1.107s，200 张全 201；同 nullifier 并发 5 次恰好放行 1 张、
     其余 4 张 409 `DOUBLE_VOTING`；落库核对 201 == 预期 201。
     ⚠️ 这组数字是 **2026-10-25 重测的**：旧的那组（257 req/s）是 v0.1.0 时期、
     凭证签发还是「一步式」时测的 —— 协议改成真两段式盲签名后压测工具一度**跑不通**
     （每张票 400 `INVALID_REQUEST`、0 票入库），现已修复并按新协议重测（修复记录见
     本文附录的 T20 任务卡）。
     ⚠️ 这是**可复现脚本 + 一份实测输出**，不是容量规划：SQLite 的单写者把并发写串行化，
     且**压测工具自己没有 prover**（只能在未加载 `crypto.vk_path` 的 debug 服务上跑）——
     容量规划按 §2.2 属 v0.3.0。

---

## 5. go / no-go（v0.2.0）

**No-Go（任一命中则不发 v0.2.0）**

> 右列是 **2026-10-21 的当前状态** —— 不是勾选结果，是「这条现在过不过」。

| # | 检查项 | 当前状态 |
|---|---|---|
| 1 | 能力 A–F 任一未完成 | ✅ **已过** —— A / B / D 已在 CLI 与门户两侧闭合（浏览器端 prover 走自建的 Go → js/wasm 路线）；C / E / F 已达成。能力 A 不再有残留：服务端的代加密链（`ballot-draft`）已删除 |
| 2 | 能力 G 的完成定义未达成 —— 领取凭证时不需要任何资格证明（T22），意味着服务端无法强制一人一票 | ✅ **已过** —— **T22 已落地（2026-10-25）**：领取凭证必须先过零知识资格证明（`eligibility-challenge` → `claim` 带 `{eligibility_proof, qualification_id, challenge}`），**服务端只见 root**、无法把资格对应到任何成员；同一份资格第二次领取撞 `eligibility_uses` 的唯一索引 → 409 `ELIGIBILITY_ALREADY_USED`；跨提案不可复用。验收 ①–⑤ 各有 e2e 证据（`tests/e2e/eligibility_test.go`），浏览器路径的跨语言证据见 `tests/e2e/wasm_eligibility_test.go`（node）与 2026-10-25 的**真浏览器人工验收**（见 §4.1 的验收表）。**部署侧的硬要求**：release 模式必须配 `crypto.eligibility_root` + `crypto.eligibility_vk_path`，否则服务启动失败 |
| 3 | 存在「服务端能拿到明文选项」的代码路径（grep 不到才过） | ✅ **已过** —— 这条检查从人工 grep 变成持续门禁 `tests/architecture/plaintext_option_test.go`，而且**不设豁免机制**：HTTP 层请求体不得有明文选项 json tag、usecase 层不得有接受明文选项的入口、服务端代加密那条链的符号不许复活、`ballot-draft` 端点不许复活（POST 它必须 404）。**残留为零** |
| 4 | 存在「proof 可以跨提案复用」的路径 | ✅ **已过** —— 公开输入由服务端自拼，跨提案 / 跨 nullifier 复用均验签失败 |
| 5 | `go build` / `go vet` / `gofmt -l` / `go test ./...` 任一失败 | ✅ **已过** —— 本轮全绿 |
| 6 | 覆盖率门槛退出码非 0 | ✅ **已过** —— 逐包分档，合计 80.80% |
| 7 | 前端 lint / tsc / vitest 任一失败 | ✅ **已过** —— 19 个测试文件 / 156 个用例全过 |
| 8 | §4.3 三项盲区没有可复现输出 | 🟡 **已过两半，剩门户 UI 自动化**（盲区 3 的「防重投」与「容量」两半已补；盲区 1 ✅ **PG 集成测试已在本机真 PostgreSQL 17.7 上实跑通过**（外部库路径，`go test ./tests/integration/ -run Postgres` 全过），**整条浏览器验收也在 PG 上跑通**（`DB_DRIVER=postgres`，计票还原出所选选项）—— 剩 testcontainers 那条路径要 CI 实跑；盲区 2 ✅ 已闭合「WASM 路径在真实浏览器里可用」这一半 —— **选票与资格两条浏览器路径都已在真浏览器里人工验过**（2026-10-25），**且整套手工流程已固化成 `scripts/browser-acceptance.sh`**（`setup` / `verify` / `stop`），⚠️ 但门户面板仍没有 UI 自动化回归） |
| 9 | README / techspec / 门户三处的匿名性声明不一致 | ✅ **已过**（2026-10-25 复核）—— 三处与 §4.1 的能力表对齐。复核时**在门户里挖出一处对用户说错的陈述**：门户「术语参考」面板仍写着「**当前版本密文由服务端生成，生成时服务端能看到你选的选项**」「还没有真实的选票电路」—— 那是 v0.1.0 的口径，v0.2.0 的密文与证明都在浏览器本地生成。已改（`fix(web): tell the truth about where the ballot is encrypted`），并加了一条守卫用例钉住它（`web/src/components/panels/panels.test.ts`）：文案里再出现「服务端能看到你选的选项」这类说法就红。**服务端那条底线由 `tests/architecture/` 守，门户文案这一半现在也守了** |

**Go**

- [ ] 上述全过
- [ ] 决策门 G1–G9 全部拍板并落到代码/文档 —— **G1 / G2 / G3 / G4 / G5 / G7 / G8 / G9 已拍板**；G6 仍待拍板
- [ ] `VERSION` 与 tag 一致 —— **`VERSION` 已改为 `0.2.0`**（2026-10-25），tag `v0.2.0` 未打（等拍板）；release workflow 会校验两者一致，不一致直接失败
- [ ] 生产部署清单全勾：[`docs/deployment-checklist.md`](./deployment-checklist.md)（`SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS` / `CRYPTO_VK_PATH` / `DB_DRIVER=postgres` / TLS / 门户独立部署 / 迁移步骤 / 密钥就位 / 启动日志自检）

---

## 6. 决策记录

**已拍板（2026-10-20）**

| # | 决策 | 结论 |
|---|---|---|
| **G1** | 版本定位 | ✅ **v0.1.0 = 已发布的内测版**；**v0.2.0 = 产品路线图目标**，不是「生产可用版」的承诺 —— 接口仍会变 |
| **G2** | 门户「按选项生成凭据」按钮 | ✅ **整个删掉**（2026-10-25）—— 它把明文选项发给服务端，正式模式与 debug 模式都没有这个入口 |
| **G3** | `ballot-draft` 生产可达性 | ✅ **端点整个删掉**（2026-10-25）—— 不只是「非 debug 404」；服务端的代加密链一起删除 |
| **G4** | 盲签名：改实现还是改文档 | ✅ **先改文档如实描述**（T12），两段式（T17）留到 M2 一起做 —— v0.2.0 承诺匿名性时必须补上 |
| **G5** | 版本号 | ✅ 当前发版 `0.1.0`；路线图目标用 **`0.2.0`**，**不用 `1.0.0`**（1.0.0 意味着 API 稳定 + 生产可用，届时做不到；见 §1.1） |
| **G7** | 电路形状与曲线 | ✅ **Groth16/BN254**（T13 已落地）—— 沿用 verifier 原本硬编码的 `ecc.BN254` |
| **G8** | 投票人领凭证的形态 | ✅ **凭证池 + 投票人匿名随机领取**（T9）—— 不再用一次性领票码。核心理由：admin 逐个分发就等于 admin 知道哪个票是谁的，直接破坏不可关联性。**代价已记下**：一人一票要靠资格证明补回（T22） |
| **范围** | 这轮做哪些 | ✅ **T3–T12 这 10 项**已全部落地；T1（`ballot-draft`）与 T2（门户隐藏按钮）后来也落地了，并进一步升级为「端点与按钮整个删除」；M2 的 T13–T18 已落地；M3 的 **T22 已落地**（2026-10-25），T19 / T20 部分落地，T21 未开始 |

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

**T22 的决策点（2026-10-25，已全部拍板并落地）**

> T22 的六条决策点（范围、名单来源、哈希原语、提交端点、树深度、名单复用）**记在设计稿里** ——
> [`docs/t22-eligibility-design.md`](./t22-eligibility-design.md) §6「决策点（✅ 已拍板）」，
> 每条都有「拍板结论 + 落地口径」两列。口径摘要：

| # | 决策 | 落地口径 |
|---|---|---|
| **T22-D1** | 方案 B 还是方案 A | **方案 B**（完整方案，已落地）—— 方案 A 未实现 |
| **T22-D2** | 名单来源与保管 | 离线工具生成，**root 与成员包分开保管**；成员标识的具体形式仍由运营方决定 |
| **T22-D3** | 哈希原语 | **MiMC**（零新依赖） |
| **T22-D4** | 提交端点 | **claim 之前**（新增 `eligibility-challenge` 端点 + claim 请求体加三件材料） |
| **T22-D5** | 树深度与规模 | **depth=10，且是编译期常量**（不是配置项）→ 单份名单上限 1024 人 |
| **T22-D6** | 名单能否跨提案复用 | **能**（去重键按提案派生 → 跨提案不可关联） |

**待拍板**

> G6 仍待拍板。G2 / G3 已在本轮拍板并落地，不再卡住 M2；**G9 已在本轮拍板并落地**（见下）。

| # | 决策 | 选项 | 建议 | 卡住什么 |
|---|---|---|---|---|
| G6 | 是否换纯 Go sqlite 驱动 | 保留 cgo（当前）/ 换 `modernc.org/sqlite` | **保留 cgo**：换驱动会波及持久化层方言差异测试，收益只是「静态链接二进制」，不值 | 发布形态 |
| G9 | CLI 是否需要本地 prover | CLI 也做本地 prover / CLI 只做「带 proof 提交」 | ✅ **已拍板：两个都要**（2026-10-21）—— 原建议是「CLI 只做带 proof 提交，生成 proof 放浏览器端」，那条建议的前提是浏览器端 prover 可行。**实测离线环境装不了 `gnark-wasm`**（模块缓存里没有）。**落地**：CLI 新增 `ballot-prove`；**浏览器侧最终也做成了**（2026-10-21，见下） | T14 |

> **G9 的后续补充（2026-10-21）**：浏览器端 prover 的落地路线**不是**原设想里的
> 「gnark 官方 wasm 模板」，而是**把仓库自己的 Go 侧电路编译成 WASM**
> （`cmd/ballot-wasm`，`GOOS=js GOARCH=wasm`）。理由：离线环境装不上
> `gnark-wasm` / `snarkjs` / `@noble/curves`，而仓库里已经有**同一套电路**的 Go 实现
> —— 直接编成 WASM 复用它，还顺带保证「客户端与服务端拼出的公开输入逐字段相同」。
> 实测：初始化 7.6 秒、出证明 2.25 秒、证明 128 字节。代价：模块约 11.9 MB
> （gzip 后约 2.8 MB），首次出票要下载它与 1.2 MB 的 proving key。

> **[§1.2](#12-v020-之后还有几个-0x0) 的 v0.3.0 / v0.4.0 / 1.0.0 推演不属于决策门。**
> 它们是方向性推演：没有完成定义、没有出口标准、没有任务卡，也没经过 G1–G9 这样的拍板。
> M2 跑完一轮后按实际进展重估 —— 届时才谈是否把它们升格为正式里程碑。

---

## 7. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| T14 浏览器端 prover 未做，release 模式下门户无法投票 | v0.2.0 的能力 D 在**门户侧**不达成，发版价值受限 | ✅ **已解除** —— 浏览器端 prover 已落地（自建的 Go → js/wasm 路线，`cmd/ballot-wasm` + 门户 Worker）：初始化 7.6 秒、出证明 2.25 秒，node 驱动的端到端验收见 `tests/e2e/wasm_prover_test.go`。**剩余风险**：Worker 里的 WASM 加载路径**没有在真实浏览器里跑过**（仓库无浏览器自动化环境），首次出票要下载 2.8 MB 的 wasm 与 1.2 MB 的 proving key |
| T13 电路的证明生成太慢，浏览器端体验不可用 | 投票流程不可用 | ✅ **已实测**：约 7917 条约束，证明约 20ms、验证约 1–2ms，性能风险基本解除。必要时仍可换电路形状（公开输入更少）而不是硬扛 |
| T17 真盲签名改动面大，拖垮整个 v0.2.0 | 发版无限延期 | ✅ **已解除** —— T17 已于 2026-10-21 落地（协议 + 门户 / CLI 接线 + e2e 验收），改动面比预估小：领域接口拆成 `BlindSign` / `IssuerPublicKey` / `VerifyCredential` 三个方法，没有新增 schema 列 |
| T7 KDF 换协议使 v0.1.0 之前的旧密文永久不可解 | 旧数据丢失 | 明确标注密文格式版本；v0.1.0 起格式即 v2，v0.2.0 前不要用旧数据做真实投票 |
| **D2 密文 v2 → v3 使 v0.1.0 的选票永久不可解** | 旧数据丢失 | v0.2.0 尚未发版，本地/内测数据可弃；CHANGELOG 必须写明「与 v0.1.0 不兼容」；发版前清库 |
| ~~**debug 模式下服务端仍能拿到明文选项**~~ | 能力 A 未达成 | ✅ **已解除**（2026-10-25）：服务端代加密的端点与代码链**整个删除**，不再依赖 `server.mode` 的取值。能力 A 现在与运行模式无关 |
| T16 把加密移到客户端后，前端成为新的攻击面 | 密钥泄露 | proving key 只在前端，**不入服务端**；前端构建产物做完整性校验 |
| 匿名随机领取牺牲了一人一票 | 可重复投票 | ✅ **已解除** —— M3 用资格证明补回（T22，2026-10-25 已落地，见 §2.3 与 §4.1 的 M3 进展表）。**剩余风险**：真实名单的内容与成员标识形式仍需运营方提供；门户的资格领取路径仍需真浏览器人工点一遍（仓库无 Playwright） |
| M1 拖太久，M2 无时间 | 路线图整体延期 | M1 与 M2 **可并行**（除 T1/T2 依赖 M2 的字段） |

---

## 附录 A · 工程细项

路线图的判断以正文 §2–§4 为准；这里保留问题描述、证据与改法，供实现时直接取用。
**已落地的任务卡保留原始的问题与改法，并附「落地核对」行** —— 逐条说明实际怎么做的、
与原设想有什么偏差、哪些验收项没达成。当前已落地：**T1、T2、T13、T15、T18**（2026-10-21）
与 **T9**（见 A.4）、**T16 / T14 / T17**（M2 批次）、**T22**（2026-10-25，见 A.3）。
**其余任务卡尚未落地**（A.3 里的 T19 / T20 是部分落地，T21 未开始）。

### A.1 挂在 M1 但等决策的

#### T1 · `ballot-draft` 在非 debug 模式关闭（决策门 G3）✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | `POST /api/v1/proposals/:id/ballot-draft` 是**公开路由**，返回写死的假 proof。任何人都能调用，且这条链路的密文由服务端生成。 |
| 证据 | `internal/infrastructure/http/handler.go:65`（路由未挂在 admin 分组下）、`:408`（常量 `developmentProofBase64`，由 `PrepareBallotDraft` 在 `:442` 返回） |
| 改法 | ① `config.Config` 增加 `DebugMode bool`（由 `server.mode` 推导：`debug`/`dev`/`development` 为 true）；② 路由在非 debug 模式返回 **404**（用 404 而不是 403，避免暴露端点存在性）；③ `server_test.go` 里现有的 `ballot-draft` 用例按模式分叉，debug 断言 201、release 断言 404 |
| 落地核对 | ① ✅ `config.Config.DebugMode`（`internal/config/config.go` 的 `isDebugMode`，`server.mode` 为 `debug`/`dev`/`development` 时为 true）；② ✅ `Handler` 新增 `NewHandlerWithTallyAndMode`，非 debug 时 `PrepareBallotDraft` **直接 404**；③ ✅ `GET /healthz` 响应新增 `debug` 布尔字段，门户据此决定要不要显示调试入口；④ ✅ 新增用例 `TestHandler_BallotDraftHiddenOutsideDebugMode` 断言非 debug → 404；⑤ ✅ **2026-10-25 升级：端点整个删除** —— `PrepareBallotDraft` / `EncryptVoteOption` / `BallotDraft` 与门户的「按选项生成凭据」入口一并删除，「非 debug 才 404」这层门控不再需要。`config.DebugMode` 保留下来只作为运行状态（`/healthz` 的 `debug` 字段） |
| 验收 | `go test ./...` 全绿；新增用例断言 release 模式 404 —— ✅ 已达成；**升级后的验收**：`tests/architecture/` 断言 ballot-draft 端点任何模式都 404 —— ✅ 已达成 |
| 预估 | 1 小时 |
| 依赖 | G3 ✅ 已拍板（§6） |

#### T2 · 门户「按选项生成凭据」按钮在非 debug 模式隐藏（决策门 G2）✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | 门户在运营侧提供「按选项生成凭据」的入口，等于把「服务端替用户选」的能力做成一键操作。 |
| 证据 | `web/src/composables/useVotePortal.ts:977` `issueCredential`、`web/src/components/panels/IdentityPanel.vue:13` |
| 改法 | 按钮按 `debug` 模式条件渲染；非 debug 时给一句说明为什么没有这个入口 |
| 落地核对 | ① ✅ `useVotePortal.ts` 读 `/healthz` 的 `debug` 字段，暴露为 `debugMode` ref；② ✅ `VotePanel.vue` 的「按选项生成凭据」按钮加 `v-if="portal.debugMode"`，非 debug 时改为一句说明文案；③ ✅ **验收方式已在 T20 批次补上**：仓库仍无 `@vue/test-utils`（离线不可安装），但**自写了 SFC 编译插件**（`web/vitest.config.ts`，用 `vue/compiler-sfc` + `typescript`，不新增依赖），因此面板可以在 vitest 里做**服务端渲染测试** —— `web/src/components/panels/panels.test.ts` 断言：debug 模式渲染出「按选项生成凭据」按钮，**非 debug 模式渲染不出这个按钮**（断言的是按钮本身，不是说明文案里出现的那个词）。⚠️ 不是浏览器渲染（无 CSS / 无事件 / 无客户端挂载）；④ ✅ **2026-10-25 升级：按钮整个删除** —— 服务端代加密的端点也没了，按钮留着只会 404。`debugMode` ref 与 `readDebugFlag` 一并删除，门户不再读 `/healthz` 的 `debug` 字段；`panels.test.ts` 的断言改为「面板不渲染任何「让服务端代生成凭据」的入口」 |
| 验收 | vitest：非 debug 模式渲染不出该按钮；debug 模式仍在 —— ✅ **已达成**（`panels.test.ts` 的「投票面板的 debug 门控」三条用例）；**升级后的验收**：面板在任何情况下都渲染不出该入口 —— ✅ 已达成 |
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
| **落地核对（部分）** | ✅ **①②已完成**（本轮补齐，路线图 T14 的前置件）：浏览器端新增 `web/src/lib/{field,sha256,keccak,mimc}.ts` 与扩展后的 `web/src/lib/nullifier.ts` —— SHA-256 映射提案 ID、legacy Keccak-256 链派生 91 个 MiMC 轮常量、Fr 上的 MiMC（轮函数 `(x+h+c)^5` + Miyaguchi–Preneel），与 Go 侧 `ballot/nullifier.go` **逐位一致**；跨语言向量由 `internal/infrastructure/crypto/ballot/nullifier_interop_test.go` 与 `web/src/lib/{mimc,nullifier}.test.ts` 共同锁定，Go 侧还真的用这组值出了一份证明并验过（**浏览器算出的 nullifier，电路认**）。✅ **③已完成**（随 prover 一起改）：nullifier 改由 `ballot.ProveBallot` 在本地随机秘密后派生（`MiMC(s, proposal_id)`），秘密不出本地；门户表单里那个可手填的 nullifier 字段只作为低频工具保留。SHA-256 没用 WebCrypto：`crypto.subtle` 要求安全上下文，门户部署在内网 http 地址下会失效 |
| **浏览器侧补齐（2026-10-21）** | ✅ **浏览器端 prover 已落地**，但**路线与任务卡原文不同**：不是「gnark 官方 wasm 模板」（离线装不上 `gnark-wasm` / `snarkjs` / `@noble/curves`），而是**把仓库自己的 Go 侧电路编译成 WASM** —— `cmd/ballot-wasm`（`GOOS=js GOARCH=wasm`），在全局对象上注册 `nullifierProver`（`init` / `prove`）。门户侧：`web/src/workers/ballot-prover.worker.ts` 在 Worker 里加载 Go 官方胶水 `wasm_exec.js` 与 `ballot-prover.wasm`，主线程 `web/src/lib/wasm-prover.ts` 只做编排；`useVotePortal.ts` 的 `generateBallotLocally` 取计票公钥 → 调 prover → 把密文 / 证明 / nullifier 写进表单。**密码学全在 Go 侧**：加密、nullifier 派生、公开输入拼装（`ballot.BuildPublicInputs`）与出证明都在 `ballot.ProveBallot` 里一次做完，因此客户端与服务端拼出的公开输入逐字段相同。**实测**：初始化 7.6 秒（编译电路）、出证明 2.25 秒、证明 128 字节。**证据**：`tests/e2e/wasm_prover_test.go` —— node 加载**门户将要加载的那个 WASM 模块**，服务端配真 verifying key，改一个字节的证明必须 400 且不落库，计票还原出客户端选的选项。**代价**：模块约 11.9 MB（gzip 约 2.8 MB）+ proving key 1.2 MB，首次出票要下载；proving key 由运营方发布到门户静态目录（`make web-proving-key`），**不进服务端、不入库** |
| **CLI 侧补齐（2026-10-21）** | ✅ **CLI 现在能自己出证明**（G9 拍板为「两个都要」）：新增命令 `ballot-prove -ballot-file -proving-key [-out]` —— 读选票文件 → 用本地 proving key 生成 Groth16 证明 → 写回文件。**证明全程不经过服务端**，proving key 只在客户端（服务端只拿 verifying key）。选票文件因此多存四项本地见证材料：`nonce`（电路约束 `a = r·G` 要用）、`option_index`（私有见证）、`num_options`（公开输入之一）、`election_public_key`（未压缩计票公钥）—— 它们**都不提交给服务端**。公开输入走服务端同一套拼装入口 `ballot.BuildPublicInputs`，客户端与服务端算出同一份才验得过；计票公钥客户端拿到的是未压缩形式、服务端用压缩形式，客户端用新增的 `ballot.CompressPoint` 转成同一形式。**证据**：`tests/e2e/client_prover_test.go` —— 服务端配**真的 verifying key**，证明由客户端本地生成，改一个字节的证明必须 400 且不落库，服务端以**非 debug 模式**启动（`ballot-draft` 是 404）因此这张票只可能来自客户端 |
| **真实浏览器验收（2026-10-25）** | ✅ **浏览器端 prover 已在真实浏览器里跑通**（Chromium / Electron）：门户静态目录的三个文件（`wasm_exec.js` / `ballot-prover.wasm` / `proving.key`）由浏览器真实加载，初始化 11.8~14.3 秒、出证明 2.9 秒、证明 128 字节；产出的密文 / nullifier / proof 被服务端以**真 verifying key** 验签通过（201），提案关闭后解密计票还原出浏览器端选的那个选项（`{"tally":{"candidate-B":1}}`）。凭证的匿名领取也在同一浏览器里跑通（claim → blind-sign → attest 三步全过）。**这次验收挖出并修掉了一个跨语言 bug**：门户把盲签名变体名写成 Go 的枚举标识符（`SHA384PSSDeterministic`），而服务端发的是 CIRCL 的**线路名**（`RSABSSA-SHA384-PSS-Deterministic`）—— 领取流程会停在 claim 之后（blind-sign 从未发出）。这类字符串不一致在单测里测不出来（两边各拿自己的常量自证），只有真浏览器对真服务端才暴露；已加 Go/TS 两侧的跨语言锁定 |
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
| **状态补充** | 加密栈已在本轮换到 **v3**（D2）：浏览器端要做的是 BN254 twisted Edwards 上的指数 ElGamal，加密的是**选项下标**。服务端计票侧已就绪（`BallotThresholdCrypto`），**T16 缺的是浏览器侧的加密实现**。另外：服务端代加密这条路**已彻底关闭** —— `ballot-draft` 端点与 `EncryptVoteOption` / `PrepareBallotDraft` / `BallotDraft` 这条链在 2026-10-25 一起删除，「服务端代码里不存在接收明文选项的字段」这条验收**没有任何豁免** |
| 验收 | ① 服务端代码里**不存在**接收明文选项的字段（持续门禁 `tests/architecture/`，**不设豁免**：请求体字段 / usecase 入口 / 代加密链不许复活 / 端点不许复活）；② e2e：浏览器投票 → 服务端落库记录里没有可还原的选项 |
| 落地核对 | ① ✅ **浏览器端选票加密**：`web/src/lib/ballot-cipher.ts` 实现 BN254 twisted Edwards 上的指数 ElGamal（加密选项下标），点编码与 Go 侧**逐字节一致**（互操作向量见 `internal/infrastructure/crypto/ballot/kat_interop_test.go` 与 `web/src/lib/ballot-cipher.test.ts`）；② ✅ **CLI 客户端加密**：`cmd/cli` 新增 `ballot-encrypt` 命令，客户端取公钥 → 本地加密 → 落盘选票文件（含 nullifier 秘密，只留本地）；③ ✅ **服务端公开计票公钥**：新接口 `GET /api/v1/proposals/:id/election-key`（未压缩形式，客户端不必实现点解压）；④ ✅ **e2e 证据**：`tests/e2e/client_side_encryption_test.go` —— 客户端本地加密 → 落库记录里只有密文 → 计票仍还原出客户端选的选项；⑤ ✅ **门户已接上**（2026-10-25 真实浏览器跑通）：浏览器端 prover 走自建的 Go → js/wasm 路线（`cmd/ballot-wasm`），`web/src/lib/wasm-prover.ts` 主线程编排 + `web/src/workers/ballot-prover.worker.ts` 在 Worker 里加载 WASM —— 实测 init 11.8–14.3 秒、prove 2.9 秒、proof 128 字节，浏览器生成的密文 / nullifier / proof 提交后服务端真验签通过；⑥ ✅ **验收 ① 从「人工 grep」升级为「持续门禁」**：新增 `tests/architecture/plaintext_option_test.go`（go/ast 扫描 `internal/`）—— HTTP 层请求体结构体不得有明文选项 json tag、usecase 层不得有接受明文选项的入口、**服务端代加密那条链的符号不许复活**、**ballot-draft 端点不许复活**（POST 它任何模式都 404）。**不设豁免机制**；⑦ ✅ **2026-10-25：豁免的那一条也删掉了** —— `ballot-draft` 端点、`EncryptVoteOption` / `PrepareBallotDraft` / `BallotDraft`、门户的「按选项生成凭据」入口一并移除。能力 A 现在**与运行模式无关**：服务端没有任何接收明文选项的入口 |
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
| 落地核对 | ① ✅ **PG 集成测试已在本机真 PostgreSQL 上实跑**（2026-10-25：用便携版 PostgreSQL 17.7 的二进制 `initdb` 起临时独立集群，走集成测试的**外部库路径** `ANONVOTE_TEST_PG_HOST`，`go test ./tests/integration/ -run Postgres` 全过 —— 含「一人一票」复合唯一索引的行为证明与并发用例）；✅ **整条浏览器验收也在 PG 上跑通**（`scripts/browser-acceptance.sh` 新增 `DB_DRIVER=postgres`，服务端连真 PG，计票还原出所选选项）；⏳ **testcontainers 那条路径仍要 CI 实跑**（CI 的 go job 跑在 ubuntu-latest，自带 Docker；缺 Docker 时集成测试**失败而不是跳过**，免得「跳过」被误读成「通过」）；✅ **compose 已补**（仓库根 `docker-compose.yml` + `make pg-test`：起库 → 跑 `go test ./tests/integration/ -run Postgres` → 关库），且集成测试新增**外部库路径**（设 `ANONVOTE_TEST_PG_HOST` 即连已有 PostgreSQL，每个测试开一个独立 schema 并在跑完时 `DROP SCHEMA ... CASCADE`）—— 两条路径共用同一份测试代码，本地与 CI 跑的是同一套断言；② 🟡 **Playwright 离线装不了，但盲区已用另一种方式部分补上**：门户 7 个 `.vue` 面板此前**既无类型检查也无测试**（`tsc` 不认 `.vue`），现在有**服务端渲染测试**（`web/src/components/panels/panels.test.ts`，16 个用例）—— 模板、setup、computed 都会真跑一遍，顺带覆盖了 T2 的 debug 门控（非 debug 模式下按钮不渲染）。⚠️ **这不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载，视觉与交互仍靠人工核对；✅ **但浏览器端的关键路径已在真实浏览器里跑通**（2026-10-25）：静态资源三件套真实加载 → WASM 出证明（init 11.8~14.3 秒、prove 2.9 秒）→ 凭证匿名领取三步全过 → 服务端真验签通过（201）→ 解密计票还原出所选选项。这次验收还挖出并修掉了一个跨语言 bug（盲签名变体名用了 Go 的枚举名而非线路名，会让领取流程停在 claim 之后）—— 这类问题单测测不出来，只有真浏览器对真服务端才暴露；③ 🟡 **防重投这一半已落地**：`tests/e2e/concurrent_nullifier_test.go` 用 8 个并发请求 + 同一 nullifier + 不同凭证，断言恰好 1 张票入库、其余 409 `DOUBLE_VOTING`（本机可复现，`go test ./tests/e2e/ -run TestAPI_ConcurrentSameNullifier_E2E`）；**容量这一半也已落地**：新工具 `cmd/loadtest`（纯 HTTP API，不碰数据库）输出吞吐 / 延迟分位 / 状态分布 / 防重投结论，`-json` 可机器读，**防重投没守住时退出码非 0**；✅ **2026-10-25 修复**：工具此前还在用 v0.1.0 的「一步式领凭证」，协议改成真两段式盲签名后**每张票都 400 `INVALID_REQUEST`、0 票入库**（工具自己也会以非 0 退出）—— 现已改为走真实的 claim → blind-sign → attest 三步（与门户、CLI 同一条链路），并补了回归测试（假服务端按 v0.2.0 协议实现，断言**领取请求体里不许出现公钥**）；本机重测 200 票 / 并发 20 → **164.1 req/s**、200 张全 201、同 nullifier 并发 5 次恰好放行 1 张、落库核对 201 == 预期 201（⚠️ SQLite 单写者 + 工具没有 prover，只能在 debug 模式跑，**不是容量规划**） |
| 验收 | 三项都有**可复现的脚本 + 输出**，并把结果贴进 PR |
| 预估 | 2–3 天 |
| 依赖 | T14（浏览器端投票链路才完整） |

#### T21 · 发版（v0.2.0）

| 项 | 内容 |
|---|---|
| 动作 | ① `VERSION` 改 `0.2.0`；② `CHANGELOG.md` 写 0.2.0 条目 —— ⚠️ **本轮确实改了密文格式**（v2 → v3，见 §2.2 与 CHANGELOG 的破坏性变更），**必须写明与 v0.1.0 的密文不兼容、旧票要作废**；③ `git tag -a v0.2.0 -m "<发版说明>"`（**注解 tag** —— release workflow 用 tag 说明当 release 正文）→ release workflow 自动构建；④ 按生产部署清单逐项核对；⑤ 同步 `web/package.json` 的 version（单一版本源是仓库根的 `VERSION`，发布流水线构建前自动注入，无需手改） |
| **落地核对** | ✅ **发版计划已成文**：[`docs/release-plan-v0.2.0.md`](./release-plan-v0.2.0.md) —— 能力口径、破坏性变更、部署必做项、验收证据、go/no-go 清单；✅ **发布流水线的门户包补上了 prover**（`release.yml` 现在自己编 `ballot-prover.wasm` + 拷 `wasm_exec.js`，并**检查 `web/dist/` 里有没有 prover，缺了直接失败** —— v0.1.0 的门户由服务端代加密所以不需要它，v0.2.0 的门户在浏览器里出证明，**没有它就投不了票**）；✅ CI 的 web job 也补了同样的构建与检查（门户构建从此在每个 PR 上都被验证，不再拖到发版那天）；⚠️ **两把 proving key 默认不进发布包**（秘密材料，且每套部署必须用与服务端配对的那对）—— 要么配仓库 secret（`PORTAL_PROVING_KEY_B64` / `PORTAL_ELIGIBILITY_PROVING_KEY_B64`），要么部署时自己补进 `web/dist/`（部署清单 §1/§6 已写明）；✅ **动作 ① 与 ② 已完成**（2026-10-25）：`VERSION` 改为 `0.2.0`，`CHANGELOG.md` 的 `[未发布]` 改为 `## [0.2.0] — 2026-10-25` 并写明密文 v2 → v3 不兼容、旧票作废；⚠️ **勘误表同步**：`docs/errata.md` E1 与 CHANGELOG 0.1.0 段里那句「v0.1.0 与 v0.2.0 之间没有已知的密文格式不兼容」**已被 v0.2.0 证伪**，两处都补了带日期的再勘误（不改 v0.1.0 的 tag 与产物）；⬜ **推送分支 + 打 tag + 发版 —— 等用户拍板**（分支一直保持未推送） |
| 验收 | `gh release view v0.2.0` 有产物（5 平台二进制 + 门户包，各带 sha256）；生产部署清单全勾 |
| 预估 | 半天 |
| 依赖 | M1、M2 全部完成；M3 的 T19/T20/T22 已落地（T20 三个盲区本机可做的部分已闭合，PG 实跑等 CI） |

#### T22 · 资格证明：把一人一票补回来（T9 留下的缺口）✅ 已落地

| 项 | 内容 |
|---|---|
| 问题 | T9 把凭证改成匿名随机领取后，**服务端无法强制一人一票**。缺口的确切来源：池里槽位带的是随机假名 `user_id`，而服务端唯一能做「一人一票」约束的字段正是 `user_id`（`vote_participations` 存 `hash(user_id)`）—— 既然 `user_id` 是假名，服务端看不出同一个人领了几次、投了几次。**这等于用「可重复投票」换来了「不可关联性」。** |
| 证据 | `internal/usecase/registration.go:140`（`FillCredentialPool` 生成随机假名 `user_id`）、`:197`（`ClaimCredential` 用领取者自带公钥签发）、`internal/domain/entity/credential_claim.go` |
| 改法 | ① 定义**资格证明**：投票人在领取凭证时额外提交一份「我有资格投这一票」的零知识证明，证明内容不暴露身份（如基于一次性设备密钥的可验证资格 / 资格名单的 Merkle 承诺）；② 领取端点校验该证明，**不通过就不给凭证**（而不是给凭证后在投票时拒 —— 那时已经浪费了一个池槽位）；③ 电路的公开输入要绑定 `proposal_id` 与资格承诺，防跨提案复用；④ 服务端对「同一资格」只能领取一次（一次性资格标识，落库只存哈希）；⑤ ✅ **CLI `credential-claim` 命令已补**（本轮落地：公开端点、不需要 admin token、请求体只有本地生成的公钥） |
| **设计方案** | 📄 **已实现**（2026-10-25）：[`docs/t22-eligibility-design.md`](./t22-eligibility-design.md)（状态：已实现，含 §8「实现与设计稿的偏离」）。核心推论：**服务端只能对它能识别的标识去重** —— 标识若由客户端自由生成，「一人一票」就只是「一设备一票」；而权威方一旦知道「谁领了哪个资格」，就可能把它与票关联。解法是「权威分配资格 + 零知识地出示资格」：**离线名单（服务端只见 root）+ Merkle 承诺 + 零知识成员资格证明**，复用现有 MiMC / Groth16 / WASM prover 栈，不引入新密码学依赖。设计稿的 6 条决策点已全部拍板（见该文 §6） |
| **落地核对** | ① ✅ **离线名单工具** `cmd/eligibility-list -members <名单文件> -out <目录>` → `root.json`（服务端用）+ 每成员一份 `eligibility-<member_id>.json`（0600，成员本人保管）；名单按成员 ID 排序，同一份名单两次生成的树根一致；② ✅ **资格电路** `internal/infrastructure/crypto/eligibility/`：Groth16 / BN254 / MiMC，**实测 6860 条约束**，公开输入**定长 5 个**（Root / ProposalID / Challenge / QualificationID / SessionBinding），**叶子、Merkle 路径、位置全私有**；`TreeDepth = 10` 是**编译期常量** → 单份名单上限 1024 人；③ ✅ **服务端**：`POST /api/v1/proposals/:id/credentials/eligibility-challenge`（公开、**无状态**，服务端不记账）→ `claim` 请求体新增 `{eligibility_proof, qualification_id, challenge}`（三件要么都给要么都不给）；**资格校验在取槽位之前**，资格不过**不消耗池槽位**；`blind-sign` / `attest` **完全不变**；④ ✅ **去重表** `eligibility_uses`（`proposal_id` + `qualification_id` + `used_at`），唯一索引 `(proposal_id, qualification_id)` —— 一人一票的强制点；**只存去重键**，不存叶子 / 秘密 / 成员 ID；⑤ ✅ **新错误码**：`ELIGIBILITY_REQUIRED`(400) / `INVALID_ELIGIBILITY_PROOF`(403) / `ELIGIBILITY_ALREADY_USED`(409) / `ELIGIBILITY_NOT_ENABLED`(409)；⑥ ✅ **运行期配置** `crypto.eligibility_root` + `crypto.eligibility_vk_path`（环境变量 `CRYPTO_ELIGIBILITY_ROOT` / `CRYPTO_ELIGIBILITY_VK_PATH`），**必须成对出现**；**release 模式缺一即启动失败**；`GET /healthz` 新增 `eligibility` 布尔字段；⑦ ✅ **CLI**：`cmd/keygen -circuit ballot\|eligibility`（两套密钥互不相通，默认文件名 `verifying.key`/`proving.key` 与 `eligibility-verifying.key`/`eligibility-proving.key`）；`cmd/cli credential-claim -eligibility <成员包.json> -proving-key <资格 proving key>`；⑧ ✅ **浏览器端**：`cmd/ballot-wasm` 暴露**四个 API** `init` / `prove` / `initEligibility` / `proveEligibility`（**同一个模块**）；静态资源只新增 `web/public/eligibility-proving.key`（`make web-eligibility-key PK=<路径>`）；门户「身份」面板「导入资格文件 → 领凭证时本地出证明」，资格包**只留内存**，不写 localStorage、不上传 |
| **偏差** | ⚠️ 与设计稿原文有 5 处偏离，逐条列在 [`docs/t22-eligibility-design.md`](./t22-eligibility-design.md) §8：**① 浏览器端是同一个 WASM 模块四个 API，不是两个模块**（静态资源只多一个 key）；**② `qualification_id` 存 64 位 hex 的 TEXT，不是 BLOB(32)**（与 `votes.nullifier` 的口径一致）；**③ 公开输入 5 个，不是 7 个** —— 叶子 / 路径 / 位置全私有，多出来的 `SessionBinding` 由服务端自算；**④ 挑战无状态**（与设计稿一致）；**⑤ D1–D6 已全部拍板**，其中 D5 的 depth=10 是**编译期常量而非配置项** |
| 验收 | ① 无资格证明的领取请求 → 拒绝（4xx），池槽位不被消耗 —— ✅ `tests/e2e/eligibility_test.go` 的 `TestAPI_EligibilityRequiredForClaim_E2E`；② 有效资格证明 → 领取成功 —— ✅ `TestAPI_EligibilityClaimSucceeds_E2E`；③ 同一份资格证明第二次领取 → 拒绝 —— ✅ `TestAPI_EligibilityCannotBeReused_E2E`；④ 资格证明在不同提案间不可复用 —— ✅ `TestAPI_EligibilityNotReusableAcrossProposals_E2E`；⑤ 领取请求体里不含任何真人身份字段 —— ✅ `TestAPI_EligibilityClaimBodyHasNoIdentityFields_E2E`。**浏览器路径的跨语言证据**：`tests/e2e/wasm_eligibility_test.go` 的 `TestAPI_WASMEligibilityProver_E2E`（node 驱动**真的 WASM 模块**出证明，服务端用**真的资格验证密钥**验过，去重键与 Go 侧自算一致） |
| **未达成** | ⚠️ **门户的浏览器端路径没有自动化**（仓库无 Playwright）—— 资格领取这条路仍需真浏览器人工点一遍（步骤见 `docs/deployment-checklist.md`）；⚠️ `cmd/loadtest` **不支持**资格证明（只在未开启资格校验的服务上可用）；⚠️ 资格包丢失**无法找回**（成员包是秘密材料，发放靠安全渠道）；⚠️ **真实名单的内容与成员标识形式仍需运营方提供**；⚠️ 单份名单上限 1024 人（`TreeDepth` 是编译期常量） |
| 预估 | 5–7 天（依赖 M2 的电路与 prover） |
| 依赖 | T13、T14（资格证明要用真实电路与客户端 prover）✅ 均已落地 |
| 备注 | **这是能力 G 的完成定义的最后一环**（§2.1）—— ✅ 已闭合。不做的话，v0.2.0 就必须继续在 README 里写明「无法保证一人一票」 |

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
两项都归到 **T22**。✅ **T22 已于 2026-10-25 落地**（见 A.3），两项缺口都已闭合。

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
**这三条后来也落地了**（T16 / T14 / T17）。

**T22（2026-10-25）实际涉及的提交**：

```
feat(crypto): enforce one-person-one-vote with a zero-knowledge eligibility proof
feat(cli,web): let the CLI and the portal prove eligibility before claiming
```

第一条是资格电路 + 服务端 + 去重表 + 配置，第二条是 CLI 与门户两条客户端路径的接线。

---

## 相关文档

| 文档 | 内容 |
|---|---|
| [`docs/release-plan-v0.2.0.md`](./release-plan-v0.2.0.md) | v0.2.0 发版计划与落地状态（**当前发版**：能力口径、破坏性变更、部署必做项、验收证据、go/no-go 清单） |
| [`docs/release-plan-v0.1.0.md`](./release-plan-v0.1.0.md) | v0.1.0 发版计划与落地状态（上一版） |
| [`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md) | 发版前整体 review：6 个 CRITICAL + 29 个 REQUIRED + 20 个 NIT 的完整证据 |
| [`docs/errata.md`](./errata.md) | 勘误表：已发布版本里的错误陈述 |
| [`docs/techspec.md`](./techspec.md) | 当前实现的接口与流程口径 |
| [`docs/project-map.md`](./project-map.md) | 分层项目地图 / 上下文索引 |
