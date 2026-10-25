# Changelog

本文件记录 nullifier 的版本变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

版本号的唯一真相源是仓库根的 `VERSION` 文件；打 tag 时 release workflow 会校验两者一致。
前端的 `web/package.json` 也跟随 `VERSION`（发布流水线在构建前注入），不再各写各的。

---

## [未发布]

v0.1.0 发布之后的变更。下一个版本是 **v0.2.0**（路线图目标，见
[`docs/roadmap.md`](./docs/roadmap.md)）。

### 破坏性变更

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
  - **门户还没用上**：`useVotePortal.ts` 仍在生成随机 nullifier，等客户端 prover
    一起接线。
- **CLI 新增 `credential-claim` 命令**：匿名领取一张凭证（与门户的「领取凭证」同一个
  公开端点，**不需要 admin token**）。此前只有门户能领，脚本化领取用不了 —— 这是
  路线图 T9 / T22 里记下的缺口。⚠️ 领取请求里**没有任何身份信息**：连公钥都不发
  （公钥只在最后的 attest 一步才交给服务端），服务端因此无法把凭证与投票人对应起来。
- **并发投票压测工具**（新命令 `cmd/loadtest`）：只走 HTTP API 建提案 → 备池领凭证 →
  并发投票 → 复核防重投，输出吞吐 / 延迟分位 / 状态分布 / 防重投结论（`-json` 输出机器可读
  报告）。**防重投没守住时以非 0 退出** —— 结论能直接当门禁用。工具自身有单元测试
  （`cmd/loadtest/main_test.go`，用 `httptest` 模拟服务端），并补上了路线图 §4.3
  「并发投票下的防重投与性能容量」盲区里一直空着的**容量**那一半。
  ⚠️ 只能在 debug 模式跑（客户端还没有 prover），且数字来自 SQLite —— **不是容量规划**。
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
  启动日志自检与回滚。第一节就把「release 模式下任何客户端都投不出票（缺 prover）」
  写在最前面 —— **这条不是部署能修的**。
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
- **门户「按选项生成凭据」按钮在非 debug 模式隐藏**，改为一句说明为什么没有这个入口。

### 变更

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

- Release 正文改用 tag 里的发版说明，而不是自动生成的提交列表；正文从 GitHub API 读 tag 对象，
  不依赖 checkout 把 tag 落成什么形态的引用。

### 测试

- **并发下的防重投现在有回归测试了**（路线图 §4.3 的第三个盲区）：8 个请求**同时**用
  同一个 `nullifier`、各自带不同凭证提交，断言**恰好一张票入库**，其余全部被唯一
  索引拦成 409 `DOUBLE_VOTING`。这条覆盖的是 usecase「先查后写」查不住的那一半 ——
  真正的防线是 `votes.nullifier` 的唯一索引。

> **本轮还没有做到的事**（别把 v0.2.0 当成已完成的版本）：浏览器端 prover 未实现，
> **release 模式下门户仍然无法完成一次投票**；明文选项对服务端不可知这条**只闭合了一半** ——
> 客户端本地加密已可用（CLI `ballot-encrypt`、浏览器端 TS 实现），但**门户还没接上**，
> debug 模式的 `ballot-draft` 仍是「服务端知道选项」的那条路径；门户的 nullifier 也还是
> 随机域元素（浏览器端已能算出与电路一致的值，等 prover 一起接线）。凭证签发的两段式
> 盲签名**已闭合**。详见 [`docs/roadmap.md`](./docs/roadmap.md) §4.1 的能力状态表。

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

[0.1.0]: https://github.com/jiangfire/nullifier/releases/tag/v0.1.0
