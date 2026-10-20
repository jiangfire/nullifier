# nullifier 发版计划 v0.1.0（内测版）

> 配套文档：`docs/release-review-2026-10-20.md`（问题证据）、`docs/project-map.md`（上下文索引）
> 分支：`release/v0.1.0-prep`（从 `main` @ `2398b00` 切出）
> 制定日期：2026-10-20

---

## 0. 一句话策略

**先发一个诚实标注边界、但代码质量达标的 v0.1.0 内测版；同时把「补齐证明链」作为 v1.0.0 的独立里程碑。**

理由：审查发现的 6 个 CRITICAL 里，C1–C3（证明链）需要「真实电路 + 客户端 prover + 密钥分发流程」，是**周级**工作；而 C4/C5/C6（静默投错票、文案失真、徽章虚高）是**小时级**且与密码学无关。把它们绑在一起会让 v0.1.0 永远发不出去。

---

## 1. 版本口径

| | v0.1.0（本计划） | v1.0.0（后续里程碑） |
|---|---|---|
| 定位 | 内测 / 演示 / 继续开发 | 真实投票 |
| 证明验证 | **关闭**（`vk_path` 为空），启动时明确告警 | 开启，真实电路 + `public_witness` |
| 选票加密位置 | **服务端**（服务端可见明文选项），门户与文档明示 | 客户端（服务端只见密文） |
| 门户投票主流程 | 可用（dev 模式） | 可用（真实 proof） |
| 文档要求 | README / techspec / 门户三处标注上述两条 | 删除标注 |

**版本号来源**：`VERSION` 文件（单一真相源），release workflow 从 tag 读取并校验一致性；`web/package.json` 的 `version` 由 CI 注入，不手改。

---

## 2. 里程碑与验收

每个里程碑是一个独立可回滚的提交组。**完成一个就提交一次**，不攒到最后。

### M1 · 后端可观测性与错误码（对应 A3 / A4 / A6 / A11）

| 改动 | 文件 |
|---|---|
| proof 校验失败返回 400 `INVALID_REQUEST`（现在误返 500） | `internal/usecase/voting.go` |
| 启动日志打印**实际生效**的 `server.mode` / `db.driver` / `db.database` / admin token 来源 | `internal/config/config.go`、`cmd/server/main.go` |
| 配置态 admin token **不再**以 Info 级别进日志 | `cmd/server/main.go` |
| `vk_path` 为空时启动打 Warn，明示「当前不验证选票证明」 | `internal/config/config.go` |

**验收**：`go test ./...` 通过；新增用例覆盖「无效 proof → 400」「release 模式无临时 token → 启动失败」。

### M2 · 生产配置硬化（对应 A5 / R5 / R6）

| 改动 | 文件 |
|---|---|
| `release/prod/production` 模式下 admin token 为空 → **启动失败**（不再用临时 token） | `internal/config/config.go` |
| `production.env.example` 补 `SERVER_ADMIN_BEARER_TOKEN` / `SERVER_CORS_ALLOWED_ORIGINS` | `configs/production.env.example` |
| `config.production.yaml` 补注释说明「需改名为 config.yaml 或用环境变量」 | `configs/config.production.yaml` |

**验收**：`go test ./...` 通过；新增用例「release 模式 + 空 token → Load 报错」。

### M3 · CLI 安全与正确性（对应 A10 / A13 / A14 / R27）

| 改动 | 文件 |
|---|---|
| URL 路径二次转义修复（`a b/c` → 不再变成 `a%2520b%252Fc`） | `cmd/cli/main.go` |
| `decrypt-setup` 不再默认把**门限份额**打到 stdout | `cmd/cli/main.go` |
| 删除 `selected_option` 字段（明文选项白白上网，服务端还不消费） | `cmd/cli/main.go` |
| help 示例日期改为 `2026-12-01` | `cmd/cli/main.go` |

**验收**：`go test ./...` 通过；新增用例覆盖 URL 构造与「不打印份额」。

### M4 · 前端：修掉静默投错票（对应 C4 / A1）

| 改动 | 文件 |
|---|---|
| 凭据记录其目标提案与目标选项 | `web/src/composables/useVotePortal.ts` |
| 选项或提案变化 → 清空 `encryptedVote` / `proof` | 同上 |
| 提交前校验「凭据目标 === 当前选中选项」，不一致则拒绝并提示 | 同上 + `web/src/components/panels/VotePanel.vue` |

**验收**：`pnpm --dir web run test` 通过；新增 composable 用例覆盖「改选项后提交被拒」。

### M5 · 前端：凭证与会话卫生（对应 A7 / A8 / A9 / R17 / R18 / R19）

| 改动 | 文件 |
|---|---|
| admin token 从 `localStorage` 改为 `sessionStorage` | `web/src/composables/useVotePortal.ts` |
| 切换提案时重置 `hasVoted` | 同上 |
| `error` 类 notice 不再 4.2 秒自动消失 | 同上 |

**验收**：`pnpm --dir web run test` + `lint` + `tsc` 全过。

### M6 · 前端文案对齐真相（对应 C5 / A2 / R22）

| 改动 | 文件 |
|---|---|
| 「计票前不会知道你选了谁」→ 改为「当前开发模式下服务端可见明文选项」 | `web/src/components/panels/VotePanel.vue` |
| 「术语参考」页 `proof` 说明同步 | `web/src/components/panels/ReferencePanel.vue` |

### M7 · 测试与脚本补强（对应 A15 / A16 / R24 / R29）

| 改动 | 文件 |
|---|---|
| 新增**跑通真实 gnark 验证**的 HTTP 层测试（vk 写 `t.TempDir()`，断言 201 / 400 缺 witness / 400 篡改） | `internal/infrastructure/http/server_test.go` |
| 覆盖率脚本不再覆盖 `GOMODCACHE` / `GOCACHE` | `scripts/check-coverage.sh` |

**验收**：新测试在**修复前应当失败**（TDD 的 RED 阶段），修复后通过。

### M8 · 文档与 CI（对应 A12 / R10 / 4.4 全部）

| 改动 | 文件 |
|---|---|
| README 覆盖率徽章改为**全仓口径**并标注门槛口径 | `README.md` |
| CI 增加 `gofmt` 检查与前端 lint / tsc / vitest | `.github/workflows/ci.yml` |
| 新建 `CHANGELOG.md` | `CHANGELOG.md` |
| 新建 `VERSION`（`0.1.0`） | `VERSION` |
| 新增 release workflow（tag → 构建 → 建 Release） | `.github/workflows/release.yml` |

---

## 3. 决策门（**在 v0.1.0 发版前需要你拍板**）

| # | 决策 | 选项 | 我的建议 |
|---|---|---|---|
| G1 | v0.1.0 的对外定位 | 内测版 / 直接叫生产版 | **内测版**，并在 README 首屏标注边界 |
| G2 | 门户是否保留「按选项生成凭据」按钮 | 保留并加警示 / 非 debug 模式隐藏 | **非 debug 模式隐藏**（R8） |
| G3 | `ballot-draft` 端点是否在生产可达 | 保留 / 非 debug 模式 404 | **非 debug 模式 404**（R8） |
| G4 | 盲签名：改实现还是改文档 | 两段式盲签名（周级） / 修正 README 措辞（小时级） | **先改文档**，v1.0.0 再动实现 |
| G5 | 首个版本号 | `0.1.0` / 其他 | `0.1.0`（可改，改 VERSION 文件即可） |

> G2 / G3 / G4 都**不阻断** v0.1.0 的代码落地；我按「建议」实现并标注，你可以在发版前一行回退。

---

## 4. v1.0.0 里程碑（**不在本计划内，需要你确认后启动**）

> 详细展开见 **[`docs/release-plan-v1.0.0.md`](./release-plan-v1.0.0.md)**（任务卡 T1–T19、
> 决策门 G6–G9、验收与风险）。下表只是摘要。

按依赖顺序，**风险最高的最先做**：

| 顺序 | 工作 | 说明 | 预估 |
|---|---|---|---|
| 1 | 真实选票电路 + `verifying.key` 生成流程 | 电路定义、密钥分发、轮换策略 | 周级 |
| 2 | 客户端 prover（浏览器 WASM） | 生成 proof + `public_witness` | 周级 |
| 3 | proof 绑定：`proposal_id` / `nullifier` / `hash(ciphertext)` 进公开输入 | 修 C3 | 天级 |
| 4 | 加密移到客户端 | 修 C5 根因 | 周级 |
| 5 | 盲签名改两段式 | 修 R2 根因 | 周级 |
| 6 | 覆盖率门槛扩到 crypto / persistence / cmd | 修 C6 | 天级 |
| 7 | PG 连接池、关掉启动自动迁移、CI 补全 | R10–R12 | 天级 |

---

## 5. 发版 go / no-go 清单

**No-Go 条件（任一命中则不发版）**

- [ ] `go build ./...` / `go vet ./...` / `gofmt -l` 有输出
- [ ] `go test ./...` 有失败
- [ ] `pnpm --dir web run lint` / `tsc --noEmit` / `run test` 任一失败
- [ ] 覆盖率门槛脚本退出码非 0
- [ ] 工作区有未提交改动
- [ ] README / techspec / 门户三处的边界声明不一致

**Go 条件**

- [ ] 上述全绿
- [ ] `git tag -l` 为空（本仓库从未发过版，本 tag 必须是首个）
- [ ] 决策门 G1–G5 已拍板
- [ ] `VERSION` 与 tag 一致
- [ ] 生产部署清单已确认（见下）

### 生产部署清单

| 项 | 要求 | 状态 |
|---|---|---|
| `SERVER_ADMIN_BEARER_TOKEN` | **必须显式设置**（M2 后临时 token 会直接启动失败） | ☐ |
| `SERVER_CORS_ALLOWED_ORIGINS` | **必须显式设置**为门户真实源 | ☐ |
| `CRYPTO_VK_PATH` | release 模式必填 | ☐ |
| `DB_DRIVER=postgres` + 连接参数 | 显式设置 | ☐ |
| 配置来源 | 确认生效的是 `config.yaml` 或环境变量，**不是** `config.production.yaml` | ☐ |
| TLS | 生产必须 HTTPS（否则 `selected_option` 类明文字段会裸奔） | ☐ |
| 门户静态资源 | `cmd/server` **不提供**静态托管，门户需单独部署 | ☐ |

---

## 6. 提交分组

按 `incremental-implementation` 原则，每个里程碑一个提交，Conventional Commits 格式：

```
chore(release): prepare v0.1.0 internal preview
fix(vote): return 400 instead of 500 for invalid proofs
fix(config): surface effective config and stop logging admin token
fix(config): require an explicit admin token in release mode
fix(cli): stop double-escaping URL paths
fix(cli): never print threshold shares to stdout
fix(web): bind ballot material to the selected option
fix(web): stop persisting the admin token in localStorage
fix(web): align anonymity copy with the actual implementation
test(http): cover real groth16 verification through the handler
ci: run gofmt and frontend checks in CI
docs: restate the coverage badge and add the changelog
```

---

## 7. 风险与回滚

| 风险 | 影响 | 缓解 |
|---|---|---|
| M2 让「没配 token 的 release 部署」直接起不来 | 现有部署中断 | 上线前先用 M1 的启动日志确认实际 token 来源；M2 单独成提交，便于回退 |
| M4 改动投票主流程 | 影响可用性 | M4 单独成提交 + 新增 composable 用例；必要时只回退 M4 |
| M8 的 CI 新增前端检查 | 可能让 CI 变红 | 本地已验证 lint/tsc/vitest 全绿；workflow 用 `--frozen-lockfile` |

每个提交都独立可 revert，不需要 `git revert` 序列。

---

## 8. 落地状态（2026-10-20）

M1–M8 的代码与文档**已全部落地**，分支 `release/v0.1.0-prep`（从 `main` @ `2398b00` 切出）。
实际提交与计划略有出入（拆得更细），以提交为准：

| 里程碑 | commit | 状态 |
|---|---|---|
| M1 | `cb78f32` `fix(vote): return 400 instead of 500 for invalid proofs` | ✅ |
| M2 | `54e1650` `fix(config): require an explicit admin token in release mode` | ✅ |
| M3 | `8100728` `fix(cli): stop double-escaping URL paths and leaking ballot choices` | ✅ |
| M4 | `795f2cf` `fix(web): bind ballot material to the selected option` | ✅ |
| M5 | `eb1d488` `fix(web): keep the admin token out of localStorage and fix stale vote state` | ✅ |
| M6 | `78ce0c7` `docs: align the anonymity claims with the actual implementation` | ✅ |
| M7 | `7ed2eb1` `test(http): cover real groth16 verification through the handler` | ✅ |
| M8 | `92e860f` / `e8bf315` / `b6f9f71` / `d9c1c3a` / `da7c5e7` / `b84a5d6` | ✅ |

M8 实际拆成 6 个提交：pnpm 构建脚本放行、CLI `version` 命令、PowerShell 覆盖率脚本
插值修复、CI 补 gofmt 与前端、发布流水线、文档。

**落地过程中新发现并已修掉的问题**（不在原 review 清单里）：

- `scripts/check-coverage.ps1` 的 `-coverprofile=$Profile` 在 PowerShell 参数模式下不插值，
  go test 把 `$Profile` 当字面量文件名写出 —— **门槛在 Windows 上从未真正生效**。
- 发布流水线最初用 `CGO_ENABLED=0` 交叉编译，实测产物启动即 FATAL
  （`go-sqlite3 requires cgo to work`），而默认配置就是 sqlite。已改为各平台原生构建 +
  冒烟启动测试。

### 尚未落地（等决策门拍板）

| 决策门 | 影响的改动 | 我的建议 |
|---|---|---|
| G2 | 非 debug 模式隐藏门户的「按选项生成凭据」按钮 | 保留现状也可发内测版，标注边界 |
| G3 | `ballot-draft` 非 debug 模式返回 404 | 建议做，R8 |
| G4 | 盲签名改文档措辞 | 建议做，小时级 |
| G1 / G5 | 版本定位与版本号 | 内测版 / `0.1.0` |

### 本计划范围内的唯一遗漏项

| 项 | 说明 | 去向 |
|---|---|---|
| A8 剩余半项 · 「查询投票状态」UI 入口 | `hasVoted` 跨提案重置已修（`eb1d488`），但 `fetchVoteStatus` 没有按钮能主动触发（唯一调用点在 `submitVote` 内部） | v1.0.0 计划的 **T11**（1 小时） |

其余未落地项全部属于证明链与生产化范畴，见
[`docs/release-plan-v1.0.0.md`](./release-plan-v1.0.0.md)（T1–T20，决策门 G6–G9）。

### 验证状态

| 检查 | 结果 |
|---|---|
| `go build` / `go vet` / `gofmt -l` | ✅ 全绿 |
| `go test ./...`（12 包） | ✅ 全绿 |
| 覆盖率门槛（bash） | ✅ 82.30% ≥ 80% |
| 覆盖率门槛（PowerShell，修复后） | ✅ 82.30% ≥ 80% |
| `pnpm install --frozen-lockfile` | ✅ 通过 |
| 前端 lint / tsc / vitest（57 用例）/ 生产构建 | ✅ 全绿 |
| 发布流水线的构建 + 冒烟 + 打包（本地模拟） | ✅ 通过 |
| PostgreSQL testcontainers 集成测试 | ⚠️ 本机无 Docker，**CI 会实跑**，分支未本地验证 |
| 门户 7 个 `.vue` 面板浏览器渲染 | ⚠️ 无自动化覆盖 |

