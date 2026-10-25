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
| 选票密文在客户端本地生成 | ✅ CLI 可以（`ballot-encrypt`） |
| **客户端生成选票证明（proof）** | ❌ **没有任何客户端能生成** |
| release 模式下完成一次投票 | ❌ **不行** —— 缺客户端 prover |

> ⚠️ **release 模式下投不出票**：门户没有浏览器端 prover，CLI 也没有本地 prover ——
> `ballot-encrypt` 的 `-proof` 参数要由「本地 prover」提供，而它还不存在（默认的
> 开发占位值只在 `vk_path` 为空的开发模式下能过验签）。
>
> **密文这一半是通的**（客户端本地加密，服务端看不到明文选项），**缺的是 proof**。
> 因此目前**门户与 CLI 都无法在 release 模式下投出一次票**。
>
> 这一条不是部署能修的 —— 它是 v0.2.0 尚未完成的部分（路线图 T14 / T16）。
> 如果你的部署依赖投票，**先别把它当成上线阻塞项排期**，而是等 T14。
> 在那之前，可用的只有：领取凭证、查看提案、查询投票状态、开发模式下的联调投票。

---

## 1. 构建产物

| 项 | 要求 | 状态 |
|---|---|---|
| 服务端二进制 | `go build ./cmd/server`（或用 release workflow 的产物） | ☐ |
| 门户静态资源 | `pnpm --dir web run build` → `web/dist/` | ☐ |
| CLI（可选） | `go build ./cmd/cli` | ☐ |
| 压测工具（可选） | `go build ./cmd/loadtest` | ☐ |

- **`cmd/server` 不提供静态托管**：门户要单独部署（nginx / Caddy / 对象存储）。
- 门户构建时 `web/package.json` 的 `version` 由发布流水线从仓库根的 `VERSION` 注入，
  **不要手改**。

---

## 2. 密钥

| 项 | 要求 | 状态 |
|---|---|---|
| 选票证明**验证**密钥 | `go run ./cmd/keygen -dir ./keys` → `keys/verifying.key`，**服务端**要有 | ☐ |
| 选票证明**证明**密钥 | `keys/proving.key` —— **只在客户端侧**，不要放进服务端，也别提交仓库 | ☐ |
| 凭证签发密钥 | 由服务端首次启动时生成到 `crypto.issuer_key_path`（默认 `issuer_keys.json`），权限 `0600` | ☐ |

- `cmd/keygen` 会打印 `verifying.key` 的 **sha256 指纹** —— 把它记进部署记录，
  用来核对「服务端加载的确实是预期那把钥匙」。
- `cmd/keygen` 默认**不覆盖**已有文件，要覆盖得显式加 `-force`。
- **验证密钥换了，客户端的证明密钥必须同步换** —— 电路变了才需要；只换其中一把会
  导致所有 proof 验签失败。
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

---

## 7. 投票运营流程（按顺序）

| 步 | 动作 | 备注 | 状态 |
|---|---|---|---|
| 1 | 创建提案 | `POST /api/v1/proposals` 或 `cli proposal-create` | ☐ |
| 2 | 生成选票证明密钥 | `cmd/keygen`（**一次一把**，与提案共用同一把电路密钥） | ☐ |
| 3 | 往凭证池投槽位 | `POST /api/v1/proposals/:id/credentials/pool`（管理端） | ☐ |
| 4 | **初始化门限解密会话** | `POST /api/v1/proposals/:id/decryption/setup`。⚠️ **必须在投票窗口打开之前** —— 服务端会拒绝「投票开放中」的提案 | ☐ |
| 5 | 开放投票窗口 | 提案的 `starts_at` / `ends_at` | ☐ |
| 6 | 投票 | ⚠️ **release 模式下当前无客户端可用**（缺 prover，见 §0）；开发模式可用 `ballot-draft` 的占位 proof 联调 | ☐ |
| 7 | 收集门限份额 | 每个计票方拿自己那一份 | ☐ |
| 8 | 发布结果 | `POST /api/v1/proposals/:id/results/publish`（**仅在提案关闭后**） | ☐ |

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

---

## 9. 已知边界（写给用的人，不是给部署的）

- **release 模式下任何客户端都投不出票**（见 §0）—— 缺的是客户端 prover，不是密文。
- **服务端无法强制一人一票**：凭证是匿名随机领取的，同一个人可以领两张票投两次。
  资格证明（路线图 T22）尚未实现。
- **防重投靠 `votes.nullifier` 的唯一索引**。数据库层要真的有唯一约束 ——
  迁移没跑成功的库不会有它。
- **门限份额只返回一次、服务端不保存明文**。`decrypt-setup` 必须带 `-out` 落盘
  （`0600`），丢了就得重新 setup（而重新 setup 会撞 `409`）。
- **门户的 `.vue` 面板没有自动化测试**（Playwright 装不上），改界面靠人工核对。

---

## 10. 回滚

| 项 | 要求 |
|---|---|
| 二进制 | 保留上一个版本的产物与指纹 |
| 数据库 | 迁移前有备份；schema 变更向后兼容，不需要回滚迁移 |
| **数据** | ⚠️ **回滚到 v0.1.0 会让 v3 密文解不开** —— 回滚前确认库里的选票可以作废 |
