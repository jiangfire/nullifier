# 生产部署清单

> 面向**部署 nullifier 服务端与门户**的人。每项都能在部署现场核对，核对完在
> 最后一列打勾。
>
> 这不是「功能说明」，是**上线前的最后一道闸** —— v0.1.0 的发版评审
> （`docs/release-review-2026-10-20.md`）里出的几起事故，全部落在下面这几行
> 上：配置没加载对、密钥没就位、迁移忘了跑、门户和后端不同源。
>
> 配套文档：接口与实现口径见 [`techspec.md`](./techspec.md)，分层上下文见
> [`project-map.md`](./project-map.md)，版本目标与能力边界见
> [`roadmap.md`](./roadmap.md)。

---

## 0. 先看清楚这个版本能做什么

| | 状态 |
|---|---|
| 服务端真实验签选票证明 | ✅ 可以（Groth16 / BN254） |
| 凭证盲签名（签发方只见盲化消息） | ✅ 可以 |
| 选票密文在客户端本地生成 | ✅ CLI 与门户都可以 |
| **客户端生成选票证明（proof）** | ✅ **CLI 可以**（`ballot-prove`）；✅ **门户也可以**（浏览器端 WASM prover） |
| **服务端强制一人一票（资格证明）** | ✅ **可以**（release 模式**必须**开启：`crypto.eligibility_root` 与 `crypto.eligibility_vk_path` **成对**配置，缺一个就启动失败） |
| release 模式下完成一次投票 | ✅ **CLI 可以**（本地加密 + 本地出证明）；✅ **门户也可以**（前提是静态资源备齐，见 §6） |
| **公开端点限流** | ✅ **可以**（v0.3.0 的 T25；release 模式**强制开启**，额度按「来源 + 路由组」计，见 §3.1） |

> ⚠️ **门户的 prover 需要四个静态资源就位**：`wasm_exec.js`、`ballot-prover.wasm`、
> `proving.key`、`eligibility-proving.key`。少任何一个，门户点「在浏览器本地生成选票」
> 或「匿名领取」都会报错（「下载 WASM 模块失败」/「下载 proving key 失败」），
> **服务端不受影响**。准备方式见 §6 与 §1。
>
> **proving key 要分发给投票人**（CLI 用户用它跑 `ballot-prove`；门户把它放在静态目录里
> 让浏览器下载）。它**不进服务端、不入库** —— 服务端拿到它就能替任何人造出合法密文与
> 证明。拿到它的人可以自己出证明，但证明只绑定密文、不绑定身份，因此这不构成新的
> 身份泄露。
>
> ⚠️ **选票与资格是两套电路、两把密钥**：Groth16 的 setup 是 per-circuit 的，
> `keys/proving.key` 与 `keys/eligibility-proving.key` **互不相通**，不能互相替代。
> 只配一把，门户的「匿名领取」或「本地生成选票」里必有一个会失败。
>
> **首次出票偏慢**：浏览器要下载约 2.8 MB（gzip）的 wasm 与 1.2 MB 的 proving key，
> 初始化证明器约 8 秒、出证明约 2 秒。这三段都在 Web Worker 里，主线程不卡。
> 开启资格校验后**还要多下载一把资格 proving key**（`eligibility-proving.key`），
> 并多一次资格证明的初始化与出证时间。
>
> **门户的浏览器路径有真实浏览器的自动化证据了**（2026-10-26，T28）：`make web-e2e`
> （`scripts/web-e2e.sh` + Playwright）在**真浏览器**里把门户的关键路径跑完 ——
> 领凭证 → 本地出票 → 提交 → 查状态 → 计票，以及「同一份资格重复领取被拒」。
> 端到端证据不再只到 node 驱动**同一个 WASM 模块**（`tests/e2e/wasm_prover_test.go`
> 与 `tests/e2e/wasm_eligibility_test.go`）。

---

## 1. 构建产物

| 项 | 要求 | 状态 |
|---|---|---|
| 服务端二进制 | `go build ./cmd/server`（或用 release workflow 的产物） | ☐ |
| 门户静态资源 | `make web-wasm`（产出 `web/public/ballot-prover.wasm` + `wasm_exec.js`）→ `pnpm --dir web run build` → `web/dist/` | ☐ |
| 门户的 proving key | `make web-proving-key PK=keys/proving.key` → `web/public/proving.key`（**只给浏览器，不给服务端**） | ☐ |
| 门户的**资格** proving key | `make web-eligibility-key PK=keys/eligibility-proving.key` → `web/public/eligibility-proving.key`（**只给浏览器，不给服务端**） | ☐ |
| CLI（可选） | `go build ./cmd/cli` | ☐ |
| 离线名单工具（可选） | `make eligibility-list` → 仓库根的 `eligibility-list` 二进制（**离线跑，不要放在投票服务机上**，见 §2.1） | ☐ |
| 压测工具（可选） | `go build ./cmd/loadtest` | ☐ |
| 审计复算工具（可选） | `go build ./cmd/audit-recompute` → 离线复算已发布的结果（**只读票库**，见 §8.2） | ☐ |

> ⚠️ **v0.3.0 的破坏性变更**：投票提交端点从 `POST /api/v1/votes` 改为
> `POST /api/v1/proposals/:proposal_id/votes`（请求体里不再有 `proposal_id`）。
> 旧路径**不再注册**（404）。**自建客户端必须改**；门户与 CLI 已同步。
> 详见 `CHANGELOG.md` 的「破坏性变更」与 [`docs/api-contract-v1.md`](./api-contract-v1.md)。

- **`cmd/server` 不提供静态托管**：门户要单独部署（nginx / Caddy / 对象存储）。
- **Windows 产物不需要额外装 MinGW**：发布包里的 `nullifier-server-windows-amd64.exe` /
  `nullifier-cli-windows-amd64.exe` 解压即用，**不依赖 `libgcc_s_seh-1.dll` 或
  `libwinpthread-1.dll`**（2026-10-25 实测 v0.1.0 已发布产物的导入表：只依赖
  `KERNEL32.dll` 与 Windows 10+ 自带的 UCRT）。Go 的内部链接器把 mingw 的支持库静态
  链进去了。Linux / macOS 产物依赖系统 libc/glibc —— 那是服务器本来就有的。
  （这条对应路线图 §6 的 **G6**：换纯 Go 驱动的「静态链接」收益在 Windows 上已经有了。）
- 门户构建时 `web/package.json` 的 `version` 由发布流水线从仓库根的 `VERSION` 注入，
  **不要手改**。
- **构建顺序**：先 `make web-wasm`（把 wasm 与胶水放进 `web/public/`），再
  `pnpm --dir web run build`（rsbuild 会把 `public/` 原样拷进 `web/dist/`）。
  顺序反了，`web/dist/` 里就没有 prover 资源。
- **两把 proving key 都要在 `web/public/` 就位**（`web-proving-key` 与
  `web-eligibility-key`），否则门户的「匿名领取」（要资格证明）与「在浏览器本地
  生成选票」各缺一块。
- **发布流水线产出的门户包里有什么**（`nullifier-web-<version>.zip`）：
  - ✅ **有** `ballot-prover.wasm` 与 `wasm_exec.js` —— 流水线自己编（非秘密，
    人人都能用同一份）。
  - ⚠️ **默认没有** `proving.key` 与 `eligibility-proving.key` —— 它们是秘密材料，
    且**每套部署必须用自己那对**（与服务端的 verifying key 配对），不能由发布
    流水线统一发出去。两条路：① 运营方把 base64 编码的 key 配成仓库 secret
    （`PORTAL_PROVING_KEY_B64` / `PORTAL_ELIGIBILITY_PROVING_KEY_B64`），
    流水线会注入进门户包；② 没配 secret 的话，**部署时必须自己把两把 key 补进
    `web/dist/`**，否则门户投不了票。
  - 流水线会检查 `web/dist/` 里有没有 prover —— **缺了就直接失败**，不会发一个
    投不了票的门户。

---

## 2. 密钥

| 项 | 要求 | 状态 |
|---|---|---|
| 选票证明**验证**密钥 | `go run ./cmd/keygen -dir ./keys` → `keys/verifying.key`，**服务端**要有 | ☐ |
| 选票证明**证明**密钥 | `keys/proving.key` —— **分发给投票人**（CLI 用它跑 `ballot-prove`），**不要**放进服务端，也别提交仓库 | ☐ |
| 资格证明**验证**密钥 | `go run ./cmd/keygen -circuit eligibility -dir ./keys` → `keys/eligibility-verifying.key`，**服务端**要有 | ☐ |
| 资格证明**证明**密钥 | `keys/eligibility-proving.key` —— **只给客户端**（CLI 用它出资格证明；门户用 `make web-eligibility-key` 放进静态目录），**不要**放进服务端，也别提交仓库 | ☐ |
| 资格名单树根 | `cmd/eligibility-list` 产出的 `root.json` 里的 `root`（64 位 hex）→ 填进 `crypto.eligibility_root`。**它不是密钥，但它是秘密的一半** | ☐ |
| 凭证签发密钥 | 由服务端首次启动时生成到 `crypto.issuer_key_path`（默认 `issuer_keys.json`），权限 `0600` | ☐ |

- `cmd/keygen` 会打印 `verifying.key` 的 **sha256 指纹** —— 把它记进部署记录，
  用来核对「服务端加载的确实是预期那把钥匙」。
- `cmd/keygen` 默认**不覆盖**已有文件，要覆盖得显式加 `-force`。
- **验证密钥换了，客户端的证明密钥必须同步换** —— 电路变了才需要；只换其中一把会
  导致所有 proof 验签失败。
- **proving key 不能保密到零**：拿到它的人可以自己出证明，但证明只绑定密文、不绑定
  身份，因此这不构成新的身份泄露。**它绝不能进服务端** —— 服务端拿到它就能替任何人
  造出合法密文与证明。
- 凭证签发密钥丢了 = 之前签发的凭证全部验不过。**它要备份**，且不要放进镜像。

### 2.1 资格材料的分发纪律（一人一票）

`cmd/eligibility-list` 的产物有**两种**，且**必须分开保管**：

| 产物 | 给谁 | 怎么给 | 泄露的后果 |
|---|---|---|---|
| `root.json`（树根） | **服务端**（填 `crypto.eligibility_root`） | 配置项 / 环境变量 | 服务端本来就只需要它 —— 它只能验证「这个叶子在这棵树里」，无法对应到任何成员 |
| `eligibility-<成员>.json`（成员包） | **本人** | **安全渠道**逐个发给本人（密信道 / 当面 / 带外） | ⚠️ **谁拿到谁就能领这一票** —— 拿到别人成员包的人可以冒名领票 |

- **资格包是秘密材料**：它含成员秘密、叶子与 Merkle 路径。不要放进邮件、IM、网盘
  同步目录；不要把整目录打包发给「所有人」。
- **root 与成员包必须分开保管**：root 给服务端，成员包给本人 —— 同一份手里同时有
  这两样，就等于同时掌握了「名单是哪些人」与「谁拿到了票」。
- **⚠️ 成员标识必须归一化后不重名**：包文件名由标识归一化而来（字母数字与 `-_.`
  之外的字符一律换成 `_`），所以**两个标识若归一到同一个文件名，工具会拒绝生成**
  （报错点名是哪两个标识撞了）。**现实名单里的中文标识正好会撞** ——
  「张三-01」与「李四-01」都归一成 `eligibility-__-01.json`。共用一个包名意味着
  有一个人拿不到自己的包（或拿到别人的），**一人一票当场失效**。
  → **标识请用只含字母数字与 `-_.` 的形式**（工号 / 选民编号 / 会员号通常如此）。
- **生成后核对一遍**：目录里 `eligibility-*.json` 的**个数**应当与 `root.json` 里的
  `count` **相等**。不等就说明产物不完整，**不要把 root 配到服务端**。
- **名单工具的运行机器不要与投票服务放在同一台机器上**：`cmd/eligibility-list` 是
  离线工具，跑在运营方（或独立登记处）的机器上，不联网、不连服务端。
- **名单可以换**：root 一换，服务端配置跟着换即可，**密钥不用重生成** ——
  前提是树深不变（`TreeDepth=10` 是编译期常量）。改树深就要重新编译并重生成密钥。
- **换名单后旧成员包失效**（root 对不上了），但**已经用掉的资格不会自动释放** ——
  `eligibility_uses` 里的记录不会因为换名单而清空。真要重新投票，开新提案。

### 2.2 部署顺序（一人一票上线）

按这个顺序走，**不要跳步**：

1. **生成资格密钥**（在密钥机上，离线）：
   `go run ./cmd/keygen -circuit eligibility -dir keys`
   → `keys/eligibility-verifying.key`（**给服务端**）与
   `keys/eligibility-proving.key`（**只给客户端**，绝不入库、绝不进服务端镜像）。
   选票那套另起一次：`go run ./cmd/keygen -dir keys` → `keys/verifying.key` +
   `keys/proving.key`。
2. **生成资格名单**（在名单机上，离线，与投票服务**不同机**）：
   `go run ./cmd/eligibility-list -members <权威名单.txt> -out <目录>`
   → `<目录>/root.json`（**给服务端配置**）与每成员一份
   `<目录>/eligibility-<成员>.json`（权限 0600，**走安全渠道发给本人**）。
3. **配服务端**：填 `crypto.eligibility_root`（`root.json` 里的 `root`）与
   `crypto.eligibility_vk_path`（步骤 1 的 `keys/eligibility-verifying.key`）。
   两项**必须成对** —— 只配一个，服务**启动失败**。
4. **迁移数据库**：`nullifier-server -migrate`（新增 `eligibility_uses` 表）。
5. **起服务端**：`GET /healthz` 的 `eligibility` 字段必须是 `true`。
6. **发布门户静态资源**：`make web-wasm` →
   `make web-proving-key PK=keys/proving.key` →
   `make web-eligibility-key PK=keys/eligibility-proving.key` →
   `pnpm --dir web run build` → 部署 `web/dist/`。
7. **成员领票**：CLI 走
   `go run ./cmd/cli credential-claim -proposal <提案> -eligibility eligibility-<成员>.json -proving-key keys/eligibility-proving.key -out cred.json`；
   门户在「我的身份」面板点「导入资格文件」后再点「匿名领取」。

---

## 3. 配置来源（**最容易出事的一项**）

| 项 | 要求 | 状态 |
|---|---|---|
| 配置文件路径 | 显式设置 `CONFIG_FILE=/path/to/config.production.yaml` | ☐ |
| 配置确实加载了 | 启动日志里的 `config_file` 字段是你给的那个路径 | ☐ |
| 没有静默回退 | `CONFIG_FILE` 指向的文件不存在时服务**直接启动失败**（这是设计如此） | ☐ |
| 环境变量覆盖 | 环境变量优先级高于配置文件 | ☐ |

> ⚠️ **viper 只自动找名为 `config.yaml` 的文件** —— `configs/config.production.yaml`
> **永远不会被自动加载**。三种正确用法：
> 1. `CONFIG_FILE=./configs/config.production.yaml`（推荐，不覆盖开发配置）
> 2. 把生产配置复制成 `configs/config.yaml`（会覆盖开发配置，先备份）
> 3. 完全用 `configs/production.env.example` 里的环境变量驱动
>
> 样例见 [`../configs/production.env.example`](../configs/production.env.example)。

### 必填环境变量

| 变量 | 要求 | 缺失后果 |
|---|---|---|
| `SERVER_MODE=release` | 必须是 `release` / `prod` / `production` | 不设就按开发模式跑，debug 入口可达 |
| `SERVER_ADMIN_BEARER_TOKEN` | 长期稳定的强随机值 | release 模式下**直接启动失败** |
| `SERVER_CORS_ALLOWED_ORIGINS` | 门户真实源，多个用**空格**分隔（不是逗号） | 门户与后端不同源时浏览器拦掉所有请求 |
| `CRYPTO_VK_PATH` | `verifying.key` 的路径 | release 模式下**直接启动失败** |
| `CRYPTO_ELIGIBILITY_ROOT` | 资格名单树根（64 位 hex 的域元素，来自 `root.json`） | release 模式下**直接启动失败** |
| `CRYPTO_ELIGIBILITY_VK_PATH` | 资格电路 verifying key（`eligibility-verifying.key`）的路径 | release 模式下**直接启动失败** |
| `DB_DRIVER=postgres` | 显式设置 | 落到 SQLite 文件库 |
| `DB_HOST` `DB_PORT` `DB_NAME` `DB_USER` `DB_PASSWORD` `DB_SSLMODE` | 按环境填 | 连不上库 |
| `DB_MAX_OPEN_CONNS` | 建议显式设置（默认 25） | 不限制连接数会打爆 PG 的 `max_connections` |
| `CRYPTO_ISSUER_KEY_PATH` | 凭证签发密钥落盘位置 | 用默认 `issuer_keys.json` |
| `CRYPTO_ISSUER_ROTATE_DAYS` | 密钥轮换周期（默认 90 天） | 长期不轮换 |
| `SERVER_TRUSTED_PROXIES` | **部署在反向代理后面时必填**（CIDR，空格分隔） | 不填则所有请求算到代理同一个 IP —— 限流会误伤真实投票人 |
| `RATE_LIMIT_*` | 按实际投票规模调额度（见 §3.1） | 额度太小会在投票窗口里把投票人挡在门外 |

> `SERVER_CORS_ALLOWED_ORIGINS` 留空在 release 模式下**会打一条启动告警但不阻止
> 启动** —— 同源部署留空是合法的。不同源部署留空，门户会静默地什么都发不出去。

> ⚠️ **资格校验的两个配置项必须成对出现**：`CRYPTO_ELIGIBILITY_ROOT` 与
> `CRYPTO_ELIGIBILITY_VK_PATH` 只配一个，**任何模式**下服务都会**启动失败**
> （「must be configured together」）；release 模式下**两个都缺**同样启动失败
> （一人一票是 release 的硬要求）。开发模式两个都不配 = **不启用**资格校验，
> 此时服务端可以匿名反复领票 —— 这不是 release 该有的状态。

> ⚠️ **限流在 release 模式是硬要求**：`RATE_LIMIT_ENABLED=false` 会让服务
> **启动失败**（默认就是开的）。公开端点上最贵的操作是投票 —— 每张票都要验一次
> Groth16 proof，不设上限时一个脚本就能把验签算力打满。

### 3.1 限流额度怎么定

**先理解限流是按什么计数的**：按**来源地址**（`server.trusted_proxies` 口径下的
客户端 IP）+ **路由组**。键里还带提案 ID —— A 提案的投票人不吃掉 B 提案的额度。

| 组 | 覆盖的端点 | 默认 rps | 默认 burst |
|---|---|---|---|
| `credentials` | 领取链路四步（challenge / claim / blind-sign / attest） | 10 | 40 |
| `votes` | 投票提交 | 10 | 40 |
| `read` | 提案列表 / 投票状态 / 计票公钥 / 结果 / 审计锚点 | 50 | 100 |

**默认值的推导**（一次完整投票：read 2 次 + credentials 3～4 次 + votes 1 次）：

- **500 人 / 5 分钟 / 共用一个出口 IP**（机构集中投票的典型形态）→
  credentials 组需 2000 次 → 约 3.5 分钟；votes 组需 500 次 → 约 50 秒。**可行**。
- 单人投票 → 毫秒级完成，远低于额度。

**所以：调额度的唯一理由是「你的投票规模/来源形态与默认假设不同」。**

- 投票人分散（各自家庭宽带）→ 默认值够用。
- 机构/学校集中投票（几百人共用一个出口 IP）→ **调高** `RATE_LIMIT_CREDENTIALS_*`
  与 `RATE_LIMIT_VOTES_*`。宁可放过一次攻击，也不要在投票窗口里把投票人挡在门外。
- 多副本部署 → 限流是**进程内**的，实际额度 = 配额的**副本数倍**，不用额外调。

> ⚠️ 调高额度前先看一眼 `anonvote_rate_limited_total` —— 它能告诉你
> 真实流量长什么样，而不是靠猜。

> 📊 **额度怎么算**：按「同一出口 IP 的峰值投票速率」推 —— `votes` 组 rps ≥ 峰值 × 1.5、
> burst ≥ 峰值 × 3；`credentials` 组要乘 4（每个投票人 4 个领取请求）；
> `read` 组 ≥ 峰值 × 2。**具体算例与各规模对应的额度见 §4.1 的配置表**
> （依据是实测容量报告 [`docs/capacity-report-v0.3.0.md`](./capacity-report-v0.3.0.md)）。

---

## 4. 数据库与迁移

| 项 | 要求 | 状态 |
|---|---|---|
| 先迁移再启动 | `nullifier-server -migrate`（**release 模式下服务不会自动改表**） | ☐ |
| 迁移跑在同一份配置下 | 迁移步骤不需要 `vk_path` 与 admin token（release 模式的三项硬要求只对正式服务生效；但 `eligibility_root` 与 `eligibility_vk_path` 的**成对**规则在任何模式下都生效） | ☐ |
| 新表 `eligibility_uses` | 由 `-migrate` 自动创建；唯一索引 `(proposal_id, qualification_id)` 就是一人一票的落地点。**迁移没跑成功就没有这张表** —— 一人一票就守不住 | ☐ |
| **新列 `credential_claims.reserved_at`** | v0.3.0 新增（凭证槽位的预留时间，TTL 10 分钟）。**从 v0.2.0 升级必须先跑迁移** —— 少了这一列，凭证池的占位机制不工作，并发领取会互相抢同一个槽位 | ☐ |
| **连接池要显式配** | `db.max_open_conns` / `db.max_idle_conns`（见 §4.1）。**不配的后果**：数据库/sql 默认只保留 2 条空闲连接，高并发下每个请求都要重开一条数据库连接 | ☐ |
| 备份 | 迁移前对生产库做一次备份 | ☐ |

- **v0.2.0 的密文格式是 v3，与 v0.1.0 的 v2 不兼容** —— 旧的加密选票**解不开**。
  从 v0.1.0 升级时，发过票的提案要视为不可用，必要时清库。
- 表结构本身是向后兼容的（新增列 / 新增索引），但**旧密文不可用**这件事与 schema
  无关。

### 4.1 资源配置建议（按投票规模）

**依据**：[`docs/capacity-report-v0.3.0.md`](./capacity-report-v0.3.0.md)（真 PostgreSQL
+ release 模式 + 真证明的实测）。

**实测能力**：单实例投票写路径 **2300 票/秒**（并发 50 时的峰值），
服务端只吃约 **1 个核**，数据库峰值 **11 条连接**、零锁等待。

**先算需求**：每个投票人要 4 个领取请求 + 1 个投票请求。

| 投票规模 / 窗口 | 需要的投票速率 | 需要的领取速率 | 单实例够不够 |
|---|---|---|---|
| 1 万人 / 10 分钟 | 17 票/s | 67 请求/s | 够（**100 倍**余量） |
| 1 万人 / 2 小时 | 1.4 票/s | 5.5 请求/s | 够 |
| 10 万人 / 2 小时 | 14 票/s | 56 请求/s | 够 |
| 10 万人 / 30 分钟 | 56 票/s | 222 请求/s | 够 |
| 100 万人 / 2 小时 | 139 票/s | 556 请求/s | 够 |

**因此**：

| 投票规模 | 投票服务 | 建议规格 | PostgreSQL |
|---|---|---|---|
| ≤ 1 万 | 1 台 | 2 核 / 4 GB | 2 核 / 8 GB |
| ≤ 10 万 | 1 台 | 2 核 / 4 GB | 2 核 / 8 GB |
| ≤ 100 万 | 1 台 + 1 台热备 | 4 核 / 8 GB | 4 核 / 16 GB（主备） |
| > 100 万 / 高度集中 | N 台（前面挂负载均衡） | 4 核 / 8 GB 每台 | 主备 + 读副本 |

**实例数怎么算**：`实例数 = 峰值投票速率 ÷ 2000`（留 15% 余量）。
按上表，**绝大多数部署 1 台就够** —— 投票服务不是算力密集型服务，
真正的算力在投票人的浏览器上（每张票出证明 23.8ms，服务端只验签 0.8ms）。

**三件必须配对的东西**（配错了不是「慢一点」，是「投不了」）：

| 项 | 怎么定 | 配错的后果 |
|---|---|---|
| **凭证池槽位数** | **≥ 投票人数**（每个投票人一张） | 池空了 → 409 `CREDENTIAL_POOL_EMPTY`，后面的人领不到票 |
| **限流额度**（同出口 IP 的集中投票） | `votes` 组 rps ≥ 峰值投票速率 × 1.5、burst ≥ × 3；`credentials` 组 rps ≥ 峰值投票速率 × 6（4 个请求/人）；`read` 组 rps ≥ 峰值投票速率 × 2。**例（1 万人 / 10 分钟）**：votes 30/60、credentials 120/240、read 40/80 | 429 `RATE_LIMITED` 把投票人挡在门外 |
| **数据库连接池** | `db.max_open_conns=16` / `db.max_idle_conns=16` | 不配 → 只保留 2 条空闲连接；开太大 → 冷启动掉到 307/s（见容量报告 §5.3） |

**数据库的 `max_connections`** 要 ≥ `实例数 × (16 + 5)`（16 是连接池，5 是迁移与运维余量）。

**投票服务的定位**：它**不是**算力密集型服务 —— 实测投票窗口内投票服务只用到约 1 个核，
数据库最多 11 条连接。**每张票的算力大头在投票人自己的浏览器上**（出证明 24ms/张，
服务端验签只要 0.8ms/张）。

**所以规模化的顺序是**：先确认「浏览器出证明够快」（现代笔记本都是毫秒级），
再按「投票窗口的峰值请求数 ÷ 单实例吞吐」算实例数，最后才是加机器。

---

## 5. 网络与 TLS

| 项 | 要求 | 状态 |
|---|---|---|
| HTTPS | **必须**。反代终止 TLS，后端监听内网地址 | ☐ |
| 反代头 | `X-Forwarded-*`（若后端需要据此判断） | ☐ |
| 管理端点隔离 | 管理面接口建议只在内网/专线可达 | ☐ |
| `Authorization` 头 | 反代**不要**剥掉 `Authorization` | ☐ |

> 明文字段（选项文本）本轮已经不走上网络了，但 admin token 与门限份额仍会经过
> 反代 —— 那条链路必须是 HTTPS。

---

## 6. 门户部署

| 项 | 要求 | 状态 |
|---|---|---|
| 独立托管 | `web/dist/` 交给静态服务器或 CDN | ☐ |
| 同源还是跨域 | 跨域时 `SERVER_CORS_ALLOWED_ORIGINS` 必须包含门户源 | ☐ |
| 管理令牌 | 门户把它存 `sessionStorage`（不落 `localStorage`），仍是明文 | ☐ |
| 浏览器要求 | 需要 `crypto.getRandomValues`（所有现代浏览器都有）；**不依赖** `crypto.subtle` | ☐ |
| **prover 资源就位** | `web/dist/` 里有 `ballot-prover.wasm`、`wasm_exec.js`、`proving.key`、`eligibility-proving.key` **四个**文件（`make web-wasm` + `make web-proving-key` + `make web-eligibility-key`），且**能被浏览器下载到**。⚠️ **用发布流水线的门户包时，后两把 key 默认不在包里**（秘密材料，每套部署用自己的）—— 见 §1 的说明 | ☐ |
| **wasm 的 MIME 与体积** | `.wasm` 要以 `application/wasm` 返回（否则 `WebAssembly.instantiate` 可能失败）；体积约 11.9 MB，**建议开 gzip/brotli**（约 2.8 MB） | ☐ |
| **静态资源路径** | 门户部署在子路径下时，资源地址按 `document.baseURI` 解析 —— 别把 `wasm_exec.js` 放在与门户不同的域名下（Worker 的 `importScripts` 受同源限制） | ☐ |

---

## 7. 投票运营流程（按顺序）

| 步 | 动作 | 备注 | 状态 |
|---|---|---|---|
| 1 | 生成资格密钥与名单 | `cmd/keygen -circuit eligibility`（**资格那把**）+ `cmd/eligibility-list -members <名单.txt> -out <目录>`。**详见 §2.2 的顺序** —— 这一步要在起服务端之前做完 | ☐ |
| 2 | 生成选票证明密钥 | `cmd/keygen`（**选票那把**，与资格那把是两套电路、两把钥匙）。`verifying.key` 部署到服务端；**`proving.key` 分发给投票人**（CLI 用它跑 `ballot-prove`；门户用 `make web-proving-key` 放进静态目录），**不要**进服务端镜像 | ☐ |
| 3 | 创建提案 | `POST /api/v1/proposals` 或 `cli proposal-create` | ☐ |
| 4 | 往凭证池投槽位 | `POST /api/v1/proposals/:id/credentials/pool`（管理端） | ☐ |
| 5 | **初始化门限解密会话** | `POST /api/v1/proposals/:id/decryption/setup`。⚠️ **必须在投票窗口打开之前** —— 服务端会拒绝「投票开放中」的提案 | ☐ |
| 6 | 开放投票窗口 | 提案的 `starts_at` / `ends_at` | ☐ |
| 7 | **领凭证（带资格证明）** | ✅ **CLI**：`credential-claim -proposal <id> -eligibility eligibility-<成员>.json -proving-key keys/eligibility-proving.key -out cred.json`；✅ **门户**：「我的身份 → 导入资格文件 → 匿名领取」。**资格不过不消耗池槽位** | ☐ |
| 8 | 投票 | ✅ **CLI**：`ballot-encrypt` → `ballot-prove -proving-key <proving.key>` → `vote-submit`；✅ **门户**：选好选项 → 点「在浏览器本地生成选票」→ 提交（首次约 12~14 秒，含编译电路）。**没有服务端代加密的路径** —— `ballot-draft` 已删除 | ☐ |
| 9 | 收集门限份额 | 每个计票方拿自己那一份 | ☐ |
| 10 | 发布结果 | `POST /api/v1/proposals/:id/results/publish`（**仅在提案关闭后**）。CLI：`result-publish -proposal <id> -shares-file <decrypt-setup 落盘的文件>` —— 这个文件可以直接用 | ☐ |

- 第 5 步的时机是最容易写错的一步：**会话被覆盖 = 已投的票解不开**。重复 setup
  返回 `409 DECRYPTION_SESSION_EXISTS`，不会覆盖已有会话。
- 第 10 步：同一提案重复发布返回 `409 RESULT_ALREADY_PUBLISHED`；重复提交同一份 share
  返回 `400 INVALID_REQUEST`（不会绕过 threshold 计数）。
- 第 7 步的**资格不过不消耗池槽位**：服务端先验资格证明，通过了才去池里取槽位
  （唯一索引冲突时槽位也不会被白白吃掉）。所以「资格文件不对」不会把凭证池耗光。

---

## 8. 上线后自检

启动日志里这几行要逐个核对：

| 日志字段 | 期望 | 状态 |
|---|---|---|
| `server_mode` | `release` | ☐ |
| `config_file` | 你指定的配置文件路径 | ☐ |
| `db.driver` | `postgres` | ☐ |
| `db.database` | 生产库名 | ☐ |
| `admin_token_source` | 来自环境变量/配置，**不是**临时生成 | ☐ |
| `proof_verification_enabled` | `true` | ☐ |
| `eligibility_enforced` | `true`（**一人一票开着**） | ☐ |

> 启动日志里的 `eligibility_enforced` 与 `GET /healthz` 的 `eligibility` 字段是同一件事：
> **这台实例到底有没有在强制一人一票**。两处都应该报「开着」。

然后手工跑一遍：

| 检查 | 期望 | 状态 |
|---|---|---|
| `GET /healthz` | `{"status":"ok","debug":false,"eligibility":true}` —— **`eligibility` 必须是 `true`**，否则这台实例没在强制一人一票（release 模式下它应该启动不了；`false` 说明配的不是你以为的那份配置） | ☐ |
| `GET /api/v1/proposals` | 能列出提案 | ☐ |
| `POST /api/v1/proposals/:id/votes`（伪造签名） | **401** `INVALID_CREDENTIAL` | ☐ |
| `POST /api/v1/proposals/:id/votes`（篡改 proof） | **400** `INVALID_REQUEST` | ☐ |
| `POST /api/v1/votes`（**旧路径**） | **404** —— 端点已收敛到提案路径，旧路径不再注册 | ☐ |
| `POST /api/v1/proposals/:id/ballot-draft` | **404**（端点已删除，任何模式都不可达） | ☐ |
| `GET /metrics`（无 admin token） | **401** —— 指标端点要 Bearer 令牌 | ☐ |
| `GET /metrics`（带 admin token） | Prometheus 文本格式；**不含**任何选票内容或身份 | ☐ |
| **限流真的生效** | 从同一个来源连打 `GET /api/v1/proposals` 50+ 次 → 出现 **429** + `Retry-After` 头；响应体是 `{"error":"rate limit exceeded","code":"RATE_LIMITED"}` | ☐ |
| **限流不影响正常投票** | 换个来源（或调高额度）后，同一个人的完整流程（领凭证 → 出票 → 提交）**一次都不该**撞上 429 | ☐ |
| `GET /metrics`（带 token，看限流计数） | `anonvote_rate_limited_total` 递增 | ☐ |
| 领取凭证（无 admin token） | 端点是公开的 —— **没有** admin token 也能调；但开启资格校验后**还必须带资格材料**，否则是 **400 `ELIGIBILITY_REQUIRED`** | ☐ |
| 管理端接口（无 token） | **401** | ☐ |
| **浏览器里领一次凭证** | 「我的身份 →（先导入资格文件）→ 匿名领取」成功，页面变成「已就绪」。**必须走到 attest**（服务端日志里能看到 `credential attested`）—— 停在 claim 之后说明浏览器端与服务端的盲签名参数对不上 | ☐ |
| **浏览器里出一张票** | 「参与投票 → 在浏览器本地生成选票」（首次约 12~14 秒）→ 提交成功，服务端日志出现 `vote submitted` | ☐ |
| **门户的这两条路径（自动化）** | `make web-e2e`（`scripts/web-e2e.sh`）在**真浏览器**里自动跑完上面两条 —— 领凭证 → 本地出票 → 提交 → 查状态 → 计票，并断言「同一份资格重复领取被拒」。CI 有独立 job（`web-e2e`）。⚠️ **自动化验的是「接线没断」，不是「部署形态没问题」** —— 真域名、反代、真实浏览器版本仍要靠上面那两行人工点 | ☐ |

### 8.1 一人一票的真实验收（两条都要跑）

**验收 A —— CLI**

```bash
# 1) 领一张（带资格证明）
go run ./cmd/cli credential-claim -proposal <提案> \
  -eligibility eligibility-<成员>.json \
  -proving-key keys/eligibility-proving.key -out cred.json
#    → 成功（201）

# 2) 同一个成员包再领一次
go run ./cmd/cli credential-claim -proposal <提案> \
  -eligibility eligibility-<成员>.json \
  -proving-key keys/eligibility-proving.key -out cred2.json
#    → 409 ELIGIBILITY_ALREADY_USED
```

**验收 B —— 浏览器**

> 这一段的前后活（两套密钥、资格名单、提案、凭证池、门限解密、门户 dev server）
> 已经固化成脚本：`bash ./scripts/browser-acceptance.sh setup` 会把环境备好并
> **逐条打印下面这些步骤**；点完之后跑 `bash ./scripts/browser-acceptance.sh verify`
> 核对服务端日志与计票结果，`stop` 收尾。
>
> 💡 **不想手动点就跑自动化**：下面这串点击已经被 Playwright 覆盖了 ——
> `make web-e2e`（`scripts/web-e2e.sh`）会在**真浏览器**里自动走完同样的步骤
> （包括「重复领取被拒」），并核对计票还原出所选的选项；CI 有独立 job（`web-e2e`）。
> ⚠️ 但**上线前仍建议手工点一遍**：自动化跑的是本地起的一套环境，
> 验的是「接线没断」；真域名、反向代理、静态资源的缓存头这些**部署形态**的问题
> 只有对着真实部署点才能发现。

1. 门户「投票大厅」→ 右上角「刷新」→ 在提案卡片上点「去投票」
   （**提案必须在投票大厅里选** —— 「参与投票」的选项列表来自大厅选中的提案，
   只手填 `proposal_id` 不会出现选项）；
2. 门户「我的身份」面板 →「导入资格文件」→ 选 `eligibility-<成员>.json`；
3. 点「匿名领取」→ 页面变成「已就绪」；
4. **重复导入同一个资格文件、再领一次** → 门户提示
   **「这份资格已经领过票了 —— 一人一票」**（服务端返回
   `409 ELIGIBILITY_ALREADY_USED`，前端文案见 `web/src/lib/error-messages.ts`）。

| 验收 | 期望 | 状态 |
|---|---|---|
| CLI 首次带资格领取 | 成功领到凭证 | ☐ |
| **CLI 重复领取（同一成员包）** | **409 `ELIGIBILITY_ALREADY_USED`** | ☐ |
| 浏览器导入资格文件 → 匿名领取 | 成功，页面显示「已就绪」 | ☐ |
| **浏览器重复领取（同一资格文件）** | **409 `ELIGIBILITY_ALREADY_USED`**（门户显示「这份资格已经领过票了 —— 一人一票」） | ☐ |
| 不带资格材料直接领 | **400 `ELIGIBILITY_REQUIRED`**（服务端强制一人一票，没有后门） | ☐ |
| 拿别人的成员包领 | 能领到 —— **资格包就是票**，拿到谁的就是谁的（这条不是缺陷，是分发纪律的前提） | ☐ |

> 这条路径在 **2026-10-25 已在真浏览器里人工验过一遍**（Chromium / Electron：导入资格文件 →
> 领取 201 → 重复领取 409 → 本地生成选票 → 提交 201 → 计票还原出所选选项）。仓库里**没有**
> 浏览器自动化，所以**每次上线仍要跑一遍**上面这张表。
>
> 💡 **要验生产库形态就把验收跑在 PostgreSQL 上**：`scripts/browser-acceptance.sh` 支持
> `DB_DRIVER=postgres`（配套 `PG_HOST` / `PG_PORT` / `PG_USER` / `PG_PASSWORD` / `PG_DATABASE`），
> 服务端会连真 PG 而不是 SQLite —— **生产用的就是 PostgreSQL**，SQLite 上验过不算数。
> 2026-10-25 已在 PostgreSQL 17.7 上跑通一遍（计票还原 `{"tally":{"beta":1},"total_votes":1}`）。
> PG 的库要**先存在**（脚本不建库）。
>
> ⚠️ 验收时注意：**「匿名领取」要等十几秒**（浏览器要下载并初始化 prover），中途别以为卡住；
> 若页面上出现 `Failed to execute 'postMessage' on 'Worker'`，说明门户这一版有 bug
> （资格包被当成响应式对象发给 Worker 了），**不要上线**。
>
> ⚠️ 「在浏览器本地生成选票」之前**一定要先把选项选对**：门户默认选中的是**第一个选项**，
> 默认点「生成」会投出第一个选项，不是你以为的那个。

> ⚠️ 「重复领取」必须在**同一个提案**下验：去重键是 `(proposal_id, qualification_id)`。
> 换到**另一个提案**时，同一份成员包要**重新出一份证明**（证明里绑了提案 ID），
> 新证明能领到 —— 也就是说**一个人可以在不同提案里各投一票**，这是设计如此。
> 把提案 A 的证明直接拿去领提案 B 会得到 **403 `INVALID_ELIGIBILITY_PROOF`**。

### 8.2 审计离线复算（投票结束后跑一遍）

**这一步是「已发布的结果可以被重算」的验收** —— 它不依赖投票服务在跑，直接读库。

```bash
# SQLite：
go run ./cmd/audit-recompute -proposal <提案ID> -shares-file <份额文件>

# PostgreSQL（读服务端同一份配置指向的同一个库；只读连接）：
CONFIG_FILE=./configs/config.production.yaml \
DB_DRIVER=postgres DB_HOST=... DB_NAME=... DB_USER=... DB_PASSWORD=... \
  go run ./cmd/audit-recompute -proposal <提案ID> -shares-file <份额文件>

# 机器可读的结论：
go run ./cmd/audit-recompute -proposal <提案ID> -shares-file <份额文件> -json
```

| 退出码 | 含义 | 该怎么办 |
|---|---|---|
| `0` | 复算结论与已发布结论**逐字节一致** | 通过 |
| `1` | **审计未通过** —— 票数 / 计票 / merkle 根 / 锚点哈希有对不上的地方 | ⛔ **不要发布结果**；先查库里是不是被改过，或复算用的份额是不是这个会话的 |
| `2` | 用法错误或运行失败（库打不开、份额不足、还原私钥失败） | 补参数后重跑；这不是「结论不一致」 |

| 验收 | 期望 | 状态 |
|---|---|---|
| 复算已发布的提案 | 退出码 `0`，输出「审计通过 —— 复算结论与已发布结论逐字节一致」 | ☐ |
| 用**别的**会话的份额复算 | 退出码 `2`，报「份额校验失败」（份额必须属于本会话） | ☐ |

> ⚠️ **份额是秘密材料**：门限份数凑齐就能还原全部选票的明文选项。份额文件用完请按
> 秘密材料处理，**不要进版本库、不要再发群里**。
>
> ⚠️ **复算工具与服务端共用同一份计票算法**（`internal/tally`）—— 所以它复算出来的
> 就是服务端当初算的那一份，而不是「另一套算法算的另一个结果」。锚点哈希的公式由
> 一条 KAT 测试钉死。
>
> ℹ️ **连接是只读的**：SQLite 走 `mode=ro`、PostgreSQL 走
> `default_transaction_read_only=on` —— 不是「我们保证不写」，是「写不了」。
> 它**不需要**投票服务在跑，也**不需要** admin token。

---

## 9. 已知边界（写给用的人，不是给部署的）

- **门户的浏览器端 prover 已在真实浏览器里跑通**（2026-10-25，Chromium / Electron）：
  静态目录的 prover 资源由浏览器真实加载 → 出证明（init 11.8~14.3 秒、prove 2.9 秒）→ 服务端
  以真 verifying key 验签通过（201）→ 提案关闭后计票还原出所选选项。
  ✅ **这条链路现在有自动化回归了**（2026-10-26，T28）：`make web-e2e` 在真浏览器里自动跑完
  「领凭证 → 本地出票 → 提交 → 查状态 → 计票」，CI 有独立 job。
  ⚠️ **但每次上线仍要人工点一次** —— 自动化验的是「接线没断」，人工点验的是**部署形态**
  （真域名、反向代理、真实浏览器版本、静态资源的缓存头）。
- **服务端可以强制一人一票了**（T22 资格证明已落地）：release 模式下服务端**强制**
  校验名单根与资格证明，没有资格证明就领不到凭证（`400 ELIGIBILITY_REQUIRED`），
  同一份资格在同一提案里第二次领是 `409 ELIGIBILITY_ALREADY_USED`。
  ⚠️ **门户的资格路径（导入资格文件 → 匿名领取）已在真实浏览器里验过两遍**（2026-10-25：
  SQLite 与 **PostgreSQL 17.7** 各一遍 —— 导入资格文件 → 领取 201 → 重复领取 409 → 本地生成
  选票 → 提交 201 → 计票还原所选选项）。
  ✅ **这条路径也有自动化回归了**（2026-10-26，T28）：`make web-e2e` 会自动断言
  「重复领取被拒（一人一票）」。
- **`cmd/loadtest` 在开启了资格校验的实例上不可用**：它不带资格材料，领取会一律返回
  `ELIGIBILITY_REQUIRED`。压测要么在**未开启**资格校验的开发实例上跑，要么改用别的
  压测方式（不要为了压测把生产实例的资格校验关掉）。
- **一人一票的粒度是「一个提案一票」**：去重键是 `(proposal_id, qualification_id)`。
  同一个人在**不同**提案里可以各投一票（见 §8.1 末尾的说明）。
- **防重投靠 `votes.nullifier` 的唯一索引**。数据库层要真的有唯一约束 ——
  迁移没跑成功的库不会有它。
- **门限份额只返回一次、服务端不保存明文**。`decrypt-setup` 必须带 `-out` 落盘
  （`0600`），丢了就得重新 setup（而重新 setup 会撞 `409`）。
- **门户的 `.vue` 面板现在有浏览器渲染回归了**（2026-10-26，T28）：`make web-e2e` 用
  Playwright 在**真浏览器**里跑门户的关键路径（CSS、事件绑定、客户端挂载全都在）。
  ⚠️ 但它只覆盖**关键路径**（领凭证 → 出票 → 提交 → 查状态 → 计票；资格重复领取被拒）——
  视觉细节、响应式布局、可访问性仍要人工核对（vitest 那套服务端渲染测试也只覆盖模板与状态机）。
- **限流已落地（T25）**：公开端点按「来源 + 路由组」限流，release 模式下**强制开启**
  （`RATE_LIMIT_ENABLED=false` 会让服务启动失败）。设计稿见
  [`t25-ratelimit-design.md`](./t25-ratelimit-design.md)。两条要知道的边界：
  - **多副本下额度按副本数放大** —— 限流是进程内的，实际额度 = 配额的副本数倍。
  - **`cmd/loadtest` 会被限流挡住**（所有请求来自同一台机器，共享一个来源额度）。
    压测容量时**先调高额度或关掉限流**，否则测出来的是限流阈值而不是服务端容量
    （关限流只允许在 debug 模式）。
- **容量规划已落地（T27）**：真 PostgreSQL + release 模式 + 真证明跑出的容量数字见
  [`docs/capacity-report-v0.3.0.md`](./capacity-report-v0.3.0.md)（单机实测约 2300 票/秒，
  并发 50 时到顶），**按投票规模的资源配置建议见 §4.1**。
  ⚠️ 复现时**每轮至少 500 票、每轮之前先热身一轮** —— 否则量到的是预热而不是容量。
  ⚠️ **面向有规模的真实投票仍需 v0.4.0** —— v0.3.0 解决的是接口稳定性与可运维性。
- **SQLite 只在开发/单机形态用**（生产是 PostgreSQL）。它有两条会打穿管理面接口的
  限制，服务端已经各自绕开（见 techspec §4.6）：备池写入按 100 行分批、临时表与
  临时索引固定放内存。**运维上只需知道一件事**：如果看到备池/投票报
  `unable to open database file`，先确认 SQLite 库的**所在目录**可写 ——
  这类报错不是业务逻辑错，是文件系统层面的事。

---

## 10. 回滚

| 项 | 要求 |
|---|---|
| 二进制 | 保留上一个版本的产物与指纹 |
| 数据库 | 迁移前有备份；schema 变更向后兼容，不需要回滚迁移 |
| **数据** | ⚠️ **回滚到 v0.1.0 会让 v3 密文解不开** —— 回滚前确认库里的选票可以作废 |
| **一人一票** | ⚠️ **回滚到没有资格证明的版本 = 服务端不再强制一人一票**（任何人都能反复领票）。`eligibility_uses` 表留着无害（旧版本不认识它）。若回滚窗口必须保住一人一票，就别回滚 |
