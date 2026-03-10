import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { apiCall } from '../lib/api';
import { localInputToRfc3339, toLocalDateTimeInput } from '../lib/datetime';
import { buildPortalSearch, getDefaultSection, isSectionAllowed, parsePortalRoute } from '../lib/navigation';
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

const workspaces: WorkspaceMeta[] = [
  { id: 'user', title: '用户端', description: '领取凭证并完成匿名投票' },
  { id: 'admin', title: '运营端', description: '创建提案、计票与审计' },
];

const sections: SectionMeta[] = [
  { id: 'hall', workspace: 'user', title: '投票大厅', description: '浏览可参与提案' },
  { id: 'identity', workspace: 'user', title: '我的身份', description: '领取或导入匿名凭证' },
  { id: 'vote', workspace: 'user', title: '参与投票', description: '按步骤生成并提交选票' },
  { id: 'ops', workspace: 'admin', title: '计票与审计', description: '提案管理与结果发布' },
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
  return typeof value === 'string' ? value : '';
}

function boolOf(object: JsonMap | null, key: string): boolean | null {
  if (object === null) {
    return null;
  }
  const value = object[key];
  return typeof value === 'boolean' ? value : null;
}

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

function newNullifier(): string {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  const suffix = Array.from(bytes, (item) => item.toString(16).padStart(2, '0')).join('');
  return `null-${Date.now()}-${suffix}`;
}

function splitShares(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function normalizeOptions(raw: string): string[] {
  return raw
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
    .filter((item, index, array) => array.indexOf(item) === index);
}

function encodeProposalDescription(summary: string, options: string[]): string {
  return JSON.stringify({ summary: summary.trim(), options });
}

function decodeProposalDescription(raw: string): { description: string; options: string[] } {
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
    const options = Array.isArray(optionField) ? optionField.filter((item): item is string => typeof item === 'string') : [];
    return { description: summary, options };
  } catch {
    return { description: raw, options: [] };
  }
}

function nowLabel(): string {
  return new Date().toLocaleString('zh-CN', { hour12: false });
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
      sectionCandidate === 'hall' || sectionCandidate === 'identity' || sectionCandidate === 'vote' || sectionCandidate === 'ops'
        ? sectionCandidate
        : getDefaultSection(workspace);
    const proposalId = typeof parsed.proposalId === 'string' ? parsed.proposalId.trim() : '';
    return {
      workspace,
      section: isSectionAllowed(workspace, section) ? section : getDefaultSection(workspace),
      proposalId,
    };
  } catch {
    return null;
  }
}

export function useVotePortal() {
  const apiBaseUrl = ref(localStorage.getItem(baseUrlStorageKey) ?? 'http://localhost:8080');
  const adminBearerToken = ref(localStorage.getItem(adminTokenStorageKey) ?? '');
  const workspace = ref<WorkspaceId>('user');
  const activeSection = ref<SectionId>('hall');
  const selectedProposalId = ref('');

  const notices = ref<Notice[]>([]);
  const activeAlert = ref<Notice | null>(null);

  const healthState = reactive({ loading: false, ok: false, text: '未检测', checkedAt: '' });
  const hallState = reactive({ loading: false, proposals: [] as ProposalItem[] });

  const identityForm = reactive({ userId: '', customPubKey: '' });
  const useCustomPubKey = ref(false);
  const generatedPubKey = ref(randomBase64(32));
  const wallet = reactive({ userId: '', userPubKey: '', signature: '', issuerKeyVersion: '', issuedAt: '' });
  const credentialLoading = ref(false);
  const credentialFileInput = ref<HTMLInputElement | null>(null);

  const voteForm = reactive({
    proposalId: '',
    ballotJson: '',
    encryptedVote: '',
    proof: '',
    nullifier: '',
    publicWitness: '',
  });
  const voteLoading = ref(false);
  const voteStatusLoading = ref(false);
  const hasVoted = ref<boolean | null>(null);
  const selectedVoteOption = ref('');
  const showAdvancedVoteFields = ref(false);

  const createProposalForm = reactive({ title: '', description: '', optionsText: '', startsAtLocal: '', endsAtLocal: '' });
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

  const identityErrors = reactive<FormErrorMap>({ userId: '', customPubKey: '' });
  const voteErrors = reactive<FormErrorMap>({
    proposalId: '',
    option: '',
    ballotJson: '',
    encryptedVote: '',
    proof: '',
    nullifier: '',
  });
  const createProposalErrors = reactive<FormErrorMap>({ title: '', options: '', startsAt: '', endsAt: '' });
  const decryptErrors = reactive<FormErrorMap>({ proposalId: '', threshold: '', trustees: '' });
  const publishErrors = reactive<FormErrorMap>({ proposalId: '', shares: '' });
  const resultErrors = reactive<FormErrorMap>({ proposalId: '' });
  const anchorErrors = reactive<FormErrorMap>({ proposalId: '' });
  const credentialStatusErrors = reactive<FormErrorMap>({ proposalId: '' });

  const currentPubKey = computed(() => (useCustomPubKey.value ? identityForm.customPubKey.trim() : generatedPubKey.value));
  const walletReady = computed(() => wallet.userId !== '' && wallet.userPubKey !== '' && wallet.signature !== '');
  const latestNotice = computed(() => notices.value[0] ?? null);
  const noticeHistory = computed(() => notices.value.slice(1));
  const selectedProposal = computed(() => {
    const byVoteForm = voteForm.proposalId.trim();
    const bySelection = selectedProposalId.value.trim();
    return (
      hallState.proposals.find((item) => item.id === byVoteForm) ??
      hallState.proposals.find((item) => item.id === bySelection) ??
      null
    );
  });
  const visibleSections = computed(() => sections.filter((section) => section.workspace === workspace.value));

  let routeReady = false;
  let alertTimer: number | undefined;

  function addNotice(level: Notice['level'], title: string, detail: string): void {
    const notice: Notice = { level, title, detail, at: nowLabel() };
    notices.value.unshift(notice);
    if (notices.value.length > 12) {
      notices.value.length = 12;
    }
    activeAlert.value = notice;
    if (alertTimer !== undefined) {
      window.clearTimeout(alertTimer);
    }
    alertTimer = window.setTimeout(() => {
      if (activeAlert.value === notice) {
        activeAlert.value = null;
      }
    }, 5500);
  }

  function dismissActiveAlert(): void {
    activeAlert.value = null;
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
      history.replaceState({}, '', `${current.pathname}${rendered}${current.hash}`);
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

  function setProposalWindow(mode: 'active' | 'closed'): void {
    if (mode === 'active') {
      createProposalForm.startsAtLocal = toLocalDateTimeInput(new Date(Date.now() - 5 * 60 * 1000));
      createProposalForm.endsAtLocal = toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000));
      return;
    }
    createProposalForm.startsAtLocal = toLocalDateTimeInput(new Date(Date.now() - 2 * 60 * 60 * 1000));
    createProposalForm.endsAtLocal = toLocalDateTimeInput(new Date(Date.now() - 60 * 1000));
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
      user_id: wallet.userId,
      user_pub_key: wallet.userPubKey,
      signature: wallet.signature,
      issuer_key_version: wallet.issuerKeyVersion,
      issued_at: wallet.issuedAt,
      exported_at: new Date().toISOString(),
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
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
    if (!(target instanceof HTMLInputElement) || !target.files || target.files.length === 0) {
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
      wallet.userId = userID;
      wallet.userPubKey = userPubKey;
      wallet.signature = signature;
      wallet.issuerKeyVersion = textOf(parsed, 'issuer_key_version');
      wallet.issuedAt = textOf(parsed, 'issued_at');
      identityForm.userId = userID;
      hasVoted.value = null;
      addNotice('success', '导入成功', `已导入凭证：${userID}`);
    } catch {
      addNotice('error', '导入失败', '文件内容不是合法 JSON');
    } finally {
      target.value = '';
    }
  }

  function updateBallotJson(selectedOption: string): void {
    voteForm.ballotJson = JSON.stringify(
      {
        selected_option: selectedOption,
        encrypted_vote: voteForm.encryptedVote,
        proof: voteForm.proof,
        nullifier: voteForm.nullifier,
        public_witness: voteForm.publicWitness,
      },
      null,
      2,
    );
  }

  function applySelectedOptionToBallot(): void {
    clearErrorGroup(voteErrors);
    if (!selectedProposal.value || selectedProposal.value.options.length === 0) {
      voteErrors.option = '当前提案没有可选项';
      addNotice('error', '填充失败', voteErrors.option);
      return;
    }
    if (selectedVoteOption.value.trim() === '') {
      voteErrors.option = '请先选择一个选项';
      addNotice('error', '填充失败', voteErrors.option);
      return;
    }
    try {
      const current = voteForm.ballotJson.trim();
      const base = current === '' ? {} : toObject(JSON.parse(current)) ?? {};
      const payload: JsonMap = {
        ...base,
        selected_option: selectedVoteOption.value,
        encrypted_vote: textOf(base, 'encrypted_vote') || voteForm.encryptedVote,
        proof: textOf(base, 'proof') || voteForm.proof,
        nullifier: textOf(base, 'nullifier') || voteForm.nullifier || newNullifier(),
        public_witness: textOf(base, 'public_witness') || voteForm.publicWitness,
      };
      voteForm.encryptedVote = textOf(payload, 'encrypted_vote');
      voteForm.proof = textOf(payload, 'proof');
      voteForm.nullifier = textOf(payload, 'nullifier');
      voteForm.publicWitness = textOf(payload, 'public_witness');
      voteForm.ballotJson = JSON.stringify(payload, null, 2);
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
      voteForm.encryptedVote = textOf(parsed, 'encrypted_vote') || textOf(parsed, 'encryptedVote');
      voteForm.proof = textOf(parsed, 'proof');
      voteForm.nullifier = textOf(parsed, 'nullifier');
      voteForm.publicWitness = textOf(parsed, 'public_witness') || textOf(parsed, 'publicWitness');
      const chosenOption = textOf(parsed, 'selected_option') || textOf(parsed, 'option');
      if (chosenOption !== '') {
        selectedVoteOption.value = chosenOption;
      }
      addNotice('success', '导入成功', '投票凭据已写入表单');
    } catch {
      voteErrors.ballotJson = 'JSON 格式不合法';
      addNotice('error', '导入失败', voteErrors.ballotJson);
    }
  }

  async function prepareBallotByOption(): Promise<void> {
    clearErrorGroup(voteErrors);
    if (!walletReady.value) {
      addNotice('error', '生成失败', '请先领取或导入匿名凭证');
      return;
    }
    if (voteForm.proposalId.trim() === '') {
      voteErrors.proposalId = '请先选择 proposal_id';
      addNotice('error', '生成失败', voteErrors.proposalId);
      return;
    }
    if (selectedVoteOption.value.trim() === '') {
      voteErrors.option = '请先选择投票选项';
      addNotice('error', '生成失败', voteErrors.option);
      return;
    }

    voteLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(voteForm.proposalId.trim())}/ballot-draft`,
      method: 'POST',
      body: { option: selectedVoteOption.value.trim() },
    });
    voteLoading.value = false;

    if (!result.ok) {
      addNotice('error', '生成失败', result.error ?? '无法根据选项生成投票凭据');
      return;
    }

    const payload = toObject(result.data);
    voteForm.encryptedVote = textOf(payload, 'encrypted_vote');
    voteForm.proof = textOf(payload, 'proof');
    voteForm.nullifier = textOf(payload, 'nullifier') || newNullifier();
    const option = textOf(payload, 'option');
    if (option !== '') {
      selectedVoteOption.value = option;
    }
    updateBallotJson(selectedVoteOption.value);
    addNotice('success', '生成成功', '已根据选项生成可提交的投票凭据');
  }

  async function checkHealth(showNotice = true): Promise<void> {
    healthState.loading = true;
    const result = await apiCall({ baseUrl: apiBaseUrl.value, path: '/healthz' });
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
      addNotice('error', '连接失败', result.error ?? '无法访问后端服务');
    }
  }

  async function loadOpenProposals(showNotice = true): Promise<void> {
    hallState.loading = true;
    const result = await apiCall({ baseUrl: apiBaseUrl.value, path: '/api/v1/proposals/open' });
    hallState.loading = false;
    if (!result.ok) {
      hallState.proposals = [];
      if (showNotice) {
        addNotice('error', '加载失败', result.error ?? '无法获取提案');
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
    hallState.proposals = items
      .map((item): ProposalItem | null => {
        const object = toObject(item);
        if (object === null) {
          return null;
        }
        const id = textOf(object, 'id');
        if (id === '') {
          return null;
        }
        const parsedDescription = decodeProposalDescription(textOf(object, 'description'));
        return {
          id,
          title: textOf(object, 'title'),
          description: parsedDescription.description,
          options: parsedDescription.options,
          status: textOf(object, 'status'),
          startsAt: textOf(object, 'starts_at'),
          endsAt: textOf(object, 'ends_at'),
        };
      })
      .filter((item): item is ProposalItem => item !== null);

    const selected = selectedProposalId.value.trim();
    const exists = hallState.proposals.some((item) => item.id === selected);
    if (!exists && hallState.proposals.length > 0) {
      adoptProposal(hallState.proposals[0].id);
    }
    if (showNotice) {
      addNotice('success', '提案已刷新', `共 ${hallState.proposals.length} 个提案`);
    }
  }

  async function issueCredential(): Promise<void> {
    clearErrorGroup(identityErrors);
    if (voteForm.proposalId.trim() === '') {
      identityErrors.userId = '请先选择 proposal_id，再领取该提案的凭证';
      addNotice('error', '签发失败', identityErrors.userId);
      return;
    }
    if (identityForm.userId.trim() === '') {
      identityErrors.userId = '请输入 user_id';
      addNotice('error', '签发失败', identityErrors.userId);
      return;
    }
    if (useCustomPubKey.value && identityForm.customPubKey.trim() === '') {
      identityErrors.customPubKey = '启用手动公钥后不能为空';
      addNotice('error', '签发失败', identityErrors.customPubKey);
      return;
    }
    if (currentPubKey.value === '') {
      identityErrors.customPubKey = '公钥不能为空';
      addNotice('error', '签发失败', identityErrors.customPubKey);
      return;
    }

    credentialLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/credentials',
      method: 'POST',
      body: {
        proposal_id: voteForm.proposalId.trim(),
        user_id: identityForm.userId.trim(),
        user_pub_key: currentPubKey.value,
      },
    });
    credentialLoading.value = false;

    if (!result.ok) {
      addNotice('error', '签发失败', result.error ?? '后端未返回凭证');
      return;
    }
    const payload = toObject(result.data);
    wallet.userId = textOf(payload, 'user_id') || identityForm.userId.trim();
    wallet.userPubKey = textOf(payload, 'user_pub_key');
    wallet.signature = textOf(payload, 'signature');
    wallet.issuerKeyVersion = textOf(payload, 'issuer_key_version');
    wallet.issuedAt = textOf(payload, 'issued_at');
    hasVoted.value = null;
    addNotice('success', '凭证已领取', '你已可以进行匿名投票');
  }

  async function submitVote(): Promise<void> {
    clearErrorGroup(voteErrors);
    if (!walletReady.value) {
      addNotice('error', '提交失败', '请先领取或导入凭证');
      return;
    }
    if (voteForm.proposalId.trim() === '') {
      voteErrors.proposalId = '请选择 proposal_id';
      addNotice('error', '提交失败', voteErrors.proposalId);
      return;
    }
    if (selectedProposal.value && selectedProposal.value.options.length > 0 && selectedVoteOption.value.trim() === '') {
      voteErrors.option = '请选择一个投票选项';
      addNotice('error', '提交失败', voteErrors.option);
      return;
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
      return;
    }

    const body: JsonMap = {
      proposal_id: voteForm.proposalId.trim(),
      encrypted_vote: voteForm.encryptedVote.trim(),
      proof: voteForm.proof.trim(),
      nullifier: voteForm.nullifier.trim(),
      user_id: wallet.userId,
      user_pub_key: wallet.userPubKey,
      credential_signature: wallet.signature,
    };
    if (selectedVoteOption.value.trim() !== '') {
      body.selected_option = selectedVoteOption.value.trim();
    }
    if (voteForm.publicWitness.trim() !== '') {
      body.public_witness = voteForm.publicWitness.trim();
    }

    voteLoading.value = true;
    const result = await apiCall({ baseUrl: apiBaseUrl.value, path: '/api/v1/votes', method: 'POST', body });
    voteLoading.value = false;

    if (!result.ok) {
      addNotice('error', '投票失败', result.error ?? '请检查投票凭据');
      return;
    }

    addNotice('success', '投票成功', '匿名选票已提交');
    await fetchVoteStatus(false);
  }

  async function fetchVoteStatus(showNotice = true): Promise<void> {
    if (!walletReady.value || voteForm.proposalId.trim() === '') {
      addNotice('error', '查询失败', '请先准备凭证与提案');
      return;
    }

    voteStatusLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(voteForm.proposalId.trim())}/vote-status`,
      query: { user_id: wallet.userId },
    });
    voteStatusLoading.value = false;

    if (!result.ok) {
      hasVoted.value = null;
      if (showNotice) {
        addNotice('error', '查询失败', result.error ?? '无法查询状态');
      }
      return;
    }
    const payload = toObject(result.data);
    hasVoted.value = boolOf(payload, 'has_voted');
    if (showNotice) {
      addNotice('success', '查询完成', hasVoted.value ? '你已投票' : '你还未投票');
    }
  }

  async function createProposal(): Promise<void> {
    clearErrorGroup(createProposalErrors);

    if (createProposalForm.title.trim() === '') {
      createProposalErrors.title = '请填写标题';
    }

    const options = normalizeOptions(createProposalForm.optionsText);
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
    if (startsAt && endsAt && new Date(startsAt).getTime() >= new Date(endsAt).getTime()) {
      createProposalErrors.endsAt = '结束时间必须晚于开始时间';
    }

    if (createProposalErrors.title || createProposalErrors.options || createProposalErrors.startsAt || createProposalErrors.endsAt) {
      addNotice('error', '创建失败', '请先修复表单中的错误');
      return;
    }

    createProposalLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: '/api/v1/proposals',
      method: 'POST',
      headers: adminHeaders(),
      body: {
        title: createProposalForm.title.trim(),
        description: encodeProposalDescription(createProposalForm.description, options),
        starts_at: startsAt,
        ends_at: endsAt,
      },
    });
    createProposalLoading.value = false;

    if (!result.ok) {
      addNotice('error', '创建失败', result.error ?? '提案创建失败');
      return;
    }

    const payload = toObject(result.data);
    const id = textOf(payload, 'id');
    if (id !== '') {
      adoptProposal(id);
    }
    addNotice('success', '提案创建成功', id === '' ? '已创建提案' : `提案编号：${id}`);
    await loadOpenProposals(false);
  }

  async function setupDecryptionSession(): Promise<void> {
    clearErrorGroup(decryptErrors);

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

    if (decryptErrors.proposalId || decryptErrors.threshold || decryptErrors.trustees) {
      addNotice('error', '初始化失败', '请先修复表单中的错误');
      return;
    }

    decryptLoading.value = true;
    const result = await apiCall({
      baseUrl: apiBaseUrl.value,
      path: `/api/v1/proposals/${encodeURIComponent(decryptForm.proposalId.trim())}/decryption/setup`,
      method: 'POST',
      headers: adminHeaders(),
      body: { threshold: Number(decryptForm.threshold), trustees: Number(decryptForm.trustees) },
    });
    decryptLoading.value = false;

    if (!result.ok) {
      addNotice('error', '初始化失败', result.error ?? '无法初始化解密会话');
      return;
    }

    const payload = toObject(result.data);
    decryptPublicKey.value = textOf(payload, 'public_key');
    const shares = payload?.shares;
    decryptShares.value = Array.isArray(shares) ? shares.filter((item): item is string => typeof item === 'string') : [];
    publishForm.sharesText = decryptShares.value.join('\n');
    addNotice('success', '会话已初始化', `已生成 ${decryptShares.value.length} 份解密份额`);
  }

  async function publishResult(): Promise<void> {
    clearErrorGroup(publishErrors);
    if (publishForm.proposalId.trim() === '') {
      publishErrors.proposalId = 'proposal_id 不能为空';
    }
    const shares = splitShares(publishForm.sharesText);
    if (shares.length === 0) {
      publishErrors.shares = '至少提供一份 share';
    }
    if (publishErrors.proposalId || publishErrors.shares) {
      addNotice('error', '发布失败', '请先修复表单中的错误');
      return;
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
      addNotice('error', '发布失败', result.error ?? '发布结果失败');
      return;
    }

    const payload = toObject(result.data);
    const votes = payload?.total_votes;
    const anchorHash = textOf(payload, 'anchor_hash');
    publishedSummary.value = `总票数 ${typeof votes === 'number' ? votes : '-'}，Anchor ${anchorHash || '-'}`;
    addNotice('success', '发布成功', publishedSummary.value);
  }

  async function fetchResult(): Promise<void> {
    clearErrorGroup(resultErrors);
    if (resultForm.proposalId.trim() === '') {
      resultErrors.proposalId = 'proposal_id 不能为空';
      addNotice('error', '查询失败', resultErrors.proposalId);
      return;
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
      addNotice('error', '查询失败', result.error ?? '无法获取计票结果');
      return;
    }

    const payload = toObject(result.data);
    const votes = payload?.total_votes;
    const anchorHash = textOf(payload, 'anchor_hash');
    resultSummary.value = `总票数 ${typeof votes === 'number' ? votes : '-'}，Anchor ${anchorHash || '-'}`;

    const tally = toObject(payload?.tally);
    resultTally.value = tally
      ? Object.entries(tally)
          .map(([option, value]) => ({ option, count: typeof value === 'number' ? value : 0 }))
          .sort((a, b) => b.count - a.count)
      : [];

    addNotice('success', '查询完成', resultSummary.value);
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
      addNotice('error', '查询失败', result.error ?? '无法获取审计锚点');
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
    addNotice('success', '查询完成', `获取到 ${anchors.value.length} 条审计锚点`);
  }

  async function fetchCredentialStatuses(): Promise<void> {
    clearErrorGroup(credentialStatusErrors);
    if (credentialStatusForm.proposalId.trim() === '') {
      credentialStatusErrors.proposalId = 'proposal_id 不能为空';
      addNotice('error', '查询失败', credentialStatusErrors.proposalId);
      return;
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
      addNotice('error', '查询失败', result.error ?? '无法获取凭证状态');
      return;
    }

    const payload = toObject(result.data);
    const items = payload?.items;
    if (!Array.isArray(items)) {
      credentialStatuses.value = [];
      addNotice('info', '查询完成', '暂无凭证状态记录');
      return;
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

    addNotice('success', '查询完成', `获取到 ${credentialStatuses.value.length} 条凭证状态`);
  }

  watch(apiBaseUrl, (value) => {
    localStorage.setItem(baseUrlStorageKey, value.trim());
  });

  watch(adminBearerToken, (value) => {
    localStorage.setItem(adminTokenStorageKey, value.trim());
  });

  watch(selectedProposalId, (value) => {
    const id = value.trim();
    if (id === '') {
      return;
    }
    voteForm.proposalId = id;
    decryptForm.proposalId = id;
    publishForm.proposalId = id;
    resultForm.proposalId = id;
    anchorForm.proposalId = id;
    credentialStatusForm.proposalId = id;

    const proposal = hallState.proposals.find((item) => item.id === id);
    if (proposal && proposal.options.length > 0 && !proposal.options.includes(selectedVoteOption.value)) {
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

  watch([workspace, activeSection, selectedProposalId], () => {
    persistRouteToStorage();
    syncRouteToUrl();
  });

  onMounted(async () => {
    const routeFromUrl = parsePortalRoute(window.location.search);
    const routeFromStorage = parseStoredRoute(localStorage.getItem(navStorageKey));
    const effectiveRoute = window.location.search.trim() === '' && routeFromStorage ? routeFromStorage : routeFromUrl;

    workspace.value = effectiveRoute.workspace;
    activeSection.value = effectiveRoute.section;
    selectedProposalId.value = effectiveRoute.proposalId;

    routeReady = true;
    syncRouteToUrl();

    setProposalWindow('active');
    createProposalForm.optionsText = '候选人 A\n候选人 B';
    voteForm.nullifier = newNullifier();

    await checkHealth(false);
    await loadOpenProposals(false);
  });

  onUnmounted(() => {
    if (alertTimer !== undefined) {
      window.clearTimeout(alertTimer);
    }
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

    identityForm,
    useCustomPubKey,
    generatedPubKey,
    regeneratePubKey,
    currentPubKey,
    credentialLoading,
    wallet,
    walletReady,
    credentialFileInput,
    identityErrors,
    issueCredential,
    openCredentialImport,
    importCredentialFile,
    exportCredentialFile,

    voteForm,
    voteLoading,
    voteStatusLoading,
    hasVoted,
    selectedVoteOption,
    showAdvancedVoteFields,
    voteErrors,
    applySelectedOptionToBallot,
    fillBallotTemplate,
    importBallotJson,
    prepareBallotByOption,
    submitVote,
    fetchVoteStatus,
    newNullifier,

    createProposalForm,
    createProposalLoading,
    createProposalErrors,
    createProposal,
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
    latestNotice,
    noticeHistory,
    activeAlert,
    dismissActiveAlert,
  };
}
