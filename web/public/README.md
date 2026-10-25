# web/public —— 门户的静态资源目录

这个目录里的东西由 **构建 / 部署流程**放进来，不是源码：

| 文件 | 来源 | 能不能进仓库 |
|---|---|---|
| `ballot-prover.wasm` | `make web-wasm`（编译 `cmd/ballot-wasm`） | ❌ 构建产物 |
| `wasm_exec.js` | `make web-wasm`（从 `$(go env GOROOT)/lib/wasm/` 拷贝） | ❌ 第三方胶水，跟着工具链走 |
| `proving.key` | `make web-proving-key PK=<路径>`（运营方从 `cmd/keygen` 拿到） | ❌ **绝不能进仓库** |
| `eligibility-proving.key` | `make web-eligibility-key PK=<路径>`（运营方从 `cmd/keygen -circuit eligibility` 拿到） | ❌ **绝不能进仓库** |

## 为什么 proving key 放在门户的静态目录

选票电路的 proving key **只在客户端侧使用**：服务端只拿 verifying key
（`crypto.vk_path`）。有它的人可以自己出证明，但证明只绑定密文、不绑定身份，
不构成新的身份泄露（身份约束来自凭证签名 + 一张凭证一票）。

浏览器端 prover 需要 proving key 才能出证明，因此它必须能被**投票人下载**。
门户的静态目录是最省事也最可控的发布位置：它由运营方单独部署，票选期间
对外公开，不进服务端镜像、不进数据库。

对应地，**verifying key 必须在服务端**（`crypto.vk_path` 或 `CRYPTO_VK_PATH`），
两边必须是同一对密钥 —— 用 `cmd/keygen` 一次生成、一次分发。

## 两把 proving key 不是同一把

门户要出两种证明，各需要各自的 proving key：

| 电路 | 客户端 proving key | 服务端 verifying key |
|---|---|---|
| 选票 | `proving.key` | `verifying.key`（`crypto.vk_path`） |
| 资格（一人一票） | `eligibility-proving.key` | `eligibility-verifying.key`（`crypto.eligibility_vk_path`） |

Groth16 的 setup 是 **per-circuit** 的，两把密钥互不相通、不能互相替代。
