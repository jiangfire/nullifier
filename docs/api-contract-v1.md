# nullifier 接口契约 v1（已冻结）

> **状态**：**已冻结**（2026-10-25，v0.3.0）。
> 本文件是 v0.3.0 的承诺：**下列三处接口从现在起只增不改**。
> 破坏性变更必须走下一个 minor 版本（v0.4.0）。
>
> 配套：[`docs/release-plan-v0.3.0.md`](./release-plan-v0.3.0.md)（本版的范围与出口标准）、
> [`docs/techspec.md`](./techspec.md)（当前实现口径）、
> [`docs/project-map.md`](./project-map.md)（上下文索引）
>
> **守卫**：`tests/architecture/api_contract_test.go` 把本文件的关键事实钉成测试 ——
> 改了一边不改另一边，CI 会红。**本文件与实现不允许漂移**。

---

## 0. 为什么要有这份文件

v0.2.0 之后的推演里，「凭证领取 / 投票提交 / 密文格式这三处接口**大概率还要再动一轮**」
是路线图 §1.1 的原话。v0.3.0 就是那一轮 —— **动完之后冻结**。

「冻结」不是一句承诺，是三样东西：

1. **本文件**：逐字段写清楚请求与响应。
2. **门禁测试**：`tests/architecture/api_contract_test.go` 双向锁死 Go 侧与 TS 侧。
3. **密文格式定版**：格式号与长度写死，不再变。

> ⚠️ **冻结的范围是「线格式」**：字段名、字段的语义、端点路径、密文格式。
> **不在范围内**：响应里可以**新增**字段（客户端必须忽略不认识的字段）、
> 服务端可以新增**端点**。这两者是「只增不改」允许的。
> **改字段名、删字段、改语义、改路径** —— 一律是破坏性变更。

---

## 1. 凭证领取（三步盲签名 + 一步挑战）

领取凭证是**公开端点**（不需要 admin token）：请求体里**不含任何真人身份字段**。

### 1.1 `POST /api/v1/proposals/{proposal_id}/credentials/eligibility-challenge`

取一次性的领取挑战。服务端**不记账**（无状态）—— 重放由去重键挡住。

**请求体**：空。

**响应 200**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `proposal_id` | string | 提案 ID |
| `root` | string | 资格名单的 Merkle 树根（十六进制域元素） |
| `depth` | number | 树深（当前是编译期常量 10） |
| `challenge` | string | 本次领取挑战（十六进制域元素） |

### 1.2 `POST /api/v1/proposals/{proposal_id}/credentials/claim`

预留一个池槽位。**这一步不签名** —— 客户端拿 `user_id` 与签发方公钥后本地盲化。

**请求体**（三件要么都给、要么都不给；都不给是合法的 —— 未开启资格校验时的形态）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `eligibility_proof` | string | 三件同进同出 | 资格证明（base64 的 Groth16 证明） |
| `qualification_id` | string | 三件同进同出 | 去重键（十六进制域元素） |
| `challenge` | string | 三件同进同出 | 1.1 拿到的挑战 |

> ⚠️ **只给一半会被拒**（400 `INVALID_REQUEST`）—— 那说明客户端出了错，
> 不能当成「没带资格」放行。

**响应 201**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `proposal_id` | string | 提案 ID |
| `user_id` | string | 池槽位的假名 |
| `issuer_key_version` | number | 签发方密钥版本 |
| `variant` | string | 盲签名变体的**线路名**（如 `RSABSSA-SHA384-PSS-Deterministic`） |
| `modulus` | string | 签发方 RSA 模数（base64） |
| `exponent` | number | 签发方 RSA 指数 |

> ⚠️ `variant` 是**线路名**，不是 Go 的枚举标识符 —— 写错它，领取会停在 claim 之后
> （blind-sign 从未发出）。这条由 `tests/architecture/eligibility_wire_test.go` 双向锁死。

### 1.3 `POST /api/v1/proposals/{proposal_id}/credentials/blind-sign`

对**盲化后**的消息签名。服务端在签名阶段**只见得到盲化消息**。

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user_id` | string | ✅ | 1.2 拿到的槽位假名 |
| `blinded_message` | string | ✅ | 客户端盲化后的消息（base64） |
| `issuer_key_version` | number | ✅ | 盲化时用的那一版密钥 |

**响应 200**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `blind_signature` | string | 盲签名（base64） |

### 1.4 `POST /api/v1/proposals/{proposal_id}/credentials/attest`

交回解盲后的最终签名，服务端验签后落库。

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user_id` | string | ✅ | 槽位假名 |
| `user_pub_key` | string | ✅ | 领取者的一次性公钥（base64） |
| `signature` | string | ✅ | 解盲后的最终签名（base64） |

**响应 201**：凭证对象 —— `id` / `proposal_id` / `user_id` / `user_pub_key` /
`signature` / `issuer_key_version` / `issued_at`。

> ⚠️ **attest 阶段服务端看得到公钥** —— 验签必须重算凭证消息。
> 能力 F 的准确口径是「**签名阶段**服务端只见盲化消息」，不是「全程看不见」。

---

## 2. 投票提交

### `POST /api/v1/proposals/{proposal_id}/votes`

> ⚠️ **v0.3.0 的破坏性变更**：这个端点原先是 `POST /api/v1/votes`，
> 且请求体里带 `proposal_id`。现在提案 ID **只从路径来**。

**请求体**（六个字段**全部必填**）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `encrypted_vote` | string | v3 选票密文（base64，65 字节） |
| `proof` | string | Groth16 证明（base64） |
| `nullifier` | string | 32 字节域元素的**规范** hex（64 位小写） |
| `user_id` | string | 凭证槽位假名 |
| `user_pub_key` | string | 一次性公钥（base64） |
| `credential_signature` | string | 凭证签名（base64） |

**响应 201**：`{"status":"ok"}`

> ⚠️ **请求体里没有 `proposal_id`** —— 一个提案 ID 只有**一个**来源。
> 也不接受 `selected_option`：服务端不消费它，而明文选项上服务端会抵消
> 「选票是加密的」这个属性。
> 公开输入由服务端自拼，**不接受** `public_witness`。

**错误码**：

| 码 | HTTP | 含义 |
|---|---|---|
| `INVALID_REQUEST` | 400 | 请求体不合法 / 证明验签失败 / 密文不合法 |
| `INVALID_CREDENTIAL` | 401 | 凭证不存在、不匹配或验签失败 |
| `DOUBLE_VOTING` | 409 | 同一个 nullifier 已经投过 |
| `USER_ALREADY_VOTED` | 409 | 同一张凭证已经投过 |
| `RATE_LIMITED` | 429 | 触发了限流（见 §2.1） |

### 2.1 限流（429）—— **加法，不构成破坏性变更**

v0.3.0 给公开端点加了限流（设计稿见
[`t25-ratelimit-design.md`](./t25-ratelimit-design.md)）。触发时服务端返回：

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 3

{"error":"rate limit exceeded","code":"RATE_LIMITED"}
```

**为什么这不算破坏性变更**（决策 V3-D8）：

- 冻结的是**线格式** —— 字段名、字段语义、端点路径、密文格式。
  429 是**新增的一种状态码**：请求体的字段与路径**一个字都没变**，
  成功路径的响应也一个字都没变。属于「只增不改」允许的加法。
- 客户端要做的只有一件事：**处理 429**（读 `Retry-After`，稍后重试）。
  不处理的后果是「偶尔失败一次」，而不是「协议对不上」。

**客户端建议**：收到 429 时按 `Retry-After` 退避后重试（同一个请求是幂等的 ——
投票提交被限流挡下时它**没有**进业务层，库里没有这张票，重试是安全的）。

> ⚠️ 注意：限流是**按来源地址**计数的。机构集中投票时所有人共用一个出口 IP，
> 额度要按实际规模调高 —— 部署清单里有这一项。

---

## 3. 选票密文格式（**已定版**）

> **v3 是「不再变」的格式。** 格式号 `0x03` 与长度 65 字节由门禁测试钉死 ——
> 改动它们就是破坏性变更，必须走下一个 minor 版本。

```
偏移  长度  内容
0     1     格式版本号：0x03（v3）
1     32    压缩点 a = r·G
33    32    压缩点 b = m·G + r·Y
```

- 总长 **65 字节**。
- 曲线：**BN254 twisted Edwards**。
- 点编码：**压缩点**（Y 的大端表示，最高位放 X 的符号位，整体小端存放）——
  与 gnark-crypto 的编码**逐字节一致**。
- 加密的是**选项下标** `m`（不是候选项名字）。
- `r` 是一次性随机数（不进密文）。

**与电路的关系**：`a = r·G`、`b = m·G + r·Y`、`m < NumOptions` 是选票电路的约束 ——
电路在 BN254 标量域里复算，所以密文必须跑在同一条曲线上。

**两侧常量**（门禁测试双向锁死）：

| | Go | TS |
|---|---|---|
| 版本号 | `ballot.CipherVersion` | `ballotCipherVersion` |
| 总长 | `ballot.CipherSize` | `ballotCipherSize` |
| 点长 | `ballot.PointSize` | `ballotPointSize` |

> ⚠️ **跨语言必须逐字节一致** —— 改了曲线参数或点编码，两边就对不上，
> 且症状是「密文解不开」而不是报错。

---

## 4. 冻结清单（门禁测试覆盖的项）

| # | 冻结项 | 门禁断言 |
|---|---|---|
| 1 | 投票提交请求体的**字段名集合** | Go 的 `submitVotePayload` json tag ↔ TS 提交的字段，两边**完全相等** |
| 2 | 投票提交的**端点路径** | Go 侧注册的路由 ↔ TS 侧请求的路径 |
| 3 | 凭证领取四个端点的**路径** | Go 侧注册的路由 ↔ TS 侧请求的路径 |
| 4 | 凭证领取请求体的**字段名集合** | Go 的 payload json tag ↔ TS 构造的字段 |
| 5 | 密文的**版本号** | Go `CipherVersion` ↔ TS `ballotCipherVersion`，且都等于 3 |
| 6 | 密文的**总长与点长** | Go `CipherSize`/`PointSize` ↔ TS 对应常量 |
| 7 | 投票提交请求体里**没有** `proposal_id` | 字段集合断言（它是路径参数） |
| 8 | 投票提交请求体里**没有** `selected_option` | 字段集合断言 |
| 9 | **错误码集合**两边一致 | Go 的 `MapError` 码集合 ↔ TS 的 `PortalErrorCode` 成员集合，完全相等（`tests/architecture/error_code_test.go`） |
| 10 | 每个错误码都有**中文文案** | 两侧各钉一条（Go 侧在门禁里，TS 侧在 `web/src/lib/error-messages.test.ts`） |

---

## 5. 怎么改一个冻结的接口

**先假定你不能改。** 冻结的接口要改，通常说明有别的地方错了。

真的需要改时，按这个顺序：

1. **确认这确实不是 bug** —— 多数「接口要改」的诉求，真实原因是客户端用错了。
2. **评估是不是能只增** —— 新增一个字段 / 新增一个端点，不破坏兼容。
3. **确实要破坏** → 走**下一个 minor 版本**（v0.4.0）：改代码 → 改本文件 →
   改门禁测试 → 写 CHANGELOG 的「破坏性变更」段 → 更新部署清单。

> **不要在补丁版本里破坏冻结的接口。** 补丁版本（v0.3.1）只修 bug。

---

## 6. 相关文档

| 文档 | 内容 |
|---|---|
| [`docs/release-plan-v0.3.0.md`](./release-plan-v0.3.0.md) | v0.3.0 的范围、任务卡与出口标准 |
| [`docs/techspec.md`](./techspec.md) | 当前实现的接口与流程口径 |
| [`docs/roadmap.md`](./roadmap.md) | 产品路线图与里程碑 |
| [`docs/deployment-checklist.md`](./deployment-checklist.md) | 生产部署清单 |
