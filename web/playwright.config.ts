import { defineConfig, devices } from '@playwright/test';

/**
 * 门户端到端测试（T28）的配置。
 *
 * 这里跑的是**真浏览器**：真 WASM prover、真盲签名、真资格证明、真服务端验签。
 * 与 vitest 那套「服务端渲染测试」互补 —— 后者验模板与状态机，前者验**接线**：
 * 跨语言的东西对不上（盲签名变体名、字段名、编码、响应式 Proxy）时，
 * 只有真浏览器能发现。
 *
 * 环境由 `scripts/web-e2e.sh` 备好（两套密钥、资格名单、提案、凭证池、
 * 解密会话、门户 dev server），参数通过环境变量传进来。
 */

/** 门户地址：环境脚本起的 dev server。 */
const portalURL = process.env.E2E_PORTAL_URL ?? 'http://127.0.0.1:3100';

export default defineConfig({
  testDir: './e2e',
  /**
   * 单条用例要走完整条链路：下载并编译证明电路（十几秒）+ 领取 + 出票 + 提交，
   * 还要**等投票窗口关闭**才能计票。默认 30 秒的超时远远不够。
   */
  timeout: 300_000,
  expect: { timeout: 30_000 },
  /**
   * 用例之间有顺序依赖（同一个提案、同一份资格、同一场投票窗口），
   * 不能并行，也不能各自开一份干净的环境。
   */
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    baseURL: portalURL,
    /** 失败时保留 trace，便于复现与定位。 */
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
  },
});
