import { beforeEach, describe, expect, it } from 'vitest';
import { createPortalApi, renderPanel } from '../../lib/render-panel';
import type { PortalApi } from '../../types/portal-api';
import type { ProposalItem } from '../../types/portal';
import HallPanel from './HallPanel.vue';
import IdentityPanel from './IdentityPanel.vue';
import OpsPanel from './OpsPanel.vue';
import OverviewPanel from './OverviewPanel.vue';
import ReferencePanel from './ReferencePanel.vue';
import SetupPanel from './SetupPanel.vue';
import VotePanel from './VotePanel.vue';

/**
 * 门户面板的渲染测试。
 *
 * 此前这 7 个 `.vue` 面板**既没有类型检查也没有任何测试** —— `tsc` 不认 `.vue`
 * 文件，所以模板里的笔误（比如引用一个不存在的变量）只会在运行时炸。
 * 这里用 Vue 的服务端渲染把面板渲染成 HTML 字符串：模板、setup、computed 都会
 * 真的跑一遍。
 *
 * **这不是浏览器渲染**：没有 CSS、没有事件绑定、没有客户端挂载。抓的是
 * 「模板引用了不存在的变量」「setup 抛错」「条件分支写反」这一类问题。
 */

const proposal: ProposalItem = {
  id: 'p-1',
  title: '年度提案',
  description: '一个用于渲染测试的提案',
  options: ['候选人 A', '候选人 B'],
  status: 'open',
  startsAt: '2026-10-25T00:00:00Z',
  endsAt: '2026-11-25T00:00:00Z',
};

function portalWithProposal(): PortalApi {
  const portal = createPortalApi();
  portal.hallState.proposals = [proposal];
  portal.opsProposalsState.proposals = [proposal];
  portal.voteForm.proposalId = proposal.id;
  return portal;
}

describe('面板渲染', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it('七个面板都能渲染出来', async () => {
    const portal = createPortalApi();
    const panels = [
      ['OverviewPanel', OverviewPanel],
      ['SetupPanel', SetupPanel],
      ['HallPanel', HallPanel],
      ['IdentityPanel', IdentityPanel],
      ['VotePanel', VotePanel],
      ['OpsPanel', OpsPanel],
      ['ReferencePanel', ReferencePanel],
    ] as const;

    for (const [name, component] of panels) {
      const html = await renderPanel(component, portal);
      expect(html.length, `${name} 渲染结果为空`).toBeGreaterThan(0);
    }
  });
});

describe('投票大厅', () => {
  it('没有开放提案时给出空态', async () => {
    const html = await renderPanel(HallPanel, createPortalApi());
    expect(html).toContain('暂无开放中的提案');
  });

  it('有提案时列出标题与选项', async () => {
    const html = await renderPanel(HallPanel, portalWithProposal());
    expect(html).toContain('年度提案');
    expect(html).toContain('候选人 A');
    expect(html).not.toContain('暂无开放中的提案');
  });
});

describe('总览', () => {
  it('没连通后端时提示先做健康检查', async () => {
    const html = await renderPanel(OverviewPanel, createPortalApi());
    expect(html).toContain('尚未连接后端服务');
  });

  it('连通后显示已连通', async () => {
    const portal = createPortalApi();
    portal.healthState.ok = true;
    portal.healthState.text = '后端可达';
    const html = await renderPanel(OverviewPanel, portal);
    expect(html).toContain('已连通');
    expect(html).not.toContain('尚未连接后端服务');
  });
});

describe('连接配置', () => {
  it('没有 admin token 时给出警告', async () => {
    const html = await renderPanel(SetupPanel, createPortalApi());
    expect(html).toContain('未配置 Admin Token');
  });

  it('配了 token 就不再警告', async () => {
    const portal = createPortalApi();
    portal.adminBearerToken = 'secret';
    const html = await renderPanel(SetupPanel, portal);
    expect(html).not.toContain('未配置 Admin Token');
  });
});

describe('身份面板', () => {
  it('展示匿名领取入口与凭证包空态', async () => {
    const html = await renderPanel(IdentityPanel, createPortalApi());
    expect(html).toContain('匿名领取凭证');
    expect(html).toContain('尚未领取凭证');
  });

  it('管理端签发需要 admin token', async () => {
    const portal = createPortalApi();
    const html = await renderPanel(IdentityPanel, portal);
    expect(html).toContain('Admin Token');
  });
});

describe('投票面板', () => {
  // 服务端代加密的入口已经整个删掉了：门户只有「在浏览器本地生成选票」这一条路。
  it('不提供「让服务端代生成凭据」的入口', async () => {
    const html = await renderPanel(VotePanel, portalWithProposal());
    expect(html).not.toContain('>按选项生成凭据</button>');
    expect(html).toContain('与匿名投票的前提冲突');
  });

  it('没领凭证时提示先去领', async () => {
    const html = await renderPanel(VotePanel, createPortalApi());
    expect(html).toContain('尚未领取匿名凭证');
  });
});

describe('运营面板', () => {
  it('没有 admin token 时提示需要令牌', async () => {
    const html = await renderPanel(OpsPanel, createPortalApi());
    expect(html).toContain('未配置 Admin Token');
  });

  it('有 admin token 时进入可操作状态', async () => {
    const portal = portalWithProposal();
    portal.adminBearerToken = 'secret';
    const html = await renderPanel(OpsPanel, portal);
    expect(html).not.toContain('未配置 Admin Token');
    expect(html).toContain('创建提案');
  });
});

describe('参考面板', () => {
  it('渲染出术语说明', async () => {
    const html = await renderPanel(ReferencePanel, createPortalApi());
    expect(html).toContain('proposal_id');
    expect(html).toContain('nullifier');
  });
});
