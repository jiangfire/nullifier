<h1 align="center">nullifier</h1>

<p align="center">
  匿名投票后端 —— 服务端数得清结果,数不出是谁投的。<br>
  <sub>Go · Gin · GORM · 零知识证明 · 门限加密 · Vue 3</sub>
</p>

<p align="center">
  <a href="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml"><img src="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-yellow.svg" alt="License: AGPL-3.0" /></a>
  <img src="https://img.shields.io/badge/go-1.25.4-00ADD8?logo=go&logoColor=white" alt="Go 1.25.4" />
  <img src="https://img.shields.io/badge/coverage-74.1%25%20(repo)-yellow.svg" alt="Coverage 74.1% (repo-wide)" />
</p>

---

## 服务端数得清结果,数不出是谁投的

投票系统最常见的做法是:一张 `votes` 表,一行一个人。这个设计的问题不在存储,而在于**服务器本身就是那个能作恶的角色** —— 它同时掌握选票内容和投票人身份,于是"我不记这一票"这句话无法被任何人验证。

nullifier 把这两个动作拆开:

- **发资格的人看不到选谁** —— 凭证用 RSA-PSS 签名签发(基于 RFC9474 盲签名库)。签发方看得到 `user_id` 和设备公钥(本来就是投票人提供的),但看不到选票内容:凭证与选票内容完全无关。
  ✅ 凭证签发本身已经是**真两段式盲签名**:客户端本地盲化,签发方在**签名阶段**只见盲化消息。(最后一次 attest 时签发方会看到公钥 —— 验签必须重算凭证消息,这与「看不到被签内容」不冲突。)
- **记选票的人看不到是谁** —— `votes` 表只有 `proposal_id`、`nullifier`、`cipher` 三列,没有用户标识。"是否参与"单独存进 `vote_participations`,且只存 `hash(user_id)`,不与票内容关联。
- **验票真伪但不验身份** —— 服务端用 gnark 验证零知识证明,证明"这张票是按合法凭证生成的",证明本身不携带任何身份信息。

结果是:管理员能看到计票结果和审计锚点,但拿到数据库也回答不了"某个人投了什么"。

> ### ⚠️ 当前版本边界（v0.1.0 内测版 + v0.2.0 证明链）
>
> 这个版本把「选票内容与投票人身份分离」的**存储与接口骨架**跑通了，并已接上**真实的
> Groth16 选票电路**（服务端能真实验签）。但**客户端侧还差最后一段**：请在下面的
> 「项目状态与限制」里读完整说明，不要把当前实现当成生产能力：
>
> - **服务端侧证明链已成立，CLI 已能在本地出证明，浏览器端 prover 还没有。** 电路、`cmd/keygen`
>   密钥生成、服务端按请求字段自拼公开输入并真实验签都已落地；**CLI 现在能自己出证明**
>   （`ballot-prove`，用本地 proving key，证明不经过服务端）。但**门户没有本地 prover** ——
>   因此 **release 模式下门户仍然无法完成一次投票**。
> - **选票密文已经能在客户端生成，CLI 已完整可用，门户还没接上。** 浏览器端（`web/src/lib/ballot-cipher.ts`）
>   与 CLI（`ballot-encrypt` 命令）都能在本地加密选票，服务端只提供公开的计票公钥；
>   **CLI 这条路已经能投出真票**（本地加密 + 本地证明，服务端真实验签）；
>   **门户尚未接线**（缺浏览器端 prover，接了也投不出去），debug 模式的 `ballot-draft`
>   仍是「服务端知道选项」的那条路径。
> - **只适合内测与开发**，不要用于真实投票。补齐客户端 prover 与门户接线的工作见
>   [`docs/roadmap.md`](./docs/roadmap.md) 的 v0.2.0 里程碑。

## 特性

**匿名性**

| 能力 | 说明 |
|---|---|
| 凭证签名 | RSA-PSS 签名(基于 RFC9474 盲签名库);**真两段式盲签名**:客户端本地盲化,签发方在签名阶段只见盲化消息,交叉攻击(拿 A 的盲签名签 B)验签失败 |
| 零知识验票 | 真实 Groth16 电路(BN254),证明「这张票加密了某个合法选项」且不携带身份 —— **服务端已能真实验签;CLI 已能本地出证明,门户的浏览器端 prover 还没有** |
| 客户端加密 | 选票密文在**客户端本地**生成(浏览器端 TS + CLI `ballot-encrypt`),服务端只提供公开的计票公钥 —— **CLI 已完整可用,门户还没接上** |
| 防重放 | 每票一个 `nullifier`,重复投票直接拒绝;nullifier 落库前归一成规范形式,变体写法绕不过唯一索引;nullifier 由秘密单向派生(浏览器端已能算出与电路一致的值) |
| 并发防重投 | 8 个请求同时用**同一个 nullifier** 提交时,数据库唯一索引兜底 —— 恰好一张票入库,其余 409(有回归测试) |
| 防冒充 | `proposal_id + user_id + user_pub_key + credential_signature` 四元组校验,并**验签凭证签名**(失败 401) |
| 选择性披露 | 只能回答"你投过票吗",不能回答"你投了什么" |

**计票与审计**

| 能力 | 说明 |
|---|---|
| 门限解密 | BN254 twisted Edwards 指数 ElGamal + Fr 上 Shamir 份额,2-of-3 份额即可恢复,单份份额无法解密 |
| 会话不可覆盖 | 重复 setup 返回 `409`,已投选票的解密能力不会被覆盖 |
| 幂等发布 | 重复 publish 返回 `409`,不重复写审计锚点 |
| 审计锚点 | 结果发布时生成 Merkle Root,可导出供外部核验 |
| 份额校验 | 拒绝重复 share,不会绕过 threshold 计数 |

**交付形态**

| 形态 | 说明 |
|---|---|
| HTTP API | 15 个 `/api/v1/*` 端点 + `/healthz`,管理面走 Bearer Token |
| CLI | 17 个命令,复用同一套 HTTP API,不绕过服务端校验（`ballot-prove` 除外 —— 它纯本地出证明） |
| Web 门户 | Vue 3 单页应用,覆盖投票端与运营端全流程 |

## 快速开始

需要 Go 1.25+。**Windows 上跑测试需要 `CGO_ENABLED=1`**(`gorm.io/driver/sqlite` 依赖 cgo)。

```bash
git clone https://github.com/jiangfire/nullifier.git
cd nullifier
go mod download
go run ./cmd/server
```

服务默认监听 `:8080`,用仓库自带的 `configs/config.yaml`(debug 模式,SQLite 文件库 `anonvote.db`,管理员令牌 `dev-admin-token`)。

### 走完一次完整投票

下面命令里的 `<proposal_id>` 替换成第 1 步返回的 id。**`decrypt-setup` 必须在提案 `starts_at` 之前完成**,否则服务端会以"投票开放中"拒绝。

```bash
# 1. 创建提案(options 是正式计票口径,至少两个)
go run ./cmd/cli -admin-token dev-admin-token proposal-create \
  -title "Board" -options "A,B" -start 2026-12-01T10:00:00Z -end 2026-12-01T12:00:00Z

# 2. 初始化 2-of-3 门限解密会话(必须在 -start 之前)
go run ./cmd/cli -admin-token dev-admin-token decrypt-setup \
  -proposal <proposal_id> -threshold 2 -trustees 3 -out shares.json

# 3. 准备凭证(二选一)
#    a) 管理员签发(需要 admin token,运营方知道票给了谁)
go run ./cmd/cli -admin-token dev-admin-token credential-issue \
  -proposal <proposal_id> -user u-1001 -out cred.json
#    b) 匿名领取(公开端点,不需要 admin token;服务端随机给一张池里的凭证)
#       —— 运营方不知道谁领了哪张,匿名性更好
go run ./cmd/cli credential-claim -proposal <proposal_id> -out cred.json

# 4. 在本地加密选票、出证明并投票
#    ballot-encrypt 在客户端本地加密:取公开的计票公钥 → 加密选项下标 → 落盘选票文件。
#    服务端全程拿不到明文选项。选票文件里的 nullifier_secret、nonce 等只留本地(提交时不会发出)。
#    -proof 省略时用开发占位值,只在开发模式(未配 crypto.vk_path)能过验签;
#    release 模式必须用下面 ballot-prove 产出的真实 proof。
go run ./cmd/cli ballot-encrypt -proposal <proposal_id> -option A -out ballot.json

#    ballot-prove 在**本地**用 proving key 生成 Groth16 证明:证明不经过服务端,
#    proving key 只在客户端(服务端只拿 verifying.key)。release 模式要投真票就得走这一步。
go run ./cmd/cli ballot-prove -ballot-file ballot.json -proving-key keys/proving.key -out ballot.json

go run ./cmd/cli vote-submit -credential-file cred.json -ballot-file ballot.json

#    备选:ballot-draft 让服务端代加密,只在 debug 模式可达(非 debug 模式返回 404),
#    服务端因此知道选了哪个 —— 这正是客户端加密要消除的路径。
go run ./cmd/cli ballot-draft -proposal <proposal_id> -option A -out ballot.json

# 5. 提案关闭后,提交份额并计票
go run ./cmd/cli -admin-token dev-admin-token result-publish \
  -proposal <proposal_id> -shares-file shares.json

# 6. 查结果与审计锚点
go run ./cmd/cli -admin-token dev-admin-token result-get -proposal <proposal_id>
go run ./cmd/cli -admin-token dev-admin-token anchors-list -proposal <proposal_id>
```

不想每次带 `-admin-token`,设个环境变量即可:

```bash
export NULLIFIER_BASE_URL=http://localhost:8080
export NULLIFIER_ADMIN_TOKEN=dev-admin-token
```

### Web 门户

```bash
pnpm --dir web install
pnpm --dir web run dev
```

打开 `http://localhost:3000`,顶部填服务地址和 Admin Token。页面分为投票大厅、我的身份、参与投票、计票与审计四块,覆盖领取/导入凭证、投票、初始化解密、发布结果、查看审计锚点。

## 架构

Clean Architecture 分层,依赖只朝内:

```text
cmd/server          服务入口          cmd/cli       CLI 客户端
     ↓                                    ↓
internal/usecase    业务编排(5 个用例) ──→ internal/domain   实体 + 仓储/密码学接口
     ↓
internal/infrastructure   http(Gin) · persistence(GORM) · crypto(gnark/kryptology)
     ↑
internal/di         Wire 装配          internal/config   viper 配置 + 运行期校验
```

| 层 | 职责 | 约束 |
|---|---|---|
| `domain` | 实体、仓储接口、密码学接口、领域错误 | 不依赖 HTTP / GORM / 具体密码学库 |
| `usecase` | Proposal / Register / Voting / Participation / Tally 五个用例编排 | 只依赖 `domain` 接口 |
| `infrastructure` | Gin 路由与错误映射、GORM 仓储与自动迁移、密码学实现 | 实现 `domain` 定义的接口 |
| `di` | Wire provider 组装 | 启动期一次性装配 |

**Stack:** Go 1.25 · Gin · GORM · Wire · gnark · kryptology · SQLite / PostgreSQL · Vue 3 · TypeScript · Rsbuild

## 配置

优先级:环境变量 > `configs/config.yaml` > 内置默认值。

| 环境变量 | 对应配置 | 默认值 | 说明 |
|---|---|---|---|
| `DB_DRIVER` | `db.driver` | `sqlite` | `sqlite` / `postgres` |
| `DB_HOST` | `db.host` | — | PostgreSQL 主机 |
| `DB_PORT` | `db.port` | — | PostgreSQL 端口 |
| `DB_NAME` | `db.name` | `anonvote` | PostgreSQL 数据库名 |
| `DB_PATH` | `db.path` | `anonvote.db` | SQLite 文件路径(设 `GO_ENV=test` 时强制内存库) |
| `DB_USER` | `db.user` | — | |
| `DB_PASSWORD` | `db.password` | — | |
| `DB_SSLMODE` | `db.sslmode` | — | 如 `disable` |
| `DB_MAX_OPEN_CONNS` | `db.max_open_conns` | `25` | 连接池上限,并发高时按 PG 的 `max_connections` 调整 |
| `DB_MAX_IDLE_CONNS` | `db.max_idle_conns` | `5` | 连接池闲置连接数 |
| `DB_CONN_MAX_LIFETIME_SECONDS` | `db.conn_max_lifetime_seconds` | `3600` | 连接最大存活时间(秒) |
| `CRYPTO_VK_PATH` | `crypto.vk_path` | 空 | **生产模式必填** |
| `CRYPTO_ISSUER_KEY_PATH` | `crypto.issuer_key_path` | `issuer_keys.json` | 签发密钥持久化 |
| `CRYPTO_ISSUER_ROTATE_DAYS` | `crypto.issuer_rotate_days` | `90` | 签发密钥轮换周期 |
| `SERVER_ADMIN_BEARER_TOKEN` | `server.admin_bearer_token` | 空 | 为空则启动时生成临时 token 并打印日志 |
| `SERVER_MODE` | `server.mode` | `release` | `release`/`prod`/`production` 时强制要求 `vk_path` |
| `SERVER_AUTO_MIGRATE` | `server.auto_migrate` | 非 release 模式 `true` | `release` 模式默认 `false`:改表是独立步骤 |
| `CONFIG_FILE` | —(不是配置项) | 空 | **显式指定配置文件路径**,如 `./configs/config.production.yaml`;指定的文件不存在时服务直接启动失败 |

生产部署参考 `configs/config.production.yaml` 与 `configs/production.env.example`。

`configs/config.production.yaml` **不会被自动加载**(viper 只找 `config.yaml`),用
`CONFIG_FILE` 显式指定:

```bash
CONFIG_FILE=./configs/config.production.yaml nullifier-server
```

这样不必改名覆盖开发配置;而且**文件不存在时服务会直接启动失败**,不会静默用开发配置跑起来。
启动日志的 `effective configuration` 会打印 `config_file`(实际加载的文件路径),
上线先核一眼这行。

`crypto.vk_path` 指向的选票电路验签密钥现在**可以由本仓库生成**:

```bash
go run ./cmd/keygen -dir ./keys      # 生成 verifying.key(0644) + proving.key(0600),打印 sha256 指纹
```

`verifying.key` 放服务端(`CRYPTO_VK_PATH` 指向它);**`proving.key` 只留在客户端侧,不要入库**。
默认不覆盖已有文件,要覆盖加 `-force`。

### 生产部署顺序:先迁移,再启动

`release` 模式下服务**不会**在启动时改表 —— 多副本同时启动会争 DDL 锁,而且改表应该是一个
能单独执行、能单独回滚的步骤。发布顺序是:

```bash
nullifier-server -migrate   # 迁移表结构,跑完即退出
nullifier-server             # 再起服务
```

迁移步骤只需要数据库连接,不要求 `CRYPTO_VK_PATH` 与 `SERVER_ADMIN_BEARER_TOKEN`。
如果忘了先迁移,服务会**直接启动失败**并提示跑 `-migrate`,而不是等第一个请求打进来才报
「no such table」。

需要临时恢复旧的「启动即迁移」行为时,设 `SERVER_AUTO_MIGRATE=true`。

## HTTP API

公共端点:

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/healthz` | 健康检查(响应含 `debug` 布尔字段) |
| `GET` | `/api/v1/proposals` | 列出全部提案(按创建时间倒序,含已关闭/未开始) |
| `GET` | `/api/v1/proposals/open` | 时间窗口内的开放提案 |
| `POST` | `/api/v1/votes` | 提交匿名投票(7 个字段全部必填;公开输入由服务端自拼,不收 `public_witness`) |
| `POST` | `/api/v1/proposals/:id/vote-status` | 持凭证材料查询是否已投票 |
| `GET` | `/api/v1/proposals/:id/election-key` | 计票公钥(公开值,客户端本地加密用;未压缩形式 base64) |
| `POST` | `/api/v1/proposals/:id/ballot-draft` | 生成选票草稿(开发联调,**仅 debug 模式**,其余模式 404) |
| `GET` | `/api/v1/proposals/:id/results` | 已发布的计票结果 |
| `GET` | `/api/v1/proposals/:id/audit-anchors` | 导出审计锚点 |
| `POST` | `/api/v1/proposals/:id/credentials/claim` | **预留**一个凭证池槽位(请求体为空,不收公钥) |
| `POST` | `/api/v1/proposals/:id/credentials/blind-sign` | 对**盲化消息**做盲签名(只覆盖池里还空着的槽位) |
| `POST` | `/api/v1/proposals/:id/credentials/attest` | 交回解盲后的签名,服务端验签后落库 |

管理端点,需 `Authorization: Bearer <admin token>`:

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/credentials` | 为指定的 `user_id` **预留**签发权(不收公钥) |
| `POST` | `/api/v1/credentials/blind-sign` | 对**盲化消息**做盲签名(可覆盖任意 `user_id`) |
| `POST` | `/api/v1/proposals` | 创建提案 |
| `GET` | `/api/v1/proposals/:id/credentials` | 该提案下凭证的 `has_voted` 状态 |
| `POST` | `/api/v1/proposals/:id/credentials/pool` | 往凭证池批量投放待领取槽位 |
| `POST` | `/api/v1/proposals/:id/decryption/setup` | 初始化门限解密会话 |
| `POST` | `/api/v1/proposals/:id/results/publish` | 提交份额并发布结果 |

## 开发

仓库根目录的 `Makefile` 是统一入口。Linux/macOS 直接 `make <target>`；Windows 在 Git for Windows 的 bash 里跑 `mingw32-make <target>`（Makefile 已自动处理 `CGO_ENABLED`）。

```bash
make help              # 列出全部目标
make check             # 提交前门禁:vet + gofmt + 前端 lint + tsc
make ci                # 复现 CI:go test ./... + 覆盖率门槛
make test              # 全部测试
make cover             # 覆盖率门槛(THRESHOLD=90 可覆盖默认值)
make web-lint web-typecheck web-test   # 前端检查
make loadtest ARGS="-votes 200 -concurrency 20"   # 并发投票压测(需要一个本地 debug 服务)
make pg-test          # PostgreSQL 集成测试(用 docker-compose 起一个库,跑完关掉)
```

等价的原始命令：

```bash
go build ./...                  # 构建
go vet ./...                    # 静态检查
gofmt -l ./cmd ./internal ./tests   # 格式检查(应无输出)
go test ./...                   # 全部测试
./scripts/check-coverage.ps1    # 覆盖率门槛(按包分档)
bash ./scripts/check-coverage.sh 80 coverage.out   # CI 实际执行的形式
```

CI 定义在 `.github/workflows/ci.yml`,执行 `go vet` + `gofmt` 检查 + `go test ./...` + 覆盖率门槛,并单独跑前端作业(lint / tsc / vitest)。PostgreSQL 集成测试走 testcontainers:本地 Docker 不可用时跳过,**CI 环境下 Docker 缺失会直接失败**,避免"跳过"被误读成"通过"。

打 `v*` tag 会触发 `.github/workflows/release.yml`:校验 tag 与 `VERSION` 一致 → 各平台 runner 原生构建(保留 cgo,默认的 sqlite 驱动依赖它)→ 产物冒烟启动测试 → 打包并生成 sha256 → 创建 GitHub Release。

覆盖率有两个口径,别混用:

| 口径 | 数值 | 覆盖范围 |
|---|---|---|
| 门槛口径 | **79.5%** | `scripts/check-coverage.sh` 选中的 11 个包:usecase / config / di / http / http-middleware / crypto / crypto-ballot / persistence / cmd-cli / cmd-keygen / cmd-server(2026-10-21 实测) |
| 全仓口径 | **74.1%** | `./...` 全部包(2026-10-20 实测) |

门槛是**逐包分档**的,每个包的最低覆盖率写在 `scripts/coverage-thresholds.txt`,单个包回退会直接让 CI 变红 —— 不再是"一个总和撑着,低覆盖率的包悄悄退化"。当前最低的一档是 `internal/infrastructure/persistence`(60.1%),抬门槛时它会先卡住。**新增代码请一并补测**。

## 项目状态与限制

这是个在推进中的实现,接手前请知道边界在哪:

- **证明链:服务端侧已通,CLI 侧也通了,门户还差一段。** 真实 Groth16 电路(`internal/infrastructure/crypto/ballot/`)、`cmd/keygen` 密钥生成流程、服务端按请求字段自拼公开输入并真实验签都已落地 —— 篡改任一公开输入都会验签失败。**CLI 现在能在本地生成 proof**(`ballot-prove`,用本地 proving key,证明不经过服务端),因此**命令行这条路能投出真票**;但**门户没有本地 prover**,因此 **release 模式下门户仍然无法完成一次投票**。`crypto.vk_path` 为空时验证器只要求 proof 非空(开发模式),本地「验证通过」不代表真的验过。
- **选票密文已经能在客户端生成,CLI 已完整可用,门户还没接上。** 浏览器端(`web/src/lib/ballot-cipher.ts`)与 CLI(`ballot-encrypt` 命令)都能在本地加密选票,服务端只提供公开的计票公钥(`GET /api/v1/proposals/:id/election-key`),落库记录里只有密文。两边的点编码**逐字节一致**,由一组跨语言已知答案向量锁死。**CLI 已能端到端投出真票**(本地加密 + 本地证明,服务端用真 verifying key 验签,见 `tests/e2e/client_prover_test.go`)。**但门户还没接上**(缺浏览器端 prover,接了也投不出去),debug 模式的 `ballot-draft` 仍是「服务端知道选项」的那条路径 —— 能力 A 因此尚未完全闭合。
- **门户的 nullifier 还是随机域元素。** 浏览器端已能按 `MiMC(秘密, 提案域元素)` 算出**电路认可**的 nullifier(纯 TypeScript 实现,与 Go 侧逐位一致,跨语言向量锁死,Go 侧还真的用这组值出过证明),但门户仍在生成随机 nullifier —— 那种 nullifier 按电路约束出不了证明。与客户端 prover 一起改。
- **凭证签名现在会被验证。** 投票落库前会调用签发方验签,伪造签名的请求返回 401。
- **凭证签发是真两段式盲签名。** 客户端本地盲化(`sha256(user_id ‖ 0x00 ‖ user_pub_key)`),签发方在**签名阶段**只见盲化消息 —— 它没有、也不需要私钥以外的任何客户端秘密。⚠️ 口径:最后一次 attest 时服务端会看到公钥与 `user_id`,因为验签必须重算凭证消息;达成的是「签发方看不到被签内容」,不是「服务端全程不知道客户端是谁」。
- **凭证不再由 admin 逐个分发。** 改为「凭证池 + 投票人匿名随机领取」：admin 往池里批量投放预签凭证，投票人自己随机领一张（`POST /api/v1/proposals/:id/credentials/claim`），运营方不再天然掌握「哪个凭证给了谁」。**代价：服务端无法强制一人一票** —— 随机领取意味着服务端看不出同一个人领了几次，资格证明是后续要补的。
- **`decryption/setup` 拒绝"投票开放中"的提案**,运营上必须在 `starts_at` 之前完成初始化。
- **生产部署必须显式设置** `SERVER_ADMIN_BEARER_TOKEN` 与 `SERVER_CORS_ALLOWED_ORIGINS`；`release` 模式下缺 admin token 会直接启动失败。`configs/config.production.yaml` **不会**被自动加载(viper 只找 `config.yaml`),用 `CONFIG_FILE=./configs/config.production.yaml` 启动即可 —— **显式指定的文件不存在时服务直接启动失败**,不会静默回退到开发配置;启动日志的 `config_file` 字段会打印实际加载的文件路径,上线先核一眼。详见该文件内的注释。

当前实现口径见 [`docs/techspec.md`](./docs/techspec.md),分层上下文索引见 [`docs/project-map.md`](./docs/project-map.md),产品路线图见 [`docs/roadmap.md`](./docs/roadmap.md)。**上线前请逐项核对** [`docs/deployment-checklist.md`](./docs/deployment-checklist.md)。

## 贡献

从 `main` 切分支,不要直接推默认分支。提交信息遵循 Conventional Commits:

```text
feat: add threshold decryption session lookup
fix: reject duplicate shares on publish
```

改动涉及接口、密码学或持久化逻辑时必须补回归测试。PR 请说明变更范围、测试结果与配置影响;改 API 附示例请求,改 `web/` 附截图。

## License

[AGPL-3.0](./LICENSE)

## 作者

[![jiangfire](https://github.com/jiangfire.png?size=100)](https://github.com/jiangfire) **[jiangfire](https://github.com/jiangfire)**
