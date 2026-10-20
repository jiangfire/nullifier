# nullifier 产品路线图

> **这是产品路线图，不是发版计划。**
> 它回答三个问题：v1.0.0 要达成什么产品状态、怎么算达成、现在走到哪。
> 逐条工程任务卡在 [§7 附录](#附录-a--工程细项未落地的任务卡)；
> 问题的完整证据与修复顺序见 [`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md)。

---

## 1. 版本格局

| 版本 | 定位 | 状态 |
|---|---|---|
| **v0.1.0** | **当前发版** —— 能跑通的内测版。接口、存储、加密、门限解密、审计的骨架都在，但**零知识证明链尚未实现**，不要用于真实投票 | 发版中（分支 `release/v0.1.0-prep`） |
| **v1.0.0** | 生产可用的匿名投票 —— 匿名性**成立**，而不是被声称 | 路线图目标，M2/M3 未开工 |
| v1.0.0 之后 | 不承诺 | — |

**v0.1.0 与 v1.0.0 的差距不在业务代码，在一条完整的零知识证明链**：
电路 → 密钥 → 客户端 prover → proof 绑定 → 客户端加密 → 真盲签名。
这条链是周级工作量，**不能和小时级的加固项捆在一起做** —— 捆在一起的结果是 v1.0.0 永远发不出去。

---

## 2. v1.0.0 的产品定义

> **服务端在任何情况下都无法得知投票人选了哪个选项，且选票可验证、可审计、可复算。**

这句话是**可证伪**的：拿一个真实投票的服务端 + 完整数据库，
第三方应当无法回答「某人选了哪个选项」，同时应当能回答
「这张票是真的吗」「计票结果对吗」「谁投过票（仅是与否）」。

### 2.1 能力清单与完成定义

每项能力都有**可验证**的完成定义 —— 不是「做了」就算完，是「能拿出证据」才算完。

| # | 能力 | v0.1.0 现状 | v1.0.0 完成定义（证据） | 里程碑 |
|---|---|---|---|---|
| A | 选票内容对服务端不可知 | ❌ 密文由服务端生成，服务端在签发凭证时就知道选了哪个 | 服务端代码里**不存在**接收明文选项的字段（grep 可验）；落库记录无法还原选项 | M2 · T16 |
| B | 选票可验证 | ❌ 无真实电路；`vk_path` 为空时任何非空 proof 都通过 | 真实 Groth16 电路 + 密钥生成流程；篡改任一公开输入 → 验证失败 | M2 · T13 |
| C | proof 与票绑定 | ❌ proof 可跨提案复用（服务端只读 `public_witness`） | 公开输入含 `(proposal_id, option_index, nullifier, hash(ciphertext))`；A 提案的 proof 投 B 提案 → 400 | M2 · T15 |
| D | 客户端能投票 | ❌ 门户在生产模式下 100% 无法完成投票（没有 prover） | 浏览器内生成 proof 与密文，e2e 跑通「选票 → 加密 → 证明 → 提交」 | M2 · T14 |
| E | 凭证不可伪造 | ❌ 投票路径**从不**验凭证签名 | 落库前 `VerifyCredential`；伪造签名的投票 → 401 | M2 · T18 |
| F | 签发方看不到被签内容 | ❌ 盲化在服务端进程内完成，等同普通签名 | 两段式盲签名：服务端全程只见盲化消息；交叉攻击（A 的盲签名 finalize 出 B）→ 失败 | M2 · T17 |
| G | 凭证分发不泄露身份 | ✅ **v0.1.0 已达成**：凭证池 + 投票人匿名随机领取，运营方不再掌握「谁领了哪张」 | 保持；补**资格证明**兜底一人一票（见 §2.3） | M3 |
| H | 生产可运维 | ❌ 生产配置模板不会被自动加载；部署靠人工核对 | 显式 `CONFIG_FILE` 支持 + 生产部署清单逐项可勾 | M3 · T19 |
| I | 端到端验收 | ❌ 三个盲区从未验证（见 §4.3） | PG 集成测试 / 浏览器真实渲染 / 并发投票与容量，各有**可复现脚本 + 输出** | M3 · T20 |

### 2.2 v1.0.0 明确不做

避免范围蔓延，以下内容**不属于** v1.0.0：

- 端到端加密之外的传输层优化、限流、压测容量规划 —— 属运维，不属本路线图
- 多租户、权限分级、投票人身份体系 —— 当前是「一个 admin 管池」的模型
- 移动端原生客户端 —— 只做浏览器 + CLI
- 密码学方案的替换（换 KEM、换曲线）—— 只做 KDF 修正，不动主体方案

### 2.3 v1.0.0 引入的新问题

凭证改为**匿名随机领取**后（能力 G），服务端**无法强制一人一票** ——
随机领取意味着服务端看不出同一个人领了几次。

这不是可以「顺手修掉」的小事：它是「不可关联性」与「一人一票」之间的直接取舍。
v1.0.0 必须用**资格证明**（例如基于一次性设备密钥的可验证资格）把一人一票补回来，
否则能力 G 的达成等于用「可重复投票」换的。

---

## 3. 里程碑

### M1 · 加固层 ✅ 已完成

**目标**：与证明链无关的工程加固 —— 让 v0.1.0 敢发内测。

**出口标准**：逐包覆盖率门槛生效、CI 全绿、已知缺陷要么修掉要么在文档里如实写明。

**状态**：**已完成**，随 v0.1.0 一起落地（见 §4.1）。

### M2 · 证明链 ⬜ 未开工

**目标**：能力 A–F —— 让「匿名性成立」从口号变成可验证的事实。

**出口标准**：能力 A–F 的完成定义逐条拿出证据；§5 的 No-Go 清单全过。

**依赖与顺序**（后一项依赖前一项，不要并行）：

```
T13 电路 + 密钥  →  T14 客户端 prover  →  T15 proof 绑定
                                              ↓
                              T16 加密移到客户端 ←─ T15
                                              ↓
                              T17 真盲签名（两段式）←─ T14
                                              ↓
                              T18 投票路径 VerifyCredential
```

**预估**：周级。这是 v1.0.0 的主体，也是唯一的硬骨头。

### M3 · 生产化 ⬜ 未开工

**目标**：能力 G 的资格证明、H 的生产配置、I 的端到端验收，以及发版本身。

**出口标准**：生产部署清单全勾；三项盲区都有可复现输出；`VERSION` 与 tag 一致。

**依赖**：M2 全部完成。

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
| T9 | 凭证改为「凭证池 + 投票人匿名随机领取」 | `17937da` |
| T10 | 覆盖率门槛扩到 9 个包并改为逐包分档；`cmd/server` 冒烟测试 | `f7ec274` |
| T11 | 门户补「查询投票状态」按钮 | `2356638` |
| T12 | 文档如实描述盲签名的真实性质 | `a94b51e` |

**T7 是协议变更**：v0.1.0 的 v1 格式密文会被直接拒绝，旧的加密选票无法解密。
升级前不要用旧数据做真实投票。

### 4.2 v0.1.0 的边界（现在能做什么、不能做什么）

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

---

## 5. go / no-go（v1.0.0）

**No-Go（任一命中则不发 v1.0.0）**

- [ ] 能力 A–F 任一未完成
- [ ] 存在「服务端能拿到明文选项」的代码路径（grep 不到才过）
- [ ] 存在「proof 可以跨提案复用」的路径
- [ ] `go build` / `go vet` / `gofmt -l` / `go test ./...` 任一失败
- [ ] 覆盖率门槛退出码非 0
- [ ] 前端 lint / tsc / vitest 任一失败
- [ ] §4.3 三项盲区没有可复现输出
- [ ] README / techspec / 门户三处的匿名性声明不一致

**Go**

- [ ] 上述全过
- [ ] 决策门 G1–G9 全部拍板并落到代码/文档
- [ ] `VERSION` 与 tag 一致
- [ ] 生产部署清单全勾：`SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS` / `CRYPTO_VK_PATH` / `DB_DRIVER=postgres` / TLS / 门户独立部署 / 迁移步骤

---

## 6. 决策记录

**已拍板（2026-10-20）**

| # | 决策 | 结论 |
|---|---|---|
| **G1** | 版本定位 | ✅ **v0.1.0 = 当前发版（内测版）**；**v1.0.0 = 产品路线图目标**，不是这一轮的发版计划 |
| **G4** | 盲签名：改实现还是改文档 | ✅ **先改文档如实描述**（T12），两段式（T17）留到 M2 一起做 —— v1.0.0 承诺匿名性时必须补上 |
| **G5** | 版本号 | ✅ 当前发版用 `0.1.0`；`1.0.0` 留给路线图目标 |
| **G8** | 投票人领凭证的形态 | ✅ **凭证池 + 投票人匿名随机领取**（T9）—— 不再用一次性领票码。核心理由：admin 逐个分发就等于 admin 知道哪个票是谁的，直接破坏不可关联性 |
| **范围** | 这轮做哪些 | ✅ **T3–T12 这 10 项**已全部落地；T1（`ballot-draft` 非 debug 404）与 T2（门户隐藏按钮）暂缓，等 G2/G3 |

**待拍板**

> G2 / G3 **不阻塞 v0.1.0 发版** —— 这两项都是「生产环境不该暴露的调试能力」，
> 保留现状并在 README / techspec 里标注边界即可发内测版；它们卡住的是路线图
> M2 的起点（客户端加密 T16 与 `ballot-draft` 端点的删除顺序）。

| # | 决策 | 选项 | 建议 | 卡住什么 |
|---|---|---|---|---|
| G2 | 门户「按选项生成凭据」按钮 | 保留并加警示 / 非 debug 隐藏 | **非 debug 隐藏** | T2 |
| G3 | `ballot-draft` 生产可达性 | 保留 / 非 debug 404 | **非 debug 404** | T1、T16 |
| G6 | 是否换纯 Go sqlite 驱动 | 保留 cgo（当前）/ 换 `modernc.org/sqlite` | **保留 cgo**：换驱动会波及持久化层方言差异测试，收益只是「静态链接二进制」，不值 | 发布形态 |
| G7 | 电路形状与曲线 | Groth16/BN254（与现有 verifier 一致）/ 其他 | **Groth16/BN254**：现有 verifier 硬编码了 `ecc.BN254`，换曲线等于重写验证器 | T13、T15 |
| G9 | CLI 是否需要本地 prover | CLI 也做本地 prover / CLI 只做「带 proof 提交」 | **CLI 只带 proof 提交**；生成 proof 放浏览器端，CLI 面向脚本化/运维场景 | T14 |

---

## 7. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| T13 电路的证明生成太慢，浏览器端体验不可用 | 投票流程不可用 | T13 就带性能评估；必要时换电路形状（公开输入更少）而不是硬扛 |
| T17 真盲签名改动面大，拖垮整个 v1.0.0 | 发版无限延期 | T17 单独成提交链；G4 允许退化为「改文档」，M2 先不做 T17 但**必须在 CHANGELOG 如实说明** |
| T7 KDF 换协议使 v0.1.0 的密文永久不可解 | 旧数据丢失 | 明确标注密文格式版本；v1.0.0 前不要用旧数据做真实投票 |
| T16 把加密移到客户端后，前端成为新的攻击面 | 密钥泄露 | proving key 只在前端，**不入服务端**；前端构建产物做完整性校验 |
| 匿名随机领取牺牲了一人一票 | 可重复投票 | M3 用资格证明补回（§2.3） |
| M1 拖太久，M2 无时间 | 路线图整体延期 | M1 与 M2 **可并行**（除 T1/T2 依赖 M2 的字段） |

---

## 附录 A · 工程细项（未落地的任务卡）

以下是**尚未落地**的任务卡。保留问题描述、证据与改法，供实现时直接取用；
路线图的判断以正文 §2–§4 为准。

### A.1 挂在 M1 但等决策的

#### T1 · `ballot-draft` 在非 debug 模式关闭（决策门 G3）

| 项 | 内容 |
|---|---|
| 问题 | `POST /api/v1/proposals/:id/ballot-draft` 是**公开路由**，返回写死的假 proof。任何人都能调用，且这条链路的密文由服务端生成。 |
| 证据 | `internal/infrastructure/http/handler.go:65`（路由未挂在 admin 分组下）、`:314`（常量 proof） |
| 改法 | ① `config.Config` 增加 `DebugMode bool`（由 `server.mode` 推导：`debug`/`dev`/`development` 为 true）；② 路由在非 debug 模式返回 **404**（用 404 而不是 403，避免暴露端点存在性）；③ `server_test.go` 里现有的 `ballot-draft` 用例按模式分叉，debug 断言 201、release 断言 404 |
| 验收 | `go test ./...` 全绿；新增用例断言 release 模式 404 |
| 预估 | 1 小时 |
| 依赖 | 需先拍板 G3 |

#### T2 · 门户「按选项生成凭据」按钮在非 debug 模式隐藏（决策门 G2）

| 项 | 内容 |
|---|---|
| 问题 | 门户在运营侧提供「按选项生成凭据」的入口，等于把「服务端替用户选」的能力做成一键操作。 |
| 证据 | `web/src/composables/useVotePortal.ts:977` `issueCredential`、`web/src/components/panels/IdentityPanel.vue:13` |
| 改法 | 按钮按 `debug` 模式条件渲染；非 debug 时给一句说明为什么没有这个入口 |
| 验收 | vitest：非 debug 模式渲染不出该按钮；debug 模式仍在 |
| 预估 | 1 小时 |
| 依赖 | 需先拍板 G2 |

### A.2 M2 · 证明链

#### T13 · 真实选票电路 + `verifying.key` 生成流程

| 项 | 内容 |
|---|---|
| 问题 | **选票的零知识电路根本不存在。** `internal/infrastructure/crypto/gnark_proof_verifier.go` 只有验证器骨架；`NewGnarkProofVerifier`（`:25`）在没有 `vk_path` 时构造出 `requireVerifyMode: false` 的实例，`Verify` 在 `:55-57` 对**任何非空 proof 直接返回 nil**。生产配置的 `crypto.vk_path: "./keys/verifying.key"` 按本仓库**无法生成**。 |
| 证据 | `gnark_proof_verifier.go:25-31`、`:55-57`、`configs/config.production.yaml` 的 `vk_path` |
| 改法 | ① 电路定义「选票属于某个提案的某个选项」：`public = (proposal_id, option_index, nullifier)`，`private = (选中选项的承诺随机数)`，用 Groth16/BN254；② 新增 `cmd/keygen` 生成 `verifying.key` + `proving.key`（proving key **不入库**，只在客户端侧使用）；③ 密钥分发与轮换说明写进 `docs/techspec.md`；④ 仓库只提交 `verifying.key` 的**生成脚本与公钥指纹**，不提交 proving key |
| 验收 | ① `cmd/keygen` 产出的 vk 能被 `NewGnarkProofVerifier` 加载；② 电路测试：正确 witness 生成 proof → 验证通过；篡改任一公开输入 → 验证失败 |
| 预估 | 3–5 天（含电路调参与性能评估） |
| 依赖 | 需先拍板 G7 |

#### T14 · 客户端 prover（浏览器 WASM）

| 项 | 内容 |
|---|---|
| 问题 | 门户在生产模式下 **100% 无法完成投票**：没有 prover 就产不出 proof。现有 `ballot-draft` 返回写死的假 proof。 |
| 证据 | `handler.go:314`、review C2 |
| 改法 | ① 用 gnark 官方 wasm 模板产出 prover WASM，放进 `web/public/` 或独立静态目录；② 门户在浏览器内生成 proof **与 `public_witness`**（`public_witness` 只能是公开输入部分，放整个 witness 会被 gnark 拒：报 `invalid witness size, got 2, expected 1`）；③ `ballot-draft` 降级为「仅返回选票选项与加密公钥」，**不再返回假 proof**；④ CLI 侧同步：CLI 目前已支持 `public_witness`，但没有本地 prover —— 需明确 CLI 是否也要 prover（G9） |
| 验收 | ① 浏览器端生成的 proof 能通过服务端 `Verify`（e2e 用例）；② `public_witness` 长度为电路公开输入个数 |
| 预估 | 5–7 天 |
| 依赖 | T13 |

#### T15 · proof 与 `proposal_id` / `nullifier` / 密文绑定

| 项 | 内容 |
|---|---|
| 问题 | `SubmitVoteRequest.PublicInputs()`（`internal/usecase/voting.go:60-64`）**造了** `proposal_id` 和 `nullifier`，但 `Verify` 收到的 `inputs` 里只有 `public_witness` 是被真正读取的。也就是说：**服务端完全没有校验 proof 与请求里的 proposal_id / nullifier / 密文是否对应** —— 攻击者可以把 A 提案的合法 proof 拿去投 B 提案。 |
| 证据 | `internal/usecase/voting.go:60-64`、`:148`、`gnark_proof_verifier.go:64` |
| 改法 | ① 电路公开输入改为 `(proposal_id, option_index, nullifier, ciphertext_hash)`；② `PublicInputs()` 按序拼出**完整的**公开输入数组（含 `hash(ciphertext)`），不再走 `public_witness` 这条旁路；③ verifier 校验公开输入与 `proposal_id`/`nullifier`/`hash(ciphertext)` 一致（不一致直接失败）；④ 移除 `public_witness` 的自由文本入口 —— 公开输入由服务端根据请求字段**自己拼** |
| 验收 | 回归测试：拿 A 提案的 proof 投 B 提案 → **400**；篡改 nullifier → 400；密文改了再投 → 400 |
| 预估 | 2–3 天 |
| 依赖 | T13 |

#### T16 · 加密移到客户端

| 项 | 内容 |
|---|---|
| 问题 | **当前版本最大的匿名性缺口**：密文由服务端生成（`ballot-draft` 拿明文 `option` 去加密），服务端在生成凭据时就知道选了哪个。 |
| 证据 | `internal/infrastructure/http/handler.go:316-330`、review C5 |
| 改法 | ① 服务端只发**加密公钥 + 电路参数**，不再接受明文选项；② 浏览器端完成「选票 → 加密 → proof → 提交」全流程；③ 服务端接口改为接受 `{encrypted_vote, proof, nullifier, ...}`，**没有明文选项字段**；④ `ballot-draft` 端点随之删除（T1 已先把它关掉） |
| 验收 | ① 服务端代码里**不存在**接收明文选项的字段（grep 验证）；② e2e：浏览器投票 → 服务端落库记录里没有可还原的选项 |
| 预估 | 3–4 天 |
| 依赖 | T13、T14、T15 |

#### T17 · 盲签名改真两段式

| 项 | 内容 |
|---|---|
| 问题 | 盲签名的**盲化在服务端内部完成**：`internal/infrastructure/crypto/kryptology_credential_issuer.go:162-181` 在同一个进程里依次 `Prepare` → `Blind` → `BlindSign` → `Finalize`，服务端全程持有明文消息 —— **不具备盲化性质**，等同于普通签名。 |
| 证据 | `kryptology_credential_issuer.go:162-181` |
| 改法 | ① 拆成两个 HTTP 端点：客户端 → `POST /credentials/blind`（提交盲化后的消息）→ 服务端 `BlindSign` 返回盲签名 → 客户端 `Finalize` 得到最终签名 → `POST /credentials/attest`（提交最终签名 + 承诺）→ 服务端用 `Verify` 校验；② 客户端（浏览器 WASM）实现 `blindrsa` 的 client 侧；③ 协议要绑定被盲化的消息承诺，防止「拿 A 的盲签名去签 B」 |
| 验收 | ① 端到端：客户端拿到的凭证，服务端**全程只见盲化消息**；② 交叉攻击测试：用 A 的盲签名 finalize 出 B 的凭证 → 校验失败；③ 服务端代码里不再出现「同一个函数里 Prepare+BlindSign+Finalize」 |
| 预估 | 5–7 天 |
| 依赖 | T14（需要客户端承载 crypto）、T9（凭证池路径） |
| 备注 | 代价高、收益是「签发方看不到被签内容」。若投票规模小、运营方可信度可接受，**改文档如实描述**（G4）也是合理选择 |

#### T18 · 投票路径补 `VerifyCredential`

| 项 | 内容 |
|---|---|
| 问题 | 凭证签名在生产路径上**从不被密码学验证**：`VerifyCredential` 只在 `internal/infrastructure/crypto/` 及其测试里出现，投票提交路径不调用 —— 任何人都能伪造凭据投票。 |
| 证据 | `kryptology_credential_issuer.go:194` 定义了它，但 `internal/usecase/voting.go` 的 `Submit` 未调用 |
| 改法 | ① `Submit` 在落库前调用 `VerifyCredential(userID, userPubKey, credentialSignature)`，失败返回 401；② 需要在 `SubmitVoteRequest` 里带上签名与公钥（当前有 `CredentialSignature`，需确认公钥来源）；③ 补「伪造签名 → 401」回归测试 |
| 验收 | 伪造签名的投票请求返回 401；合法凭证正常落库 |
| 预估 | 1–2 天 |
| 依赖 | T16（否则明文选项仍在服务端） |

### A.3 M3 · 生产化与发版

#### T19 · 生产配置落地

| 项 | 内容 |
|---|---|
| 问题 | `configs/config.production.yaml` 的 `cors_allowed_origins: []` 仍为空；且该文件**永远不会被自动加载**（viper `SetConfigName("config")`，`internal/config/config.go:27`）。 |
| 证据 | `configs/config.production.yaml`、`internal/config/config.go:27` |
| 改法 | ① 给 viper 加 `CONFIG_FILE` 显式路径支持（**推荐**：比改名更不容易出错）；② CORS 填门户真实源 |
| 验收 | 用 `CONFIG_FILE=./configs/config.production.yaml` 启动，启动日志显示 `server.mode=release` 且 CORS 生效 |
| 预估 | 半天 |
| 依赖 | 无 |

#### T20 · 端到端验收（三个盲区）

| 项 | 内容 |
|---|---|
| 问题 | review §5 列的三项**从未被验证**：① PostgreSQL testcontainers 集成测试；② 门户 7 个 `.vue` 面板的浏览器实际渲染；③ 并发投票下的防重投与性能容量。 |
| 证据 | `docs/release-review-2026-10-20.md` §5 |
| 改法 | ① Docker 环境跑通 `tests/integration`（CI 会实跑，本地补一份 compose）；② 引入 Playwright 覆盖关键路径（领凭证 → 投票 → 查询状态 → 计票）；③ 用 `hey`/`k6` 写并发投票脚本，验证防重投（唯一索引兜底）与容量 |
| 验收 | 三项都有**可复现的脚本 + 输出**，并把结果贴进 PR |
| 预估 | 2–3 天 |
| 依赖 | T14（浏览器端投票链路才完整） |

#### T21 · 发版（v1.0.0）

| 项 | 内容 |
|---|---|
| 动作 | ① `VERSION` 改 `1.0.0`；② `CHANGELOG.md` 写 1.0.0 条目（**必须**写明与 0.1.0 的密文不兼容，见 T7）；③ `git tag v1.0.0` → release workflow 自动构建；④ 按 v0.1.0 的生产部署清单逐项核对 |
| 验收 | `gh release view v1.0.0` 有产物；生产部署清单全勾 |
| 预估 | 半天 |
| 依赖 | M1、M2、M3 全部完成 |

---

## 附录 B · 提交分组建议

按 `incremental-implementation` 原则，每张任务卡一个提交，Conventional Commits：

```
chore(release): gate the ballot draft endpoint behind debug mode
fix(web): hide the option-scoped credential button outside debug
feat(circuit): add the ballot circuit and the key generation flow
feat(prover): prove ballots in the browser
fix(vote): bind the proof to the proposal, nullifier and ciphertext
feat(vote): encrypt ballots on the client
fix(credential): use a two-phase blind signature
fix(vote): verify the credential signature before storing the vote
feat(config): support an explicit config file path
test(e2e): cover the browser voting flow and concurrent voting
```

---

## 相关文档

| 文档 | 内容 |
|---|---|
| [`docs/release-plan-v0.1.0.md`](./release-plan-v0.1.0.md) | v0.1.0 发版计划与落地状态（**当前发版**） |
| [`docs/release-review-2026-10-20.md`](./release-review-2026-10-20.md) | 发版前整体 review：6 个 CRITICAL + 29 个 REQUIRED + 20 个 NIT 的完整证据 |
| [`docs/techspec.md`](./techspec.md) | 当前实现的接口与流程口径 |
| [`docs/project-map.md`](./project-map.md) | 分层项目地图 / 上下文索引 |
