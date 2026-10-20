import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { useVotePortal } from './useVotePortal';

/**
 * node 环境没有 DOM，composable 初始化时就要读 localStorage，这里补一个最小实现。
 */
const storage = new Map<string, string>();
Object.defineProperty(globalThis, 'localStorage', {
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value);
    },
    removeItem: (key: string) => {
      storage.delete(key);
    },
    clear: () => {
      storage.clear();
    },
  },
  configurable: true,
});

/** node 环境没有 window，composable 用它做定时器与地址栏同步。 */
Object.defineProperty(globalThis, 'window', {
  value: {
    setTimeout: (handler: () => void, timeout?: number) =>
      setTimeout(handler, timeout) as unknown as number,
    clearTimeout: (timer: number) => clearTimeout(timer),
    location: {
      href: 'http://localhost:3000/',
      search: '',
      origin: 'http://localhost:3000',
    },
  },
  configurable: true,
});

/** node 环境没有 sessionStorage，管理令牌不应该落到 localStorage。 */
const sessionStore = new Map<string, string>();
Object.defineProperty(globalThis, 'sessionStorage', {
  value: {
    getItem: (key: string) => sessionStore.get(key) ?? null,
    setItem: (key: string, value: string) => {
      sessionStore.set(key, value);
    },
    removeItem: (key: string) => {
      sessionStore.delete(key);
    },
    clear: () => {
      sessionStore.clear();
    },
  },
  configurable: true,
});

interface FetchCall {
  url: string;
  body: Record<string, unknown> | null;
}

function stubFetch(
  routes: Array<{ match: (url: string) => boolean; payload: unknown }>,
): FetchCall[] {
  const calls: FetchCall[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown, init?: RequestInit) => {
      const url = String(input);
      let body: Record<string, unknown> | null = null;
      if (init?.body) {
        body = JSON.parse(String(init.body)) as Record<string, unknown>;
      }
      calls.push({ url, body });
      const route = routes.find((item) => item.match(url));
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify(route ? route.payload : {}),
      };
    }),
  );
  return calls;
}

function setupPortal() {
  storage.clear();
  sessionStore.clear();
  storage.set('anonvote.front.base_url', 'http://localhost:8080');

  const portal = useVotePortal();
  Object.assign(portal.wallet, {
    proposalId: 'p1',
    userId: 'u-1',
    userPubKey: 'pub-key',
    signature: 'signature',
  });
  portal.voteForm.proposalId = 'p1';
  portal.selectedVoteOption.value = 'A';
  return portal;
}

const draftRoute = {
  match: (url: string) => url.includes('ballot-draft'),
  payload: {
    proposal_id: 'p1',
    option: 'A',
    encrypted_vote: 'cipher-A',
    proof: 'proof-A',
    nullifier: 'null-1',
  },
};

describe('投票凭据与当前意图的绑定', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('按选项生成凭据后，材料绑定到该选项', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();

    await portal.prepareBallotByOption();

    expect(portal.voteForm.encryptedVote).toBe('cipher-A');
    expect(portal.voteForm.proof).toBe('proof-A');
    expect(portal.voteForm.ballotOption).toBe('A');
    expect(portal.voteForm.ballotProposalId).toBe('p1');
    expect(portal.ballotTargetCurrent.value).toBe(true);
  });

  it('改了选项之后，已生成的凭据被清空', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();
    await portal.prepareBallotByOption();

    portal.selectedVoteOption.value = 'B';
    await nextTick();

    expect(portal.voteForm.encryptedVote).toBe('');
    expect(portal.voteForm.proof).toBe('');
    expect(portal.voteForm.ballotOption).toBe('');
  });

  it('换了提案之后，已生成的凭据被清空', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();
    await portal.prepareBallotByOption();

    portal.voteForm.proposalId = 'p2';
    await nextTick();

    expect(portal.voteForm.encryptedVote).toBe('');
    expect(portal.voteForm.proof).toBe('');
  });

  it('凭据还指向旧选项时拒绝提交（防止静默投错票）', async () => {
    const calls = stubFetch([
      draftRoute,
      { match: (url) => url.includes('/api/v1/votes'), payload: { status: 'ok' } },
    ]);
    const portal = setupPortal();
    await portal.prepareBallotByOption();

    // 同步改选项，watcher 还没执行：材料还在，但绑定的目标已经过期。
    portal.selectedVoteOption.value = 'B';

    const submitted = await portal.submitVote();

    expect(submitted).toBe(false);
    expect(calls.some((call) => call.url.includes('/api/v1/votes'))).toBe(false);
  });

  it('目标一致时正常提交', async () => {
    const calls = stubFetch([
      draftRoute,
      { match: (url) => url.includes('/api/v1/votes'), payload: { status: 'ok' } },
    ]);
    const portal = setupPortal();
    await portal.prepareBallotByOption();

    const submitted = await portal.submitVote();

    expect(submitted).toBe(true);
    const voteCall = calls.find((call) => call.url.includes('/api/v1/votes'));
    expect(voteCall).toBeDefined();
    expect(voteCall?.body?.encrypted_vote).toBe('cipher-A');
    // 明文选项不该发给服务端。
    expect(voteCall?.body).not.toHaveProperty('selected_option');
  });

  it('导入的凭据带 selected_option 时绑定到该选项', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();

    portal.voteForm.ballotJson = JSON.stringify({
      proposal_id: 'p1',
      encrypted_vote: 'cipher-imported',
      proof: 'proof-imported',
      nullifier: 'null-imported',
      selected_option: 'B',
    });
    portal.importBallotJson();
    await nextTick();

    expect(portal.voteForm.encryptedVote).toBe('cipher-imported');
    expect(portal.voteForm.ballotOption).toBe('B');
    expect(portal.voteForm.ballotProposalId).toBe('p1');
  });
});

describe('凭证与会话卫生', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('管理令牌只进 sessionStorage，不落 localStorage', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();

    portal.adminBearerToken.value = 'super-secret';
    await nextTick();

    expect(sessionStore.get('anonvote.front.admin_token')).toBe('super-secret');
    expect(storage.get('anonvote.front.admin_token')).toBeUndefined();
  });

  it('换提案时重置「是否已投票」', async () => {
    stubFetch([draftRoute]);
    const portal = setupPortal();

    portal.hasVoted.value = true;
    portal.adoptProposal('p2');
    await nextTick();

    expect(portal.hasVoted.value).toBeNull();
  });

  it('错误提示不自动消失', async () => {
    stubFetch([
      draftRoute,
      { match: (url) => url.includes('/api/v1/votes'), payload: { status: 'ok' } },
    ]);
    const portal = setupPortal();
    await portal.prepareBallotByOption();

    // 让投票失败：把材料清空到不满足提交条件。
    portal.voteForm.encryptedVote = '';
    await portal.submitVote();
    await nextTick();

    const errors = portal.notices.value.filter((item) => item.level === 'error');
    expect(errors.length).toBeGreaterThan(0);

    // 模拟 4.2 秒过去：错误提示仍然在。
    await new Promise((resolve) => setTimeout(resolve, 30));
    await nextTick();
    const stillThere = portal.notices.value.filter(
      (item) => item.level === 'error',
    );
    expect(stillThere.length).toBe(errors.length);
  });
});
