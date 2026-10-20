<h1 align="center">nullifier</h1>

<p align="center">
  匿名投票后端 —— 服务端数得清结果,数不出是谁投的。<br>
  <sub>Go · Gin · GORM · 零知识证明 · 门限加密 · Vue 3</sub>
</p>

<p align="center">
  <a href="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml"><img src="https://github.com/jiangfire/nullifier/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-yellow.svg" alt="License: AGPL-3.0" /></a>
  <img src="https://img.shields.io/badge/go-1.25.4-00ADD8?logo=go&logoColor=white" alt="Go 1.25.4" />
  <img src="https://img.shields.io/badge/coverage-62.0%25%20(repo)-yellow.svg" alt="Coverage 62.0% (repo-wide)" />
</p>

---

## 服务端数得清结果,数不出是谁投的

投票系统最常见的做法是:一张 `votes` 表,一行一个人。这个设计的问题不在存储,而在于**服务器本身就是那个能作恶的角色** —— 它同时掌握选票内容和投票人身份,于是"我不记这一票"这句话无法被任何人验证。

nullifier 把这两个动作拆开:

- **发资格的人看不到选谁** —— 凭证用 RSA-PSS 签名签发(基于 RFC9474 盲签名库)。签发方看得到 `user_id` 和设备公钥(本来就是投票人提供的),但看不到选票内容:凭证与选票内容完全无关。
- **记选票的人看不到是谁** —— `votes` 表只有 `proposal_id`、`nullifier`、`cipher` 三列,没有用户标识。"是否参与"单独存进 `vote_participations`,且只存 `hash(user_id)`,不与票内容关联。
- **验票真伪但不验身份** —— 服务端用 gnark 验证零知识证明,证明"这张票是按合法凭证生成的",证明本身不携带任何身份信息。

结果是:管理员能看到计票结果和审计锚点,但拿到数据库也回答不了"某个人投了什么"。

> ### ⚠️ 当前版本边界(v0.1.0 内测版)
>
> 这个版本把「选票内容与投票人身份分离」的**存储与接口骨架**跑通了,但**证明链还没有实现**。请在下面的「项目状态与限制」里读完整说明,不要把当前实现当成生产能力:
>
> - **没有真实的选票零知识电路。** `crypto.vk_path` 为空时,服务端的 proof 验证器接受任何非空 proof;仓库里唯一的 gnark 电路是测试用的玩具电路。生产配置指向的 `verifying.key` 无法由本仓库生成。
> - **密文由服务端生成。** `POST /api/v1/proposals/:id/ballot-draft` 接收明文选项并加密,因此**服务端在生成凭据时就知道你选了哪个**。本文档与门户界面都不再声称"服务端看不到选项"。
> - **只适合内测与开发**,不要用于真实投票。补齐证明链的工作见 [`docs/release-plan-v0.1.0.md`](./docs/release-plan-v0.1.0.md) 的 v1.0.0 里程碑。

## 特性

**匿名性**

| 能力 | 说明 |
|---|---|
| 凭证签名 | RSA-PSS 签名(基于 RFC9474 盲签名库);签发方看不到选票内容,因为凭证与选票无关 |
| 零知识验票 | gnark 验证选票合法性,证明不携带身份 —— **尚无真实选票电路,开发模式下不验 proof** |
| 防重放 | 每票一个 `nullifier`,重复投票直接拒绝 |
| 防冒充 | `proposal_id + user_id + user_pub_key + credential_signature` 四元组校验 |
| 选择性披露 | 只能回答"你投过票吗",不能回答"你投了什么" |

**计票与审计**

| 能力 | 说明 |
|---|---|
| 门限解密 | kyber ElGamal 封装,2-of-3 份额即可恢复,单份份额无法解密 |
| 会话不可覆盖 | 重复 setup 返回 `409`,已投选票的解密能力不会被覆盖 |
| 幂等发布 | 重复 publish 返回 `409`,不重复写审计锚点 |
| 审计锚点 | 结果发布时生成 Merkle Root,可导出供外部核验 |
| 份额校验 | 拒绝重复 share,不会绕过 threshold 计数 |

**交付形态**

| 形态 | 说明 |
|---|---|
| HTTP API | 12 个 `/api/v1/*` 端点 + `/healthz`,管理面走 Bearer Token |
| CLI | 14 个命令,复用同一套 HTTP API,不绕过服务端校验 |
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

# 3. 签发匿名凭证
go run ./cmd/cli -admin-token dev-admin-token credential-issue \
  -proposal <proposal_id> -user u-1001 -out cred.json

# 4. 生成选票草稿并投票
go run ./cmd/cli ballot-draft -proposal <proposal_id> -option A -out ballot.json
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
internal/infrastructure   http(Gin) · persistence(GORM) · crypto(gnark/kyber/kryptology)
     ↑
internal/di         Wire 装配          internal/config   viper 配置 + 运行期校验
```

| 层 | 职责 | 约束 |
|---|---|---|
| `domain` | 实体、仓储接口、密码学接口、领域错误 | 不依赖 HTTP / GORM / 具体密码学库 |
| `usecase` | Proposal / Register / Voting / Participation / Tally 五个用例编排 | 只依赖 `domain` 接口 |
| `infrastructure` | Gin 路由与错误映射、GORM 仓储与自动迁移、密码学实现 | 实现 `domain` 定义的接口 |
| `di` | Wire provider 组装 | 启动期一次性装配 |

**Stack:** Go 1.25 · Gin · GORM · Wire · gnark · kyber · kryptology · SQLite / PostgreSQL · Vue 3 · TypeScript · Rsbuild

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
| `CRYPTO_VK_PATH` | `crypto.vk_path` | 空 | **生产模式必填** |
| `CRYPTO_ISSUER_KEY_PATH` | `crypto.issuer_key_path` | `issuer_keys.json` | 签发密钥持久化 |
| `CRYPTO_ISSUER_ROTATE_DAYS` | `crypto.issuer_rotate_days` | `90` | 签发密钥轮换周期 |
| `SERVER_ADMIN_BEARER_TOKEN` | `server.admin_bearer_token` | 空 | 为空则启动时生成临时 token 并打印日志 |
| `SERVER_MODE` | `server.mode` | `release` | `release`/`prod`/`production` 时强制要求 `vk_path` |

生产部署参考 `configs/config.production.yaml` 与 `configs/production.env.example`。

## HTTP API

公共端点:

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/healthz` | 健康检查 |
| `GET` | `/api/v1/proposals` | 列出全部提案(按创建时间倒序,含已关闭/未开始) |
| `GET` | `/api/v1/proposals/open` | 时间窗口内的开放提案 |
| `POST` | `/api/v1/votes` | 提交匿名投票(7 个字段全部必填) |
| `POST` | `/api/v1/proposals/:id/vote-status` | 持凭证材料查询是否已投票 |
| `POST` | `/api/v1/proposals/:id/ballot-draft` | 生成选票草稿(开发联调) |
| `GET` | `/api/v1/proposals/:id/results` | 已发布的计票结果 |
| `GET` | `/api/v1/proposals/:id/audit-anchors` | 导出审计锚点 |

管理端点,需 `Authorization: Bearer <admin token>`:

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/credentials` | 签发匿名凭证 |
| `POST` | `/api/v1/proposals` | 创建提案 |
| `GET` | `/api/v1/proposals/:id/credentials` | 该提案下凭证的 `has_voted` 状态 |
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
```

等价的原始命令：

```bash
go build ./...                  # 构建
go vet ./...                    # 静态检查
gofmt -l ./cmd ./internal ./tests   # 格式检查(应无输出)
go test ./...                   # 全部测试
./scripts/check-coverage.ps1    # 覆盖率门槛(默认 80%)
bash ./scripts/check-coverage.sh 80 coverage.out   # CI 实际执行的形式
```

CI 定义在 `.github/workflows/ci.yml`,执行 `go vet` + `gofmt` 检查 + `go test ./...` + 覆盖率门槛,并单独跑前端作业(lint / tsc / vitest)。PostgreSQL 集成测试走 testcontainers:本地 Docker 不可用时跳过,**CI 环境下 Docker 缺失会直接失败**,避免"跳过"被误读成"通过"。

打 `v*` tag 会触发 `.github/workflows/release.yml`:校验 tag 与 `VERSION` 一致 → 各平台 runner 原生构建(保留 cgo,默认的 sqlite 驱动依赖它)→ 产物冒烟启动测试 → 打包并生成 sha256 → 创建 GitHub Release。

覆盖率有两个口径,别混用:

| 口径 | 数值 | 覆盖范围 |
|---|---|---|
| 门槛口径 | **82.3%** | `scripts/check-coverage.sh` 选中的 5 个包:usecase / config / di / http / http-middleware |
| 全仓口径 | **62.0%** | `./...` 全部包(2026-10-20 实测) |

门槛余量只有 2.3 个百分点,且 `internal/infrastructure/crypto`、`internal/infrastructure/persistence`、`cmd/cli`、`cmd/server` **不在门槛内** —— 这些包的覆盖率回退不会让 CI 失败。**新增代码请一并补测**。

## 项目状态与限制

这是个在推进中的实现,接手前请知道边界在哪:

- **证明链尚未实现。** 服务端只有 proof 验证器的骨架:没有真实选票电路、没有密钥生成流程、没有客户端 prover。`crypto.vk_path` 为空时任何非空 proof 都会通过(开发模式);加载 vk 后 `public_witness` 变成必填,而现有客户端都拿不到它。
- **`ballot-draft` 是开发联调接口**,返回的是便于串通流程的草稿数据。**密文由服务端生成,服务端在生成时就知道选了哪个** —— 这是当前版本最大的匿名性缺口。生产环境应由真实钱包/客户端生成 proof 与密文;后端只负责验证,不代替用户端生成证明。这条链路尚未完成。
- **`decryption/setup` 拒绝"投票开放中"的提案**,运营上必须在 `starts_at` 之前完成初始化。
- **生产部署必须显式设置** `SERVER_ADMIN_BEARER_TOKEN` 与 `SERVER_CORS_ALLOWED_ORIGINS`;`release` 模式下缺 admin token 会直接启动失败。`configs/config.production.yaml` **不会**被自动加载(viper 只找 `config.yaml`),详见该文件内的注释。

当前实现口径见 [`docs/techspec.md`](./docs/techspec.md),分层上下文索引见 [`docs/project-map.md`](./docs/project-map.md)。

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
