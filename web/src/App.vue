
<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { apiCall } from './lib/api';

type SectionId = 'hall' | 'identity' | 'vote' | 'ops';
type JsonMap = Record<string, unknown>;

interface SectionMeta {
  id: SectionId;
  title: string;
  description: string;
}

interface ProposalItem {
  id: string;
  title: string;
  description: string;
  options: string[];
  status: string;
  startsAt: string;
  endsAt: string;
}

interface Notice {
  level: 'success' | 'error' | 'info';
  title: string;
  detail: string;
  at: string;
}

interface AnchorItem {
  id: string;
  anchorHash: string;
  merkleRoot: string;
  createdAt: string;
}

const sections: SectionMeta[] = [
  { id: 'hall', title: '投票大厅', description: '浏览可参与提案' },
  { id: 'identity', title: '我的身份', description: '领取匿名凭证' },
  { id: 'vote', title: '参与投票', description: '导入投票凭据并提交' },
  { id: 'ops', title: '计票与审计', description: '发布结果与审计锚点' },
];

const baseUrlStorageKey = 'anonvote.front.base_url';
const apiBaseUrl = ref(localStorage.getItem(baseUrlStorageKey) ?? 'http://localhost:8080');
watch(apiBaseUrl, (value) => localStorage.setItem(baseUrlStorageKey, value.trim()));

const activeSection = ref<SectionId>('hall');
const notices = ref<Notice[]>([]);

const healthState = reactive({ loading: false, ok: false, text: '未检测', checkedAt: '' });
const hallState = reactive({ loading: false, proposals: [] as ProposalItem[] });
const selectedProposalId = ref('');

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

const createProposalForm = reactive({ title: '', description: '', optionsText: '', startsAt: '', endsAt: '' });
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

const currentPubKey = computed(() => (useCustomPubKey.value ? identityForm.customPubKey.trim() : generatedPubKey.value));
const walletReady = computed(() => wallet.userId !== '' && wallet.userPubKey !== '' && wallet.signature !== '');
const latestNotice = computed(() => notices.value[0] ?? null);
const selectedProposal = computed(() =>
  hallState.proposals.find((item) => item.id === voteForm.proposalId.trim()) ??
  hallState.proposals.find((item) => item.id === selectedProposalId.value.trim()) ??
  null,
);

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

onMounted(async () => {
  setProposalWindow('active');
  createProposalForm.optionsText = '候选人 A\n候选人 B';
  voteForm.nullifier = newNullifier();
  await checkHealth(false);
  await loadOpenProposals(false);
});

function addNotice(level: Notice['level'], title: string, detail: string): void {
  notices.value.unshift({ level, title, detail, at: new Date().toLocaleString('zh-CN', { hour12: false }) });
  if (notices.value.length > 8) {
    notices.value.length = 8;
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
  return JSON.stringify({
    summary: summary.trim(),
    options,
  });
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

function toRfc3339(date: Date): string {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

function setProposalWindow(mode: 'active' | 'closed'): void {
  if (mode === 'active') {
    createProposalForm.startsAt = toRfc3339(new Date(Date.now() - 5 * 60 * 1000));
    createProposalForm.endsAt = toRfc3339(new Date(Date.now() + 60 * 60 * 1000));
    return;
  }
  createProposalForm.startsAt = toRfc3339(new Date(Date.now() - 2 * 60 * 60 * 1000));
  createProposalForm.endsAt = toRfc3339(new Date(Date.now() - 60 * 1000));
}
function adoptProposal(id: string, section?: SectionId): void {
  if (id.trim() === '') {
    return;
  }
  selectedProposalId.value = id.trim();
  if (section !== undefined) {
    activeSection.value = section;
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

function applySelectedOptionToBallot(): void {
  if (!selectedProposal.value || selectedProposal.value.options.length === 0) {
    addNotice('error', '填充失败', '当前提案没有可选项');
    return;
  }
  if (selectedVoteOption.value.trim() === '') {
    addNotice('error', '填充失败', '请先选择一个选项');
    return;
  }
  try {
    const current = voteForm.ballotJson.trim();
    const base = current === '' ? {} : toObject(JSON.parse(current)) ?? {};
    const payload: JsonMap = {
      ...base,
      selected_option: selectedVoteOption.value,
      encrypted_vote: textOf(base, 'encrypted_vote'),
      proof: textOf(base, 'proof'),
      nullifier: textOf(base, 'nullifier') || newNullifier(),
      public_witness: textOf(base, 'public_witness'),
    };
    voteForm.ballotJson = JSON.stringify(payload, null, 2);
    if (voteForm.nullifier.trim() === '') {
      voteForm.nullifier = textOf(payload, 'nullifier');
    }
    addNotice('success', '已写入选项', '选项已同步到投票凭据 JSON');
  } catch {
    addNotice('error', '填充失败', '当前投票凭据 JSON 不是合法格式');
  }
}

function fillBallotTemplate(): void {
  voteForm.ballotJson = JSON.stringify(
    { selected_option: selectedVoteOption.value, encrypted_vote: '', proof: '', nullifier: newNullifier(), public_witness: '' },
    null,
    2,
  );
}

function importBallotJson(): void {
  if (voteForm.ballotJson.trim() === '') {
    addNotice('error', '导入失败', '请先粘贴投票凭据 JSON');
    return;
  }
  try {
    const parsed = toObject(JSON.parse(voteForm.ballotJson));
    if (parsed === null) {
      addNotice('error', '导入失败', 'JSON 顶层必须是对象');
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
    addNotice('error', '导入失败', 'JSON 格式不合法');
  }
}

async function prepareBallotByOption(): Promise<void> {
  if (!walletReady.value) {
    addNotice('error', '生成失败', '请先领取或导入匿名凭证');
    return;
  }
  if (voteForm.proposalId.trim() === '') {
    addNotice('error', '生成失败', '请先选择 proposal_id');
    return;
  }
  if (selectedVoteOption.value.trim() === '') {
    addNotice('error', '生成失败', '请先选择投票选项');
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
  voteForm.nullifier = textOf(payload, 'nullifier');
  const option = textOf(payload, 'option');
  if (option !== '') {
    selectedVoteOption.value = option;
  }
  voteForm.ballotJson = JSON.stringify(
    {
      selected_option: selectedVoteOption.value,
      encrypted_vote: voteForm.encryptedVote,
      proof: voteForm.proof,
      nullifier: voteForm.nullifier,
      public_witness: voteForm.publicWitness,
    },
    null,
    2,
  );
  addNotice('success', '生成成功', '已根据选项生成可提交的投票凭据');
}

async function checkHealth(showNotice = true): Promise<void> {
  healthState.loading = true;
  const result = await apiCall({ baseUrl: apiBaseUrl.value, path: '/healthz' });
  healthState.loading = false;
  healthState.checkedAt = new Date().toLocaleString('zh-CN', { hour12: false });
  if (result.ok) {
    healthState.ok = true;
    healthState.text = '服务在线';
    if (showNotice) addNotice('success', '连接成功', '后端服务可访问');
    return;
  }
  healthState.ok = false;
  healthState.text = '服务不可用';
  if (showNotice) addNotice('error', '连接失败', result.error ?? '无法访问后端服务');
}

async function loadOpenProposals(showNotice = true): Promise<void> {
  hallState.loading = true;
  const result = await apiCall({ baseUrl: apiBaseUrl.value, path: '/api/v1/proposals/open' });
  hallState.loading = false;
  if (!result.ok) {
    hallState.proposals = [];
    if (showNotice) addNotice('error', '加载失败', result.error ?? '无法获取提案');
    return;
  }
  const payload = toObject(result.data);
  const items = payload?.items;
  if (!Array.isArray(items)) {
    hallState.proposals = [];
    if (showNotice) addNotice('info', '暂无提案', '当前没有可参与提案');
    return;
  }
  hallState.proposals = items
    .map((item): ProposalItem | null => {
      const object = toObject(item);
      if (object === null) return null;
      const id = textOf(object, 'id');
      if (id === '') return null;
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
  if (selectedProposalId.value === '' && hallState.proposals.length > 0) adoptProposal(hallState.proposals[0].id);
  if (showNotice) addNotice('success', '提案已刷新', `共 ${hallState.proposals.length} 个提案`);
}

async function issueCredential(): Promise<void> {
  if (identityForm.userId.trim() === '') {
    addNotice('error', '签发失败', '请输入 user_id');
    return;
  }
  if (currentPubKey.value === '') {
    addNotice('error', '签发失败', '公钥不能为空');
    return;
  }
  credentialLoading.value = true;
  const result = await apiCall({
    baseUrl: apiBaseUrl.value,
    path: '/api/v1/credentials',
    method: 'POST',
    body: { user_id: identityForm.userId.trim(), user_pub_key: currentPubKey.value },
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
  if (!walletReady.value) {
    addNotice('error', '提交失败', '请先领取凭证');
    return;
  }
  if (voteForm.proposalId.trim() === '') {
    addNotice('error', '提交失败', '请选择 proposal_id');
    return;
  }
  if (voteForm.encryptedVote.trim() === '' || voteForm.proof.trim() === '' || voteForm.nullifier.trim() === '') {
    addNotice('error', '提交失败', '请导入完整投票凭据');
    return;
  }
  if (selectedProposal.value && selectedProposal.value.options.length > 0 && selectedVoteOption.value.trim() === '') {
    addNotice('error', '提交失败', '请选择一个投票选项');
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
  if (voteForm.publicWitness.trim() !== '') body.public_witness = voteForm.publicWitness.trim();
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
    if (showNotice) addNotice('error', '查询失败', result.error ?? '无法查询状态');
    return;
  }
  const payload = toObject(result.data);
  hasVoted.value = boolOf(payload, 'has_voted');
  if (showNotice) addNotice('success', '查询完成', hasVoted.value ? '你已投票' : '你还未投票');
}

async function createProposal(): Promise<void> {
  if (
    createProposalForm.title.trim() === '' ||
    createProposalForm.startsAt.trim() === '' ||
    createProposalForm.endsAt.trim() === ''
  ) {
    addNotice('error', '创建失败', '请填写标题和时间');
    return;
  }
  const options = normalizeOptions(createProposalForm.optionsText);
  if (options.length < 2) {
    addNotice('error', '创建失败', '请至少提供 2 个投票选项');
    return;
  }
  createProposalLoading.value = true;
  const result = await apiCall({
    baseUrl: apiBaseUrl.value,
    path: '/api/v1/proposals',
    method: 'POST',
    body: {
      title: createProposalForm.title.trim(),
      description: encodeProposalDescription(createProposalForm.description, options),
      starts_at: createProposalForm.startsAt.trim(),
      ends_at: createProposalForm.endsAt.trim(),
    },
  });
  createProposalLoading.value = false;
  if (!result.ok) {
    addNotice('error', '创建失败', result.error ?? '提案创建失败');
    return;
  }
  const payload = toObject(result.data);
  const id = textOf(payload, 'id');
  if (id !== '') adoptProposal(id);
  addNotice('success', '提案创建成功', id === '' ? '已创建提案' : `提案编号：${id}`);
  await loadOpenProposals(false);
}

async function setupDecryptionSession(): Promise<void> {
  if (decryptForm.proposalId.trim() === '') {
    addNotice('error', '初始化失败', 'proposal_id 不能为空');
    return;
  }
  decryptLoading.value = true;
  const result = await apiCall({
    baseUrl: apiBaseUrl.value,
    path: `/api/v1/proposals/${encodeURIComponent(decryptForm.proposalId.trim())}/decryption/setup`,
    method: 'POST',
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
  if (publishForm.proposalId.trim() === '') {
    addNotice('error', '发布失败', 'proposal_id 不能为空');
    return;
  }
  const shares = splitShares(publishForm.sharesText);
  if (shares.length === 0) {
    addNotice('error', '发布失败', '至少提供一份 share');
    return;
  }
  publishLoading.value = true;
  const result = await apiCall({
    baseUrl: apiBaseUrl.value,
    path: `/api/v1/proposals/${encodeURIComponent(publishForm.proposalId.trim())}/results/publish`,
    method: 'POST',
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
  if (resultForm.proposalId.trim() === '') {
    addNotice('error', '查询失败', 'proposal_id 不能为空');
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
  if (anchorForm.proposalId.trim() === '') {
    addNotice('error', '查询失败', 'proposal_id 不能为空');
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
      if (object === null) return null;
      const id = textOf(object, 'id');
      if (id === '') return null;
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
</script>
<template>
  <div class="app">
    <header class="hero">
      <div>
        <p class="tagline">Anonymous Vote Portal</p>
        <h1>匿名投票门户</h1>
        <p class="intro">面向用户与运营人员的业务页面，不再展示调试面板。</p>
      </div>
      <div class="hero-side">
        <label class="label">
          后端地址
          <input v-model.trim="apiBaseUrl" placeholder="http://localhost:8080" />
        </label>
        <button class="secondary" type="button" :disabled="healthState.loading" @click="checkHealth()">
          {{ healthState.loading ? '检测中...' : '检测服务连接' }}
        </button>
        <p :class="['health', healthState.ok ? 'ok' : 'bad']">
          {{ healthState.text }} <span>{{ healthState.checkedAt }}</span>
        </p>
      </div>
    </header>

    <nav class="tabs">
      <button v-for="item in sections" :key="item.id" :class="{ active: activeSection === item.id }" @click="activeSection = item.id">
        <strong>{{ item.title }}</strong>
        <small>{{ item.description }}</small>
      </button>
    </nav>

    <main class="layout">
      <section v-if="activeSection === 'hall'" class="card">
        <header class="head">
          <div>
            <h2>投票大厅</h2>
            <p>从这里选择提案并进入投票流程。</p>
          </div>
          <button class="secondary" :disabled="hallState.loading" @click="loadOpenProposals()">
            {{ hallState.loading ? '刷新中...' : '刷新提案' }}
          </button>
        </header>
        <div v-if="hallState.proposals.length === 0" class="empty">暂无开放提案</div>
        <div v-else class="proposals">
          <article v-for="item in hallState.proposals" :key="item.id" class="proposal">
            <h3>{{ item.title || '未命名提案' }}</h3>
            <p class="desc">{{ item.description || '暂无描述' }}</p>
            <ul v-if="item.options.length > 0" class="options-list">
              <li v-for="option in item.options" :key="option">{{ option }}</li>
            </ul>
            <p class="meta">状态：{{ item.status || '-' }}</p>
            <p class="meta">开始：{{ item.startsAt || '-' }}</p>
            <p class="meta">结束：{{ item.endsAt || '-' }}</p>
            <div class="row">
              <button @click="adoptProposal(item.id, 'vote')">参与投票</button>
              <button class="secondary" @click="adoptProposal(item.id, 'ops')">计票管理</button>
            </div>
          </article>
        </div>
      </section>

      <section v-else-if="activeSection === 'identity'" class="card">
        <header class="head">
          <div>
            <h2>我的身份</h2>
            <p>先领取匿名凭证，再去投票。</p>
          </div>
        </header>
        <form class="form" @submit.prevent="issueCredential">
          <label class="label">
            用户标识（user_id）
            <input v-model.trim="identityForm.userId" placeholder="例如 u-1001" />
          </label>
          <label class="switch">
            <input v-model="useCustomPubKey" type="checkbox" /> 手动输入公钥（高级）
          </label>
          <label v-if="useCustomPubKey" class="label">
            公钥（Base64）
            <textarea v-model.trim="identityForm.customPubKey" rows="3" />
          </label>
          <div v-else class="box">
            <p>已自动生成设备公钥：</p>
            <code>{{ currentPubKey }}</code>
            <button class="secondary" type="button" @click="generatedPubKey = randomBase64(32)">重新生成</button>
          </div>
          <div class="row">
            <button class="secondary" type="button" @click="openCredentialImport">导入凭证文件</button>
            <button class="secondary" type="button" @click="exportCredentialFile">导出凭证文件</button>
          </div>
          <input ref="credentialFileInput" class="hidden-input" type="file" accept="application/json" @change="importCredentialFile" />
          <button :disabled="credentialLoading">{{ credentialLoading ? '签发中...' : '领取匿名凭证' }}</button>
        </form>
        <article class="wallet" :class="{ ready: walletReady }">
          <h3>凭证包</h3>
          <p v-if="!walletReady">尚未领取凭证</p>
          <div v-else>
            <p>user_id: {{ wallet.userId }}</p>
            <p>issuer_key_version: {{ wallet.issuerKeyVersion || '-' }}</p>
            <p>issued_at: {{ wallet.issuedAt || '-' }}</p>
            <p class="mono">user_pub_key: {{ wallet.userPubKey }}</p>
            <p class="mono">signature: {{ wallet.signature }}</p>
          </div>
        </article>
      </section>

      <section v-else-if="activeSection === 'vote'" class="card">
        <header class="head">
          <div>
            <h2>参与投票</h2>
            <p>导入钱包生成的投票凭据，然后提交匿名选票。</p>
          </div>
          <button class="secondary" :disabled="hallState.loading" @click="loadOpenProposals(false)">同步提案</button>
        </header>
        <form class="form" @submit.prevent="submitVote">
          <label class="label">
            proposal_id
            <input v-model.trim="voteForm.proposalId" />
          </label>
          <fieldset v-if="selectedProposal && selectedProposal.options.length > 0" class="option-box">
            <legend>选择投票选项</legend>
            <label v-for="option in selectedProposal.options" :key="option" class="option-item">
              <input v-model="selectedVoteOption" :value="option" type="radio" />
              <span>{{ option }}</span>
            </label>
          </fieldset>
          <div v-if="hallState.proposals.length > 0" class="chip-row">
            <button v-for="item in hallState.proposals" :key="item.id" class="chip" type="button" @click="adoptProposal(item.id)">
              {{ item.title || item.id }}
            </button>
          </div>
          <label class="label">
            投票凭据 JSON
            <textarea
              v-model.trim="voteForm.ballotJson"
              rows="5"
              placeholder='{"encrypted_vote":"...","proof":"...","nullifier":"...","public_witness":"..."}'
            />
          </label>
          <div class="row">
            <button class="secondary" type="button" :disabled="voteLoading || !walletReady" @click="prepareBallotByOption">
              {{ voteLoading ? '生成中...' : '按选项生成投票凭据' }}
            </button>
            <button class="secondary" type="button" @click="importBallotJson">导入凭据</button>
            <button class="secondary" type="button" @click="fillBallotTemplate">填模板</button>
            <button class="secondary" type="button" @click="applySelectedOptionToBallot">写入选项</button>
            <button class="secondary" type="button" @click="voteForm.nullifier = newNullifier()">新 nullifier</button>
          </div>
          <details>
            <summary>高级字段（一般由钱包自动填充）</summary>
            <label class="label">encrypted_vote<textarea v-model.trim="voteForm.encryptedVote" rows="2" /></label>
            <label class="label">proof<textarea v-model.trim="voteForm.proof" rows="2" /></label>
            <label class="label">nullifier<input v-model.trim="voteForm.nullifier" /></label>
            <label class="label">public_witness（可选）<textarea v-model.trim="voteForm.publicWitness" rows="2" /></label>
          </details>
          <button :disabled="voteLoading || !walletReady">{{ voteLoading ? '提交中...' : '提交匿名选票' }}</button>
        </form>
        <div class="status">
          <p>当前提案：{{ voteForm.proposalId || '-' }}</p>
          <p>凭证状态：{{ walletReady ? '已就绪' : '未就绪' }}</p>
          <button class="secondary" :disabled="voteStatusLoading || !walletReady" @click="fetchVoteStatus()">
            {{ voteStatusLoading ? '查询中...' : '查询投票状态' }}
          </button>
          <p v-if="hasVoted !== null" :class="['pill', hasVoted ? 'done' : 'todo']">{{ hasVoted ? '你已投票' : '你还未投票' }}</p>
        </div>
      </section>

      <section v-else class="card">
        <header class="head">
          <div>
            <h2>计票与审计</h2>
            <p>覆盖创建提案、解密会话、发布结果、查询结果、审计锚点。</p>
          </div>
        </header>
        <div class="ops">
          <form class="panel" @submit.prevent="createProposal">
            <h3>创建提案</h3>
            <label class="label">标题<input v-model.trim="createProposalForm.title" /></label>
            <label class="label">描述<textarea v-model.trim="createProposalForm.description" rows="2" /></label>
            <label class="label">选项（每行一个）<textarea v-model.trim="createProposalForm.optionsText" rows="4" /></label>
            <label class="label">starts_at<input v-model.trim="createProposalForm.startsAt" /></label>
            <label class="label">ends_at<input v-model.trim="createProposalForm.endsAt" /></label>
            <div class="row">
              <button class="secondary" type="button" @click="setProposalWindow('active')">开放窗口</button>
              <button class="secondary" type="button" @click="setProposalWindow('closed')">结束窗口</button>
            </div>
            <button :disabled="createProposalLoading">{{ createProposalLoading ? '创建中...' : '创建提案' }}</button>
          </form>

          <form class="panel" @submit.prevent="setupDecryptionSession">
            <h3>初始化解密会话</h3>
            <label class="label">proposal_id<input v-model.trim="decryptForm.proposalId" /></label>
            <label class="label">threshold<input v-model.number="decryptForm.threshold" type="number" min="1" /></label>
            <label class="label">trustees<input v-model.number="decryptForm.trustees" type="number" min="1" /></label>
            <button :disabled="decryptLoading">{{ decryptLoading ? '初始化中...' : '初始化' }}</button>
            <p v-if="decryptPublicKey" class="mono">public_key: {{ decryptPublicKey }}</p>
          </form>

          <form class="panel" @submit.prevent="publishResult">
            <h3>发布结果</h3>
            <label class="label">proposal_id<input v-model.trim="publishForm.proposalId" /></label>
            <label class="label">shares<textarea v-model.trim="publishForm.sharesText" rows="3" /></label>
            <button :disabled="publishLoading">{{ publishLoading ? '发布中...' : '发布结果' }}</button>
            <p>{{ publishedSummary }}</p>
          </form>

          <form class="panel" @submit.prevent="fetchResult">
            <h3>查询结果</h3>
            <label class="label">proposal_id<input v-model.trim="resultForm.proposalId" /></label>
            <button :disabled="resultLoading">{{ resultLoading ? '查询中...' : '查询' }}</button>
            <p>{{ resultSummary }}</p>
            <ul v-if="resultTally.length > 0" class="tally">
              <li v-for="item in resultTally" :key="item.option"><span>{{ item.option }}</span><strong>{{ item.count }}</strong></li>
            </ul>
          </form>

          <form class="panel" @submit.prevent="fetchAnchors">
            <h3>审计锚点</h3>
            <label class="label">proposal_id<input v-model.trim="anchorForm.proposalId" /></label>
            <button :disabled="anchorLoading">{{ anchorLoading ? '查询中...' : '查询锚点' }}</button>
            <div v-if="anchors.length > 0" class="anchors">
              <article v-for="item in anchors" :key="item.id">
                <p><strong>{{ item.id }}</strong> · {{ item.createdAt }}</p>
                <p class="mono">anchor_hash: {{ item.anchorHash }}</p>
                <p class="mono">merkle_root: {{ item.merkleRoot }}</p>
              </article>
            </div>
          </form>
        </div>
      </section>

      <aside class="card side">
        <h2>最新动态</h2>
        <article v-if="latestNotice" :class="['notice', latestNotice.level]">
          <h3>{{ latestNotice.title }}</h3>
          <p>{{ latestNotice.detail }}</p>
          <time>{{ latestNotice.at }}</time>
        </article>
        <p v-if="notices.length === 0">暂无动态</p>
        <div class="history">
          <article v-for="item in notices.slice(1)" :key="`${item.at}-${item.title}`" :class="['notice', item.level]">
            <h3>{{ item.title }}</h3>
            <p>{{ item.detail }}</p>
            <time>{{ item.at }}</time>
          </article>
        </div>
      </aside>
    </main>
  </div>
</template>

<style scoped>
.app {
  width: min(1220px, 100%);
  margin: 0 auto;
  padding: 20px 14px 34px;
  display: grid;
  gap: 12px;
}

.hero {
  border: 1px solid var(--line);
  border-radius: 16px;
  background: linear-gradient(120deg, #214b5b, #39706e);
  color: #eff7f7;
  padding: 16px;
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: 14px;
}

.tagline {
  margin: 0 0 6px;
  font-size: 0.74rem;
  text-transform: uppercase;
  letter-spacing: 0.14em;
}

h1,
h2,
h3 {
  margin: 0;
  font-family: 'Rockwell', 'Times New Roman', serif;
}

.intro {
  margin: 8px 0 0;
}

.hero-side {
  display: grid;
  gap: 8px;
  align-content: start;
}

.health {
  margin: 0;
  font-size: 0.82rem;
}

.health.ok {
  color: #b6ffd8;
}

.health.bad {
  color: #ffd0c0;
}

.tabs {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.tabs button {
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px;
  text-align: left;
  display: grid;
  gap: 4px;
  background: var(--panel);
}

.tabs button.active {
  border-color: var(--brand);
  box-shadow: 0 8px 16px rgba(26, 95, 107, 0.15);
}

.tabs small {
  color: var(--text-soft);
}

.layout {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 12px;
}

.card {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 12px;
  background: var(--panel);
}

.head {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: start;
}

.head p {
  margin: 6px 0 0;
  color: var(--text-soft);
}

.proposals {
  margin-top: 10px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 10px;
}

.proposal {
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 10px;
  display: grid;
  gap: 5px;
  background: #f8f8f3;
}

.desc,
.meta {
  margin: 0;
  color: var(--text-soft);
  font-size: 0.84rem;
}

.options-list {
  margin: 2px 0 0;
  padding-left: 18px;
  color: var(--text-main);
  font-size: 0.84rem;
  display: grid;
  gap: 3px;
}

.empty {
  margin-top: 10px;
  border: 1px dashed var(--line-strong);
  border-radius: 10px;
  padding: 12px;
  color: var(--text-soft);
}

.form {
  margin-top: 10px;
  display: grid;
  gap: 9px;
}

.label {
  display: grid;
  gap: 6px;
  color: var(--text-soft);
  font-size: 0.84rem;
}

.switch {
  color: var(--text-soft);
  font-size: 0.84rem;
  display: flex;
  gap: 8px;
  align-items: center;
}

.box {
  border: 1px dashed var(--line-strong);
  border-radius: 10px;
  padding: 10px;
  display: grid;
  gap: 8px;
}

.box p {
  margin: 0;
  color: var(--text-soft);
}

.box code {
  font-family: 'Consolas', 'Courier New', monospace;
  word-break: break-all;
  font-size: 0.78rem;
}

.wallet {
  margin-top: 10px;
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px;
  background: #f8f8f3;
}

.wallet.ready {
  border-color: rgba(73, 131, 105, 0.45);
}

.wallet p {
  margin: 0 0 5px;
}

.chip-row {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.option-box {
  border: 1px dashed var(--line-strong);
  border-radius: 10px;
  padding: 8px 10px;
  margin: 0;
  display: grid;
  gap: 6px;
}

.option-box legend {
  color: var(--text-soft);
  font-size: 0.82rem;
  padding: 0 4px;
}

.option-item {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-main);
}

.chip {
  border: 1px solid var(--line-strong);
  background: #f3f5eb;
  color: var(--text-main);
}

.status {
  margin-top: 10px;
  border-top: 1px dashed var(--line-strong);
  padding-top: 10px;
  display: grid;
  gap: 7px;
}

.status p {
  margin: 0;
}

.pill {
  justify-self: start;
  border-radius: 999px;
  padding: 3px 9px;
  font-size: 0.8rem;
}

.pill.done {
  border: 1px solid rgba(73, 131, 105, 0.5);
  color: #2f6f51;
}

.pill.todo {
  border: 1px solid var(--line-strong);
  color: var(--text-soft);
}

.ops {
  margin-top: 10px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 10px;
}

.panel {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: #f8f8f3;
  padding: 10px;
  display: grid;
  gap: 8px;
}

.panel p {
  margin: 0;
  color: var(--text-soft);
  font-size: 0.82rem;
}

.tally {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 4px;
}

.tally li {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 4px 7px;
  display: flex;
  justify-content: space-between;
}

.anchors {
  display: grid;
  gap: 7px;
}

.anchors article {
  border: 1px dashed var(--line-strong);
  border-radius: 8px;
  padding: 7px;
}

.anchors p {
  margin: 0 0 3px;
}

.side {
  display: grid;
  gap: 8px;
  align-content: start;
  max-height: 82vh;
  overflow: auto;
}

.history {
  display: grid;
  gap: 7px;
}

.notice {
  border: 1px solid var(--line);
  border-radius: 9px;
  padding: 8px;
  background: #f8f8f3;
  display: grid;
  gap: 4px;
}

.notice h3 {
  font-size: 0.92rem;
}

.notice p {
  margin: 0;
  color: var(--text-soft);
  font-size: 0.82rem;
}

.notice time {
  font-size: 0.74rem;
  color: var(--text-soft);
}

.notice.success {
  border-color: rgba(73, 131, 105, 0.35);
}

.notice.error {
  border-color: rgba(188, 79, 64, 0.35);
}

.notice.info {
  border-color: rgba(92, 126, 153, 0.35);
}

input,
textarea {
  width: 100%;
  border: 1px solid var(--line);
  border-radius: 9px;
  padding: 9px 10px;
  color: var(--text-main);
  font: inherit;
  background: #f8f8f3;
}

input:focus,
textarea:focus {
  outline: none;
  border-color: var(--brand);
}

button {
  border: none;
  border-radius: 9px;
  padding: 9px 12px;
  background: linear-gradient(130deg, var(--brand), var(--brand-deep));
  color: #f8fcfc;
  font-weight: 700;
  cursor: pointer;
}

button.secondary {
  background: transparent;
  border: 1px solid var(--line-strong);
  color: var(--text-main);
}

button:disabled {
  opacity: 0.62;
  cursor: not-allowed;
}

.row {
  display: grid;
  gap: 7px;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
}

.mono {
  font-family: 'Consolas', 'Courier New', monospace;
  word-break: break-all;
}

.hidden-input {
  display: none;
}

@media (max-width: 1140px) {
  .layout {
    grid-template-columns: 1fr;
  }

  .side {
    max-height: none;
  }

  .tabs {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 860px) {
  .hero {
    grid-template-columns: 1fr;
  }
}
</style>
