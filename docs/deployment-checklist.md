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
| release 模式下完成一次投票 | ✅ **CLI 可以**（本地加密 + 本地出证明）；✅ **门户也可以**（前提是静态资源备齐，见 §6） |

> ⚠️ **门户的 prover 需要三个静态资源就位**：`wasm_exec.js`、`ballot-prover.wasm`、
> `proving.key`。少任何一个，门户点「在浏览器本地生成选票」都会报错
> （「下载 WASM 模块失败」/「下载 proving key 失败」），**服务端不受影响**。
> 准备方式见 §6 与 §1。
>
> **proving key 要分发给投票人**（CLI 用户用它跑 `ballot-prove`；门户把它放在静态目录里
> 让浏览器下载）。它**不进服务端、不入库** —— 服务端拿到它就能替任何人造出合法密文与
> 证明。拿到它的人可以自己出证明，但证明只绑定密文、不绑定身份，因此这不构成新的
> 身份泄露。
>
> **首次出票偏慢**：浏览器要下载约 2.8 MB（gzip）的 wasm 与 1.2 MB 的 proving key，
> 初始化证明器约 8 秒、出证明约 2 秒。这三段都在 Web Worker 里，主线程不卡。
>
> **门户的浏览器路径仍未在真实浏览器里验过**：仓库里没有浏览器自动化环境，
> 端到端证据是用 node 驱动**同一个 WASM 模块**跑出来的（`tests/e2e/wasm_prover_test.go`）。

---

## 1. 构建产物

| 项 | 要求 | 状态 |
|---|---|---|
| 服务端二进制 | `go build ./cmd/server`（或用 release workflow 的产物） | ☐ |
| 门户静态资源 | `make web-wasm`（产出 `web/public/ballot-prover.wasm` + `wasm_exec.js`）→ `pnpm --dir web run build` → `web/dist/` | ☐ |
| 门户的 proving key | `make web-proving-key PK=keys/proving.key` → `web/public/proving.key`（**只给浏览器，不给服务端**） | ☐ |
| CLI（可选） | `go build ./cmd/cli` | ☐ |
| 压测工具（可选） | `go build ./cmd/loadtest` | ☐ |

- **`cmd/server` 不提供静态托管**：门户要单独部署（nginx / Caddy / 对象存储）。
- 门户构建时 `web/package.json` 的 `version` 由发布流水线从仓库根的 `VERSION` 注入，
  **不要手改**。
- **构建顺序**：先 `make web-wasm`（把 wasm 与胶水放进 `web/public/`），再
  `pnpm --dir web run build`（rsbuild 会把 `public/` 原样拷进 `web/dist/`）。
  顺序反了，`web/dist/` 里就没有 prover 资源。

---

## 2. 密钥

| 项 | 要求 | 状态 |
|---|---|---|
| 选票证明**验证**密钥 | `go run ./cmd/keygen -dir ./keys` → `keys/verifying.key`，**服务端**要有 | ☐ |
| 选票证明**证明**密钥 | `keys/proving.key` —— **分发给投票人**（CLI 用它跑 `ballot-prove`），**不要**放进服务端，也别提交仓库 | ☐ |
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
| `DB_DRIVER=postgres` | 显式设置 | 落到 SQLite 文件库 |
| `DB_HOST` `DB_PORT` `DB_NAME` `DB_USER` `DB_PASSWORD` `DB_SSLMODE` | 按环境填 | 连不上库 |
| `DB_MAX_OPEN_CONNS` | 建议显式设置（默认 25） | 不限制连接数会打爆 PG 的 `max_connections` |
| `CRYPTO_ISSUER_KEY_PATH` | 凭证签发密钥落盘位置 | 用默认 `issuer_keys.json` |
| `CRYPTO_ISSUER_ROTATE_DAYS` | 密钥轮换周期（默认 90 天） | 长期不轮换 |

> `SERVER_CORS_ALLOWED_ORIGINS` 留空在 release 模式下**会打一条启动告警但不阻止
> 启动** —— 同源部署留空是合法的。不同源部署留空，门户会静默地什么都发不出去。

---

## 4. 数据库与迁移

| 项 | 要求 | 状态 |
|---|---|---|
| 先迁移再启动 | `nullifier-server -migrate`（**release 模式下服务不会自动改表**） | ☐ |
| 迁移跑在同一份配置下 | 迁移步骤不需要 `vk_path` 与 admin token | ☐ |
| 备份 | 迁移前对生产库做一次备份 | ☐ |

- **v0.2.0 的密文格式是 v3，与 v0.1.0 的 v2 不兼容** —— 旧的加密选票**解不开**。
  从 v0.1.0 升级时，发过票的提案要视为不可用，必要时清库。
- 表结构本身是向后兼容的（新增列 / 新增索引），但**旧密文不可用**这件事与 schema
  无关。

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
| **prover 资源就位** | `web/dist/` 里有 `ballot-prover.wasm`、`wasm_exec.js`、`proving.key` 三个文件（`make web-wasm` + `make web-proving-key`），且**能被浏览器下载到** | ☐ |
| **wasm 的 MIME 与体积** | `.wasm` 要以 `application/wasm` 返回（否则 `WebAssembly.instantiate` 可能失败）；体积约 11.9 MB，**建议开 gzip/brotli**（约 2.8 MB） | ☐ |
| **静态资源路径** | 门户部署在子路径下时，资源地址按 `document.baseURI` 解析 —— 别把 `wasm_exec.js` 放在与门户不同的域名下（Worker 的 `importScripts` 受同源限制） | ☐ |

---

## 7. 投票运营流程（按顺序）

| 步 | 动作 | 备注 | 状态 |
|---|---|---|---|
| 1 | 创建提案 | `POST /api/v1/proposals` 或 `cli proposal-create` | ☐ |
| 2 | 生成选票证明密钥 | `cmd/keygen`（**一次一把**，与提案共用同一把电路密钥）。`verifying.key` 部署到服务端；**`proving.key` 分发给投票人**（CLI 用它跑 `ballot-prove`；门户用 `make web-proving-key` 放进静态目录），**不要**进服务端镜像 | ☐ |
| 3 | 往凭证池投槽位 | `POST /api/v1/proposals/:id/credentials/pool`（管理端） | ☐ |
| 4 | **初始化门限解密会话** | `POST /api/v1/proposals/:id/decryption/setup`。⚠️ **必须在投票窗口打开之前** —— 服务端会拒绝「投票开放中」的提案 | ☐ |
| 5 | 开放投票窗口 | 提案的 `starts_at` / `ends_at` | ☐ |
| 6 | 投票 | ✅ **CLI**：`ballot-encrypt` → `ballot-prove -proving-key <proving.key>` → `vote-submit`；✅ **门户**：选好选项 → 点「在浏览器本地生成选票」→ 提交（首次约 12~14 秒，含编译电路）；开发模式也可用 `ballot-draft` 的占位 proof 联调 | ☐ |
| 7 | 收集门限份额 | 每个计票方拿自己那一份 | ☐ |
| 8 | 发布结果 | `POST /api/v1/proposals/:id/results/publish`（**仅在提案关闭后**）。CLI：`result-publish -proposal <id> -shares-file <decrypt-setup 落盘的文件>` —— 这个文件可以直接用 | ☐ |

- 第 4 步的时机是最容易写错的一步：**会话被覆盖 = 已投的票解不开**。重复 setup
  返回 `409 DECRYPTION_SESSION_EXISTS`，不会覆盖已有会话。
- 第 8 步：同一提案重复发布返回 `409 RESULT_ALREADY_PUBLISHED`；重复提交同一份 share
  返回 `400 INVALID_REQUEST`（不会绕过 threshold 计数）。

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

然后手工跑一遍：

| 检查 | 期望 | 状态 |
|---|---|---|
| `GET /healthz` | `{"status":"ok","debug":false}` | ☐ |
| `GET /api/v1/proposals` | 能列出提案 | ☐ |
| `POST /api/v1/votes`（伪造签名） | **401** `INVALID_CREDENTIAL` | ☐ |
| `POST /api/v1/votes`（篡改 proof） | **400** `INVALID_REQUEST` | ☐ |
| `POST /api/v1/proposals/:id/ballot-draft` | **404**（release 模式不可达） | ☐ |
| 领取凭证（无 admin token） | 能领到 —— 领取端点是公开的 | ☐ |
| 管理端接口（无 token） | **401** | ☐ |
| **浏览器里领一次凭证** | 「我的身份 → 匿名领取」成功，页面变成「已就绪」。**必须走到 attest**（服务端日志里能看到 `credential attested`）—— 停在 claim 之后说明浏览器端与服务端的盲签名参数对不上 | ☐ |
| **浏览器里出一张票** | 「参与投票 → 在浏览器本地生成选票」（首次约 12~14 秒）→ 提交成功，服务端日志出现 `vote submitted` | ☐ |

---

## 9. 已知边界（写给用的人，不是给部署的）

- **门户的浏览器端 prover 已在真实浏览器里跑通**（2026-10-25，Chromium / Electron）：
  静态目录的三个文件由浏览器真实加载 → 出证明（init 11.8~14.3 秒、prove 2.9 秒）→ 服务端
  以真 verifying key 验签通过（201）→ 提案关闭后计票还原出所选选项。
  但**仓库里仍没有浏览器自动化**（无 Playwright），这条链路**每次上线仍要人工点一次**
  「在浏览器本地生成选票」并提交一票 —— 而且这一步能抓到自动化抓不到的东西：跨语言字符串
  对不上（例如盲签名变体名）时，流程会停在半路，服务端只看到 claim、收不到 blind-sign。
- **服务端无法强制一人一票**：凭证是匿名随机领取的，同一个人可以领两张票投两次。
  资格证明（路线图 T22）尚未实现。
- **防重投靠 `votes.nullifier` 的唯一索引**。数据库层要真的有唯一约束 ——
  迁移没跑成功的库不会有它。
- **门限份额只返回一次、服务端不保存明文**。`decrypt-setup` 必须带 `-out` 落盘
  （`0600`），丢了就得重新 setup（而重新 setup 会撞 `409`）。
- **门户的 `.vue` 面板只有渲染测试，不是浏览器渲染**（Playwright 装不上）：模板与
  条件分支有自动化覆盖，但视觉与交互仍要人工核对。

---

## 10. 回滚

| 项 | 要求 |
|---|---|
| 二进制 | 保留上一个版本的产物与指纹 |
| 数据库 | 迁移前有备份；schema 变更向后兼容，不需要回滚迁移 |
| **数据** | ⚠️ **回滚到 v0.1.0 会让 v3 密文解不开** —— 回滚前确认库里的选票可以作废 |
