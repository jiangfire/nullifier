import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { apiCall } from '../lib/api';
import {
  hasBallotTargetDrifted,
  isBallotTargetCurrent,
  isTargetUnknown,
} from '../lib/ballot';
import { localInputToRfc3339, toLocalDateTimeInput } from '../lib/datetime';
import {
  base64ToBytes,
  blindCredentialMessage,
  bytesToBase64,
  finalizeCredentialSignature,
  parseIssuerKey,
} from '../lib/credential';
import { generateNullifier } from '../lib/nullifier';
import { generateLocalBallot } from '../lib/wasm-prover';
import { generateEligibilityProof } from '../lib/eligibility-prover';
import type { EligibilityMemberPackage } from '../lib/wasm-prover-protocol';
import { describeApiError } from '../lib/error-messages';
import {
  buildPortalSearch,
  getDefaultSection,
  isSectionAllowed,
  parsePortalRoute,
} from '../lib/navigation';
import {
  appendOptionRow,
  canRemoveOptionRow,
  normalizeOptionList,
  removeOptionRow,
} from '../lib/options';
import type {
  AnchorItem,
  CredentialStatusItem,
  FormErrorMap,
  JsonMap,
  Notice,
  PortalRouteState,
  ProposalItem,
  SectionId,
  SectionMeta,
  WorkspaceId,
  WorkspaceMeta,
} from '../types/portal';

const baseUrlStorageKey = 'anonvote.front.base_url';
const adminTokenStorageKey = 'anonvote.front.admin_token';
const navStorageKey = 'anonvote.front.nav';
const onboardingStorageKey = 'anonvote.front.onboarding_seen';

const workspaces: WorkspaceMeta[] = [
  { id: 'user', title: '用户端', description: '领取凭证并完成匿名投票' },
  { id: 'admin', title: '运营端', description: '创建提案、计票与审计' },
];

const sections: SectionMeta[] = [
  {
    id: 'overview',
    workspace: 'shared',
    title: '系统概览',
    description: '先看状态、路径和当前上下文',
  },
  {
    id: 'setup',
    workspace: 'shared',
    title: '连接配置',
    description: '配置后端地址、令牌和联调前检查',
  },
  {
    id: 'guide',
    workspace: 'shared',
    title: '使用教程',
    description: '先看流程与术语说明',
  },
  {
    id: 'hall',
    workspace: 'user',
    title: '投票大厅',
    description: '浏览可参与提案',
  },
  {
    id: 'identity',
    workspace: 'user',
    title: '我的身份',
    description: '领取或导入匿名凭证',
  },
  {
    id: 'vote',
    workspace: 'user',
    title: '参与投票',
    description: '按步骤生成并提交选票',
  },
  {
    id: 'ops',
    workspace: 'admin',
    title: '计票与审计',
    description: '提案管理与结果发布',
  },
];

function clearErrorGroup(group: FormErrorMap): void {
  for (const key of Object.keys(group)) {
    group[key] = '';
  }
}

function toObject(value: unknown): JsonMap | null {
  if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
    return value as JsonMap;
  }
  return null;
}

function textOf(object: JsonMap | null, key: string): string {
  if (object === null) {
    return '';
  }
  const value = object[key];
  if (typeof value === 'string') {
    return value;
  }
  // The API returns some display fields (for example issuer_key_version) as
  // numbers; render them as text instead of dropping them.
  if (typeof value === 'number') {
    return String(value);
  }
  return '';
}

function boolOf(object: JsonMap | null, key: string): boolean | null {
  if (object === null) {
    return null;
  }
  const value = object[key];
  return typeof value === 'boolean' ? value : null;
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

/**
 * 匿名领取的重试次数。
 *
 * 服务端取槽位时**不占位**（占位会变成「只预留不 attest 就能耗尽池」的 DoS），
 * 所以并发领取会抢到同一个槽位。抢输的那个换一个槽位重来 —— 但重试要重新
 * 盲化（新的 user_id 意味着新的消息），所以重试只能在客户端做。
 */
const claimMaxAttempts = 5;

/**
 * 判断错误是不是「槽位被别人抢走」—— 只有这种情况值得重试。
 *
 * 409 有两种：槽位被抢（换个槽位重来就行）与凭证池已空（重试也没用）。
 */
function isSlotTaken(status: number, code: string | null): boolean {
  return status === 409 && code === 'CREDENTIAL_EXISTS';
}

/** 凭证两段式流程的结果。 */
type CredentialFlowResult =
  | { ok: true; credential: JsonMap }
  | {
      ok: false;
      title: string;
      message: string;
      /** 换一个槽位重来就能成（只有槽位被抢时为真）。 */
      retryable?: boolean;
    };

function randomBase64(size: number): string {
  const bytes = new Uint8Array(size);
  crypto.getRandomValues(bytes);
  let output = '';
  for (const value of bytes) {
    output += String.fromCharCode(value);
  }
  return btoa(output);
}

function regenerateBase64Key(): string {
  return randomBase64(32);
}

/**
 * 生成一个 nullifier。
 *
 * 服务端要求 nullifier 是 32 字节的规范域元素编码（十六进制）：
 * 同一个值不能有两种字节表示，否则双重投票的唯一索引就拦不住了。
 */
function newNullifier(): string {
  return generateNullifier();
}

function splitShares(raw: string): string[] {  return raw
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function decodeProposalDescription(raw: string): {
  description: string;
  options: string[];
} {
  const text = raw.trim();
  if (text === '') {
    return { description: '', options: [] };
  }
  try {
    const parsed = toObject(JSON.parse(text));
    if (parsed === null) {
      return { description: raw, options: [] };
    }
    const summary = textOf(parsed, 'summary') || textOf(parsed, 'description');
    const optionField = parsed.options;
    const options = Array.isArray(optionField)
      ? optionField.filter((item): item is string => typeof item === 'string')
      : [];
    return { description: summary, options };
  } catch {
    return { description: raw, options: [] };
  }
}

function nowLabel(): string {
  return new Date().toLocaleString('zh-CN', { hour12: false });
}

// parseProposalItems 把接口返回的 items 归一化成 ProposalItem 列表。
// 大厅只取开放提案、运营端取全部提案，两者结构一致，共用这段解析。
function parseProposalItems(raw: unknown): ProposalItem[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .map((item): ProposalItem | null => {
      const object = toObject(item);
      if (object === null) {
        return null;
      }
      const id = textOf(object, 'id');
      if (id === '') {
        return null;
      }
      const parsedDescription = decodeProposalDescription(
        textOf(object, 'description'),
      );
      const rawOptions = object.options;
      const options = Array.isArray(rawOptions)
        ? rawOptions
            .filter((item): item is string => typeof item === 'string')
            .map((item) => item.trim())
            .filter((item) => item !== '')
        : parsedDescription.options;
      return {
        id,
        title: textOf(object, 'title'),
        description: parsedDescription.description,
        options,
        status: textOf(object, 'status'),
        startsAt: textOf(object, 'starts_at'),
        endsAt: textOf(object, 'ends_at'),
      };
    })
    .filter((item): item is ProposalItem => item !== null);
}

function parseStoredRoute(raw: string | null): PortalRouteState | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as Partial<PortalRouteState>;
    if (!parsed || typeof parsed !== 'object') {
      return null;
    }
    const workspace = parsed.workspace === 'admin' ? 'admin' : 'user';
    const sectionCandidate = parsed.section;
    const section: SectionId =
      sectionCandidate === 'overview' ||
      sectionCandidate === 'setup' ||
      sectionCandidate === 'guide' ||
      sectionCandidate === 'hall' ||
      sectionCandidate === 'identity' ||
      sectionCandidate === 'vote' ||
      sectionCandidate === 'ops'
        ? sectionCandidate
        : getDefaultSection(workspace);
    const proposalId =
      typeof parsed.proposalId === 'string' ? parsed.proposalId.trim() : '';
    return {
      workspace,
      section: isSectionAllowed(workspace, section)
        ? section
        : getDefaultSection(workspace),
      proposalId,
    };
  } catch {
    return null;
  }
}

export function useVotePortal() {
  const apiBaseUrl = ref(
    localStorage.getItem(baseUrlStorageKey) ?? 'http://localhost:8080',
  );
  const adminBearerToken = ref(
    sessionStorage.getItem(adminTokenStorageKey) ?? '',
  );
  const workspace = ref<WorkspaceId>('user');
  const activeSection = ref<SectionId>('overview');
  const selectedProposalId = ref('');

  const notices = ref<Notice[]>([]);
  const guideModeVisible = ref(false);

  const healthState = reactive({
    loading: false,
    ok: false,
    text: '未检测',
    checkedAt: '',
  });
  const hallState = reactive({
    loading: false,
    proposals: [] as ProposalItem[],
  });
  // 运营端持有全部提案（含已关闭、未开始），用来挑选 proposal_id。
  const opsProposalsState = reactive({
    loading: false,
    proposals: [] as ProposalItem[],
  });

  const identityForm = reactive({ userId: '', customPubKey: '' });
  const useCustomPubKey = ref(false);
  const generatedPubKey = ref(randomBase64(32));
  const wallet = reactive({
    proposalId: '',
    userId: '',
    userPubKey: '',
    signature: '',
    issuerKeyVersion: '',
    issuedAt: '',
  });
  const credentialLoading = ref(false);
  const credentialFileInput = ref<HTMLInputElement | null>(null);

  // 资格包：成员自己拿到的 JSON 文件（离线名单工具生成）。
  //
  // 导入它，领取凭证时门户就能在本地出资格证明 —— 服务端据此强制「一人一票」，
  // 却始终不知道这份资格属于谁。没导入时能不能领，取决于服务端是否强制。
  const eligibilityPackage = ref<EligibilityMemberPackage | null>(null);
  const eligibilityPackageName = ref('');
  const eligibilityFileInput = ref<HTMLInputElement | null>(null);
  const eligibilityError = ref('');

  const voteForm = reactive({
    proposalId: '',
    ballotJson: '',
    encryptedVote: '',
    proof: '',
    nullifier: '',
    // 这份凭据材料是针对哪个提案、哪个选项生成的。
    // 选项或提案变了就必须作废它，否则会投出旧选项。
    ballotProposalId: '',
    ballotOption: '',
  });
  const voteLoading = ref(false);
  const voteStatusLoading = ref(false);
  const hasVoted = ref<boolean | null>(null);
  // 投票状态的查询错误常驻显示在投票面板上，不只靠一条会被划走的通知。
  const voteStatusError = ref('');
  const selectedVoteOption = ref('');
  const showAdvancedVoteFields = ref(false);

  const createProposalForm = reactive({
    title: '',
    description: '',
    options: ['', ''] as string[],
    startsAtLocal: '',
    endsAtLocal: '',
  });
  const createProposalLoading = ref(false);

  const decryptForm = reactive({ proposalId: '', threshold: 2, trustees: 3 });
  const decryptLoading = ref(false);
  const decryptPublicKey = ref('');
  const decryptShares = ref<string[]>([]);

  const publishForm = reactive({ proposalId: '', sharesText: '' });
  const publishLoading = ref(false);
  const publishedSummary = ref('');

  const resultForm = reactive({ proposalId: '' });
  const resultLoading = ref(false);
  const resultSummary = ref('');
  const resultTally = ref<Array<{ option: string; count: number }>>([]);

  const anchorForm = reactive({ proposalId: '' });
  const anchorLoading = ref(false);
  const anchors = ref<AnchorItem[]>([]);
  const credentialStatusForm = reactive({ proposalId: '' });
  const credentialStatusLoading = ref(false);
  const credentialStatuses = ref<CredentialStatusItem[]>([]);

  const identityErrors = reactive<FormErrorMap>({
    userId: '',
    customPubKey: '',
  });
  const voteErrors = reactive<FormErrorMap>({
    proposalId: '',
    option: '',
    ballotJson: '',
    encryptedVote: '',
    proof: '',
    nullifier: '',
  });
  const createProposalErrors = reactive<FormErrorMap>({
    title: '',
    options: '',
    startsAt: '',
    endsAt: '',
  });
  const decryptErrors = reactive<FormErrorMap>({
    proposalId: '',
    threshold: '',
    trustees: '',
  });
  const publishErrors = reactive<FormErrorMap>({ proposalId: '', shares: '' });
  const resultErrors = reactive<FormErrorMap>({ proposalId: '' });
  const anchorErrors = reactive<FormErrorMap>({ proposalId: '' });
  const credentialStatusErrors = reactive<FormErrorMap>({ proposalId: '' });

  const currentPubKey = computed(() =>
    useCustomPubKey.value
      ? identityForm.customPubKey.trim()
      : generatedPubKey.value,
  );
  const walletReady = computed(
    () =>
      wallet.userId !== '' &&
      wallet.userPubKey !== '' &&
      wallet.signature !== '',
  );
  // Credentials are issued per proposal; a credential imported or issued for
  // another proposal cannot be used here.
  const walletMatchesProposal = computed(() => {
    const credentialProposal = wallet.proposalId.trim();
    const target = voteForm.proposalId.trim();
    if (credentialProposal === '' || target === '') {
      return true;
    }
    return credentialProposal === target;
  });
  const adminTokenReady = computed(() => adminBearerToken.value.trim() !== '');
  const selectedProposal = computed(() => {
    const byVoteForm = voteForm.proposalId.trim();
    const bySelection = selectedProposalId.value.trim();
    return (
      hallState.proposals.find((item) => item.id === byVoteForm) ??
      hallState.proposals.find((item) => item.id === bySelection) ??
      null
    );
  });
  const visibleSections = computed(() =>
    sections.filter(
      (section) =>
        section.workspace === 'shared' || section.workspace === workspace.value,
    ),
  );

  let routeReady = false;
  const noticeTimers = new Map<string, number>();

  function addNotice(
    level: Notice['level'],
    title: string,
    detail: string,
  ): void {
    const notice: Notice = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      level,
      title,
      detail,
      at: nowLabel(),
    };
    notices.value.unshift(notice);
    if (notices.value.length > 4) {
      const removed = notices.value.splice(4);
      for (const item of removed) {
        const timer = noticeTimers.get(item.id);
        if (timer !== undefined) {
          window.clearTimeout(timer);
          noticeTimers.delete(item.id);
        }
      }
    }
    if (level === 'error') {
      // 错误提示不自动消失：投票失败这类信息如果几秒后自己没了，
      // 面板看起来和「就绪」状态一模一样，用户根本不会知道自己投失败了。
      return;
    }
    const timer = window.setTimeout(() => {
      dismissNotice(notice.id);
    }, 4200);
    noticeTimers.set(notice.id, timer);
  }

  function dismissNotice(id: string): void {
    notices.value = notices.value.filter((item) => item.id !== id);
    const timer = noticeTimers.get(id);
    if (timer !== undefined) {
      window.clearTimeout(timer);
      noticeTimers.delete(id);
    }
  }

  function markOnboardingSeen(): void {
    localStorage.setItem(onboardingStorageKey, '1');
  }

  function openGuideMode(): void {
    setActiveSection('guide');
    guideModeVisible.value = true;
  }

  function closeGuideMode(): void {
    guideModeVisible.value = false;
    markOnboardingSeen();
  }

  function startGuideJourney(target: WorkspaceId): void {
    guideModeVisible.value = false;
    markOnboardingSeen();
    if (target === 'admin') {
      setWorkspace('admin');
      setActiveSection('ops');
      addNotice(
        'info',
        '已切到运营引导',
        '先创建未开始的提案，再初始化解密，最后等投票结束后发布结果。',
      );
      return;
    }
    setWorkspace('user');
    setActiveSection('hall');
    addNotice(
      'info',
      '已切到用户引导',
      '先选提案，再领取匿名凭证，最后生成投票材料并提交。',
    );
  }

  function adminHeaders(): Record<string, string> {
    const token = adminBearerToken.value.trim();
    if (token === '') {
      return {};
    }
    return { Authorization: `Bearer ${token}` };
  }

  function syncRouteToUrl(): void {
    if (!routeReady) {
      return;
    }
    const nextSearch = buildPortalSearch({
      workspace: workspace.value,
      section: activeSection.value,
      proposalId: selectedProposalId.value,
    });
    const current = new URL(window.location.href);
    const rendered = nextSearch === '' ? '' : `?${nextSearch}`;
    if (current.search !== rendered) {
      history.replaceState(
        {},
        '',
        `${current.pathname}${rendered}${current.hash}`,
      );
    }
  }

  function persistRouteToStorage(): void {
    const payload: PortalRouteState = {
      workspace: workspace.value,
      section: activeSection.value,
      proposalId: selectedProposalId.value.trim(),
    };
    localStorage.setItem(navStorageKey, JSON.stringify(payload));
  }

  function setWorkspace(next: WorkspaceId): void {
    workspace.value = next;
    if (!isSectionAllowed(next, activeSection.value)) {
      activeSection.value = getDefaultSection(next);
    }
  }

  function setActiveSection(next: SectionId): void {
    if (!isSectionAllowed(workspace.value, next)) {
      return;
    }
    activeSection.value = next;
  }

  function setProposalWindow(mode: 'scheduled' | 'active' | 'closed'): void {
    if (mode === 'scheduled') {
      createProposalForm.startsAtLocal = toLocalDateTimeInput(
        new Date(Date.now() + 15 * 60 * 1000),
      );
      createProposalForm.endsAtLocal = toLocalDateTimeInput(
        new Date(Date.now() + 75 * 60 * 1000),
      );
      return;
    }
    if (mode === 'active') {
      createProposalForm.startsAtLocal = toLocalDateTimeInput(
        new Date(Date.now() - 5 * 60 * 1000),
      );
      createProposalForm.endsAtLocal = toLocalDateTimeInput(
        new Date(Date.now() + 60 * 60 * 1000),
      );
      return;
    }
    createProposalForm.startsAtLocal = toLocalDateTimeInput(
      new Date(Date.now() - 2 * 60 * 60 * 1000),
    );
    createProposalForm.endsAtLocal = toLocalDateTimeInput(
      new Date(Date.now() - 60 * 1000),
    );
  }

  function regeneratePubKey(): void {
    generatedPubKey.value = regenerateBase64Key();
  }

  function adoptProposal(id: string, section?: SectionId): void {
    const trimmed = id.trim();
    if (trimmed === '') {
      return;
    }
    selectedProposalId.value = trimmed;
    if (section) {
      if (section === 'ops') {
        setWorkspace('admin');
      } else {
        setWorkspace('user');
      }
      setActiveSection(section);
    }
  }

  function exportCredentialFile(): void {
    if (!walletReady.value) {
      addNotice('error', '导出失败', '请先领取或导入凭证');
      return;
    }
    const payload = {
      version: 1,
      proposal_id: wallet.proposalId.trim() || voteForm.proposalId.trim(),
      user_id: wallet.userId,
      user_pub_key: wallet.userPubKey,
      signature: wallet.signature,
      issuer_key_version: wallet.issuerKeyVersion,
      issued_at: wallet.issuedAt,
      exported_at: new Date().toISOString(),
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], {
      type: 'application/json',
    });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `credential-${wallet.userId || 'user'}.json`;
    link.click();
    URL.revokeObjectURL(link.href);
    addNotice('success', '导出成功', '凭证文件已下载');
  }

  function openCredentialImport(): void {
    credentialFileInput.value?.click();
  }

  async function importCredentialFile(event: Event): Promise<void> {
    const target = event.target;
    if (
      !(target instanceof HTMLInputElement) ||
      !target.files ||
      target.files.length === 0
    ) {
      return;
    }
    const file = target.files[0];
    try {
      const content = await file.text();
      const parsed = toObject(JSON.parse(content));
      if (parsed === null) {
        addNotice('error', '导入失败', '凭证文件格式无效');
        return;
      }
      const userID = textOf(parsed, 'user_id');
      const userPubKey = textOf(parsed, 'user_pub_key');
      const signature = textOf(parsed, 'signature');
      if (userID === '' || userPubKey === '' || signature === '') {
        addNotice('error', '导入失败', '凭证文件缺少必要字段');
        return;
      }
      const proposalID = textOf(parsed, 'proposal_id').trim();
      wallet.proposalId = proposalID;
      wallet.userId = userID;
      wallet.userPubKey = userPubKey;
      wallet.signature = signature;
      wallet.issuerKeyVersion = textOf(parsed, 'issuer_key_version');
      wallet.issuedAt = textOf(parsed, 'issued_at');
      identityForm.userId = userID;
      hasVoted.value = null;
      if (proposalID !== '') {
        adoptProposal(proposalID);
      }
      addNotice(
        'success',
        '导入成功',
        proposalID === ''
          ? `已导入凭证：${userID}`
          : `已导入凭证：${userID}（提案 ${proposalID}）`,
      );
    } catch {
      addNotice('error', '导入失败', '文件内容不是合法 JSON');
    } finally {
      target.value = '';
    }
  }

  function clearBallotMaterial(): void {
    voteForm.encryptedVote = '';
    voteForm.proof = '';
    voteForm.ballotProposalId = '';
    voteForm.ballotOption = '';
  }

  /** 绑定投票凭据材料的目标（提案 + 选项）。 */
  function bindBallotMaterial(proposalId: string, option: string): void {
    voteForm.ballotProposalId = proposalId.trim();
    voteForm.ballotOption = option.trim();
  }

  /** 当前意图：材料必须对得上「哪个提案的哪个选项」。 */
  const currentBallotTarget = computed(() => ({
    proposalId: voteForm.proposalId.trim(),
    option: selectedVoteOption.value.trim(),
  }));

  /** 材料绑定的目标。 */
  const boundBallotTarget = computed(() => ({
    proposalId: voteForm.ballotProposalId.trim(),
    option: voteForm.ballotOption.trim(),
  }));

  /** 材料是否仍然对得上当前意图（用来禁用提交按钮）。 */
  const ballotTargetCurrent = computed(() =>
    isBallotTargetCurrent(boundBallotTarget.value, currentBallotTarget.value),
  );

  function updateBallotJson(selectedOption: string): void {
    voteForm.ballotJson = JSON.stringify(
      {
        selected_option: selectedOption,
        encrypted_vote: voteForm.encryptedVote,
        proof: voteForm.proof,
        nullifier: voteForm.nullifier,
      },
      null,
      2,
    );
  }

  function applySelectedOptionToBallot(): void {
    clearErrorGroup(voteErrors);
    if (
      !selectedProposal.value ||
      selectedProposal.value.options.length === 0
    ) {
      voteErrors.option = '当前提案没有可选项';
      addNotice('error', '填充失败', voteErrors.option);
      return;
    }
    if (selectedVoteOption.value.trim() === '') {
      voteErrors.option = '请先选择一个选项';
      addNotice('error', '填充失败', voteErrors.option);
      return;
    }
    if (!ballotTargetCurrent.value) {
      voteErrors.option = '投票凭据对应的选项与当前选择不一致，请重新生成凭据';
      addNotice('error', '填充失败', voteErrors.option);
      return;
    }
    try {
      const current = voteForm.ballotJson.trim();
      const base = current === '' ? {} : (toObject(JSON.parse(current)) ?? {});
      const payload: JsonMap = {
        ...base,
        selected_option: selectedVoteOption.value,
        encrypted_vote:
          textOf(base, 'encrypted_vote') || voteForm.encryptedVote,
        proof: textOf(base, 'proof') || voteForm.proof,
        nullifier:
          textOf(base, 'nullifier') || voteForm.nullifier || newNullifier(),
      };
      voteForm.encryptedVote = textOf(payload, 'encrypted_vote');
      voteForm.proof = textOf(payload, 'proof');
      voteForm.nullifier = textOf(payload, 'nullifier');
      voteForm.ballotJson = JSON.stringify(payload, null, 2);
      // 材料本来没有绑定目标时（外部导入且没带 selected_option），
      // 以用户这次显式写入的选项为准。
      if (isTargetUnknown(boundBallotTarget.value)) {
        bindBallotMaterial(voteForm.proposalId, selectedVoteOption.value);
      }
      addNotice('success', '写入成功', '选项已同步到投票凭据 JSON');
    } catch {
      voteErrors.ballotJson = '当前投票凭据 JSON 不是合法格式';
      addNotice('error', '填充失败', voteErrors.ballotJson);
    }
  }

  function fillBallotTemplate(): void {
    voteForm.nullifier = newNullifier();
    updateBallotJson(selectedVoteOption.value);
    addNotice('info', '模板已填充', '已生成可编辑的投票凭据模板');
  }

  function importBallotJson(): void {
    clearErrorGroup(voteErrors);
    if (voteForm.ballotJson.trim() === '') {
      voteErrors.ballotJson = '请先粘贴投票凭据 JSON';
      addNotice('error', '导入失败', voteErrors.ballotJson);
      return;
    }
    try {
      const parsed = toObject(JSON.parse(voteForm.ballotJson));
      if (parsed === null) {
        voteErrors.ballotJson = 'JSON 顶层必须是对象';
        addNotice('error', '导入失败', voteErrors.ballotJson);
        return;
      }
      voteForm.encryptedVote =
        textOf(parsed, 'encrypted_vote') || textOf(parsed, 'encryptedVote');
      voteForm.proof = textOf(parsed, 'proof');
      voteForm.nullifier = textOf(parsed, 'nullifier');
      const chosenOption =
        textOf(parsed, 'selected_option') || textOf(parsed, 'option');
      // 外部导入的凭据：读到 selected_option 就绑定上，读不到就保持「不可判定」，
      // 不去猜它的选项。
      bindBallotMaterial(
        textOf(parsed, 'proposal_id') || voteForm.proposalId,
        chosenOption,
      );
      if (chosenOption !== '') {
        selectedVoteOption.value = chosenOption;
      }
      addNotice('success', '导入成功', '投票凭据已写入表单');
    } catch {
      voteErrors.ballotJson = 'JSON 格式不合法';
      addNotice('error', '导入失败', voteErrors.ballotJson);
    }
  }

  /**
   * 在浏览器本地生成选票（密文 + nullifier + 零知识证明）。
   *
   * 这是正式（非 debug）模式下唯一的出票入口：密文由浏览器里的 WASM prover
   * 生成，服务端全程只看到密文、nullifier 与证明，看不到选了哪个选项。
   *
   * 依赖门户静态目录里的三个文件（`wasm_exec.js` / `ballot-prover.wasm` /
   * `proving.key`），由 `make web-wasm` 与 `make web-proving-key` 准备。
   */
  async function generateBallotLocally(): Promise<boolean> {
    clearErrorGroup(voteErrors);
    if (!walletReady.value) {
      addNotice('error', '生成失败', '请先领取或导入匿名凭证');
      return false;
    }
    const proposalId = voteForm.proposalId.trim();
    if (proposalId === '') {
      voteErrors.proposalId = '请先选择 proposal_id';
      addNotice('error', '生成失败', voteErrors.proposalId);
      return false;
    }
    const proposal = selectedProposal.value;
    if (!proposal || proposal.options.length === 0) {
      voteErrors.option = '当前提案没有可选项';
      addNotice('error', '生成失败', voteErrors.option);
      return false;
    }
    const option = selectedVoteOption.value.trim();
    const optionIndex = proposal.options.indexOf(option);
    if (optionIndex < 0) {
      voteErrors.option = '请选择一个投票选项';
      addNotice('error', '生成失败', voteErrors.option);
      return false;
    }

    voteLoading.value = true;
    try {
      const keyResult = await apiCall({
        baseUrl: apiBaseUrl.value,
        path: `/api/v1/proposals/${encodeURIComponent(proposalId)}/election-key`,
      });
      if (!keyResult.ok) {
        addNotice(
          'error',
          '生成失败',
          describeApiError(
            keyResult.error,
            keyResult.code,
            '该提案还没初始化解密，请先在「计票与审计」完成「初始化解密」',
          ),
        );
        return false;
      }
      const keyPayload = toObject(keyResult.data);
      const electionPublicKey = textOf(keyPayload, 'public_key');
      if (electionPublicKey === '') {
        addNotice('error', '生成失败', '服务端没有返回计票公钥');
        return false;
      }

      const result = await generateLocalBallot({
        proposalId,
        electionPublicKey,
        optionIndex,
        numOptions: proposal.options.length,
      });

      voteForm.encryptedVote = result.ciphertext;
      voteForm.proof = result.proof;
      voteForm.nullifier = result.nullifier;
      voteForm.ballotProposalId = proposalId;
      voteForm.ballotOption = option;
      updateBallotJson(option);
      addNotice(
        'success',
        '生成成功',
        `已在浏览器本地生成密文与证明（初始化 ${Math.round(result.initMs / 100) / 10}s，出证明 ${Math.round(result.proveMs / 100) / 10}s）；服务端看不到你选了哪个选项`,
      );
      return true;
    } catch (error) {
      const detail = error instanceof Error ? error.message : '浏览器端生成选票失败';
      addNotice('error', '生成失败', detail);
      return false;
    } finally {
      voteLoading.value = false;
    }
  }

  async function checkHealth(showNotice = true): Promise<void> {
    healthState.loading = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/healthz',
    });
    healthState.loading = false;
    healthState.checkedAt = nowLabel();
    if (result.ok) {
      healthState.ok = true;
      healthState.text = '服务在线';
      if (showNotice) {
        addNotice('success', '连接成功', '后端服务可访问');
      }
      return;
    }
    healthState.ok = false;
    healthState.text = '服务不可用';
    if (showNotice) {
      addNotice(
        'error',
        '连接失败',
        describeApiError(result.error, result.code, '无法访问后端服务'),
      );
    }
  }

  async function loadOpenProposals(
    showNotice = true,
    preserveSelection = false,
  ): Promise<void> {
    hallState.loading = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/proposals/open',
    });
    hallState.loading = false;
    if (!result.ok) {
      hallState.proposals = [];
      if (showNotice) {
        addNotice(
          'error',
          '加载失败',
          describeApiError(result.error, result.code, '无法获取开放提案'),
        );
      }
      return;
    }
    const payload = toObject(result.data);
    const items = payload?.items;
    if (!Array.isArray(items)) {
      hallState.proposals = [];
      if (showNotice) {
        addNotice('info', '暂无提案', '当前没有可参与提案');
      }
      return;
    }
    hallState.proposals = parseProposalItems(items);

    const selected = selectedProposalId.value.trim();
    const exists = hallState.proposals.some((item) => item.id === selected);
    if (!preserveSelection && !exists && hallState.proposals.length > 0) {
      adoptProposal(hallState.proposals[0].id);
    }
    if (showNotice) {
      addNotice(
        'success',
        '提案已刷新',
        `共 ${hallState.proposals.length} 个提案`,
      );
    }
  }

  // loadAllProposals 拉取全部提案（含已关闭、未开始），
  // 供运营端选择 proposal_id，不必再手工抄编号。
  async function loadAllProposals(showNotice = true): Promise<void> {
    opsProposalsState.loading = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/proposals',
    });
    opsProposalsState.loading = false;
    if (!result.ok) {
      opsProposalsState.proposals = [];
      if (showNotice) {
        addNotice(
          'error',
          '加载失败',
          describeApiError(result.error, result.code, '无法获取提案列表'),
        );
      }
      return;
    }
    const payload = toObject(result.data);
    opsProposalsState.proposals = parseProposalItems(payload?.items);
    if (showNotice) {
      addNotice(
        'success',
        '提案列表已更新',
        `共 ${opsProposalsState.proposals.length} 个提案`,
      );
    }
  }

  /**
   * 匿名领取凭证。
   *
   * 走真两段式盲签名：客户端本地盲化凭证消息，服务端只见盲化消息；
   * 客户端解盲后再把最终签名交回去，服务端验签后落库。
   *
   * 预留请求里**没有任何身份信息**：服务端随机给出一个凭证假名，服务端与运营方
   * 都无法把凭证和投票人对应起来。
   *
   * 槽位被别人抢走时换一个槽位重来（服务端取槽位时不占位，并发领取会撞车）。
   */
  async function claimCredential(): Promise<boolean> {
    clearErrorGroup(identityErrors);
    const proposalId = voteForm.proposalId.trim();
    if (proposalId === '') {
      identityErrors.userId = '请先选择 proposal_id，再领取该提案的凭证';
      addNotice('error', '领取失败', identityErrors.userId);
      return false;
    }

    const userPubKey = regenerateBase64Key();
    credentialLoading.value = true;
    let failure: CredentialFlowResult | null = null;
    for (let attempt = 0; attempt < claimMaxAttempts; attempt += 1) {
      const result = await claimCredentialOnce(proposalId, userPubKey);
      if (result.ok) {
        credentialLoading.value = false;
        const credential = result.credential;
        wallet.proposalId = textOf(credential, 'proposal_id') || proposalId;
        wallet.userId = textOf(credential, 'user_id');
        wallet.userPubKey = textOf(credential, 'user_pub_key') || userPubKey;
        wallet.signature = textOf(credential, 'signature');
        wallet.issuerKeyVersion = textOf(credential, 'issuer_key_version');
        wallet.issuedAt = textOf(credential, 'issued_at');
        hasVoted.value = null;
        addNotice(
          'success',
          '凭证已领取',
          '凭证与你的身份没有绑定，服务端也不知道是谁领的',
        );
        return true;
      }
      if (!result.retryable) {
        credentialLoading.value = false;
        addNotice('error', result.title, result.message);
        return false;
      }
      failure = result;
    }
    credentialLoading.value = false;
    addNotice(
      'error',
      failure?.title ?? '领取失败',
      failure?.message ?? '凭证池的槽位一直被占用，请稍后重试',
    );
    return false;
  }

  /** 匿名领取的单次尝试：预留 → 盲化 → 盲签名 → 解盲 → attest。 */
  async function claimCredentialOnce(
    proposalId: string,
    userPubKey: string,
  ): Promise<CredentialFlowResult> {
    // 资格材料在**领取之前**备好：证明不过就不该消耗池槽位。
    let claimBody: Record<string, string> = {};
    try {
      const eligibility = await buildEligibilityBody(proposalId);
      if (eligibility !== null) {
        claimBody = eligibility;
      }
    } catch (error) {
      return { ok: false, title: '资格证明失败', message: describeError(error) };
    }

    const reserve = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/claim`,
      method: 'POST',
      body: claimBody,
    });
    if (!reserve.ok) {
      return {
        ok: false,
        title: '领取失败',
        message: describeApiError(reserve.error, reserve.code, '凭证池可能已经领完'),
      };
    }
    const reservation = toObject(reserve.data);
    return completeCredentialBlindSignature(proposalId, userPubKey, reservation, {
      path: `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/blind-sign`,
      buildBody: (issuerKeyVersion, blindedMessage) => ({
        user_id: textOf(reservation, 'user_id'),
        blinded_message: blindedMessage,
        issuer_key_version: issuerKeyVersion,
      }),
    });
  }

  /**
   * 备好领取请求里的资格材料。
   *
   * 服务端开启了资格校验时必须带：先向服务端要一次挑战，再用资格包在本地
   * 出零知识证明。没导入资格包时返回 null —— 服务端会回
   * `ELIGIBILITY_REQUIRED`，门户据此提示先导入资格文件。
   */
  async function buildEligibilityBody(
    proposalId: string,
  ): Promise<Record<string, string> | null> {
    const pack = eligibilityPackage.value;
    if (pack === null) {
      return null;
    }

    const challengeResult = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/eligibility-challenge`,
      method: 'POST',
      body: {},
    });
    if (!challengeResult.ok) {
      throw new Error(
        describeApiError(challengeResult.error, challengeResult.code, '服务端没有签发领取挑战'),
      );
    }
    const challenge = toObject(challengeResult.data);
    const root = textOf(challenge, 'root');
    const challengeValue = textOf(challenge, 'challenge');
    if (root === '' || challengeValue === '') {
      throw new Error('服务端返回的领取挑战不完整');
    }

    const proof = await generateEligibilityProof({
      proposalId,
      challenge: challengeValue,
      root,
      memberPackage: pack,
    });
    return {
      eligibility_proof: proof.proof,
      qualification_id: proof.qualificationId,
      challenge: challengeValue,
    };
  }

  /** 打开资格文件选择框。 */
  function openEligibilityImport(): void {
    eligibilityFileInput.value?.click();
  }

  /** 清掉已导入的资格包。 */
  function clearEligibilityPackage(): void {
    eligibilityPackage.value = null;
    eligibilityPackageName.value = '';
    eligibilityError.value = '';
  }

  /**
   * 导入成员自己的资格包。
   *
   * 资格包是**秘密材料**：谁拿到它谁就能领这一票。因此门户只把它留在内存里
   * （不写进 localStorage、不上传），刷新页面就要重新导入。
   */
  async function importEligibilityPackage(event: Event): Promise<void> {
    const target = event.target;
    eligibilityError.value = '';
    if (!(target instanceof HTMLInputElement) || !target.files || target.files.length === 0) {
      return;
    }
    const file = target.files[0];
    target.value = '';
    try {
      const raw = await file.text();
      const parsed: unknown = JSON.parse(raw);
      const pack = toObject(parsed);
      if (pack === null) {
        throw new Error('资格文件不是 JSON 对象');
      }
      const secret = textOf(pack, 'secret');
      const leaf = textOf(pack, 'leaf');
      const root = textOf(pack, 'root');
      const path = pack.path;
      if (secret === '' || leaf === '' || root === '') {
        throw new Error('资格文件缺少 secret / leaf / root');
      }
      if (!Array.isArray(path) || path.some((item) => typeof item !== 'string')) {
        throw new Error('资格文件的 path 不对');
      }
      const depth = pack.depth;
      if (typeof depth !== 'number' || !Number.isFinite(depth) || depth <= 0) {
        throw new Error('资格文件的 depth 不对');
      }
      const index = pack.index;
      if (typeof index !== 'number' || !Number.isFinite(index) || index < 0) {
        throw new Error('资格文件的 index 不对');
      }
      eligibilityPackage.value = {
        member_id: textOf(pack, 'member_id'),
        secret,
        leaf,
        index,
        path: path as string[],
        root,
        depth,
      };
      eligibilityPackageName.value = file.name;
      addNotice('success', '资格文件已导入', '领凭证时会用它在本机出资格证明');
    } catch (error) {
      eligibilityPackage.value = null;
      eligibilityPackageName.value = '';
      eligibilityError.value = describeError(error);
      addNotice('error', '资格文件无效', eligibilityError.value);
    }
  }

  /**
   * 走完凭证盲签名的客户端侧：本地盲化 → 盲签名 → 本地解盲 → attest。
   *
   * 匿名领取与管理端签发只差「预留」与「盲签名」两步的端点，中间完全一样。
   */
  async function completeCredentialBlindSignature(
    proposalId: string,
    userPubKey: string,
    reservation: JsonMap | null,
    blindSign: {
      path: string;
      headers?: Record<string, string>;
      buildBody: (
        issuerKeyVersion: number,
        blindedMessage: string,
      ) => Record<string, unknown>;
    },
  ): Promise<CredentialFlowResult> {
    if (reservation === null) {
      return { ok: false, title: '领取失败', message: '服务端返回的预留结果格式不对' };
    }
    const userId = textOf(reservation, 'user_id');
    if (userId === '') {
      return { ok: false, title: '领取失败', message: '服务端没有返回 user_id' };
    }

    let issuerKey;
    try {
      issuerKey = parseIssuerKey(reservation);
    } catch (error) {
      return {
        ok: false,
        title: '领取失败',
        message: `签发方公钥不可用：${describeError(error)}`,
      };
    }

    let blinded;
    try {
      blinded = blindCredentialMessage(issuerKey, userId, base64ToBytes(userPubKey));
    } catch (error) {
      return {
        ok: false,
        title: '领取失败',
        message: `本地盲化失败：${describeError(error)}`,
      };
    }

    const blindSignResult = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: blindSign.path,
      method: 'POST',
      headers: blindSign.headers,
      body: blindSign.buildBody(
        issuerKey.version,
        bytesToBase64(blinded.blindedMessage),
      ),
    });
    if (!blindSignResult.ok) {
      return {
        ok: false,
        title: '盲签名失败',
        message: describeApiError(
          blindSignResult.error,
          blindSignResult.code,
          '服务端拒绝了这次盲签名',
        ),
      };
    }
    const blindSignature = textOf(toObject(blindSignResult.data), 'blind_signature');
    if (blindSignature === '') {
      return { ok: false, title: '盲签名失败', message: '服务端没有返回盲签名' };
    }

    let signature;
    try {
      signature = finalizeCredentialSignature(
        blinded,
        base64ToBytes(blindSignature),
        issuerKey,
      );
    } catch (error) {
      return { ok: false, title: '解盲失败', message: describeError(error) };
    }

    const attestResult = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(proposalId)}/credentials/attest`,
      method: 'POST',
      body: {
        user_id: userId,
        user_pub_key: userPubKey,
        signature: bytesToBase64(signature),
      },
    });
    if (!attestResult.ok) {
      return {
        ok: false,
        title: '凭证落库失败',
        message: describeApiError(
          attestResult.error,
          attestResult.code,
          '服务端没有接受这张凭证',
        ),
        // 槽位被别人抢走时，换一个槽位重来就能成。
        retryable: isSlotTaken(attestResult.status, attestResult.code),
      };
    }
    const credential = toObject(attestResult.data);
    if (credential === null) {
      return { ok: false, title: '凭证落库失败', message: '服务端返回的凭证格式不对' };
    }
    return { ok: true, credential };
  }

  async function issueCredential(): Promise<boolean> {
    clearErrorGroup(identityErrors);
    const proposalId = voteForm.proposalId.trim();
    if (proposalId === '') {
      identityErrors.userId = '请先选择 proposal_id，再领取该提案的凭证';
      addNotice('error', '签发失败', identityErrors.userId);
      return false;
    }
    if (!adminTokenReady.value) {
      addNotice('error', '签发失败', '领取凭证需要先填写 Admin Token');
      return false;
    }
    if (identityForm.userId.trim() === '') {
      identityErrors.userId = '请输入 user_id';
      addNotice('error', '签发失败', identityErrors.userId);
      return false;
    }
    if (useCustomPubKey.value && identityForm.customPubKey.trim() === '') {
      identityErrors.customPubKey = '启用手动公钥后不能为空';
      addNotice('error', '签发失败', identityErrors.customPubKey);
      return false;
    }
    if (currentPubKey.value === '') {
      identityErrors.customPubKey = '公钥不能为空';
      addNotice('error', '签发失败', identityErrors.customPubKey);
      return false;
    }

    credentialLoading.value = true;
    const reserve = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/credentials',
      method: 'POST',
      headers: adminHeaders(),
      body: {
        proposal_id: proposalId,
        user_id: identityForm.userId.trim(),
      },
    });
    if (!reserve.ok) {
      credentialLoading.value = false;
      addNotice(
        'error',
        '签发失败',
        describeApiError(reserve.error, reserve.code, '后端未返回凭证'),
      );
      return false;
    }
    const reservation = toObject(reserve.data);
    const result = await completeCredentialBlindSignature(
      proposalId,
      currentPubKey.value,
      reservation,
      {
        path: '/api/v1/credentials/blind-sign',
        headers: adminHeaders(),
        buildBody: (issuerKeyVersion, blindedMessage) => ({
          proposal_id: proposalId,
          user_id: textOf(reservation, 'user_id'),
          blinded_message: blindedMessage,
          issuer_key_version: issuerKeyVersion,
        }),
      },
    );
    credentialLoading.value = false;

    if (!result.ok) {
      addNotice('error', result.title, result.message);
      return false;
    }
    const credential = result.credential;
    wallet.proposalId = textOf(credential, 'proposal_id') || proposalId;
    wallet.userId = textOf(credential, 'user_id') || identityForm.userId.trim();
    wallet.userPubKey = textOf(credential, 'user_pub_key') || currentPubKey.value;
    wallet.signature = textOf(credential, 'signature');
    wallet.issuerKeyVersion = textOf(credential, 'issuer_key_version');
    wallet.issuedAt = textOf(credential, 'issued_at');
    hasVoted.value = null;
    addNotice('success', '凭证已领取', '你已可以进行匿名投票');
    return true;
  }

  async function submitVote(): Promise<boolean> {
    clearErrorGroup(voteErrors);
    if (!walletReady.value) {
      addNotice('error', '提交失败', '请先领取或导入凭证');
      return false;
    }
    if (voteForm.proposalId.trim() === '') {
      voteErrors.proposalId = '请选择 proposal_id';
      addNotice('error', '提交失败', voteErrors.proposalId);
      return false;
    }
    if (!walletMatchesProposal.value) {
      voteErrors.proposalId = `当前凭证属于提案 ${wallet.proposalId}，不能用于 ${voteForm.proposalId.trim()}`;
      addNotice('error', '提交失败', voteErrors.proposalId);
      return false;
    }
    if (
      selectedProposal.value &&
      selectedProposal.value.options.length > 0 &&
      selectedVoteOption.value.trim() === ''
    ) {
      voteErrors.option = '请选择一个投票选项';
      addNotice('error', '提交失败', voteErrors.option);
      return false;
    }
    if (!ballotTargetCurrent.value) {
      // 凭据是针对别的选项生成的：提交它会记下旧选项，而界面上显示的是新选项。
      voteErrors.option = '投票凭据对应的选项与当前选择不一致，请重新生成凭据';
      addNotice('error', '提交失败', voteErrors.option);
      return false;
    }
    if (voteForm.encryptedVote.trim() === '') {
      voteErrors.encryptedVote = 'encrypted_vote 不能为空';
    }
    if (voteForm.proof.trim() === '') {
      voteErrors.proof = 'proof 不能为空';
    }
    if (voteForm.nullifier.trim() === '') {
      voteErrors.nullifier = 'nullifier 不能为空';
    }
    if (voteErrors.encryptedVote || voteErrors.proof || voteErrors.nullifier) {
      addNotice('error', '提交失败', '请先生成或导入完整投票凭据');
      return false;
    }

    const body: JsonMap = {
      encrypted_vote: voteForm.encryptedVote.trim(),
      proof: voteForm.proof.trim(),
      nullifier: voteForm.nullifier.trim(),
      user_id: wallet.userId,
      user_pub_key: wallet.userPubKey,
      credential_signature: wallet.signature,
    };
    // 注意：不要发送 selected_option。服务端不消费这个字段，而明文选项上服务端
    // 会抵消「选票是加密的」这个属性。选票是否对得上当前选项由本地绑定校验。
    // 也不发送 proposal_id：它只从路径来（见 docs/api-contract-v1.md）。

    voteLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(voteForm.proposalId.trim())}/votes`,
      method: 'POST',
      body,
    });
    voteLoading.value = false;

    if (!result.ok) {
      addNotice(
        'error',
        '投票失败',
        describeApiError(result.error, result.code, '请检查投票凭据'),
      );
      return false;
    }

    addNotice('success', '投票成功', '匿名选票已提交');
    await fetchVoteStatus(false);
    return true;
  }

  async function fetchVoteStatus(showNotice = true): Promise<void> {
    voteStatusError.value = '';
    if (!walletReady.value || voteForm.proposalId.trim() === '') {
      voteStatusError.value = '请先准备凭证与提案';
      if (showNotice) {
        addNotice('error', '查询失败', voteStatusError.value);
      }
      return;
    }

    voteStatusLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(voteForm.proposalId.trim())}/vote-status`,
      method: 'POST',
      body: {
        user_id: wallet.userId,
        user_pub_key: wallet.userPubKey,
        credential_signature: wallet.signature,
      },
    });
    voteStatusLoading.value = false;

    if (!result.ok) {
      hasVoted.value = null;
      voteStatusError.value = describeApiError(
        result.error,
        result.code,
        '无法查询投票状态',
      );
      if (showNotice) {
        addNotice('error', '查询失败', voteStatusError.value);
      }
      return;
    }
    const payload = toObject(result.data);
    hasVoted.value = boolOf(payload, 'has_voted');
    if (showNotice) {
      addNotice(
        'success',
        '查询完成',
        hasVoted.value ? '你已投票' : '你还未投票',
      );
    }
  }

  /** 追加一个候选项输入行。 */
  function addProposalOption(): void {
    createProposalForm.options = appendOptionRow(createProposalForm.options);
  }

  /** 删除指定候选项输入行；行数已到下限时不动。 */
  function removeProposalOption(index: number): void {
    createProposalForm.options = removeOptionRow(
      createProposalForm.options,
      index,
    );
  }

  /** 当前还能不能继续删除候选项行。 */
  function canRemoveProposalOption(): boolean {
    return canRemoveOptionRow(createProposalForm.options);
  }

  async function createProposal(): Promise<boolean> {
    clearErrorGroup(createProposalErrors);
    if (!adminTokenReady.value) {
      addNotice('error', '创建失败', '创建提案需要先填写 Admin Token');
      return false;
    }

    if (createProposalForm.title.trim() === '') {
      createProposalErrors.title = '请填写标题';
    }

    const options = normalizeOptionList(createProposalForm.options);
    if (options.length < 2) {
      createProposalErrors.options = '请至少提供 2 个投票选项';
    }

    const startsAt = localInputToRfc3339(createProposalForm.startsAtLocal);
    const endsAt = localInputToRfc3339(createProposalForm.endsAtLocal);
    if (startsAt === null) {
      createProposalErrors.startsAt = '请填写有效的开始时间';
    }
    if (endsAt === null) {
      createProposalErrors.endsAt = '请填写有效的结束时间';
    }
    if (
      startsAt &&
      endsAt &&
      new Date(startsAt).getTime() >= new Date(endsAt).getTime()
    ) {
      createProposalErrors.endsAt = '结束时间必须晚于开始时间';
    }

    if (
      createProposalErrors.title ||
      createProposalErrors.options ||
      createProposalErrors.startsAt ||
      createProposalErrors.endsAt
    ) {
      addNotice('error', '创建失败', '请先修复表单中的错误');
      return false;
    }

    createProposalLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/proposals',
      method: 'POST',
      headers: adminHeaders(),
      body: {
        title: createProposalForm.title.trim(),
        description: createProposalForm.description.trim(),
        options,
        starts_at: startsAt,
        ends_at: endsAt,
      },
    });
    createProposalLoading.value = false;

    if (!result.ok) {
      addNotice(
        'error',
        '创建失败',
        describeApiError(result.error, result.code, '提案创建失败'),
      );
      return false;
    }

    const payload = toObject(result.data);
    const id = textOf(payload, 'id');
    if (id !== '') {
      adoptProposal(id);
    }
    addNotice(
      'success',
      '提案创建成功',
      id === '' ? '已创建提案' : `提案编号：${id}`,
    );
    await loadOpenProposals(false, true);
    return true;
  }

  async function setupDecryptionSession(): Promise<boolean> {
    clearErrorGroup(decryptErrors);
    if (!adminTokenReady.value) {
      addNotice('error', '初始化失败', '初始化解密会话需要先填写 Admin Token');
      return false;
    }

    if (decryptForm.proposalId.trim() === '') {
      decryptErrors.proposalId = 'proposal_id 不能为空';
    }
    if (Number(decryptForm.threshold) < 1) {
      decryptErrors.threshold = 'threshold 必须大于 0';
    }
    if (Number(decryptForm.trustees) < 1) {
      decryptErrors.trustees = 'trustees 必须大于 0';
    }
    if (Number(decryptForm.threshold) > Number(decryptForm.trustees)) {
      decryptErrors.threshold = 'threshold 不能大于 trustees';
    }

    if (
      decryptErrors.proposalId ||
      decryptErrors.threshold ||
      decryptErrors.trustees
    ) {
      addNotice('error', '初始化失败', '请先修复表单中的错误');
      return false;
    }

    decryptLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(decryptForm.proposalId.trim())}/decryption/setup`,
      method: 'POST',
      headers: adminHeaders(),
      body: {
        threshold: Number(decryptForm.threshold),
        trustees: Number(decryptForm.trustees),
      },
    });
    decryptLoading.value = false;

    if (!result.ok) {
      addNotice(
        'error',
        '初始化失败',
        describeApiError(result.error, result.code, '无法初始化解密会话'),
      );
      return false;
    }

    const payload = toObject(result.data);
    decryptPublicKey.value = textOf(payload, 'public_key');
    const shares = payload?.shares;
    decryptShares.value = Array.isArray(shares)
      ? shares.filter((item): item is string => typeof item === 'string')
      : [];
    publishForm.sharesText = decryptShares.value.join('\n');
    addNotice(
      'success',
      '会话已初始化',
      `已生成 ${decryptShares.value.length} 份解密份额`,
    );
    return true;
  }

  async function publishResult(): Promise<boolean> {
    clearErrorGroup(publishErrors);
    if (!adminTokenReady.value) {
      addNotice('error', '发布失败', '发布结果需要先填写 Admin Token');
      return false;
    }
    if (publishForm.proposalId.trim() === '') {
      publishErrors.proposalId = 'proposal_id 不能为空';
    }
    const shares = splitShares(publishForm.sharesText);
    if (shares.length === 0) {
      publishErrors.shares = '至少提供一份 share';
    }
    if (publishErrors.proposalId || publishErrors.shares) {
      addNotice('error', '发布失败', '请先修复表单中的错误');
      return false;
    }

    publishLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(publishForm.proposalId.trim())}/results/publish`,
      method: 'POST',
      headers: adminHeaders(),
      body: { shares },
    });
    publishLoading.value = false;

    if (!result.ok) {
      addNotice(
        'error',
        '发布失败',
        describeApiError(
          result.error,
          result.code,
          '发布结果失败，请确认已提交足够的解密份额',
        ),
      );
      return false;
    }

    const payload = toObject(result.data);
    const votes = payload?.total_votes;
    const anchorHash = textOf(payload, 'anchor_hash');
    publishedSummary.value = `总票数 ${typeof votes === 'number' ? votes : '-'}，Anchor ${anchorHash || '-'}`;
    addNotice('success', '发布成功', publishedSummary.value);
    return true;
  }

  async function fetchResult(): Promise<boolean> {
    clearErrorGroup(resultErrors);
    if (resultForm.proposalId.trim() === '') {
      resultErrors.proposalId = 'proposal_id 不能为空';
      addNotice('error', '查询失败', resultErrors.proposalId);
      return false;
    }

    resultLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(resultForm.proposalId.trim())}/results`,
    });
    resultLoading.value = false;

    if (!result.ok) {
      resultSummary.value = '';
      resultTally.value = [];
      addNotice(
        'error',
        '查询失败',
        describeApiError(
          result.error,
          result.code,
          '该提案还没有发布结果，或结果尚未成功解密',
        ),
      );
      return false;
    }

    const payload = toObject(result.data);
    const votes = payload?.total_votes;
    const anchorHash = textOf(payload, 'anchor_hash');
    resultSummary.value = `总票数 ${typeof votes === 'number' ? votes : '-'}，Anchor ${anchorHash || '-'}`;

    const tally = toObject(payload?.tally);
    resultTally.value = tally
      ? Object.entries(tally)
          .map(([option, value]) => ({
            option,
            count: typeof value === 'number' ? value : 0,
          }))
          .sort((a, b) => b.count - a.count)
      : [];

    addNotice('success', '查询完成', resultSummary.value);
    return true;
  }

  async function fetchAnchors(): Promise<void> {
    clearErrorGroup(anchorErrors);
    if (anchorForm.proposalId.trim() === '') {
      anchorErrors.proposalId = 'proposal_id 不能为空';
      addNotice('error', '查询失败', anchorErrors.proposalId);
      return;
    }

    anchorLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(anchorForm.proposalId.trim())}/audit-anchors`,
    });
    anchorLoading.value = false;

    if (!result.ok) {
      anchors.value = [];
      addNotice(
        'error',
        '查询失败',
        describeApiError(
          result.error,
          result.code,
          '该提案还没有可核验的审计锚点',
        ),
      );
      return;
    }

    const payload = toObject(result.data);
    const items = payload?.items;
    if (!Array.isArray(items)) {
      anchors.value = [];
      addNotice('info', '查询完成', '暂无审计锚点');
      return;
    }

    anchors.value = items
      .map((item): AnchorItem | null => {
        const object = toObject(item);
        if (object === null) {
          return null;
        }
        const id = textOf(object, 'id');
        if (id === '') {
          return null;
        }
        return {
          id,
          anchorHash: textOf(object, 'anchor_hash'),
          merkleRoot: textOf(object, 'merkle_root'),
          createdAt: textOf(object, 'created_at'),
        };
      })
      .filter((item): item is AnchorItem => item !== null);
    addNotice(
      'success',
      '查询完成',
      `获取到 ${anchors.value.length} 条审计锚点`,
    );
  }

  async function fetchCredentialStatuses(): Promise<boolean> {
    clearErrorGroup(credentialStatusErrors);
    if (!adminTokenReady.value) {
      addNotice('error', '查询失败', '查询凭证状态需要先填写 Admin Token');
      return false;
    }
    if (credentialStatusForm.proposalId.trim() === '') {
      credentialStatusErrors.proposalId = 'proposal_id 不能为空';
      addNotice('error', '查询失败', credentialStatusErrors.proposalId);
      return false;
    }

    credentialStatusLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(credentialStatusForm.proposalId.trim())}/credentials`,
      headers: adminHeaders(),
    });
    credentialStatusLoading.value = false;

    if (!result.ok) {
      credentialStatuses.value = [];
      addNotice(
        'error',
        '查询失败',
        describeApiError(result.error, result.code, '该提案还没有签发任何凭证'),
      );
      return false;
    }

    const payload = toObject(result.data);
    const items = payload?.items;
    if (!Array.isArray(items)) {
      credentialStatuses.value = [];
      addNotice('info', '查询完成', '暂无凭证状态记录');
      return true;
    }

    credentialStatuses.value = items
      .map((item): CredentialStatusItem | null => {
        const object = toObject(item);
        if (object === null) {
          return null;
        }
        const credentialId = textOf(object, 'credential_id');
        if (credentialId === '') {
          return null;
        }
        return {
          credentialId,
          proposalId: textOf(object, 'proposal_id'),
          userId: textOf(object, 'user_id'),
          userPubKey: textOf(object, 'user_pub_key'),
          issuerKeyVersion: textOf(object, 'issuer_key_version'),
          issuedAt: textOf(object, 'issued_at'),
          hasVoted: boolOf(object, 'has_voted') === true,
        };
      })
      .filter((item): item is CredentialStatusItem => item !== null);

    addNotice(
      'success',
      '查询完成',
      `获取到 ${credentialStatuses.value.length} 条凭证状态`,
    );
    return true;
  }

  watch(apiBaseUrl, (value) => {
    localStorage.setItem(baseUrlStorageKey, value.trim());
  });

  watch(adminBearerToken, (value) => {
    // 管理令牌不落 localStorage：同源下的任意脚本、共享浏览器都能直接读到。
    // sessionStorage 至少随标签页/会话结束而失效。
    sessionStorage.setItem(adminTokenStorageKey, value.trim());
  });

  watch(selectedProposalId, (value) => {
    const id = value.trim();
    if (id === '') {
      return;
    }
    // 换提案就重置「是否已投票」：上一个提案的状态不能套到新提案上。
    hasVoted.value = null;
    voteForm.proposalId = id;
    decryptForm.proposalId = id;
    publishForm.proposalId = id;
    resultForm.proposalId = id;
    anchorForm.proposalId = id;
    credentialStatusForm.proposalId = id;

    const proposal = hallState.proposals.find((item) => item.id === id);
    if (
      proposal &&
      proposal.options.length > 0 &&
      !proposal.options.includes(selectedVoteOption.value)
    ) {
      selectedVoteOption.value = proposal.options[0];
    }
  });

  watch(selectedProposal, (proposal) => {
    if (!proposal || proposal.options.length === 0) {
      return;
    }
    if (!proposal.options.includes(selectedVoteOption.value)) {
      selectedVoteOption.value = proposal.options[0];
    }
  });

  // 选项或提案变了，已生成的凭据就不再对应当前意图 —— 作废它，
  // 否则会投出旧选项而界面上显示的是新选项。
  watch([selectedVoteOption, () => voteForm.proposalId], () => {
    if (!hasBallotTargetDrifted(boundBallotTarget.value, currentBallotTarget.value)) {
      return;
    }
    clearBallotMaterial();
    addNotice(
      'info',
      '目标已变更',
      '投票凭据已清空，请按当前选项重新生成后再提交',
    );
  });

  watch([workspace, activeSection, selectedProposalId], () => {
    persistRouteToStorage();
    syncRouteToUrl();
  });

  onMounted(async () => {
    const routeFromUrl = parsePortalRoute(window.location.search);
    const routeFromStorage = parseStoredRoute(
      localStorage.getItem(navStorageKey),
    );
    const effectiveRoute =
      window.location.search.trim() === '' && routeFromStorage
        ? routeFromStorage
        : routeFromUrl;
    const hasSeenOnboarding =
      localStorage.getItem(onboardingStorageKey) === '1';

    workspace.value = effectiveRoute.workspace;
    activeSection.value = effectiveRoute.section;
    selectedProposalId.value = effectiveRoute.proposalId;

    if (!hasSeenOnboarding) {
      workspace.value = 'user';
      activeSection.value = 'overview';
      guideModeVisible.value = true;
    }

    routeReady = true;
    syncRouteToUrl();

    setProposalWindow('scheduled');
    createProposalForm.options = ['候选人 A', '候选人 B'];
    voteForm.nullifier = newNullifier();

    await checkHealth(false);
    await loadOpenProposals(false);
    await loadAllProposals(false);
  });

  onUnmounted(() => {
    for (const timer of noticeTimers.values()) {
      window.clearTimeout(timer);
    }
    noticeTimers.clear();
  });

  return {
    workspaces,
    sections,
    visibleSections,
    workspace,
    activeSection,
    selectedProposalId,
    setWorkspace,
    setActiveSection,

    apiBaseUrl,
    adminBearerToken,
    healthState,
    checkHealth,

    hallState,
    loadOpenProposals,
    selectedProposal,
    adoptProposal,

    opsProposalsState,
    loadAllProposals,

    identityForm,
    useCustomPubKey,
    generatedPubKey,
    regeneratePubKey,
    currentPubKey,
    credentialLoading,
    wallet,
    walletReady,
    adminTokenReady,
    credentialFileInput,
    identityErrors,
    issueCredential,
    claimCredential,
    openCredentialImport,
    importCredentialFile,
    exportCredentialFile,
    eligibilityPackage,
    eligibilityPackageName,
    eligibilityFileInput,
    eligibilityError,
    openEligibilityImport,
    importEligibilityPackage,
    clearEligibilityPackage,

    voteForm,
    voteLoading,
    voteStatusLoading,
    voteStatusError,
    hasVoted,
    selectedVoteOption,
    ballotTargetCurrent,
    showAdvancedVoteFields,
    voteErrors,
    applySelectedOptionToBallot,
    fillBallotTemplate,
    importBallotJson,
    generateBallotLocally,
    submitVote,
    fetchVoteStatus,
    newNullifier,

    createProposalForm,
    createProposalLoading,
    createProposalErrors,
    createProposal,
    addProposalOption,
    removeProposalOption,
    canRemoveProposalOption,
    setProposalWindow,

    decryptForm,
    decryptLoading,
    decryptPublicKey,
    decryptShares,
    decryptErrors,
    setupDecryptionSession,

    publishForm,
    publishLoading,
    publishedSummary,
    publishErrors,
    publishResult,

    resultForm,
    resultLoading,
    resultSummary,
    resultTally,
    resultErrors,
    fetchResult,

    anchorForm,
    anchorLoading,
    anchors,
    anchorErrors,
    fetchAnchors,

    credentialStatusForm,
    credentialStatusLoading,
    credentialStatuses,
    credentialStatusErrors,
    fetchCredentialStatuses,

    notices,
    guideModeVisible,
    openGuideMode,
    closeGuideMode,
    startGuideJourney,
    dismissNotice,
  };
}
