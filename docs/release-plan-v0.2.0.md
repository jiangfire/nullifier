# nullifier 发版计划 v0.2.0（真实投票能力）

> 配套文档：[`docs/roadmap.md`](./roadmap.md)（能力与任务卡）、
> [`docs/deployment-checklist.md`](./deployment-checklist.md)（生产部署清单）、
> [`docs/project-map.md`](./project-map.md)（上下文索引）、
> [`docs/t22-eligibility-design.md`](./t22-eligibility-design.md)（一人一票的设计）
>
> 分支：`feat/v0.2.0-m2-proof-chain`（从 `main` 切出，**尚未推送**）
> 制定日期：2026-10-25
> 状态：**发版准备完成，等拍板**（见 §6）

---

## 0. 一句话策略

**v0.1.0 是「能演示」，v0.2.0 是「能真投」** —— 选票的加密与证明全部搬到客户端，
服务端再也拿不到明文选项；同时用零知识资格证明把 T9 丢掉的「一人一票」补回来。

这一版的**部署侧变化比代码侧大**：release 模式新增两项硬要求，门户静态目录新增
四个文件，旧密文全部作废。发版说明与部署清单必须写清楚，否则第一次部署就会起不来。

---

## 1. 版本口径

| 能力 | v0.1.0 | v0.2.0 |
|---|---|---|
| **A** 选票内容对服务端不可知 | ❌ 服务端加密，明文选项在服务端 | ✅ **客户端加密**，服务端只见密文（`tests/architecture/` 持续守卫，**不设豁免**） |
| **B** 选票可验证 | ❌ 无真实电路，`vk_path` 为空时任何非空 proof 都通过 | ✅ 真实 Groth16 电路（7917 条约束）+ 密钥生成流程 |
| **C** proof 与票绑定 | ❌ proof 可跨提案复用 | ✅ 公开输入由服务端自拼，跨提案/跨 nullifier 复用均验签失败 |
| **D** 客户端能投票 | ❌ 门户在生产模式下无法完成投票 | ✅ 浏览器内出 proof 与密文（`cmd/ballot-wasm` → 门户 Worker） |
| **E** 凭证不可伪造 | ❌ 投票路径从不验凭证签名 | ✅ 落库前 `VerifyCredential`，伪造签名 → 401 |
| **F** 签发方看不到被签内容 | ❌ 盲化在服务端进程内完成 | ✅ 真两段式盲签名，交叉攻击失败 |
| **G** 凭证分发不泄露身份 | ✅ 池里随机领 | ✅ **保持**，并用资格证明补回「一人一票」（T22） |
| **H** 生产可运维 | 🟡 无显式配置文件路径 | 🟡 `CONFIG_FILE` 显式路径 + 启动日志可核对；**CORS 真实源仍是部署项** |
| **I** 端到端验收 | ❌ 三个盲区从未验证 | 🟡 三盲区各补一部分（见 §4） |

**版本号来源**：`VERSION` 文件（单一真相源），release workflow 从 tag 读取并校验；
`web/package.json` 的 `version` 由流水线注入，不手改。

---

## 2. 发布动作（按顺序）

| # | 动作 | 说明 |
|---|---|---|
| 1 | `VERSION` 改 `0.2.0` | release workflow 会校验 tag `v0.2.0` 与 `VERSION` 一致，不一致直接失败 |
| 2 | `CHANGELOG.md` 的 `[未发布]` 改成 `## [0.2.0] - <日期>` | **必须写明密文格式 v2 → v3 不兼容**（下面 §3） |
| 3 | `git tag -a v0.2.0 -m "<发版说明>"` | **注解 tag** —— release workflow 用 tag 说明当 release 正文 |
| 4 | 推分支 + 推 tag | ⚠️ **需拍板**（见 §6） |
| 5 | 按 [`docs/deployment-checklist.md`](./deployment-checklist.md) 逐项核对 | 本文件 §5 是它的摘要 |
| 6 | `gh release view v0.2.0` 看产物 | 5 个平台的服务端/CLI 二进制 + 门户包，各带 sha256 |

> ⚠️ **tag 必须在 `VERSION` 改完之后打**，且要在**所有提交都推上去之后**打 ——
> release workflow 用 tag 上的源码构建，tag 指向的提交少一个就少一个文件。

### 2.1 发版说明正文（tag 注解，可直接用）

release workflow 把**注解 tag 的说明**原样当作 GitHub Release 正文（不是提交列表）。
下面这段就是可直接用的正文 —— 关键是三件事说清：**这版能干什么、破坏性变更、部署方必须做什么**。

```text
v0.2.0 —— 客户端证明链与一人一票

选票的加密与零知识证明全部搬到客户端：服务端只见密文，拿不到明文选项。
「一人一票」由零知识资格证明兑现 —— 服务端只见名单根，不见名单，无法把资格对应到任何成员。

⚠️ 破坏性变更（升级前必读）
- 选票密文格式 v2 → v3，与 v0.1.0 完全不兼容：v0.1.0 投过的票无法解密，发版前请清库。
- POST /api/v1/proposals/:proposal_id/ballot-draft 已删除（服务端代加密的入口整个没有了）。
- 凭证签发改真两段式盲签名：claim（只预留）→ blind-sign → attest。
- 投票请求体不再接受 public_witness（公开输入由服务端自行拼装）。
- 门限分享从 kyber 的 ed25519 改为 Fr 上的 Shamir。

部署必做项
- release 模式必须配 crypto.eligibility_root 与 crypto.eligibility_vk_path（成对出现，缺一即启动失败）。
- 门户包必须带 ballot-prover.wasm 与 wasm_exec.js（流水线已内置并校验）；两把 proving key
  是秘密材料，默认不进发布包，需部署方自备或用 secret 注入。
- 详见 docs/deployment-checklist.md。

本版不承诺生产可用：门户无 UI 自动化回归、PG 集成测试依赖 CI 实跑、真实名单待运营方提供、
容量数据只是压测实测，不是容量规划。
```

打 tag（注解 tag，正文从文件读，避免 shell 引号地狱）：

```bash
git tag -a v0.2.0 -F release-notes-0.2.0.txt   # 文件内容就是上面那段
git push origin feat/v0.2.0-m2-proof-chain v0.2.0
```

> ⚠️ `release-notes-0.2.0.txt` 是**临时文件**，别提交进仓库（加进 `.gitignore` 或打完好删）。

---

## 3. 破坏性变更（部署前必须知道）

1. **选票密文 v2 → v3，与 v0.1.0 完全不兼容**。v3 = 1 字节版本 `0x03` + 两个 32 字节
   压缩点（BN254 twisted Edwards），加密的是**选项下标**。原因：零知识电路的算术在
   BN254 标量域里，v0.1.0 的 ed25519 + HKDF 群运算无法在电路里复算。
   **旧的加密选票无法解密 —— 用 v0.1.0 密文投过的票要视为不可用，发 v0.2.0 前请清库。**
2. **投票请求体不再接受 `public_witness`** —— 公开输入改由服务端自拼。
3. **`POST /api/v1/proposals/:id/ballot-draft` 已删除**（连带 CLI 的 `ballot-draft` 与
   门户的「按选项生成凭据」入口）。它让服务端拿明文选项去加密选票 —— 能力 A 的
   底线唯一的例外。**任何模式都不可达**（POST 它必须 404）。
4. **凭证签发改成真两段式盲签名**：`claim` 只预留（**不再收 `user_pub_key`**）→
   `blind-sign` → `attest`。自建客户端要跟着改。
5. **门限分享从 kyber 的 ed25519 改为 Fr 上的 Shamir**（`go.dedis.ch/kyber/v3` 依赖已移除）。
6. **release 模式新增两项硬要求**（见 §5.1）—— 配不齐**服务启动失败**。

---

## 4. 验收证据（本轮实测）

| 项 | 证据 | 状态 |
|---|---|---|
| 后端测试 | `go test -count=1 ./...` 全过 | ✅ |
| 覆盖率门槛 | 逐包分档，合计 **80.80%**（门槛 77%） | ✅ |
| 静态检查 | `go vet` 干净、`gofmt -l` 无输出 | ✅ |
| 前端 | eslint 0 / `tsc --noEmit` 0 / vitest **19 文件 156 用例** | ✅ |
| 架构不变量 | `tests/architecture/` —— 服务端不得拿到明文选项（**不设豁免**） | ✅ |
| 能力 A 底线 | 同上 + `ballot-draft` 端点任何模式都 404 | ✅ |
| 能力 B/C | 篡改公开输入 / 跨提案复用 → 验签失败（`tests/e2e/`） | ✅ |
| 能力 E/F | 伪造凭证签名 → 401；盲签名交叉攻击失败 | ✅ |
| 能力 G | `tests/e2e/eligibility_test.go` 验收 ①–⑤ + `tests/e2e/wasm_eligibility_test.go`（node 驱动真 WASM） | ✅ |
| **浏览器路径** | **真浏览器（Chromium / Electron）里两条路径都走通**（2026-10-25）：选票链路与资格链路（导入资格文件 → 匿名领取 201 → **重复领取 409** → 本地生成选票 → 提交 201 → 计票还原 `{"tally":{"beta":1},"total_votes":1}`）。整套手工流程已固化成 `scripts/browser-acceptance.sh`（`setup` / `verify` / `stop`） | ✅ |
| **PG 集成测试** | `tests/integration/eligibility_postgres_test.go` 等 2 个新用例（一人一票的复合唯一索引在 PG 上的行为） | ✅ **已在本机真 PostgreSQL 17.7 上实跑通过**（2026-10-25：便携版 PG 起临时集群 + 集成测试的外部库路径，`go test ./tests/integration/ -run Postgres` 全过）；⏳ **testcontainers 那条路径仍要 CI 实跑** —— CI 的 go job 跑在 ubuntu-latest（自带 Docker），**缺 Docker 时测试失败而不是跳过** |
| **浏览器路径（PG 形态）** | 同上，但服务端连的是**生产库形态**（PG + 资格校验强制）：`scripts/browser-acceptance.sh` 的 `DB_DRIVER=postgres` | ✅ **已跑通**（2026-10-25）：导入资格文件 → 匿名领取 201 → 重复领取 409 → 本地生成选票 → 提交 201 → 计票还原 `{"tally":{"beta":1},"total_votes":1}`（此前所有浏览器证据都跑在 SQLite 上） |
| **门户 UI 自动化** | 无 | ⚠️ 仓库**没有** Playwright，门户交互只能人工点（步骤见部署清单 §8.1 / `scripts/browser-acceptance.sh`） |
| 容量 | `cmd/loadtest`（200 票 / 并发 20 → 257 req/s，同 nullifier 并发 8 次恰好放行 1 张） | ⚠️ **不是容量规划** —— SQLite 单写者；容量规划按路线图属 v0.3.0 |

---

## 5. 部署侧必须做的事（摘要）

### 5.1 服务端（release 模式）

| 项 | 要求 | 不配的后果 |
|---|---|---|
| `crypto.vk_path` | 选票电路的 **verifying key** | **启动失败** |
| `crypto.eligibility_root` **+** `crypto.eligibility_vk_path` | 资格名单树根 + 资格电路 verifying key，**必须成对出现** | **启动失败**（只配一个也算失败） |
| `server.admin_bearer_token` | 管理员 Bearer token | **启动失败**（不再有临时 token） |
| `server.auto_migrate` | release 模式默认**不自动改表** | 需先跑 `nullifier-server -migrate`（新增表 `eligibility_uses`） |

### 5.2 门户静态资源（`web/dist/` 四个文件）

| 文件 | 来源 | 能不能公开 |
|---|---|---|
| `ballot-prover.wasm` | `make web-wasm`（发布流水线自动编） | ✅ 公开 |
| `wasm_exec.js` | Go 官方胶水（发布流水线自动拷） | ✅ 公开 |
| `proving.key` | `make web-proving-key PK=…` | ⚠️ **客户端材料**，每套部署用自己的 |
| `eligibility-proving.key` | `make web-eligibility-key PK=…` | ⚠️ **同上** |

> ⚠️ **发布流水线产出的门户包里默认没有那两把 key**（秘密材料，且必须与服务端那把
> verifying key 配对）。要么把 base64 编码的 key 配成仓库 secret
> （`PORTAL_PROVING_KEY_B64` / `PORTAL_ELIGIBILITY_PROVING_KEY_B64`），要么**部署时
> 自己补进 `web/dist/`**。流水线会检查 prover 在不在 —— **缺 prover 直接失败**。

### 5.3 一人一票的材料分发

1. `cmd/eligibility-list -members <名单文件> -out <目录>` → `root.json`（给服务端）
   + 每成员一份 `eligibility-<member_id>.json`（0600，**走安全渠道发给本人**）。
2. `root.json` 里的 `root` 填进 `crypto.eligibility_root`。
3. **root 与成员包必须分开保管** —— 同一份手里同时有这两样，就等于同时掌握了
   「名单是哪些人」与「谁拿到了票」。
4. ⚠️ 名单工具**不要与投票服务同机**跑（它是离线工具，不联网）。

---

## 6. go / no-go 清单

**技术侧已就绪**（§4 全绿）。**下面这几件要人拍板或要人提供材料**：

| # | 事项 | 谁 | 状态 |
|---|---|---|---|
| 1 | **是否推送分支 + 打 tag 发 v0.2.0** | 用户 | ⬜ 未决 |
| 2 | **是否引入 Playwright 做门户 UI 自动化**（体量大、影响 CI；离线装不了） | 用户 | ⬜ 未决 |
| 3 | **真实名单的内容与成员标识形式**（工号 / 选民编号 / 会员号……） | 运营方 | ⬜ 未提供 |
| 4 | **PG 集成测试的 testcontainers 路径实跑** | CI | ⬜ 待 CI（**外部库路径已在本机真 PG 17.7 实跑通过**，断言与 CI 同一套） |
| 5 | 决策门 G6（路线图 §5） | 用户 | ⬜ 未拍板 |
| 6 | 生产部署清单逐项核对（真实环境） | 运营方 | ⬜ 未执行 |

---

## 7. 风险与回滚

| 风险 | 处置 |
|---|---|
| 密文格式不兼容导致旧票解不开 | **发版前清库**（v0.1.0 的票一律作废） |
| 门户没带 prover 或缺 key → 投不了票 | 流水线检查 prover（缺则失败）；key 由部署方补，§5.2 |
| release 模式配不齐 → 服务起不来 | 启动即失败并打印缺哪一项，**不会静默降级** |
| 资格材料分发失误 → 一人一票失效或泄露 | root 与成员包分开保管；成员包走安全渠道；工具离线跑 |
| 回滚 | 回滚到 v0.1.0 需要**再次清库**（密文格式来回不兼容） |

---

## 8. 落地状态（2026-10-25）

- ✅ 代码侧全部完成并通过门禁（§4）。
- ✅ 浏览器路径在真浏览器里验过两遍（2026-10-25：人工一遍 + 脚本一遍）。
- ✅ **PG 形态也验过一遍**（2026-10-25）：便携版 PostgreSQL 17.7 起临时集群 →
  集成测试外部库路径全过 → `scripts/browser-acceptance.sh` 的 `DB_DRIVER=postgres`
  跑完整条浏览器流程，计票还原 `{"tally":{"beta":1},"total_votes":1}`。
  **生产用的就是 PostgreSQL —— 这是本版最贴近生产形态的一条证据。**
- ✅ **§2 的第 1、2 步已在分支上完成**：`VERSION` → `0.2.0`，`CHANGELOG.md` 的
  `[未发布]` → `## [0.2.0] — 2026-10-25`（并写明密文 v2 → v3 不兼容）。
  剩下的是第 3 步（打注解 tag）与第 4 步（推分支 + 推 tag）—— **等拍板**。
- ⚠️ 分支 `feat/v0.2.0-m2-proof-chain` **尚未推送**。
- ⬜ 发版动作（§2）等拍板。
