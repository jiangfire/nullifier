import { readFileSync } from 'node:fs';
import { expect, test, type Page } from '@playwright/test';

/**
 * 门户的两条端到端路径（T28）。
 *
 * 覆盖的正是 [`docs/deployment-checklist.md`](../../docs/deployment-checklist.md)
 * §8 / §8.1 里那张「每次上线都要人工点一遍」的表：
 *
 * 1. **投票路径**：领凭证 → 在浏览器本地生成选票 → 提交 → 查询状态 → 计票；
 * 2. **资格路径**：导入资格文件 → 匿名领取 → 重复领取被拒（一人一票）。
 *
 * 为什么必须是真浏览器：门户的密码学在 **WASM + Web Worker** 里，
 * 盲签名的变体名、字段名、编码只要有一处跨语言对不上，流程就会停在半路
 * （服务端只看到 claim，收不到 blind-sign）。node 驱动的单元测试测不出这个，
 * vitest 的服务端渲染测试更测不出 —— 只有真浏览器能把整条链接上。
 *
 * 环境（两套密钥、资格名单、提案、凭证池、解密会话、门户 dev server）
 * 由 `scripts/web-e2e.sh` 备好，参数通过环境变量传进来。
 */

/** 后端地址。 */
const API_BASE = process.env.E2E_API_BASE ?? '';
/** 管理端 Bearer 令牌（发布结果要用）。 */
const ADMIN_TOKEN = process.env.E2E_ADMIN_TOKEN ?? '';
/** 本场投票的提案 ID。 */
const PROPOSAL_ID = process.env.E2E_PROPOSAL_ID ?? '';
/** 提案标题（大厅里靠它找到那张卡片）。 */
const PROPOSAL_TITLE = process.env.E2E_PROPOSAL_TITLE ?? '';
/** 投票窗口的开始时刻（RFC3339）。 */
const STARTS_AT = process.env.E2E_STARTS_AT ?? '';
/** 投票窗口的结束时刻（RFC3339）—— 计票必须等它过去。 */
const ENDS_AT = process.env.E2E_ENDS_AT ?? '';
/** 成员自己的资格包（导入门户的那份文件）。 */
const ELIGIBILITY_FILE = process.env.E2E_ELIGIBILITY_FILE ?? '';
/** 门限份额文件（`decrypt-setup -out` 的产物）。 */
const SHARES_FILE = process.env.E2E_SHARES_FILE ?? '';
/** 本次要投的选项。 */
const ACCEPT_OPTION = process.env.E2E_ACCEPT_OPTION ?? '';

/** 等到某个时刻。投票窗口的开与关都靠它对齐。 */
async function waitUntil(iso: string): Promise<void> {
  const target = Date.parse(iso);
  if (Number.isNaN(target)) {
    throw new Error(`E2E_STARTS_AT / E2E_ENDS_AT 不是合法时间：${iso}`);
  }
  const delay = target - Date.now();
  if (delay > 0) {
    await new Promise((resolve) => setTimeout(resolve, delay));
  }
}

/** 全局消息条（右上角的提示）。 */
function notice(page: Page, text: string) {
  return page.locator('.notice', { hasText: text });
}

/** 读出门限份额，拼成门户「发布结果」输入框要的格式。 */
function readShares(): string {
  const raw = JSON.parse(readFileSync(SHARES_FILE, 'utf8')) as { shares?: string[] };
  const shares = raw.shares ?? [];
  if (shares.length === 0) {
    throw new Error(`份额文件里没有份额：${SHARES_FILE}`);
  }
  return shares.join(' ');
}

test('门户：领凭证 → 本地出票 → 提交 → 查状态 → 计票；资格重复领取被拒', async ({ page }) => {
  // 首次访问会弹出新手引导浮层（它会挡住所有点击）。
  // 这里直接把它标成「已看过」—— 引导浮层不是本用例要测的东西。
  await page.addInitScript(() => {
    window.localStorage.setItem('anonvote.front.onboarding_seen', '1');
  });

  await test.step('配置后端连接', async () => {
    await page.goto('/');
    await page.locator('.shell-nav').getByRole('button', { name: '连接配置' }).click();
    await page.locator('#setup-api-base-url').fill(API_BASE);
    await page.locator('#setup-admin-token').fill(ADMIN_TOKEN);
    await page.getByRole('button', { name: '检测连接' }).click();
    await expect(page.locator('.pill', { hasText: '已连通' })).toBeVisible();
  });

  await test.step('导入资格文件并匿名领取凭证', async () => {
    await page.locator('.shell-nav').getByRole('button', { name: '我的身份' }).click();
    await page.locator('#identity-claim-proposal-id').fill(PROPOSAL_ID);

    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('button', { name: '导入资格文件' }).click();
    await (await chooser).setFiles(ELIGIBILITY_FILE);
    await expect(notice(page, '资格文件已导入')).toBeVisible();

    // 领取要现下载 prover、编译资格电路、再出资格证明：几十秒。
    await page.getByRole('button', { name: '匿名领取' }).click();
    // exact 是必须的：面板里的卡片标题「匿名领取凭证」加上徽标「已就绪」
    // 拼起来也含「凭证已就绪」，不精确匹配会撞上两个元素。
    await expect(page.getByText('凭证已就绪', { exact: true })).toBeVisible({
      timeout: 180_000,
    });
  });

  await test.step('同一份资格重复领取被拒（一人一票）', async () => {
    await page.getByRole('button', { name: '匿名领取' }).click();
    await expect(notice(page, '这份资格已经领过票了')).toBeVisible({
      timeout: 180_000,
    });
  });

  await test.step('投票窗口打开后在投票大厅选中提案', async () => {
    // 窗口没开时提案不在大厅里（服务端只列开放中的投票）——
    // 反复刷新直到它出现（窗口刚开的那一瞬可能还差几毫秒）。
    await waitUntil(STARTS_AT);
    await page.locator('.shell-nav').getByRole('button', { name: '投票大厅' }).click();
    await expect
      .poll(
        async () => {
          await page.getByRole('button', { name: '刷新' }).click();
          return page.locator('.card', { hasText: PROPOSAL_TITLE }).count();
        },
        { timeout: 60_000, message: '投票窗口打开后大厅里应出现这场提案' },
      )
      .toBeGreaterThan(0);
    await page
      .locator('.card', { hasText: PROPOSAL_TITLE })
      .getByRole('button', { name: '去投票' })
      .click();
  });

  await test.step('在浏览器本地生成选票', async () => {
    // 门户默认选中的是**第一个选项**：要投 beta 就得自己点一下。
    await page.getByRole('radio', { name: ACCEPT_OPTION }).check();
    await page.getByRole('button', { name: '在浏览器本地生成选票' }).click();
    await expect(notice(page, '生成成功')).toBeVisible({ timeout: 180_000 });
    await expect(page.locator('#vote-encrypted-vote')).not.toHaveValue('');
  });

  await test.step('提交匿名选票', async () => {
    await page.getByRole('button', { name: '提交匿名选票' }).click();
    await expect(notice(page, '投票成功')).toBeVisible({ timeout: 60_000 });
  });

  await test.step('查询投票状态', async () => {
    await page.getByRole('button', { name: '查询投票状态' }).click();
    await expect(notice(page, '查询完成')).toBeVisible({ timeout: 60_000 });
    await expect(page.getByText('你已投过这一票')).toBeVisible();
  });

  await test.step('投票窗口关闭后发布结果并核对计票', async () => {
    // 计票的前提是提案已关闭 —— 服务端只认窗口结束，不认「手动关闭」。
    await waitUntil(ENDS_AT);
    await page.locator('.shell-nav').getByRole('button', { name: '运营' }).click();
    await page.locator('.stepper-item', { hasText: '发布结果' }).click();
    await page.locator('#ops-publish-proposal-id').fill(PROPOSAL_ID);
    await page.locator('#ops-publish-shares').fill(readShares());
    // 作用域限定在表单里：步骤条上的「发布结果」也是一个按钮，
    // 不限定就会撞上两个同名元素。
    await page
      .locator('#guide-anchor-admin-publish')
      .getByRole('button', { name: '发布结果' })
      .click();
    await expect(notice(page, '发布成功')).toBeVisible({ timeout: 60_000 });

    await page.locator('#ops-result-proposal-id').fill(PROPOSAL_ID);
    await page
      .locator('#guide-anchor-admin-result')
      .getByRole('button', { name: '查询' })
      .click();
    await expect(notice(page, '查询完成')).toBeVisible({ timeout: 60_000 });
    // 限定在结果表单里：消息条上也有同一句摘要（发布成功 / 查询完成各一条）。
    await expect(
      page.locator('#guide-anchor-admin-result').getByText('总票数 1'),
    ).toBeVisible();
    const row = page.locator('.tally-row', { hasText: ACCEPT_OPTION });
    await expect(row.locator('.tally-count')).toHaveText('1');
  });
});
