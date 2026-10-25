# Changelog

本文件记录 nullifier 的版本变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

版本号的唯一真相源是仓库根的 `VERSION` 文件；打 tag 时 release workflow 会校验两者一致。
前端的 `web/package.json` 也跟随 `VERSION`（发布流水线在构建前注入），不再各写各的。

---

## [未发布]

v0.2.0 之后的变更。本版的主题是**「接口不再动」** —— 完成定义与验收证据见
[`docs/release-plan-v0.3.0.md`](./docs/release-plan-v0.3.0.md)，路线图见
[`docs/roadmap.md`](./docs/roadmap.md)（里程碑 M4）。

> ⚠️ **升级前必读**：投票提交端点改了路径，旧的 `POST /api/v1/votes` 不再存在 ——
> 详见下面的「破坏性变更」。

### 破坏性变更

- **投票提交端点从 `POST /api/v1/votes` 改为 `POST /api/v1/proposals/:proposal_id/votes`，
  请求体里不再有 `proposal_id` 字段。**
  一个提案 ID 现在只有**一个**来源（路径）。此前两份来源不一致时服务端只能猜，猜错就是
  「把票投到别的提案去」。旧路径**不再注册**（返回 404）。
  - 自建客户端需要改路径并去掉请求体里的 `proposal_id`；门户与 CLI 已同步
    （门户改用 `web/src/lib/api-contract.ts` 里声明的冻结路径）。
  - 请求体的其余六个字段（`encrypted_vote` / `proof` / `nullifier` / `user_id` /
    `user_pub_key` / `credential_signature`）**不变**。

> **本版之后**，字段名、端点路径与密文格式**冻结**：响应可新增字段、服务端可新增端点
> （客户端须忽略不认识的字段）；改字段名 / 删字段 / 改语义 / 改路径 = 破坏性变更，
> 须走下一个 minor 版本。完整口径见
> [`docs/api-contract-v1.md`](./docs/api-contract-v1.md)，并由一条**跨语言门禁**守着
> （`tests/architecture/api_contract_test.go` 拿 Go 的 struct tag 与 TS 的 interface
> 双向比对，改一边不改另一边 CI 就红）。

### 新增

- **公开端点限流** —— 凭证领取、投票提交、只读端点按「来源 + 路由组」限流，
  超限返回 **429** + `Retry-After` + `{"error":…,"code":"RATE_LIMITED"}`。
  - **release 模式下强制开启**：`RATE_LIMIT_ENABLED=false` 会让服务**启动失败**
    （默认就是开的）。公开端点上最贵的操作是投票 —— 每张票都要验一次 Groth16 proof，
    不设上限时一个脚本就能把验签算力打满。
  - **额度按路由组分档**（领取 / 投票 / 只读），默认偏宽松：宁可放过一次攻击，
    也不要在真实投票窗口里把投票人挡在门外。真实部署规模决定最终值。
  - **限流器的键是每进程随机盐的哈希** —— 内存里没有明文来源地址（它常驻内存，
    比日志更容易被转储）。
  - **桶有空闲清理** —— 不清理就是内存泄漏（攻击者轮换来源地址就能把它撑大）。
  - ⚠️ **限流是进程内的**：多副本部署时实际额度是配额的**副本数倍**。
  - ⚠️ **部署在反向代理后面时必须配 `server.trusted_proxies`**：gin 默认**信任所有
    代理**，不配的话伪造 `X-Forwarded-For` 就能绕过限流（这条同时修掉了日志里
    `client_ip` 同样可伪造的既有问题）。
  - 设计稿见 [`docs/t25-ratelimit-design.md`](./docs/t25-ratelimit-design.md)。
- **审计离线复算工具 `cmd/audit-recompute`** —— 拿计票份额**脱离服务端**重算计票与锚点，
  与库内已发布的结果**逐字节**比对，不一致时**退出码非 0**（可当门禁用）。
  - **与服务端共用同一份计票算法**（新包 `internal/tally`）：两边各写一份算法，一旦漂移，
    复算结论就失去意义 —— 那不是审计，是第二套计票。锚点哈希的公式用 KAT 钉死。
  - **只读票库**：SQLite 走 `mode=ro`、PostgreSQL 走 `default_transaction_read_only=on`
    —— 不是「我们保证不写」，是「写不了」。
  - **不碰服务端**：直接读库，不需要投票服务在跑，也不要 admin token。
  - ⚠️ **计票份额是秘密材料** —— 凑齐门限份数就能还原全部选票的明文选项。
- **Prometheus 监控指标** —— `GET /metrics` 输出 Prometheus 文本格式（需 admin Bearer 令牌）：
  投票接受 / 被拒（按错误码分）、凭证领取 / 盲签名 / attest、资格证明校验结果、
  计票结果发布数、HTTP 请求数与耗时直方图。
  - **只暴露计数与耗时**：指标里没有任何选票内容、候选人票数或身份 ——
    有一条测试把允许出现的标签名**逐个钉死**，多一个标签就是多一条泄漏路径。
- **冻结的接口契约文档** `docs/api-contract-v1.md` + 跨语言门禁
  `tests/architecture/api_contract_test.go`（见上面的「破坏性变更」）。
- **错误码集合的跨语言门禁** `tests/architecture/error_code_test.go` —— Go 侧
  `MapError` 的码集合 ↔ TS 侧 `PortalErrorCode` 的成员集合，双向比对。
  服务端加了码而门户不认识时，**两边各自的自测都是绿的**，用户看到的是一句英文原文 ——
  这正是「两边各拿自己的常量自证是测不出这类问题」的典型，所以要有门禁。
  顺手补齐了门户本就缺的 `CREDENTIAL_POOL_EMPTY`（凭证池领完）。

### 变更

- **新增配置项**：`rate_limit.*`（额度）与 `server.trusted_proxies`（可信代理网段）。
  两者都有环境变量（`RATE_LIMIT_*` / `SERVER_TRUSTED_PROXIES`）。
  `server.trusted_proxies` **默认为空 = 不信任任何代理** —— 这会改变
  `X-Forwarded-For` 的处理口径：反代后面**必须**显式配置。
- **计票的规范算法从 `internal/usecase/tally.go` 抽到新包 `internal/tally`** ——
  服务端与审计复算工具调的是同一个 `Compute`。对外行为**不变**。
- **门限份额的工具抽到新包 `internal/shares`**（哈希、归属校验、文件读取）——
  CLI 的 `result-publish` 与审计工具共用同一份份额文件格式解析。
- **`internal/di/wire_gen.go` 补上 `//go:build !wireinject` 约束** —— 此前缺这道约束，
  `wire` 生成器跑不起来（生成代码与 `wire.go` 同时参与编译而重复声明）。
  已用它重新生成了装配代码。

### 修复

- **指标中间件的注册顺序是硬要求**（写在代码注释里）：必须排在错误处理器**之前** ——
  gin 的中间件先进后出，越早注册的回程越晚，这样才拿得到最终状态码；
  顺序反了指标里就会全是 200。

---

## [0.2.0] — 2026-10-25

v0.1.0 发布之后的变更。本版的完成定义与验收证据见
[`docs/release-plan-v0.2.0.md`](./docs/release-plan-v0.2.0.md)，路线图见
[`docs/roadmap.md`](./docs/roadmap.md)。

**已发布**：[GitHub Release v0.2.0](https://github.com/jiangfire/nullifier/releases/tag/v0.2.0)
（tag `v0.2.0` @ `abc86ca`；5 个平台二进制 + 门户包，各带 sha256）。

> ⚠️ **升级前必读**：选票密文格式 v2 → v3 **不兼容**，用 v0.1.0 密文投过的票无法解密 ——
> 详见下面的「破坏性变更」。

### 破坏性变更

- **`POST /api/v1/proposals/:proposal_id/ballot-draft` 端点已删除**（连带 CLI 的 `ballot-draft`
  命令与门户的「按选项生成凭据」入口）。它让**服务端拿明文选项去加密选票** —— 能力 A
  （选票内容对服务端不可知）的底线唯一的例外。服务端现在**没有任何**接收明文选项的入口，
  与 `server.mode` 无关；这条不变量由 `tests/architecture/` 持续守卫（**不设豁免机制**）。
  自建客户端如果用过这个端点，改用 `ballot-encrypt`（CLI）或门户的
  「在浏览器本地生成选票」。
- **选票密文格式从 v2 升到 v3，与 v0.1.0 的密文完全不兼容。** v3 = 1 字节版本 `0x03` + 两个
  32 字节压缩点（BN254 twisted Edwards），共 65 字节，加密的是**选项下标**。原因：零知识电路的
  算术发生在 BN254 标量域里，v0.1.0 用的 ed25519 + HKDF 群运算**无法在电路中复算** —— 不换栈
  就无法证明「这份密文确实是本提案计票公钥加密的某个合法选项」。**旧的加密选票无法解密**：
  用 v0.1.0 密文做过的真实投票要视为不可用，发 v0.2.0 前请清库。
- **投票请求体不再接受 `public_witness` 字段。** 公开输入改由服务端根据请求字段、提案与计票
  会话自行拼装 —— 客户端无法自选公开输入，一份 proof 因此只能用于它被生成的那份密文。
- **门限分享从 kyber 的 ed25519 改为 Fr 上的 Shamir**（`go.dedis.ch/kyber/v3` 依赖已移除）。
- **凭证签发从「一次性领取」改为「真两段式盲签名」，接口变了。**
  - `POST /api/v1/proposals/:id/credentials/claim` 现在**只做预留**：请求体是空的，
    **不再接收 `user_pub_key`**（旧字段一律忽略）。服务端一旦知道公钥就能算出凭证消息
    的原文，盲化当场失效 —— 它手上有私钥，根本不需要解盲。
  - 新增 `POST /api/v1/proposals/:id/credentials/blind-sign`（公开，只为池里还空着的槽位
    签名）与 `POST /api/v1/proposals/:id/credentials/attest`（公开，先验签再落库）。
  - 管理端新增 `POST /api/v1/credentials/blind-sign`；`POST /api/v1/credentials` 从「直接
    签发」改为「为指定的 `user_id` 预留」。
  - 领域接口 `domaincrypto.CredentialIssuer` 变成 `BlindSign` / `IssuerPublicKey` /
    `VerifyCredential` 三个方法，一次性的 `Issue` **已删除**。
  - 自建客户端（门户 / CLI / 第三方）需要跟着改：领取凭证从「一次 POST 拿到凭证」变成
    「预留 → 本地盲化 → 盲签名 → 本地解盲 → attest」四步。

### 新增

- **一人一票：资格证明（能力 G 的完成定义已闭合）**。此前凭证是「池里随机领一张」，
  服务端看不出同一个人领了几次 —— **可重复投票**。现在领凭证必须先证明「我在资格名单里」。
  - **权威名单 → 离线工具 → 服务端只拿到 root**：新命令 `cmd/eligibility-list -members <名单文件>
    -out <目录>` 由权威成员名单生成两份**必须分开保管**的产物 —— `root.json`（给服务端）
    与每成员一份 `eligibility-<member_id>.json`（0600，成员本人保管）。
    **服务端只拿到 root**：它能验证「这个资格在树里」，却**无法把资格对应到任何成员**
    （它没有名单，也没有成员包）。
  - **零知识成员资格证明**：新电路 `internal/infrastructure/crypto/eligibility/`（Groth16 / BN254 /
    MiMC，**实测 6860 条约束**）。公开输入**定长 5 个**（Root / ProposalID / Challenge /
    QualificationID / SessionBinding），**叶子、Merkle 路径、位置全私有** —— 公开输入的个数
    不随树深变化，服务端不需要知道树的结构。`TreeDepth = 10` 是**编译期常量**，
    单份名单上限 1024 人。
  - **去重键** `QualificationID = MiMC(Leaf, ProposalID)`：同一成员同一提案稳定（第二次领撞
    唯一索引 → 409），跨提案不同（**跨提案不可关联**）。落库在新表 `eligibility_uses`
    （`proposal_id` + `qualification_id` + `used_at`，唯一索引 `(proposal_id, qualification_id)`）——
    这就是一人一票的强制点。**只存去重键**，不存叶子 / 秘密 / 成员 ID。
  - **新协议**（三步，blind-sign / attest 两步**完全不变**）：
    ① `POST /api/v1/proposals/:id/credentials/eligibility-challenge`（公开，**无状态** ——
    服务端不记账，重放由去重键挡住）→ `{proposal_id, root, depth, challenge}`；
    ② `POST /api/v1/proposals/:id/credentials/claim` 的请求体多了三件
    `{eligibility_proof, qualification_id, challenge}`（**三件要么都给要么都不给**）；
    ③ `blind-sign` / `attest` 不变。**资格不过不消耗池槽位** —— 校验在取槽位之前。
  - **新错误码**：`ELIGIBILITY_REQUIRED`(400) / `INVALID_ELIGIBILITY_PROOF`(403) /
    `ELIGIBILITY_ALREADY_USED`(409) / `ELIGIBILITY_NOT_ENABLED`(409)。
  - **命令行**：`cmd/keygen -circuit ballot|eligibility` —— 两套电路的密钥**互不相通**
    （默认文件名 `verifying.key`/`proving.key` 与 `eligibility-verifying.key`/
    `eligibility-proving.key`）；`cmd/cli credential-claim -eligibility <成员包.json>
    -proving-key <资格 proving key>` 走完整的资格领取路径。
  - **浏览器端**：`cmd/ballot-wasm` 在同一个模块上暴露四个 API —— `init` / `prove` /
    `initEligibility` / `proveEligibility`（静态资源只新增
    `web/public/eligibility-proving.key`，`make web-eligibility-key PK=<路径>`）。
    门户「身份」面板新增「导入资格文件 → 领凭证时本地出证明」；**资格包只留内存**，
    不写 localStorage、不上传，刷新页面就要重新导入。
  - **端到端证据**：`tests/e2e/eligibility_test.go` —— 无资格被拒且不消耗槽位 / 有效资格可领 /
    重复资格被拒 / 跨提案不可复用 / 请求体无真人身份字段，五条验收逐条有例；
    `tests/e2e/wasm_eligibility_test.go` —— 用 node 驱动**真的 WASM 模块**出资格证明，
    服务端用**真的资格验证密钥**验过，去重键与 Go 侧自算一致（浏览器路径的跨语言证据）。
  - **跨语言线路有守卫**（新文件 `tests/architecture/eligibility_wire_test.go`）：门户这一侧只做
    JSON 解析与转发，**最容易出错的不是数学而是字符串** —— 成员包字段名、出证明请求的字段名、
    WASM API 名、静态资源名，两边各钉一条断言（Go 侧读 struct tag 与 `api.Set` 的键，
    TS 侧读接口字段与调用名）。改了一边不改另一边，这里就会红。
  - **没有降级放行的口子**：资格验证密钥缺失一律**启动失败**；开了资格校验但**去重仓储缺失**
    也**报错**，不悄悄放行 —— 那等于把一人一票悄悄关掉。
  - ⚠️ **部署侧是破坏性调整**：**release 模式现在必须配 `crypto.eligibility_root` +
    `crypto.eligibility_vk_path`（环境变量 `CRYPTO_ELIGIBILITY_ROOT` /
    `CRYPTO_ELIGIBILITY_VK_PATH`），否则服务启动失败**。两项必须**成对出现** ——
    只配一个等于「不校验」。**部署动作**：① 用 `cmd/eligibility-list` 生成 `root.json`
    （把里面的 `root` 填给 `crypto.eligibility_root`）与每成员一份
    `eligibility-<member_id>.json`（走安全渠道发给本人）；② 用
    `cmd/keygen -circuit eligibility` 生成资格电路的密钥，`eligibility-verifying.key`
    放服务端（`crypto.eligibility_vk_path`），`eligibility-proving.key` 只留在客户端侧；
    ③ 迁移照旧（`nullifier-server -migrate`，新增表 `eligibility_uses`）。
    **没开资格校验的部署不受影响**：两项都不配时行为与从前完全一样 —— `claim` 的请求体
    仍然可以为空，旧的 claim 请求体继续可用；**但这种配置在 release 模式下不再被允许**。
    `GET /healthz` 新增 `eligibility` 布尔字段，运营能一眼看到服务端到底有没有在强制一人一票。
  - ⚠️ **还没闭合的**：门户的资格领取路径**仍需在真浏览器里人工点一遍**（仓库里没有
    Playwright，自动化证据只到 node 驱动的 WASM 模块）；`cmd/loadtest` **不支持**资格证明
    （只在未开启资格校验的服务上可用）；**资格包丢失无法找回**（成员包是秘密材料，发放靠安全
    渠道）；**真实名单的内容与成员标识形式仍需运营方提供**。
  - 设计与落地的口径见 [`docs/t22-eligibility-design.md`](./docs/t22-eligibility-design.md)
    （含「实现与设计稿的偏离」一节）。

- **浏览器验收流程固化成脚本**（新脚本 `scripts/browser-acceptance.sh`，新命令
  `make browser-acceptance`）：把「在真浏览器里点一遍」的**前后**全部自动化 ——
  `setup` 备好环境（两套密钥 → 资格名单 → 提案 → 凭证池 → 门限解密 → 门户 dev server）
  并**逐条打印人工步骤**；`verify` 核对服务端日志里的三件事（`credential attested` /
  `vote submitted` / `eligibility already used`），等窗口关闭后发布结果并核对
  **计票还原出的就是浏览器端选的那个选项**；`stop` 收尾。
  ⚠️ **闭合的是「每次上线不用从零手搓」，不是「浏览器自动化」** —— 中间那一下导入
  资格文件与点击仍然要人来做，仓库里依然没有 Playwright（盲区 2 的现状见
  [`docs/roadmap.md`](./docs/roadmap.md) §4.3）。

- **真实的选票零知识电路**（新包 `internal/infrastructure/crypto/ballot/`）：Groth16 / BN254，
  twisted Edwards 曲线。**公开输入 9 个**（提案 ID 域元素、nullifier、计票公钥、密文的两个点、
  选项数），**私有见证 3 个**（选项下标、nonce、nullifier 秘密）。约束 `a=r·G`、`b=m·G+r·Y`、
  `m<NumOptions`、`nullifier=MiMC(secret, proposal_id)`。约 7917 条约束，证明与验证都是毫秒量级。
  - **公开输入里没有选项下标**：放进去等于把明文选项交给服务端，与「选票内容对服务端不可知」
    直接冲突。选项下标降为**私有见证 + 范围检查**。
- **密钥生成流程**（新命令 `cmd/keygen`）：生成 `verifying.key`（0644，服务端验签用）与
  `proving.key`（0600，**只在客户端侧**），打印两者的 sha256 指纹，默认不覆盖已有文件
  （要覆盖加 `-force`）。
- **选票密文现在可以在客户端生成**（能力 A 的核心，新脚本 `web/src/lib/ballot-cipher.ts`）：
  BN254 twisted Edwards 上的指数 ElGamal，加密的是**选项下标**。点编码与 Go 侧
  **逐字节一致**，一致性由一组跨语言已知答案向量锁死
  （`internal/infrastructure/crypto/ballot/kat_interop_test.go` 与
  `web/src/lib/ballot-cipher.test.ts`）—— 浏览器端生成的密文必须能被 Go 侧解回同一个选项下标。
- **CLI 新增 `ballot-encrypt` 命令**：取计票公钥 → 在本地加密 → 落盘选票文件。
  选票文件里的 `nullifier_secret` 只留本地（prover 需要它才能满足电路的 nullifier 约束），
  提交时不发给服务端。
- **CLI 新增 `ballot-prove` 命令：现在命令行能自己出证明，票是真的。**
  读选票文件 → 用**本地 proving key** 生成 Groth16 证明 → 写回选票文件。
  - **证明全程不经过服务端**：proving key 只在客户端（服务端只拿 verifying key），
    服务端从始至终没见过明文选项。
  - 选票文件因此多存四项**本地**见证材料：`nonce`（电路约束 `a = r·G` 要用）、
    `option_index`（选项下标，私有见证）、`num_options`（提案选项数，公开输入之一）、
    `election_public_key`（未压缩的计票公钥）。它们**都不提交给服务端**。
  - 公开输入走服务端同一套拼装入口（`ballot.BuildPublicInputs`）：客户端与服务端算出
    同一份公开输入，证明才验得过。计票公钥客户端拿到的是未压缩形式、服务端用压缩形式，
    新增的 `ballot.CompressPoint` 负责转换。
  - 端到端证据：`tests/e2e/client_prover_test.go` —— 服务端配**真的 verifying key**
    （不是开发模式的「proof 非空即通过」），证明由客户端本地生成；改一个字节的证明
    必须 400 且不落库。服务端以**非 debug 模式**启动（`ballot-draft` 是 404），
    因此这张票只可能来自客户端。
  - ⚠️ **门户的 prover 见下面那条**：CLI 这条路不需要浏览器。
- **浏览器端 prover 落地：门户现在能在本地出票并提交**（能力 D 的门户侧）。
  - 路线**不是**「gnark 官方 wasm 模板」——离线环境装不上 `gnark-wasm` / `snarkjs` /
    `@noble/curves`。实际做法是**把仓库自己的 Go 侧电路编译成 WASM**：新命令模块
    `cmd/ballot-wasm`（`GOOS=js GOARCH=wasm`），在全局对象上注册 `nullifierProver`
    （`init` 加载 proving key、`prove` 出票）。
  - 门户侧：`web/src/workers/ballot-prover.worker.ts` 在 Worker 里加载 Go 官方胶水
    `wasm_exec.js` 与 `ballot-prover.wasm`，主线程 `web/src/lib/wasm-prover.ts` 只做
    编排；`useVotePortal.ts` 新增 `generateBallotLocally`：取计票公钥 → 调 prover →
    把密文 / 证明 / nullifier 写进表单 → 走原有的提交路径。投票面板新增
    「在浏览器本地生成选票」按钮（debug 模式下的服务端代生成入口保留为次要按钮 —— **后续
    已整个删除**，见上方「破坏性变更」）。
  - **密码学全在 Go 侧**：加密、nullifier 派生、公开输入拼装（`ballot.BuildPublicInputs`）
    与出证明都在新增的 `ballot.ProveBallot` 里一次做完 —— 客户端与服务端因此拼出
    **逐字段相同**的公开输入，证明才验得过。nonce 与 nullifier 秘密只在这一次调用里
    出现，出了函数就拼不出与密文匹配的见证。
  - **实测**：初始化 7.6 秒（编译电路）、出证明 2.25 秒、证明 128 字节。端到端证据：
    `tests/e2e/wasm_prover_test.go` —— node 加载**门户将要加载的那个 WASM 模块**，
    服务端配真 verifying key，改一个字节的证明必须 400 且不落库，计票还原出客户端
    选的选项。
  - **代价**：模块约 11.9 MB（gzip 后约 2.8 MB）+ proving key 1.2 MB，首次出票要下载。
    两者由 `make web-wasm` 与 `make web-proving-key PK=<路径>` 放进门户静态目录，
    **都不进仓库**。proving key 只在客户端侧 —— 服务端仍然只拿 verifying key。
  - ⚠️ **仓库里没有浏览器自动化环境**：Worker 里的 WASM 加载路径只在 node 下验过，
    真实浏览器渲染仍未验证。
- **服务端公开计票公钥**（新公开端点 `GET /api/v1/proposals/:id/election-key`）：返回
  **未压缩**形式的计票公钥（X 与 Y 各 32 字节大端，base64）—— 客户端因此不必实现点解压
  （那需要在标量域里求平方根）。
- **浏览器端现在能算出与电路一致的 nullifier。** 新增纯 TypeScript 实现
  （`web/src/lib/field.ts` `sha256.ts` `keccak.ts` `mimc.ts`，扩展 `nullifier.ts`）：
  SHA-256 把提案 ID 映射成域元素、legacy Keccak-256 链派生 91 个 MiMC 轮常量、
  Fr 上的 MiMC（轮函数 `(x+h+c)^5` + Miyaguchi–Preneel），与 Go 侧
  `ballot/nullifier.go` **逐位一致**。跨语言向量由
  `internal/infrastructure/crypto/ballot/nullifier_interop_test.go` 与
  `web/src/lib/mimc.test.ts` / `web/src/lib/nullifier.test.ts` 共同锁定，Go 侧还真的
  用这组值出了一份证明并验过 —— **浏览器算出的 nullifier，电路认**。
  - SHA-256 没用 WebCrypto 的 `crypto.subtle`：那要求安全上下文，门户被部署在内网
    http 地址下会直接不可用。
  - **门户已用上**：出票时 nullifier 由 `ballot.ProveBallot` 在本地随机秘密后派生
    （`MiMC(secret, proposal_id)`），秘密不出本地。
- **CLI 新增 `credential-claim` 命令**：匿名领取一张凭证（与门户的「领取凭证」同一个
  公开端点，**不需要 admin token**）。此前只有门户能领，脚本化领取用不了 —— 这是
  路线图 T9 / T22 里记下的缺口。⚠️ 领取请求里**没有任何身份信息**：连公钥都不发
  （公钥只在最后的 attest 一步才交给服务端），服务端因此无法把凭证与投票人对应起来。
- **并发投票压测工具**（新命令 `cmd/loadtest`）：只走 HTTP API 建提案 → 备池领凭证 →
  并发投票 → 复核防重投，输出吞吐 / 延迟分位 / 状态分布 / 防重投结论（`-json` 输出机器可读
  报告）。**防重投没守住时以非 0 退出** —— 结论能直接当门禁用。工具自身有单元测试
  （`cmd/loadtest/main_test.go`，用 `httptest` 模拟服务端），并补上了路线图 §4.3
  「并发投票下的防重投与性能容量」盲区里一直空着的**容量**那一半。
  ⚠️ 只能在 debug 模式跑（**压测工具自己没有 prover** —— 门户与 CLI 都有），且数字来自
  SQLite —— **不是容量规划**。
- **浏览器端的盲签名 client 侧已就绪**（新模块 `web/src/lib/blind-rsa.ts` + `sha384.ts`）：
  RFC 9474 RSABSSA-PSS-Deterministic 的 `Prepare` / `Blind` / `Finalize`，以及
  EMSA-PSS 编码、MGF1、SHA-384 —— 纯 TypeScript 实现，不依赖任何外部密码学包。
  与服务端 CIRCL `blindsign/blindrsa` **逐字节一致**（跨语言向量由
  `internal/infrastructure/crypto/blind_rsa_interop_test.go` 与
  `web/src/lib/blind-rsa.test.ts` 共同锁定）。
  - 关键事实：CIRCL 的 client 侧**只需要签发方的 RSA 公钥**，不需要私钥 ——
    这正是「两段式盲签名」能落地的前提。
  - ✅ **两段式协议与门户 / CLI 接线已完成**（见「安全」一节）。
- **浏览器端的凭证盲签名客户端**（新模块 `web/src/lib/credential.ts`）：`credentialMessage`
  （`sha256(user_id ‖ 0x00 ‖ user_pub_key)`）、`parseIssuerKey`、`blindCredentialMessage`、
  `finalizeCredentialSignature`。凭证消息与 Go 侧 `crypto.CredentialMessage` 同口径，
  跨语言向量由 `internal/infrastructure/crypto/blind_rsa_interop_test.go` 与
  `web/src/lib/credential.test.ts` 共同锁定。
- **Go 侧的盲签名客户端**（新模块 `internal/infrastructure/crypto/credential_client.go`）：
  `CredentialBlindClient.Blind` / `Finalize`，CLI 与端到端测试共用。只需签发方公钥，
  零新依赖（同库 CIRCL）。
- **PostgreSQL 集成测试可以在本地跑了**（新文件 `docker-compose.yml` + 新目标
  `make pg-test`）：起一个 `postgres:16-alpine` → 跑 `go test ./tests/integration/ -run Postgres`
  → 关库。集成测试同时支持**外部库模式**（设 `ANONVOTE_TEST_PG_HOST` 即连已有的
  PostgreSQL，每个测试开一个独立 schema 并在跑完时删掉）—— CI 走 testcontainers、
  本地走 compose，**两条路径共用同一份断言**。
- **生产部署清单**（新文档 `docs/deployment-checklist.md`）：把上线前要核对的项拆成
  可勾的条目 —— 配置来源（`CONFIG_FILE`）、必填环境变量、密钥就位与指纹、迁移先行、
  TLS、门户独立部署、投票运营流程（**解密会话必须在投票窗口打开之前初始化**）、
  启动日志自检与回滚。第一节就把「release 模式下**门户**投不出票（缺浏览器端 prover）」
  写在最前面 —— **这条不是部署能修的**（CLI 已可用）。
- **门户 7 个 `.vue` 面板第一次有了自动化测试**（`web/src/components/panels/panels.test.ts`，
  15 个用例）：此前它们**既没有类型检查也没有任何测试** —— `tsc` 不认 `.vue` 文件，
  模板里的笔误只会在运行时炸。现在用 Vue 的服务端渲染把面板渲染成 HTML 字符串，
  模板、setup、computed 都真跑一遍，顺带覆盖了 T2 的 debug 门控（非 debug 模式下
  「按选项生成凭据」按钮**不渲染**）。
  - 为了能在 vitest 里跑 `.vue`，`web/vitest.config.ts` 里**自写了 SFC 编译插件**：
    离线环境装不了 `@vitejs/plugin-vue`，但 `vue` 自带 `vue/compiler-sfc`，
    `typescript` 又是已有的 devDependency —— **不新增任何依赖**。
  - ⚠️ **不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。视觉与交互仍靠
    人工核对（Playwright 离线装不上）。
- `GET /healthz` 响应新增 `debug` 布尔字段（由 `server.mode` 推导）。
- **服务现在能用 `CONFIG_FILE` 显式指定配置文件**：`CONFIG_FILE=./configs/config.production.yaml`
  启动即可加载生产配置 —— 那个文件此前**永远不会被自动加载**（viper 只找 `config.yaml`），
  要用它只能改名覆盖开发配置。**显式指定的文件不存在时服务直接启动失败**，
  不会静默回退到开发配置。启动日志的 `effective configuration` 新增 `config_file`
  字段，打印实际加载的配置文件路径。

### 安全

- **凭证签发现在是真两段式盲签名：签发方在签名阶段看不到被签内容。** 此前 `Prepare` →
  `Blind` → `BlindSign` → `Finalize` 全在服务端进程内完成 —— 服务端全程持有被签消息，
  等同普通签名。现在客户端本地盲化，服务端只对**盲化消息**做一次 RSA 私钥运算。
  - **口径**：达成的是「**签名阶段**服务端只见盲化消息」。attest 阶段服务端仍然看得到
    `user_id` 与 `user_pub_key` —— 验签必须重算凭证消息
    （`sha256(user_id ‖ 0x00 ‖ user_pub_key)`）。这不是「全程看不见」，文档按前者写。
  - **交叉攻击被挡住**：拿 A 的盲签名去签 B，验签必然失败。
  - **公开的盲签名端点必须绑定池槽位**，否则签发方就是「任意消息」的签名预言机。
- **凭证池槽位的竞争由客户端解决。** 预留**不占位**（占位会让「只预留不 attest」的客户端
  永久消耗池槽位，变成 DoS），所以并发领取会抢同一个槽位；撞车的那个由客户端换一个槽位
  重来（最多 5 次，只在 409 `CREDENTIAL_EXISTS` 时重试 —— 409 的
  `CREDENTIAL_POOL_EMPTY` 不重试）。
- **投票路径现在会验证凭证签名。** 此前 `VerifyCredential` 只存在于密码学层，投票提交路径
  **从不调用** —— 任何人都能伪造凭据投票。现在落库前调用签发方验签，失败返回 **401**。
- **nullifier 落库前归一成规范形式（64 位小写 hex）。** 票表的唯一索引建在**字符串**上：
  客户端用大写或带空白的形式提交同一个 nullifier，若原样落库就能占掉多行、绕开唯一索引 ——
  防重投会失效。现在查重与落库都用规范形式。
- **`ballot-draft` 在非 debug 模式返回 404**（不是 403，避免暴露端点存在性）。该端点让服务端
  按明文选项加密，生产环境不该可达。debug 模式由 `server.mode`（`debug`/`dev`/`development`）推导。
  —— **后续已整个删除**（见上方「破坏性变更」）。
- **门户「按选项生成凭据」按钮在非 debug 模式隐藏**，改为一句说明为什么没有这个入口。
  —— **后续已整个删除**（见上方「破坏性变更」）。

### 变更

- **启动日志的 `effective configuration` 多了 `eligibility_enforced` 字段**，与
  `proof_verification_enabled` 并排 —— 运营一眼就能看到「一人一票到底开没开」。
- **HTTP 端点新增 1 个公开端点**：`POST /api/v1/proposals/:id/credentials/eligibility-challenge`
  （签发一次领取挑战，无状态）。
- **release 模式下 `cors_allowed_origins` 为空会打启动告警。** 门户与后端不同源时
  浏览器会拦掉所有跨域请求,这类事故在启动日志里看不出来。**不阻止启动** ——
  同源部署留空是合法的。
- **路线图目标版本从 v1.0.0 改为 v0.2.0。** `1.0.0` 在语义化版本里意味着
  「API 稳定 + 生产可用」，而 v0.2.0 发出去时证明链虽然通了、接口却仍会变，
  标成 1.0.0 是过度承诺。**版本号表达的是承诺强度，不是工作量** ——
  安全与匿名性的底线一条都没放松，go/no-go 清单照旧。
- `web/package.json` 的 `version` 从写死的 `1.0.0` 改为跟随仓库根的 `VERSION`。
- **路线图补上 v0.2.0 之后的版本线推演**（`docs/roadmap.md` §1.2 / §1.3）：按现有能力缺口与
  「v0.2.0 明确不做」的四类内容推演，现实预期是 **含 v0.2.0 自身在内共 3–4 档 0.x.0**
  （v0.3.0 接口冻结、v0.4.0 多租户与身份体系，必要时再插一档 v0.5.0），最后才是 1.0.0；
  同时给出晋级 1.0.0 的四条判据。**这是推演，不是承诺** —— 档数取决于接口何时能冻结。
- 新增 [`docs/errata.md`](./docs/errata.md)（勘误表）：登记已发布版本里的错误陈述。
  首条 E1 记录 v0.1.0 源码树中 4 个文件、7 处「v0.1.0 的 v1 密文会被 v0.1.0 拒绝」的
  错误措辞 —— v0.1.0 自身即 v2 密文格式，被拒绝的是 v0.1.0 **之前**产出的 v1 密文。
  **仅文档措辞，代码与已发布产物不受影响。**
- 覆盖率门槛的分档包从 9 个扩到 **11 个**（新增 `internal/infrastructure/crypto/ballot`、
  `cmd/keygen`），见 `scripts/coverage-thresholds.txt`。
- **CLI 命令从 14 个增到 16 个**（新增 `ballot-encrypt` 与 `credential-claim`），
  **HTTP 端点从 14 个增到 15 个**（新增公开的 `GET /api/v1/proposals/:id/election-key`）。
- **`credential-claim` 与 `credential-issue` 改走真两段式盲签名。** 命令的用法与输出格式
  不变（`credential-claim -proposal <id>` 仍是一句话领到凭证），但内部流程变成
  「预留 → 本地盲化 → 盲签名 → 本地解盲 → attest」。槽位被抢走时命令会换一个槽位重来
  （最多 5 次）。
- **CLI 的 HTTP 错误现在带状态码**（`*httpError`）：领取凭证时「槽位被别人抢走」是
  409 `CREDENTIAL_EXISTS`，客户端据此决定重试。
- **HTTP 端点从 15 个增到 19 个**：新增公开的 `credentials/blind-sign`、
  `credentials/attest` 与管理端的 `credentials/blind-sign`。

### 修复

- **名单工具在成员标识归一化后重名时会静默覆盖成员包**（2026-10-25 发版前排查时实测到）。
  成员包的文件名由标识归一化而来（字母数字与 `-_.` 之外的字符一律换成 `_`），于是
  **「张三-01」与「李四-01」会落到同一个文件** `eligibility-__-01.json`：
  - 不带 `-force` 时，工具写到第二个成员就报一句看不懂的 `already exists` ——
    但 **`root.json` 已经落盘了**，目录里只剩一份包，而 root 声称 2 名成员；
  - 带 `-force` 时更糟：工具**报成功**（退出码 0、「已生成 2 名成员」），root 写着
    `count = 2`，磁盘上却**只有 1 个包**，内容是后一个成员的 —— 前一个人的包被
    **静默覆盖**。
  - **这是「一人一票」级别的事**：资格包是「谁拿到谁就能领这一票」，运营方按 root
    声称的人数分发，被覆盖掉的那个人要么拿不到包、要么拿到**别人的**包 —— 两个人
    就能领同一票。
  - **修复**：撞名在**写盘前**就被拒绝（报错点名是哪两个标识撞到了哪个文件，并给出
    改法），**带不带 `-force` 都拒绝**；落盘顺序改为**成员包先写、`root.json` 最后写**
    —— 失败时目录里没有 root，不会留下一份「声称 N 人、实际 N-1 个包」的名单被误上线。
  - **回归测试**：`cmd/eligibility-list/main_test.go` 的
    `TestGenerateListRejectsCollidingFileNames` 与 `TestGenerateListWritesOnePackagePerMember`。
- **名单工具的产物格式没有任何测试守着**（2026-10-25 发版前排查时发现）。
  `cmd/eligibility-list` 落盘的 JSON 要被客户端与服务端读懂 —— 运营方跑工具生成名单 →
  root 配到服务端 → 成员包发给本人 → 本人用它领票。中间隔着一次序列化与一次反序列化，
  **字段名、编码方式、路径长度错任何一样，都要等到真实投票时才会暴露**（那时票已经开了）。
  之前的 `tests/e2e/eligibility_test.go` 用的是**在测试里手搓**的名单，走「同一个包直接
  构造」的路，看不见工具落盘那一层。
  - 现在 `cmd/eligibility-list/main_test.go` 从**文件读回**工具产物，跑完整的
    「工具产物 → 客户端出证明 → 服务端用 root.json 验签通过」往返，并钉住产物契约
    （文件名、字段、路径长度 = `TreeDepth`、成员包 0600、包与 root 必须成套）。
  - 该包此前**一个测试都没有**，现在也进了覆盖率受控包列表（门槛 82%，实测 87.6%）。
- **备池接口在大批量下必然失败（SQLite）**，2026-10-25 排查「备池 500」时挖出来的两处：
  - **一条 SQL 的绑定参数有上限**：`credential_claims` 有 7 列，一次性写 **4681 行以上**
    就报 `too many SQL variables` —— 而 `POST /credentials/pool` 允许一次投到 **10000**
    个槽位，**那条路径必然 500**。现在分批写（100 行一条语句，批次在同一个事务里），
    实测 10000 个槽位 72 毫秒写完。
  - **多行 INSERT 的主键去重会建临时索引**，索引撑破内存后溢写临时文件；本机
    （Windows，用户目录下的临时目录）实测这个临时文件建不出来 → 整条 INSERT 以
    `SQLITE_CANTOPEN` 失败，接口只报一句看不懂的
    `unable to open database file: The system cannot find the file specified.`。
    现在驱动的 ConnectHook 对**每条新连接**设 `PRAGMA temp_store=MEMORY`，临时表与
    临时索引不再落到文件系统上。
  - 两条各有回归测试：`internal/infrastructure/persistence/credential_claim_repository_test.go`
    的 `TestCredentialClaimRepository_SaveLargeBatch`（**用文件库 + UUID 长度的 id** ——
    内存库与短 id 都复现不出来）。
- **压测工具 `cmd/loadtest` 在 v0.2.0 的协议下完全跑不通**（2026-10-25 实测发现）。
  它还在用 v0.1.0 的「一步式领凭证」（`claim` 直接带 `user_pub_key`），而 v0.2.0 的凭证
  签发是**真两段式盲签名**（claim 只预留 → 本地盲化 + 服务端盲签名 → 交回解盲后的签名
  验签落库）—— 于是它领到的凭证**没有签名**，每张票都 400 `INVALID_REQUEST`、0 票入库。
  **报告看上去像「服务端扛不住」**（状态分布全是 400、落库核对 0）—— 而它是「服务端扛不扛得住」
  的唯一证据来源，坏掉就等于容量与防重投的结论整个失真。
  - 已改为走真实的 claim → blind-sign → attest 三步，与门户、CLI 同一条链路；
    槽位被别人抢走时（409 `CREDENTIAL_EXISTS`）换槽位重试。
  - **补了回归测试**：`cmd/loadtest/main_test.go` 的假服务端改为按 v0.2.0 的三段式协议实现
    （claim 只预留、blind-sign 签盲化消息、attest 验签落库），并断言**领取请求体里不许出现
    公钥** —— 哪天退回一步式，这条用例就红。
  - 顺带修掉一个**报告缺陷**：一轮比计时器分辨率还快时（本机约 1ms），耗时与吞吐测不准，
    旧代码会写出「0 req/s」（会被误读成「没跑」）。现在报告多一个
    `timing_resolution_limited` 标记，人类可读输出写「低于计时分辨率」。
- **门户对用户宣称「服务端能看到你选了哪个选项」—— 那是 v0.1.0 的口径，v0.2.0 已经不是了。**
  门户「术语参考」面板写着：「当前版本还没有真实的选票电路，开发模式下由服务端代生成」、
  「**当前版本密文由服务端生成，生成时服务端能看到你选的选项**；『服务端看不到选项』要等
  客户端本地加密落地」。**密文与证明现在都在浏览器本地生成**（`cmd/ballot-wasm` → 门户
  Worker），服务端从头到尾看不到选项、拿到数据库也还原不出来 —— 门户却还在说旧话。
  这是产品对外承诺说错，不是措辞问题。
  - 术语面板改成实话：`proof`（零知识证明在浏览器里生成，服务端只验签）、
    `encrypted_vote`（密文在浏览器里生成，服务端只见密文）、并补上
    `eligibility_proof`（一人一票的凭据，零知识、服务端只见名单的根）。
  - **加了守卫用例**（`web/src/components/panels/panels.test.ts`）：文案里再出现
    「服务端能看到你选的选项」「密文由服务端生成」「还没有真实的选票电路」这类说法就红。
    服务端那条底线由 `tests/architecture/` 守，**门户文案这一半现在也守了**。
- **README 与 techspec 的版本边界停在过渡口径。** README 的标题还写着「v0.1.0 内测版 +
  v0.2.0 证明链」、「客户端侧还差最后一段」—— 客户端侧（CLI 与浏览器）都已闭合，那句话
  已经不成立。改成按当前实现陈述，并**把「不建议直接用于真实投票」的理由换成本轮真实的
  缺口**（浏览器路径无 UI 自动化回归、PG 集成测试待 CI 实跑、真实名单待运营方提供、
  只有压测数据没有容量规划），指向发版计划与验收脚本。
  techspec §7.1 的门户能力清单里也还留着「**按选项生成投票草稿（仅 debug 模式可见）**」——
  那条路（服务端代加密）连同 `ballot-draft` 端点一起删掉了，**同一节下面几行却写着
  「『按选项生成凭据』按钮已删除」**，文档自相矛盾；已删掉那条。
- **门户 prover 的静态资源清单少算了一个。** 旧文案写「三个静态资源」，而 release 模式下
  是**四个**（`wasm_exec.js` / `ballot-prover.wasm` / `proving.key` /
  `eligibility-proving.key`）—— 漏了资格那把 key，运营方照着做会漏备。
- **门户的资格领取在真浏览器里必然失败：发给 prover Worker 的成员包是 Vue 的响应式 Proxy。**
  资格包存在 `ref` 里，读出来是 Proxy，而 `postMessage` 的结构化克隆克隆不了 Proxy ——
  浏览器直接抛 `DataCloneError: #<Object> could not be cloned.`，证明根本发不出去，页面提示
  「资格证明失败」。**node 驱动的 WASM 用例测不出来**（它不走 Worker），2026-10-25 的真实
  浏览器验收才暴露出来。
  - 现在拼请求体时把成员包**逐字段拷成普通对象**（在边界上保证可克隆）；前端新增一条回归
    用例：响应式 ref 里的成员包拼出的请求体必须不是 reactive，且能 `structuredClone` ——
    去掉修复即红。
- **`web/.gitignore` 漏了资格 proving key。** 旧的三件套（`ballot-prover.wasm`、
  `wasm_exec.js`、`proving.key`）都被忽略了，新增的 `public/eligibility-proving.key` 却**没有**
  —— 它是可以被误提交的密钥材料。现在与选票 proving key 同等对待：只进门户静态目录，不进仓库。
- **门户的盲签名变体名此前写错了，领取凭证这条路在真浏览器里是断的。** 服务端在
  `issuer_key` 响应里回的 `variant` 是 CIRCL `blindsign/blindrsa` 的**线路名**
  `RSABSSA-SHA384-PSS-Deterministic`，门户写的是 Go 侧的枚举标识符
  `SHA384PSSDeterministic` —— 两个不同的字符串。`parseIssuerKey` 因此直接抛错，流程停在
  预留之后（服务端只看到 claim，blind-sign 从未发出），界面上表现为「领取失败：签发方公钥
  不可用」；**匿名领取与管理端签发两条路都走不通**。CLI 不受影响（它比的是同一个库的
  `String()`），单测也测不出来（两边各拿自己的常量自证）—— 2026-10-25 的真实浏览器验收
  才暴露出来。
  - 门户改用线路名；Go 侧新增 `TestKAT_BlindRsaVariantName`、TS 侧新增一条断言，两边指向
    同一个字符串。
- **`result-publish -shares-file` 读不了 `decrypt-setup -out` 落盘的文件。** `decrypt-setup`
  写的是整个会话对象（`{"proposal_id","public_key","shares":[...]}`），而 `loadShares` 只认
  JSON 数组或纯文本行 —— 把刚生成的会话文件直接喂回去，会被当成一整行垃圾，服务端回
  400 `share does not belong to session`。现在可以直接吃这个文件（也仍接受 JSON 数组与
  纯文本行）。
- Release 正文改用 tag 里的发版说明，而不是自动生成的提交列表；正文从 GitHub API 读 tag 对象，
  不依赖 checkout 把 tag 落成什么形态的引用。

### 测试

- **浏览器端 prover 在真实浏览器里跑通了**（2026-10-25，Chromium / Electron）：门户静态目录的
  三个文件（`wasm_exec.js` / `ballot-prover.wasm` / `proving.key`）由浏览器真实加载，
  初始化 11.8~14.3 秒、出证明 2.9 秒、证明 128 字节；产出的密文 / nullifier / proof 被服务端
  以**真 verifying key** 验签通过（201），提案关闭后解密计票还原出浏览器端选的那个选项
  （`{"tally":{"candidate-B":1}}`）。凭证的匿名领取（claim → blind-sign → attest）也在同一
  浏览器里跑通。
- **并发下的防重投现在有回归测试了**（路线图 §4.3 的第三个盲区）：8 个请求**同时**用
  同一个 `nullifier`、各自带不同凭证提交，断言**恰好一张票入库**，其余全部被唯一
  索引拦成 409 `DOUBLE_VOTING`。这条覆盖的是 usecase「先查后写」查不住的那一半 ——
  真正的防线是 `votes.nullifier` 的唯一索引。
- **PostgreSQL 上真跑过了**（2026-10-25）：本机没有 Docker，但有便携版 PostgreSQL 17.7 的
  二进制 —— 用 `initdb` 起一个**临时独立集群**（trust 认证、独立端口、数据目录放在 gitignore
  的 `.gocache/` 下，不碰机器上原有的实例），走集成测试的**外部库路径**
  （`ANONVOTE_TEST_PG_HOST`）跑，**全过**：一人一票的复合唯一索引在 PG 上的行为证明
  （同键第二次写必须失败、换提案必须仍能写、同一提案下另一份资格也必须能写）、
  并发记同一份资格**恰好一个**成功、并发投票恰好一张票入库。
  `scripts/browser-acceptance.sh` 也新增了 `DB_DRIVER=postgres`：**整条浏览器验收在 PG 上
  跑通一遍** —— 导入资格文件 → 匿名领取 201 → 重复领取 409「这份资格已经领过票了 —— 一人一票」
  → 浏览器本地生成选票 → 提交 201 → 计票还原 `{"tally":{"beta":1},"total_votes":1}`。
  **生产用的就是 PostgreSQL** —— 此前所有浏览器证据都跑在 SQLite 上，这条补上了。
  ⚠️ **testcontainers 那条路径仍要 CI 实跑**（CI 的 go job 在 ubuntu-latest，自带 Docker；
  缺 Docker 时集成测试**失败而不是跳过**）。两条路径共用同一份测试代码、同一套断言。
- **压测数字按新协议重测**（2026-10-25）：`cmd/loadtest` 修好之后重跑 200 票 / 并发 20 →
  **164.1 req/s**（p50=3.56ms p90=325.6ms p99=1.107s），200 张全 201，同 nullifier 并发 5 次
  恰好放行 1 张、其余 4 张 409 `DOUBLE_VOTING`，落库核对 201 == 预期 201，退出码 0。
  旧的那组「257 req/s」是 v0.1.0「一步式领凭证」时期测的，**已作废**。
- **修掉一个 flaky 用例**：`TestAPI_VoteFlow_ConcurrentSameUser_E2E` 打在 SQLite 共享缓存的
  内存库上 —— 共享缓存用表级锁、`busy_timeout` 对它无效，两个并发投票会双双抛
  `database table is locked`，「恰好一张票入库」的断言偶发失败。改用文件库 + WAL
  （与 `concurrent_nullifier_test.go` 同一套 DSN）后连跑 5 次稳定通过。
- **「服务端不得拿到明文选项」从人工 grep 变成持续门禁**（新包 `tests/architecture/`，
  `plaintext_option_test.go`）：用 go/ast 扫 `internal/` —— HTTP 层请求体结构体不得有明文选项
  json tag；usecase 层不得有接受明文选项的入口；**服务端代加密那条链的符号不许复活**
  （`PrepareBallotDraft` / `EncryptVoteOption` / `BallotDraft` / `proposalOptionIndex`）；
  **ballot-draft 端点不许复活**（POST 它任何模式都 404）。
- **那条唯一豁免也删掉了**：服务端代加密的端点与代码链（`ballot-draft` → `EncryptVoteOption`）
  整体删除，CLI 的 `ballot-draft` 命令与门户的「按选项生成凭据」入口一并移除。**守卫测试
  不设豁免机制** —— 能力 A 的底线没有例外。

> **本轮还没有做到的事**（别把 v0.2.0 当成已完成的版本）：**一人一票已经落地**（T22，
> 资格证明），但它的**浏览器端路径没有自动化** —— 仓库里没有 Playwright，门户的资格领取
> 仍需真浏览器人工点一遍；门户面板整体也仍**没有浏览器自动化回归**（交互靠人工核对）；
> PostgreSQL 集成测试的 **testcontainers 路径仍待 CI 实跑**（外部库路径已在本机真 PG 17.7
> 上实跑通过）。凭证签发的两段式盲签名**已闭合**，
> 明文选项对服务端不可知这条**已彻底闭合**（无残留、无豁免）。
> 详见 [`docs/roadmap.md`](./docs/roadmap.md) §4.1 的能力状态表。

### CI 与发布

- **发布流水线现在会自己编浏览器端 prover，并检查门户包里有没有它**。
  v0.1.0 的门户由服务端代加密，不需要 prover；**v0.2.0 的门户在浏览器里出证明** ——
  没有 `ballot-prover.wasm` 与 `wasm_exec.js`，门户就**投不了票**。而这两个文件是
  **构建产物、不入库**（`web/.gitignore`），所以流水线必须自己编：
  `release.yml` 的 web job 增加了「编 wasm + 拷胶水」和「检查 `web/dist/` 里有没有
  prover —— **缺了直接失败**」两步。**不会发一个投不了票的门户。**
- **CI 的 web job 也补上了同样的构建与检查**（此前 CI 根本不构建门户）：门户构建从此
  在每个 PR 上都被验证，不再拖到发版那天才发现构建坏了。
- **两把 proving key 默认不进发布包** —— 它们是秘密材料，且每套部署必须用**与自家
  服务端那把 verifying key 配对**的 key，不能由发布流水线统一发出去。两条路：
  ① 把 base64 编码的 key 配成仓库 secret（`PORTAL_PROVING_KEY_B64` /
  `PORTAL_ELIGIBILITY_PROVING_KEY_B64`），流水线会注入进门户包；② 没配 secret 的话，
  **部署时必须自己把两把 key 补进 `web/dist/`**（部署清单 §1/§6 已写明）。流水线会
  检查 prover 在不在，**但 key 不在检查范围**（它本来就是秘密）。
- **新增 [`docs/release-plan-v0.2.0.md`](./docs/release-plan-v0.2.0.md)**：v0.2.0 的
  发版计划 —— 能力口径（A–I 对照 v0.1.0）、破坏性变更、部署侧必做项、本轮验收证据、
  go/no-go 清单与回滚口径。**发版动作本身（推分支 / 打 tag）已于 2026-10-25 拍板并执行。**

---

## [0.1.0] — 2026-10-20

首个版本。**内测版**：把「选票内容与投票人身份分离」的存储与接口骨架跑通，
但**零知识证明链尚未实现**，不要用于真实投票。边界详见 README 的「当前版本边界」。

### 修复

- 证明校验失败改为返回 400 `INVALID_REQUEST`，之前误返 500 并被记进生产错误日志。
- 门户凭据绑定目标选项：改选项或换提案会作废旧凭据，提交前也会校验一致性 ——
  修掉「界面显示 B、实际记下 A」的静默错票。
- CLI 与门户都不再发送 `selected_option`：服务端不消费该字段，而明文选项上服务端
  会抵消「选票是加密的」这个属性。
- CLI 的 URL 路径不再被二次转义，含空格或 `/` 的 proposal id 不再必然 404。
- CLI 的 `decrypt-setup` 强制要求 `-out`，门限解密份额不再默认打到 stdout。
- 门户的管理令牌改存 `sessionStorage`，不再明文留在 `localStorage`。
- 门户切换提案时重置「是否已投票」，错误提示不再 4.2 秒自动消失。
- PowerShell 版覆盖率脚本此前把 `$Profile` 当字面量文件名写出，现在正确插值。
- 管理令牌改为常数时间比较（先做 SHA-256 摘要再逐字节比，避免长度与前缀成为
  计时侧信道），并拒绝含 CR/LF/NUL 的畸形 `Authorization` 头 —— 此前
  `Bearer <token>\r\n` 里的尾随 CRLF 会被 `TrimSpace` 悄悄裁掉而当成合法头放行。
- 引导浮层补焦点陷阱与 Esc 关闭：此前焦点不进浮层，Tab 会跑到后面的页面里。
- 门户补「查询投票状态」按钮：此前该能力只在投票成功后的副作用里被触发，
  投票人无法在投票前主动查询。

### 变更

- `release` 模式下 `server.admin_bearer_token` 为空将直接启动失败，不再使用每次重启
  都会变的临时 token。
- 启动日志打印实际生效的配置（`server.mode` / `db.driver` / `db.database` /
  admin token 来源 / 是否开启证明验证），并且不再把配置态的长期 admin token 写进日志。
- `crypto.vk_path` 为空时启动明确告警「证明校验已关闭」。
- 覆盖率门槛脚本不再覆盖 `GOMODCACHE` / `GOCACHE`，CI 的依赖缓存不再失效。
- `cmd/cli` 新增 `version` 命令，共 14 个命令。
- **改表从服务启动里拆出来**：新增 `nullifier-server -migrate`（跑完迁移即退出），
  `release` 模式默认不在启动时改表。忘跑迁移时服务会**直接启动失败**并提示，
  而不是等第一个请求打进来才报「no such table」。迁移步骤不要求
  `CRYPTO_VK_PATH` 与 `SERVER_ADMIN_BEARER_TOKEN`。
- PostgreSQL 连接池参数显式配置（`db.max_open_conns` / `db.max_idle_conns` /
  `db.conn_max_lifetime_seconds`，默认 25 / 5 / 3600 秒），不再依赖
  `database/sql` 的「不限制连接数」默认值。
- **选票密文格式升到 v2**：密钥派生改用 HKDF-SHA256（RFC 5869，标准库
  `crypto/hkdf`），salt 每次加密随机生成并随密文传输，info 绑定协议版本、用途与
  双方公钥。**这是一次协议变更：v1 格式的旧密文（v0.1.0 之前产出）会被直接拒绝，
  升级后旧的加密选票无法解密。**
  > **勘误（2026-10-21）**：本条在 v0.1.0 发布时写的是「v0.1.0 的旧密文（v1 格式）会被
  > 直接拒绝」—— **那是错的**。v0.1.0 自身即 v2 密文格式，被拒绝的是 v0.1.0 **之前**产出的
  > v1 密文；v0.1.0 与 v0.2.0 之间**没有**已知的密文格式不兼容。
  > 详见 [`docs/errata.md`](./docs/errata.md) E1。
  >
  > **再勘误（2026-10-25，v0.2.0 开发期）**：上面那句「v0.1.0 与 v0.2.0 之间没有已知的密文
  > 格式不兼容」**已被证伪** —— v0.2.0 的选票密文从 v2 升到 v3，**v0.1.0 的旧密文在 v0.2.0
  > 里解不开**。原因与影响见本文件 0.2.0 段的「破坏性变更」。
- **凭证改为「凭证池 + 投票人匿名随机领取」**：admin 先往池里批量投放预签凭证，
  投票人自己随机领一张（`POST /api/v1/proposals/:id/credentials/claim`，
  公开）。此前由 admin 逐个分发，等于运营方天然掌握「哪个凭证给了谁」，
  直接抵消不可关联性。**代价：服务端不再能强制一人一票** —— 随机领取意味着
  服务端看不出同一个人领了几次，资格证明是后续要补的。池空时返回 409
  `CREDENTIAL_POOL_EMPTY`。

### 文档

- README 补「当前版本边界」告示，匿名性描述与实现对齐（原描述与实际行为相反）。
- **盲签名如实描述**：文档里写明盲化步骤在服务端内部完成，**当前不具备真正的
  盲签名性质**（签发方看得见被签内容），真正的两段式盲签名是 v1.0.0 的任务。
- 覆盖率徽章改为全仓口径（74.1%），正文区分门槛口径（79.0%）与全仓口径。
- 新增 `docs/project-map.md`（分层上下文索引）、`docs/release-plan-v0.1.0.md`（v0.1.0 发版计划）、
  `docs/release-review-2026-10-20.md`（发版前整体 review）、
  `docs/roadmap.md`（产品路线图：v1.0.0 的能力清单、里程碑与决策门）。

> 注：上面的「v1.0.0」是 v0.1.0 发布当时的路线图目标。该目标版本后来改名为
> **v0.2.0**，见上方 `[未发布]` 段。

### 测试

- 新增跑通真实 gnark 验证的 HTTP 层测试：有效 proof → 201、缺 `public_witness` → 400、
  被篡改的 `public_witness` → 400。之前所有 HTTP 测试用的都是空操作验证器。
- 新增门户 composable 测试（9 个用例）与投票凭据绑定规则单测（11 个用例）。
- 新增 `cmd/server` 冒烟测试：真的把服务拉起来、打一次 `/healthz`（200）再关停；
  另覆盖 `-migrate` 独立迁移与 release 模式缺密钥时的启动失败。
  `cmd/server` 覆盖率从 0% 升到 77.4%。
- 新增 `cmd/cli` 命令级测试，覆盖全部 14 个命令的路径、方法、admin 头与落盘行为，
  以及服务端报错的映射方式。`cmd/cli` 覆盖率从 24.6% 升到 83.8%。

### CI 与发布

- CI 补上 `gofmt` 检查，并新增前端作业（lint / tsc / vitest）。
- **覆盖率门槛扩到 9 个包并改为逐包分档**：门槛值写在
  `scripts/coverage-thresholds.txt`，单个包覆盖率回退会直接让 CI 变红。
  此前门槛只跑 5 个包，`crypto` / `persistence` / `cmd/cli` / `cmd/server`
  都不在门槛内，它们的回退不会让 CI 变红。
- 新增 tag `v*` 触发的发布流水线：校验 tag 与 `VERSION` 一致，在各平台 runner 上原生
  构建（保留 cgo，否则默认的 sqlite 驱动无法启动），对产物做冒烟启动测试，打包并
  生成 sha256 校验和，最后创建 GitHub Release。
- 发布流水线的 macOS Intel 构建从 `macos-13` 改到 `macos-15-intel`：GitHub 已下线
  `macos-13` 标签，用它会让该 job 永远排队、整个发版卡住。
- 发布 job 补上 checkout：`gh release create --verify-tag` 要用本地 git 解析 tag，
  没有仓库会直接失败。Release 正文改用 tag 里的发版说明，而不是自动生成的提交列表。

### 已知限制

- 没有真实的选票零知识电路，`verifying.key` 无法由本仓库生成。
- 密文由服务端生成，服务端在生成凭据时就知道选了哪个。
- 盲签名的盲化在服务端内部完成，不具备真正的盲化性质。
- 凭证改为匿名随机领取后，**服务端无法强制一人一票** —— 需要资格证明兜底。
- 门户 7 个 `.vue` 面板仍无自动化测试。

[0.2.0]: https://github.com/jiangfire/nullifier/releases/tag/v0.2.0
[0.1.0]: https://github.com/jiangfire/nullifier/releases/tag/v0.1.0
