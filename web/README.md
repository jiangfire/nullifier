# 匿名投票前端门户（web）

这是一个面向用户和运营人员的业务页面，不是接口调试台。
页面围绕真实流程组织：浏览提案、领取凭证、参与投票、计票与审计。

## 本地运行

```bash
pnpm install
pnpm run dev
```

默认地址：`http://localhost:3000`

## 已覆盖接口

1. `GET /healthz`
2. `POST /api/v1/credentials`
3. `POST /api/v1/proposals`
4. `GET /api/v1/proposals/open`
5. `POST /api/v1/votes`
6. `POST /api/v1/proposals/:proposal_id/vote-status`
7. `GET /api/v1/proposals/:proposal_id/credentials`
8. `POST /api/v1/proposals/:proposal_id/ballot-draft`
9. `POST /api/v1/proposals/:proposal_id/decryption/setup`
10. `POST /api/v1/proposals/:proposal_id/results/publish`
11. `GET /api/v1/proposals/:proposal_id/results`
12. `GET /api/v1/proposals/:proposal_id/audit-anchors`

## 页面模块

1. `投票大厅`：查看开放提案并进入投票
2. `我的身份`：领取或导入匿名凭证；领取接口需要管理员令牌，支持凭证文件导入/导出
3. `参与投票`：按提案选项选择投票意向，支持“一键生成投票凭据”或导入投票凭据 JSON 后提交
4. `计票与审计`：创建提案（支持多选项）、初始化解密、发布结果、查询结果、查看审计锚点、查看提案内凭证状态

## 推荐使用顺序

1. 在顶部填后端地址并检测连接
2. 在 `投票大厅` 先选定提案
3. 在 `计票与审计` 为该提案初始化解密会话
4. 在 `我的身份` 领取或导入该提案的匿名凭证
5. 在 `参与投票` 按选项生成投票凭据并提交（或导入外部凭据）
6. 回到 `计票与审计` 查看提案内凭证状态、结果和审计锚点

## 鉴权说明

- 投票状态查询不再接受裸 `user_id` 枚举，前端会使用当前凭证材料发起校验式查询。
- 凭证签发和所有管理接口都需要在页面顶部填写 `Admin Token`，前端会自动附带 `Authorization: Bearer <token>`。
- 凭证状态面板位于 `计票与审计` 中，只显示某提案下哪些凭证已投票，不显示具体投票内容。

提示：在 `参与投票` 中可使用“按选项生成投票凭据”，该能力依赖后端
`POST /api/v1/proposals/:proposal_id/ballot-draft`，并需要先初始化对应提案的解密会话。
后端当前会拒绝“投票开放中”的 `decryption/setup`，因此应在提案开始前完成初始化。
该草稿凭据用于本地联调（开发模式），生产模式请使用真实钱包/客户端生成 proof 与密文。

## 质量检查

```bash
pnpm run lint
pnpm run build
pnpm exec tsc --noEmit
```
