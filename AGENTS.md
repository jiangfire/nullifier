# AGENTS.md

基于 Clean Architecture 的匿名投票系统：Go 后端（Gin + GORM + Wire）+ Vue 3 门户前端 + 复用 HTTP API 的 CLI。

## Setup commands

推荐用仓库根目录的 `Makefile` 作为统一入口（Windows 下在 Git for Windows 的 bash 里跑 `mingw32-make <target>`，Linux/macOS 直接 `make <target>`；`make help` 列出全部目标）：

- `make deps` / `make build` / `make run` / `make test` / `make vet` / `make fmt-check` / `make cover`
- `make check` — 提交前快速门禁（vet + gofmt + 前端 lint + tsc）
- `make ci` — 复现 CI 流水线（`go test ./...` + 覆盖率门槛）
- `make web-install` / `web-dev` / `web-build` / `web-test` / `web-lint` / `web-typecheck` / `web-format`
- `make web-wasm` —— 编译**浏览器端 prover**（`cmd/ballot-wasm` → `web/public/ballot-prover.wasm`）并把 Go 官方胶水 `wasm_exec.js` 拷进 `web/public/`；`make web-proving-key PK=<路径>` 把**选票** proving key 放进同一目录；`make web-eligibility-key PK=<路径>` 把**资格** proving key 放进去。**门户投票需要这四个文件就位**（构建顺序：`web-wasm` → `web-build`）
- `make eligibility-list` —— 构建**离线名单工具** `cmd/eligibility-list`（一人一票的资格名单生成器；离线跑，不联网、不连服务端）
- **两套电路、两把密钥**：`go run ./cmd/keygen -dir keys` 出选票那把（`verifying.key` 给服务端 / `proving.key` 只给客户端），`go run ./cmd/keygen -circuit eligibility -dir keys` 出资格那把（`eligibility-verifying.key` 给服务端 / `eligibility-proving.key` 只给客户端）。Groth16 的 setup 是 per-circuit 的，**两把互不相通，不能互相替代**

等价的原始命令：

- 安装依赖：`go mod download`；前端 `pnpm --dir web install`
- 启动服务：`go run ./cmd/server`（默认 SQLite 文件库 `anonvote.db`）
- 构建：`go build ./...`；前端 `pnpm --dir web run build`
- 测试：`go test ./...`（**Windows 需 `CGO_ENABLED=1`**，`gorm.io/driver/sqlite` 依赖 cgo；Makefile 在 Windows 上已自动导出该变量）
- 静态检查：`go vet ./...` 与 `gofmt -l ./cmd ./internal ./tests`（应无输出）
- 覆盖率门槛：`./scripts/check-coverage.ps1`（PowerShell）/ `bash ./scripts/check-coverage.sh`（Bash）。**按包分档**：每个包的最低覆盖率写在 `scripts/coverage-thresholds.txt`，逐包判定；脚本的第一个参数是「表里没有的包」的兜底阈值，默认 80%
- 前端：`pnpm --dir web run dev`（rsbuild，默认 :3000）、`run build`、`run test`（vitest）、`exec tsc --noEmit`
- CLI 工作流：`go run ./cmd/cli help` 查看全部 16 个命令
- 资格名单（离线）：`go run ./cmd/eligibility-list -members <名单.txt> -out <目录>` → `<目录>/root.json`（给服务端配 `crypto.eligibility_root`）+ 每成员一份 `<目录>/eligibility-<成员>.json`（0600，**走安全渠道发给本人**）
- 带资格领票：`go run ./cmd/cli credential-claim -proposal <提案> -eligibility eligibility-<成员>.json -proving-key keys/eligibility-proving.key -out cred.json`

## Project layout

- `cmd/server` — HTTP 服务入口；`cmd/cli` — 命令行客户端（复用 HTTP API，不绕过服务端校验）；`cmd/loadtest` — 并发投票压测工具（只走 HTTP API，防重投失守时退出码非 0）；`cmd/ballot-wasm` — **浏览器端 prover**（`GOOS=js GOARCH=wasm` 构建，暴露 `nullifierProver` 全局对象；**四个 API**：`init` / `prove` / `initEligibility` / `proveEligibility`，同一个 WASM 模块；非 js/wasm 平台有 stub）；`cmd/eligibility-list` — **离线资格名单工具**（一人一票的名单生成器，不联网、不连服务端）；`cmd/keygen` — 出两套密钥（`-circuit ballot` 默认 / `-circuit eligibility`）
- `internal/domain` — 实体、仓储接口、密码学接口、领域错误（不依赖 HTTP/GORM/具体密码学库）；资格证明相关的领域件：`domain/crypto/eligibility.go`（资格校验接口）、`domain/entity/eligibility_use.go`（去重记录）、`domain/repository/eligibility_use_repo.go`（仓储接口）
- `internal/usecase` — 业务编排（Proposal / Register / Voting / Participation / Tally），`usecase/mocks` 为 gomock 生成物
- `internal/infrastructure` — `http`（Gin 路由/中间件/错误映射）、`persistence`（GORM 仓储与自动迁移）、`crypto`（gnark / kyber / kryptology）；`crypto/eligibility`（Merkle 名单承诺 + 资格电路 + prover；`TreeDepth=10` 是**编译期常量**，公开输入定长 5 个（root / proposal_id / challenge / 去重键 / 会话绑定），私有见证是秘密、位置与路径，实测约 6860 条约束）、`crypto/mimc`（选票与资格**共用**的 MiMC 实现，`Constants()` 供跨语言 KAT）、`crypto/eligibility_service.go`（资格校验的领域适配层，**没有**「跳过验签」的降级模式）
- `internal/di` — Wire 依赖注入；`internal/config` — viper 配置加载与运行期校验
- **T22 一人一票（资格证明）** — 新表 `eligibility_uses`（唯一索引 `(proposal_id, qualification_id)`，只存去重键 `MiMC(叶子, 提案ID)`）；新端点 `POST /api/v1/proposals/:proposal_id/credentials/eligibility-challenge`（返回 `proposal_id` / `root` / `depth` / `challenge`），`POST /api/v1/proposals/:proposal_id/credentials/claim` 的请求体多出 `eligibility_proof` / `qualification_id` / `challenge`（**三件要么都给、要么都不给**；资格不过**不消耗**池槽位）；`GET /healthz` 多出 `eligibility` 布尔字段（运维据此确认这台实例是否在强制一人一票）；错误码 `ELIGIBILITY_REQUIRED`(400) / `INVALID_ELIGIBILITY_PROOF`(403) / `ELIGIBILITY_ALREADY_USED`(409) / `ELIGIBILITY_NOT_ENABLED`(409)
- `tests/` — `integration`（testcontainers + PG）、`e2e`、`architecture`（架构不变量守卫）、`testutil`；**单元测试不在这里**，而是与实现同包的 `_test.go`（如 `internal/usecase/tally_test.go`）
- `web/` — Vue 3 + TypeScript + Rsbuild 单页门户（`src/App.vue` 只做外壳与引导编排，页面拆在 `src/components/panels/`，设计令牌在 `src/styles/`，后端错误文案映射在 `src/lib/error-messages.ts`）；**浏览器端 prover** 在 `src/lib/wasm-prover.ts`（主线程编排）+ `src/workers/ballot-prover.worker.ts`（Worker 里加载 WASM），**资格 prover** 在 `src/lib/eligibility-prover.ts`（同一条 Worker 链路，密码学全在 WASM 里；门户「身份」面板负责「导入资格文件」），静态资源目录 `web/public/`（wasm、胶水、两把 proving key 都是构建产物，不入库）
- `configs/` — `config.yaml`（开发）、`config.production.yaml`、`production.env.example`
- `docs/` — `techspec.md`（当前实现口径，改接口/流程优先更新）、`project-map.md`（**分层项目地图 / 上下文索引**：子系统职责、关键文件、模式、陷阱、验证手册；改动前先读对应章节，不必通读全仓）、`deployment-checklist.md`（**生产部署清单**：上线前逐项核对配置、密钥、迁移、TLS、门户部署与投票运营流程）、`release-plan-v0.1.0.md`（**v0.1.0 发版计划与落地状态**，当前发版）、`roadmap.md`（**产品路线图**：v0.2.0 的能力清单与完成定义、里程碑 M1–M3、决策门 G1–G9、工程细项附录；T3–T12、T17 与 T22 已落地）、`release-review-2026-10-20.md`（发版前整体 review：6 个 CRITICAL + 29 个 REQUIRED + 20 个 NIT 的完整证据与修复顺序）、`t22-eligibility-design.md`（**T22 资格证明设计稿**：一人一票的方案对比、协议、电路与待拍板决策点；已落地）、`errata.md`（**勘误表**：已发布版本里的错误陈述；发现发布版本有问题时先查这里）
- `scripts/` — 覆盖率门槛脚本；`.github/workflows/ci.yml` — CI 定义；`.github/workflows/release.yml` — tag 触发的发布流水线（原生构建 + 冒烟测试 + sha256 + GitHub Release）

## Code style

- Go 走 `gofmt` 默认风格；包名小写短名，导出符号 `PascalCase`，测试函数 `TestXxx`
- **服务端运行日志统一用注入的 `*zap.Logger`**，禁止在服务路径引入 `fmt.Println` / `log.Printf`；CLI 的纯文本输出属于命令结果，不受此限
- 前端：组件 `PascalCase.vue`，组合式函数 `useXxx.ts`，工具模块小写语义名（`lib/datetime.ts`）
- Prettier：`singleQuote: true`（`web/.prettierrc`）；ESLint 扁平配置 `web/eslint.config.mjs`
- 仓库有 `.gitattributes`（`* text=auto eol=lf`）：**提交前保持 LF**，不要引入 CRLF

## Testing instructions

- 后端用 Go 原生测试 + `testify`；集成测试集中在 `tests/integration`，HTTP 与持久化测试贴近实现文件
- 前端用 vitest，测试文件与源码同目录（如 `web/src/lib/datetime.test.ts`）；`.vue` 面板走**服务端渲染测试**（`web/src/components/panels/panels.test.ts`），`.vue` 能在 vitest 里跑是因为 `web/vitest.config.ts` 里自写了 SFC 编译插件（`vue/compiler-sfc` + `typescript`，不新增依赖）
- 改动涉及接口、密码学或持久化逻辑时**必须补回归测试**；合入前至少 `go test ./...` 且覆盖率脚本通过
- `tests/e2e/wasm_prover_test.go` 会**编译 WASM 模块并用 node 驱动它**（缺 node 时跳过；CI 的 go job 装了 node，因此 CI 上会真跑）。改 `cmd/ballot-wasm` 或 `ballot.ProveBallot` 时这条用例要一起看
- `tests/e2e/eligibility_test.go` 是 **T22 的五条验收**：缺资格材料被拒（`ELIGIBILITY_REQUIRED`）/ 带资格领取成功 / 同一资格不能重复领（`ELIGIBILITY_ALREADY_USED`）/ 证明不能跨提案复用（`INVALID_ELIGIBILITY_PROOF`）/ 领取请求体里没有真人身份字段。`tests/e2e/wasm_eligibility_test.go` 驱动**真的 WASM 模块**（`initEligibility` / `proveEligibility`）出资格证明并让服务端真实验签通过（缺 node 时跳过）
- **资格证明与选票证明是两套电路、两把 key**：Groth16 的 setup 是 per-circuit 的，`keys/proving.key` 与 `keys/eligibility-proving.key` 互不相通；`cmd/ballot-wasm` 暴露 `init` / `prove` / `initEligibility` / `proveEligibility` 四个 API（同一个 WASM 模块）。两套电路**共用** `internal/infrastructure/crypto/mimc`，所以跨语言 KAT **两边各钉一条**（Go 侧 `internal/infrastructure/crypto/mimc/mimc_test.go` 的 `TestKAT_MimcConstants` + TS 侧 `web/src/lib/mimc.test.ts`）—— 改 MiMC 常量或轮次必须同时改两侧并都跑一遍
- **跨语言的东西要双向锁死，字符串也算**：Go↔TS 的密码学除向量外，变体名 / 字段名 / 编码这些**字符串**也要两边各钉一条断言（如盲签名变体的线路名 `RSABSSA-SHA384-PSS-Deterministic`：`internal/infrastructure/crypto/blind_rsa_interop_test.go` 的 `TestKAT_BlindRsaVariantName` + `web/src/lib/credential.test.ts`）。两边各拿自己的常量自证是**测不出**这类问题的 —— 2026-10-25 的真实浏览器验收就是靠「真浏览器对真服务端」把它挖出来的
- **「服务端不得拿到明文选项」有守卫测试**：能力 A 的底线由 `tests/architecture/plaintext_option_test.go` 把守（go/ast 扫 `internal/`：请求体字段 / usecase 入口 / **服务端代加密那条链不许复活** / **ballot-draft 端点不许复活**）。**不设豁免机制** —— 改 HTTP 请求体结构或 usecase 入口前先跑 `go test ./tests/architecture/`
- **门户的浏览器端路径没有自动化**：仓库里没有 Playwright，改完门户的密码学接线要用真浏览器人工点一遍（领凭证 → 本地生成选票 → 提交 → 计票），步骤见 `docs/deployment-checklist.md` §8；**资格路径**也要人工点一遍（导入资格文件 → 匿名领取 → 重复领取应得到 `ELIGIBILITY_ALREADY_USED`），见 `docs/deployment-checklist.md` §8.1
- 覆盖率有两个口径，别混用：**门槛口径 80.8%**（`check-coverage.sh` 选中的 11 个包：usecase / config / di / http / http-middleware / crypto / crypto-ballot / persistence / cmd-cli / cmd-keygen / cmd-server），**全仓口径 72.7%**（2026-10-25 实测）。门槛是**逐包分档**的（门槛值见 `scripts/coverage-thresholds.txt`），单个包回退会直接让 CI 变红；`internal/infrastructure/persistence`（63.9%）是当前最低的一档，抬门槛时它会先卡住
- CI 实际执行：`go test ./...` + `bash ./scripts/check-coverage.sh 80 coverage.ci.out`，**另加** gofmt 检查、前端 lint / tsc / vitest（`.github/workflows/ci.yml`）

## PR & commit conventions

- 从 `main` 切分支，不要直接推送默认分支
- Conventional Commits：`feat:` / `fix:` / `chore:` / `docs:` / `refactor:`，简短祈使句（例：`fix: tighten credential issuance validation`）
- PR 需说明变更范围、测试结果、配置影响；改 API 附示例请求，改 `web/` 附截图
- 若变更 `configs/`、鉴权或密钥路径，明确列出部署调整项

## Security

- 绝不提交真实密钥、令牌或生产数据库；`.env` 已在 `.gitignore`
- 生产模式（`server.mode=release/prod/production`）必须显式设置 `crypto.vk_path`（或 `CRYPTO_VK_PATH`），否则服务启动失败
- **一人一票是 release 模式的硬要求**：`crypto.eligibility_root` 与 `crypto.eligibility_vk_path`（或 `CRYPTO_ELIGIBILITY_ROOT` / `CRYPTO_ELIGIBILITY_VK_PATH`）**必须成对出现**，只配一个或两个都不配，服务都**启动失败**。领域适配层（`internal/infrastructure/crypto/eligibility_service.go`）**没有**「跳过验签」的降级模式 —— 不要加
- **服务端只见名单根，不见名单**：`crypto.eligibility_root` 是 64 位 hex 的域元素，服务端能验证「这个叶子在这棵树里」，**无法把叶子对应到任何成员**。`eligibility_uses` 只存去重键 `MiMC(叶子, 提案ID)`，不存叶子、不存成员 ID、不存资格秘密
- **资格包是秘密材料**：`cmd/eligibility-list` 产出的 `eligibility-<成员>.json`（成员秘密 + 叶子 + Merkle 路径，0600）**谁拿到谁就能领这一票** —— 只能走安全渠道发给本人
- **root 与成员包必须分开保管**：`root.json` 给服务端，成员包给本人；同一份手里同时有这两样，就等于同时掌握了「名单是哪些人」与「谁拿到了票」
- **名单工具（`cmd/eligibility-list`）的运行机器不要与投票服务同机** —— 它是离线工具，不联网、不连服务端
- **资格 proving key 只在客户端**：`keys/eligibility-proving.key` 只给客户端（CLI 本地出资格证明；门户放在静态目录 `web/public/eligibility-proving.key`）—— **不进服务端镜像、不入库、不提交仓库**
- **proving key 只在客户端**：服务端只拿 verifying key（`crypto.vk_path`）；CLI 用 `ballot-prove -proving-key` 在本地出证明，proof 不经过服务端。**浏览器端 prover**（`cmd/ballot-wasm` → WASM）由门户在 Worker 里加载，proving key 发布在**门户静态目录**（`web/public/proving.key`）供浏览器下载 —— **不进服务端镜像、不入库、不提交仓库**。`cmd/keygen` 生成的 `proving.key`（0600）**不入库**
- 管理面接口（凭证签发、创建提案、凭证状态、解密 setup、发布结果）需 `Authorization: Bearer <admin token>`；未配置时服务启动会生成仅当前进程有效的临时 token 并打印日志
- 凭证签发走**真两段式盲签名**：`credentials/claim` 只预留（**不收公钥**）→ `credentials/blind-sign` 对盲化消息签名 → `credentials/attest` 验签后落库；公开的 blind-sign 只覆盖池里还空着的槽位
- 票据表只存匿名投票数据；`vote_participations` 仅存 `hash(user_id)`，用于「知道你是否投过票」但不关联票内容
