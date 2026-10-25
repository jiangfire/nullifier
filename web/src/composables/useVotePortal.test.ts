import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { bytesToBase64 } from '../lib/credential';
import { useVotePortal } from './useVotePortal';

/**
 * 浏览器端 prover 的桩。
 *
 * WASM 模块在 node 环境里跑不起来（没有 WebAssembly 胶水、也没有 Worker），
 * 这里只验证门户这一侧的编排：取计票公钥、传选项下标、把结果写进表单。
 */
const { generateLocalBallotMock } = vi.hoisted(() => ({
  generateLocalBallotMock: vi.fn(),
}));

vi.mock('../lib/wasm-prover', () => ({
  generateLocalBallot: generateLocalBallotMock,
}));

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

interface FetchRoute {
  match: (url: string) => boolean;
  /** 可以是常量，也可以是函数 —— 后者用于「第 1 次失败、第 2 次成功」这类场景。 */
  payload: unknown | (() => unknown);
  status?: number | (() => number);
}

function stubFetch(routes: FetchRoute[]): FetchCall[] {
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
      const status =
        typeof route?.status === 'function' ? route.status() : (route?.status ?? 200);
      const payload =
        typeof route?.payload === 'function' ? route.payload() : route?.payload;
      return {
        ok: status >= 200 && status < 300,
        status,
        text: async () => JSON.stringify(payload ?? {}),
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

/**
 * 签发方公钥（2048 位 RSA）。
 *
 * 与 Go 侧跨语言向量测试用的是同一把测试密钥 —— 它只出现在测试里，不参与任何
 * 生产路径。
 */
const issuerModulusHex =
  'a8f6a0ae71df41acaa192da461e1e12503dd57e23646fd22fa6130e7fb821895f' +
  '1508a7bd20d5e173736552a9b2e1d2758d97777eaa7350e668239574fb01126a5' +
  '06eeb4773451ef64e948b1b0f5f6365c20e037dc5ab824e32a425777cfa8e4205e' +
  '9be1684fe103d13c628266cf41205d8d849ac977f2f0f400fdb2547c3803e03f5' +
  'e512b32275b8079db69250633dd10c8e3dd77e8246f550bd236a4fad1535dbe06' +
  '17c5b7bc24c1cdb94b2f865e77eee33f03fe593709555f1c1969cc555da085f53' +
  'e267c058aac495c180e2583974dee64d5f9094c025504d4545e5ba65a52c7dbf5b' +
  'ccac52c05244d8f09b9196590e80ad0a0ecfdf0cf2be35325a745a9';

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16);
  }
  return bytes;
}

/** 「预留签发权」的响应。 */
function reservationPayload(attempt = 1) {
  return {
    proposal_id: 'p1',
    user_id: attempt === 1 ? 'pseudonym-from-pool' : 'pseudonym-from-pool-2',
    issuer_key_version: 1,
    // 服务端线路上的变体名，与 `supportedBlindSignatureVariant` 一致。
    variant: 'RSABSSA-SHA384-PSS-Deterministic',
    modulus: bytesToBase64(hexToBytes(issuerModulusHex)),
    exponent: 65537,
  };
}

/**
 * 盲签名的响应。
 *
 * 这里给的是一串占位字节：门户只关心解盲这一步能跑通（长度必须等于密钥长度），
 * 真正的验签在服务端做。
 */
function blindSignaturePayload() {
  return { blind_signature: bytesToBase64(new Uint8Array(256)) };
}

/** attest 之后的凭证。 */
function attestedCredential(userId = 'pseudonym-from-pool') {
  return {
    id: 'cred-1',
    proposal_id: 'p1',
    user_id: userId,
    user_pub_key: 'pub-key',
    signature: 'sig',
    issuer_key_version: 1,
    issued_at: '2026-10-21T00:00:00Z',
  };
}

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

describe('匿名领取凭证', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('走完整两段式：预留不带公钥，盲签名只见盲化消息', async () => {
    const calls = stubFetch([
      { match: (url) => url.includes('/credentials/claim'), payload: reservationPayload() },
      { match: (url) => url.includes('/credentials/blind-sign'), payload: blindSignaturePayload() },
      { match: (url) => url.includes('/credentials/attest'), payload: attestedCredential() },
    ]);
    const portal = setupPortal();

    const claimed = await portal.claimCredential();

    expect(claimed).toBe(true);

    // ① 预留阶段：请求体是空的 —— 服务端一旦知道公钥就能算出消息原文，
    //    盲化当场失效，所以公钥只能在 attest 那一步才出现。
    const reserveCall = calls.find((item) => item.url.includes('/credentials/claim'));
    expect(reserveCall).toBeDefined();
    expect(reserveCall?.url).toContain('/api/v1/proposals/p1/credentials/claim');
    expect(reserveCall?.body).not.toHaveProperty('user_pub_key');
    expect(reserveCall?.body).not.toHaveProperty('user_id');
    expect(Object.keys(reserveCall?.body ?? {})).toEqual([]);

    // ② 盲签名阶段：服务端拿到的只是盲化消息。
    const blindSignCall = calls.find((item) => item.url.includes('/credentials/blind-sign'));
    expect(blindSignCall).toBeDefined();
    expect(blindSignCall?.body?.user_id).toBe('pseudonym-from-pool');
    expect(blindSignCall?.body?.issuer_key_version).toBe(1);
    expect(String(blindSignCall?.body?.blinded_message ?? '')).not.toBe('');

    // ③ attest 阶段：公钥与最终签名一起交回。
    const attestCall = calls.find((item) => item.url.includes('/credentials/attest'));
    expect(attestCall).toBeDefined();
    expect(attestCall?.body?.user_id).toBe('pseudonym-from-pool');
    expect(String(attestCall?.body?.user_pub_key ?? '')).not.toBe('');
    expect(String(attestCall?.body?.signature ?? '')).not.toBe('');

    expect(portal.wallet.userId).toBe('pseudonym-from-pool');
  });

  it('槽位被别人抢走时换一个槽位重来', async () => {
    let claimAttempts = 0;
    let attestAttempts = 0;
    const calls = stubFetch([
      {
        match: (url) => url.includes('/credentials/claim'),
        payload: () => {
          claimAttempts += 1;
          return reservationPayload(claimAttempts);
        },
      },
      { match: (url) => url.includes('/credentials/blind-sign'), payload: blindSignaturePayload() },
      {
        match: (url) => url.includes('/credentials/attest'),
        status: () => {
          attestAttempts += 1;
          return attestAttempts === 1 ? 409 : 200;
        },
        payload: () =>
          attestAttempts === 1
            ? { code: 'CREDENTIAL_EXISTS', error: '凭证已存在' }
            : attestedCredential('pseudonym-from-pool-2'),
      },
    ]);
    const portal = setupPortal();

    const claimed = await portal.claimCredential();

    expect(claimed).toBe(true);
    // 预留重试了一次：换一个槽位重新盲化。
    const reserveCalls = calls.filter((item) => item.url.includes('/credentials/claim'));
    expect(reserveCalls.length).toBe(2);
    expect(portal.wallet.userId).toBe('pseudonym-from-pool-2');
  });

  it('凭证池领完时不重试', async () => {
    const calls = stubFetch([
      {
        match: (url) => url.includes('/credentials/claim'),
        status: 409,
        payload: { code: 'CREDENTIAL_POOL_EMPTY', error: '凭证池已空' },
      },
    ]);
    const portal = setupPortal();

    const claimed = await portal.claimCredential();

    expect(claimed).toBe(false);
    const reserveCalls = calls.filter((item) => item.url.includes('/credentials/claim'));
    expect(reserveCalls.length).toBe(1);
    const errors = portal.notices.value.filter((item) => item.level === 'error');
    expect(errors.length).toBeGreaterThan(0);
  });

  it('领取失败时给出提示', async () => {
    stubFetch([
      {
        match: (url) => url.includes('/credentials/claim'),
        payload: { error: { code: 'CREDENTIAL_POOL_EMPTY' } },
        status: 409,
      },
    ]);
    const portal = setupPortal();

    const claimed = await portal.claimCredential();

    expect(claimed).toBe(false);
    const errors = portal.notices.value.filter((item) => item.level === 'error');
    expect(errors.length).toBeGreaterThan(0);
  });
});

describe('查询投票状态', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('可以主动查询并更新「是否已投票」', async () => {
    const calls = stubFetch([
      {
        match: (url) => url.includes('/vote-status'),
        payload: { has_voted: true },
      },
    ]);
    const portal = setupPortal();

    await portal.fetchVoteStatus();

    const statusCall = calls.find((call) => call.url.includes('/vote-status'));
    expect(statusCall).toBeDefined();
    expect(statusCall?.url).toContain('/api/v1/proposals/p1/vote-status');
    expect(statusCall?.body?.user_id).toBe('u-1');
    expect(portal.hasVoted.value).toBe(true);
  });

  it('查询失败时留下常驻的行内错误', async () => {
    stubFetch([
      {
        match: (url) => url.includes('/vote-status'),
        payload: { error: { code: 'INTERNAL_ERROR' } },
        status: 500,
      },
    ]);
    const portal = setupPortal();

    await portal.fetchVoteStatus();

    expect(portal.voteStatusError.value).not.toBe('');
    expect(portal.hasVoted.value).toBeNull();
  });

  it('查询成功后清掉上一次的行内错误', async () => {
    let failing = true;
    const calls: FetchCall[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: unknown, init?: RequestInit) => {
        const url = String(input);
        const body = init?.body
          ? (JSON.parse(String(init.body)) as Record<string, unknown>)
          : null;
        calls.push({ url, body });
        const status = failing ? 500 : 200;
        failing = false;
        return {
          ok: status === 200,
          status,
          text: async () => JSON.stringify({ has_voted: false }),
        };
      }),
    );
    const portal = setupPortal();

    await portal.fetchVoteStatus();
    expect(portal.voteStatusError.value).not.toBe('');

    await portal.fetchVoteStatus();
    expect(portal.voteStatusError.value).toBe('');
    expect(portal.hasVoted.value).toBe(false);
  });
});

describe('浏览器本地出票', () => {
  const proposalItem = {
    id: 'p1',
    title: '提案一',
    description: '',
    options: ['A', 'B'],
    status: 'open',
    startsAt: '',
    endsAt: '',
  };

  beforeEach(() => {
    vi.unstubAllGlobals();
    generateLocalBallotMock.mockReset();
    generateLocalBallotMock.mockResolvedValue({
      ciphertext: 'cipher-local',
      nullifier: 'null-local',
      proof: 'proof-local',
      initMs: 7000,
      proveMs: 2000,
    });
  });

  function setupPortalWithProposal() {
    const portal = setupPortal();
    portal.hallState.proposals = [proposalItem];
    portal.selectedVoteOption.value = 'B';
    return portal;
  }

  it('取计票公钥、调用 prover、把材料写进表单', async () => {
    const calls = stubFetch([
      {
        match: (url) => url.includes('election-key'),
        payload: { public_key: 'KEY-BASE64', encoding: 'uncompressed' },
      },
    ]);
    const portal = setupPortalWithProposal();

    const ok = await portal.generateBallotLocally();

    expect(ok).toBe(true);
    expect(
      calls.some((call) => call.url.includes('/api/v1/proposals/p1/election-key')),
    ).toBe(true);
    expect(generateLocalBallotMock).toHaveBeenCalledWith({
      proposalId: 'p1',
      electionPublicKey: 'KEY-BASE64',
      optionIndex: 1,
      numOptions: 2,
    });
    expect(portal.voteForm.encryptedVote).toBe('cipher-local');
    expect(portal.voteForm.proof).toBe('proof-local');
    expect(portal.voteForm.nullifier).toBe('null-local');
    expect(portal.voteForm.ballotOption).toBe('B');
    expect(portal.voteForm.ballotProposalId).toBe('p1');
  });

  it('prover 失败时给出错误提示，且不写表单', async () => {
    stubFetch([
      {
        match: (url) => url.includes('election-key'),
        payload: { public_key: 'KEY-BASE64', encoding: 'uncompressed' },
      },
    ]);
    generateLocalBallotMock.mockRejectedValue(
      new Error('下载 WASM 模块失败：HTTP 404'),
    );
    const portal = setupPortalWithProposal();

    const ok = await portal.generateBallotLocally();

    expect(ok).toBe(false);
    expect(portal.voteForm.proof).toBe('');
    const errors = portal.notices.value.filter((item) => item.level === 'error');
    expect(errors.length).toBeGreaterThan(0);
    expect(errors[0]?.detail).toContain('下载 WASM 模块失败');
  });

  it('没有选中的提案选项时不调用 prover', async () => {
    stubFetch([
      {
        match: (url) => url.includes('election-key'),
        payload: { public_key: 'KEY-BASE64', encoding: 'uncompressed' },
      },
    ]);
    const portal = setupPortalWithProposal();
    portal.selectedVoteOption.value = '不存在的选项';

    const ok = await portal.generateBallotLocally();

    expect(ok).toBe(false);
    expect(generateLocalBallotMock).not.toHaveBeenCalled();
  });
});