<h1 align="center">nullifier</h1>

<p align="center">
  匿名投票后端 —— 服务端数得清结果,数不出是谁投的。<br>
  <sub>Go · Gin · GORM · 零知识证明 · 门限加密 · Vue 3</sub>
</p>

<p align="center">
  <a href="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml"><img src="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-yellow.svg" alt="License: AGPL-3.0" /></a>
  <img src="https://img.shields.io/badge/go-1.25.4-00ADD8?logo=go&logoColor=white" alt="Go 1.25.4" />
  <img src="https://img.shields.io/badge/coverage-72.7%25%20(repo)-yellow.svg" alt="Coverage 72.7% (repo-wide)" />
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

> ### ⚠️ 当前版本边界（v0.2.0 的实现，**尚未发版**）
>
> 这个版本把「选票内容与投票人身份分离」整条链跑通了：选票的加密与证明都在客户端完成，
> 服务端只见密文；「一人一票」由零知识资格证明兑现。但**生产化验收还没全部闭合** ——
> 请在下面的「项目状态与限制」里读完整说明，不要把当前实现当成已经过生产验收的能力：
>
> - **服务端侧证明链已成立，CLI 与浏览器都能在本地出证明。** 电路、`cmd/keygen`
>   密钥生成、服务端按请求字段自拼公开输入并真实验签都已落地；**CLI 用本地 proving key
>   出证明**（`ballot-prove`），**浏览器也能**（`cmd/ballot-wasm` 编译成 WASM，门户在
>   Worker 里跑）。两条路的证明都不经过服务端。
> - **选票密文在客户端生成，CLI 与门户两侧都已接上。** 浏览器端与 CLI 都能在本地加密选票，
>   服务端只提供公开的计票公钥；**门户现在有「在浏览器本地生成选票」入口**。
>   服务端**没有任何**接收明文选项的入口（`ballot-draft` 已删除，守卫测试持续盯着）。
> - **一人一票已落地：领凭证要过一次零知识「资格证明」。** 运营方用离线工具把权威成员名单
>   做成一份 Merkle 承诺，**服务端只拿到名单的根** —— 它能验证「你有资格」，却**无法把你
>   对应到任何成员**。同一份资格在同一个提案里只能领一次（跨提案互不关联）。
>   ⚠️ **这是 release 模式的硬要求**：生产部署必须配好名单根与资格验证密钥，
>   否则服务**启动失败** —— 详见下面的「配置」。
> - **仍不建议直接用于真实投票** —— 还没闭合的是**生产化验收**与**运营材料**：
>   门户的浏览器路径**没有 UI 自动化回归**（每次上线仍要人工点一遍，脚本见
>   `scripts/browser-acceptance.sh`）；PostgreSQL 集成测试的 **testcontainers 路径待 CI 实跑**
>   （外部库路径已在本机真 PostgreSQL 17.7 上实跑通过，浏览器验收也在 PG 上跑通一遍）；
>   权威名单的真实内容与成员标识形式仍需运营方提供；容量只有压测数据、**没有容量规划**。
>   发版口径见 [`docs/release-plan-v0.2.0.md`](./docs/release-plan-v0.2.0.md)。

## 特性

**匿名性**

| 能力 | 说明 |
|---|---|
| 凭证签名 | RSA-PSS 签名(基于 RFC9474 盲签名库);**真两段式盲签名**:客户端本地盲化,签发方在签名阶段只见盲化消息,交叉攻击(拿 A 的盲签名签 B)验签失败 |
| 零知识验票 | 真实 Groth16 电路(BN254),证明「这张票加密了某个合法选项」且不携带身份 —— **服务端已能真实验签;CLI 与浏览器都能在本地出证明** |
| 客户端加密 | 选票密文在**客户端本地**生成(浏览器端 WASM prover + CLI `ballot-encrypt`),服务端只提供公开的计票公钥 —— **CLI 与门户两侧都已接上** |
| 防重放 | 每票一个 `nullifier`,重复投票直接拒绝;nullifier 落库前归一成规范形式,变体写法绕不过唯一索引;nullifier 由秘密单向派生(浏览器端已能算出与电路一致的值) |
| 并发防重投 | 8 个请求同时用**同一个 nullifier** 提交时,数据库唯一索引兜底 —— 恰好一张票入库,其余 409(有回归测试) |
| 防冒充 | `proposal_id + user_id + user_pub_key + credential_signature` 四元组校验,并**验签凭证签名**(失败 401) |
| 一人一票 | 领凭证要过一次**零知识资格证明**:运营方的权威名单做成 Merkle 承诺,**服务端只见名单的根** —— 它能验证「你有资格」,却无法把你对应到任何成员。同一份资格在同一个提案里只能领一次(跨提案互不关联)。**CLI** 用 `credential-claim -eligibility` 走这条路,**门户**在「身份」面板导入资格文件后本地出证明 |
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
| HTTP API | 18 个 `/api/v1/*` 端点 + `/healthz`,管理面走 Bearer Token |
| CLI | 16 个命令,复用同一套 HTTP API,不绕过服务端校验（`ballot-prove` 除外 —— 它纯本地出证明） |
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
#       服务端开了资格校验(生产模式必开)时还要带上资格证明:
#         -eligibility <资格文件> -proving-key <资格 proving key>
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

# 浏览器端 prover 的静态资源(先备齐,否则门户点「在浏览器本地生成选票」会报错)
make web-wasm                          # -> web/public/ballot-prover.wasm + wasm_exec.js
make web-proving-key PK=keys/proving.key   # -> web/public/proving.key(只给浏览器,不给服务端)
make web-eligibility-key PK=keys/eligibility-proving.key   # -> web/public/eligibility-proving.key
                                       #    (只在服务端开了资格校验时才需要)

pnpm --dir web run dev
```

打开 `http://localhost:3000`,顶部填服务地址和 Admin Token。页面分为投票大厅、我的身份、参与投票、计票与审计四块,覆盖领取/导入凭证、投票、初始化解密、发布结果、查看审计锚点。

投票路径:先领取凭证 → 选提案与选项 → 点「在浏览器本地生成选票」(首次约 10 秒:
下载并初始化证明器)→ 提交。密文、nullifier 与证明都在浏览器本地生成,服务端拿不到
你选了哪个选项。

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
| `CRYPTO_ELIGIBILITY_ROOT` | `crypto.eligibility_root` | 空 | 资格名单的 Merkle 根(十六进制)。**与 `CRYPTO_ELIGIBILITY_VK_PATH` 必须成对出现**;**生产模式必填** |
| `CRYPTO_ELIGIBILITY_VK_PATH` | `crypto.eligibility_vk_path` | 空 | 资格电路的验签密钥。**与 `CRYPTO_ELIGIBILITY_ROOT` 必须成对出现**;**生产模式必填** |
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

### 一人一票:资格名单与资格密钥

生产模式**必须**配好资格校验,否则服务启动失败。分三步:

```bash
# 1. 生成资格名单(离线工具,跑在运营方或独立登记处的机器上)
go run ./cmd/eligibility-list -members members.txt -out ./eligibility-out
#    -> eligibility-out/root.json                 部署到服务端(crypto.eligibility_root)
#    -> eligibility-out/eligibility-<member>.json  逐个走安全渠道发给本人

# 2. 生成资格电路的密钥(与选票电路的密钥互不相通)
go run ./cmd/keygen -circuit eligibility -dir ./keys
#    -> keys/eligibility-verifying.key  放服务端(crypto.eligibility_vk_path)
#    -> keys/eligibility-proving.key    只留在客户端侧,不要入库

# 3. 领凭证时带上资格文件
go run ./cmd/cli credential-claim -proposal <proposal_id> \
  -eligibility ./eligibility-out/eligibility-<member>.json \
  -proving-key ./keys/eligibility-proving.key -out cred.json
```

**服务端只拿到名单的根**:它能验证「这个资格在名单里」,却无法把资格对应到任何成员。
同一份资格在同一个提案里只能领一次(跨提案互不关联)。
⚠️ 资格文件是**秘密材料** —— 拿到它就能领这一票,发放必须走安全渠道;**丢了找不回来**。
名单规模上限 1024 人(树深是编译期常量)。

**没开启资格校验的部署**(两项都不配)行为与从前完全一样,`claim` 的请求体仍然可以为空 ——
但**生产模式不再允许这种配置**。`GET /healthz` 的 `eligibility` 字段、启动日志的
`eligibility_enforced` 字段,都会告诉你服务端到底有没有在强制一人一票。

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
| `GET` | `/healthz` | 健康检查(响应含 `debug` 与 `eligibility` 两个布尔字段) |
| `GET` | `/api/v1/proposals` | 列出全部提案(按创建时间倒序,含已关闭/未开始) |
| `GET` | `/api/v1/proposals/open` | 时间窗口内的开放提案 |
| `POST` | `/api/v1/votes` | 提交匿名投票(7 个字段全部必填;公开输入由服务端自拼,不收 `public_witness`) |
| `POST` | `/api/v1/proposals/:id/vote-status` | 持凭证材料查询是否已投票 |
| `GET` | `/api/v1/proposals/:id/election-key` | 计票公钥(公开值,客户端本地加密用;未压缩形式 base64) |
| `GET` | `/api/v1/proposals/:id/results` | 已发布的计票结果 |
| `GET` | `/api/v1/proposals/:id/audit-anchors` | 导出审计锚点 |
| `POST` | `/api/v1/proposals/:id/credentials/eligibility-challenge` | 签发一次领取挑战(公开、无状态);返回 `{proposal_id, root, depth, challenge}` |
| `POST` | `/api/v1/proposals/:id/credentials/claim` | **预留**一个凭证池槽位(不收公钥);开了资格校验时请求体带 `{eligibility_proof, qualification_id, challenge}`,资格不过不消耗槽位 |
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
| 门槛口径 | **81.1%** | `scripts/check-coverage.sh` 选中的 12 个包:usecase / config / di / http / http-middleware / crypto / crypto-ballot / persistence / cmd-cli / cmd-keygen / cmd-eligibility-list / cmd-server(2026-10-25 实测) |
| 全仓口径 | **72.7%** | `./...` 全部包(2026-10-25 实测) |

门槛是**逐包分档**的,每个包的最低覆盖率写在 `scripts/coverage-thresholds.txt`,单个包回退会直接让 CI 变红 —— 不再是"一个总和撑着,低覆盖率的包悄悄退化"。当前最低的一档是 `internal/infrastructure/persistence`(63.9%),抬门槛时它会先卡住。**新增代码请一并补测**。

## 项目状态与限制

这是个在推进中的实现,接手前请知道边界在哪:

- **证明链:服务端、CLI、浏览器三侧都通了。** 真实 Groth16 电路(`internal/infrastructure/crypto/ballot/`)、`cmd/keygen` 密钥生成流程、服务端按请求字段自拼公开输入并真实验签都已落地 —— 篡改任一公开输入都会验签失败。**CLI 能本地生成 proof**(`ballot-prove`,用本地 proving key,证明不经过服务端);**浏览器也能**(`cmd/ballot-wasm` 编译成 WASM,门户在 Web Worker 里加载它出票)。`crypto.vk_path` 为空时验证器只要求 proof 非空(开发模式),本地「验证通过」不代表真的验过。
- **选票密文在客户端生成,CLI 与门户两侧都已闭合。** 服务端只提供公开的计票公钥(`GET /api/v1/proposals/:id/election-key`),落库记录里只有密文。门户的出票入口是「在浏览器本地生成选票」;CLI 是 `ballot-encrypt` + `ballot-prove`。两边的点编码**逐字节一致**,由一组跨语言已知答案向量锁死。端到端证据:`tests/e2e/client_prover_test.go`(CLI)与 `tests/e2e/wasm_prover_test.go`(浏览器 WASM 模块,node 驱动)。**服务端没有任何接收明文选项的入口** —— `ballot-draft`(服务端代加密)已删除,`tests/architecture/` 持续盯着这条底线。
- **浏览器端 prover 的静态资源要部署时备齐。** `wasm_exec.js`、`ballot-prover.wasm`（约 11.9 MB，gzip 后约 2.8 MB）、`proving.key`（约 1.2 MB）、`eligibility-proving.key`（约 1.1 MB）—— **release 模式下是四个，缺一不可**：`make web-wasm`、`make web-proving-key PK=<路径>`、`make web-eligibility-key PK=<路径>`。首次出票要下载它们并初始化证明器（浏览器实测约 12~14 秒，含编译电路），出证明约 3 秒。**两把 proving key 只在客户端** —— 服务端只拿 verifying key；**它们不是同一把**（选票电路与资格电路各一套 setup）。⚠️ **发布流水线的门户包里默认没有那两把 key**（秘密材料，每套部署要用与自己服务端配对的那对）—— 部署时必须自己补进 `web/dist/`，详见 [`docs/deployment-checklist.md`](./docs/deployment-checklist.md) §1/§6。
- **门户的浏览器路径已在真实浏览器里跑过（2026-10-25）。** 端到端证据有三条：node 驱动同一个 WASM 模块的自动化用例（`tests/e2e/wasm_prover_test.go`），一次**真实浏览器**（Chromium / Electron）的人工验收（静态资源三件套真实加载 → 出证明 init 11.8~14.3 秒、prove 2.9 秒 → 服务端以真 verifying key 验签通过 201 → 提案关闭后计票还原出所选选项），以及把整套人工流程**固化成脚本**的 `scripts/browser-acceptance.sh`（`setup` 备好环境并逐条打印步骤 → 人在浏览器里点 → `verify` 核对服务端日志与计票 → `stop` 收尾）。⚠️ 但**仓库里仍然没有浏览器自动化**（无 Playwright），门户的 UI 交互仍要人工核对；首次上线仍建议跑一次 `bash ./scripts/browser-acceptance.sh setup` 人工点一遍 —— 上一次真实验收正是靠它挖出了一个跨语言 bug（盲签名变体名用了 Go 的枚举名而不是线路名，领取流程会停在 claim 之后）。
- **凭证签名现在会被验证。** 投票落库前会调用签发方验签,伪造签名的请求返回 401。
- **凭证签发是真两段式盲签名。** 客户端本地盲化(`sha256(user_id ‖ 0x00 ‖ user_pub_key)`),签发方在**签名阶段**只见盲化消息 —— 它没有、也不需要私钥以外的任何客户端秘密。⚠️ 口径:最后一次 attest 时服务端会看到公钥与 `user_id`,因为验签必须重算凭证消息;达成的是「签发方看不到被签内容」,不是「服务端全程不知道客户端是谁」。
- **凭证不再由 admin 逐个分发。** 改为「凭证池 + 投票人匿名随机领取」：admin 往池里批量投放预签凭证，投票人自己随机领一张（`POST /api/v1/proposals/:id/credentials/claim`），运营方不再天然掌握「哪个凭证给了谁」。**一人一票由资格证明补回**（见下）。
- **一人一票靠零知识资格证明强制。** 运营方的权威名单做成 Merkle 承诺，**服务端只拿到名单的根** —— 它能验证「这个资格在名单里」，却**无法把资格对应到任何成员**。去重键 `QualificationID = MiMC(Leaf, ProposalID)` 按提案派生，同一提案里同一资格只能领一次（跨提案互不关联），强制点是新表 `eligibility_uses` 的唯一索引。领凭证前先取一次挑战（`eligibility-challenge`），把资格证明带进 claim；**资格不过不消耗池槽位**。CLI 走 `credential-claim -eligibility`，门户在「身份」面板导入资格文件后本地出证明（资格包只留内存，不写 localStorage、不上传）。
  - ⚠️ **release 模式必须配 `crypto.eligibility_root` + `crypto.eligibility_vk_path`，否则启动失败。** 两项必须成对出现 —— 缺了资格校验的服务端只是个能发凭证的服务器。生产部署的三步（生成名单 → 生成资格密钥 → 部署）见上面的「一人一票：资格名单与资格密钥」。
  - ⚠️ **门户的资格领取路径没有浏览器自动化**（仓库无 Playwright）—— 自动化证据只到 node 驱动的 WASM 模块，真浏览器里人工点一遍仍要做（`bash ./scripts/browser-acceptance.sh setup` 会备好环境并打印步骤，点完跑 `verify` 核对）。⚠️ `cmd/loadtest` 不支持资格证明（只在未开启资格校验的服务上可用）。⚠️ **资格包丢失找不回来**（它是秘密材料）。⚠️ **真实名单的内容与成员标识形式仍需运营方提供**；单份名单上限 1024 人。
- **`decryption/setup` 拒绝"投票开放中"的提案**,运营上必须在 `starts_at` 之前完成初始化。
- **生产部署必须显式设置** `SERVER_ADMIN_BEARER_TOKEN` 与 `SERVER_CORS_ALLOWED_ORIGINS`；`release` 模式下缺 admin token 会直接启动失败,**缺 `CRYPTO_ELIGIBILITY_ROOT` / `CRYPTO_ELIGIBILITY_VK_PATH` 也会直接启动失败**(一人一票是硬要求)。`configs/config.production.yaml` **不会**被自动加载(viper 只找 `config.yaml`),用 `CONFIG_FILE=./configs/config.production.yaml` 启动即可 —— **显式指定的文件不存在时服务直接启动失败**,不会静默回退到开发配置;启动日志的 `config_file` 字段会打印实际加载的文件路径,上线先核一眼。详见该文件内的注释。

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
