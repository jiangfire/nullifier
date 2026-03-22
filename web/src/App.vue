<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue';
import { useVotePortal } from './composables/useVotePortal';
import type { SectionId, WorkspaceId } from './types/portal';

const portalState = useVotePortal();
const portal = reactive(portalState);
const credentialFileInput = portalState.credentialFileInput;
const showVoteTools = ref(false);

type OpsStepId =
  | 'create'
  | 'decrypt'
  | 'publish'
  | 'result'
  | 'credential'
  | 'anchor';
type GuideJourneyId = 'user' | 'admin';
type PrimaryNavId = 'overview' | 'setup' | 'guide' | 'userflow' | 'adminflow';
type GuideJourneyStep = {
  key: string;
  title: string;
  detail: string;
  workspace: WorkspaceId;
  section: SectionId;
  opsStep?: OpsStepId;
  anchor: string;
};

const opsStep = ref<OpsStepId>('create');
const opsSteps: Array<{
  id: OpsStepId;
  order: number;
  title: string;
  description: string;
}> = [
  {
    id: 'create',
    order: 1,
    title: '创建提案',
    description: '定义标题、选项和时间窗口',
  },
  {
    id: 'decrypt',
    order: 2,
    title: '初始化解密',
    description: '生成门限会话和份额',
  },
  {
    id: 'publish',
    order: 3,
    title: '发布结果',
    description: '提交 shares 并上链锚定',
  },
  {
    id: 'result',
    order: 4,
    title: '查询结果',
    description: '查看 tally 与总票数',
  },
  {
    id: 'credential',
    order: 5,
    title: '凭证状态',
    description: '查看哪些凭证已投票',
  },
  {
    id: 'anchor',
    order: 6,
    title: '审计锚点',
    description: '核验 anchor_hash 与 merkle_root',
  },
];
const userGuideSteps: GuideJourneyStep[] = [
  {
    key: 'user-hall',
    title: '先选一场投票',
    detail: '在投票大厅里选中你要参与的提案，系统会自动带上 proposal_id。',
    workspace: 'user',
    section: 'hall',
    anchor: 'guide-anchor-user-hall',
  },
  {
    key: 'user-identity',
    title: '领取匿名凭证',
    detail:
      '进入“我的身份”，为当前 proposal_id 领取匿名凭证。这是投票资格，不是明文实名票据。',
    workspace: 'user',
    section: 'identity',
    anchor: 'guide-anchor-user-identity',
  },
  {
    key: 'user-vote-prepare',
    title: '生成投票材料',
    detail:
      '选择候选项后直接点击“按选项生成凭据”。正常情况下不需要手写 proof 和 encrypted_vote。',
    workspace: 'user',
    section: 'vote',
    anchor: 'guide-anchor-user-vote-prepare',
  },
  {
    key: 'user-vote-submit',
    title: '提交匿名选票',
    detail: '确认材料已经生成，再提交匿名选票。提交成功后这轮用户引导就结束。',
    workspace: 'user',
    section: 'vote',
    anchor: 'guide-anchor-user-vote-submit',
  },
];
const adminGuideSteps: GuideJourneyStep[] = [
  {
    key: 'admin-create',
    title: '先创建未开始的提案',
    detail:
      '开始时间必须放在未来，先让系统生成 proposal_id，再继续后面的初始化解密。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'create',
    anchor: 'guide-anchor-admin-create',
  },
  {
    key: 'admin-decrypt',
    title: '初始化解密会话',
    detail:
      '投票开始前先完成这一项。它会生成 public_key 和 shares，后者留到最终发布结果时再提交。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'decrypt',
    anchor: 'guide-anchor-admin-decrypt',
  },
  {
    key: 'admin-publish',
    title: '投票结束后发布结果',
    detail:
      '这里提交的 shares 就是上一步生成的解密份额。只有数量够 threshold 才能成功出结果。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'publish',
    anchor: 'guide-anchor-admin-publish',
  },
  {
    key: 'admin-result',
    title: '查看最终票数',
    detail: '结果发布成功后，在这里查询 tally 和总票数，确认整轮计票已完成。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'result',
    anchor: 'guide-anchor-admin-result',
  },
];
const guidedPath = ref<GuideJourneyId | null>(null);
const guidedStepIndex = ref(0);
const guidedSteps = computed(() =>
  guidedPath.value === 'admin'
    ? adminGuideSteps
    : guidedPath.value === 'user'
      ? userGuideSteps
      : [],
);
const currentGuideStep = computed(
  () => guidedSteps.value[guidedStepIndex.value] ?? null,
);
const guideProgressLabel = computed(() =>
  currentGuideStep.value
    ? `${guidedStepIndex.value + 1} / ${guidedSteps.value.length}`
    : '',
);
const primaryNavItems: Array<{
  id: PrimaryNavId;
  title: string;
  description: string;
}> = [
  { id: 'overview', title: '系统概览', description: '先看当前状态与入口' },
  {
    id: 'userflow',
    title: '用户投票',
    description: '选提案、领凭证、提交选票',
  },
  {
    id: 'adminflow',
    title: '运营计票',
    description: '建提案、解密、发布结果',
  },
];
const userWorkflowSections: SectionId[] = ['hall', 'identity', 'vote'];
const adminWorkflowSections: SectionId[] = ['ops'];
const workflowNavItems = computed(() =>
  (portal.workspace === 'admin' ? adminWorkflowSections : userWorkflowSections)
    .map((id) => portal.sections.find((item) => item.id === id))
    .filter(
      (item): item is NonNullable<(typeof portal.sections)[number]> =>
        item !== undefined,
    ),
);
const activePrimaryNavId = computed<PrimaryNavId>(() => {
  if (
    portal.activeSection === 'overview' ||
    portal.activeSection === 'setup' ||
    portal.activeSection === 'guide'
  ) {
    return 'overview';
  }
  return portal.workspace === 'admin' ? 'adminflow' : 'userflow';
});
const workflowHeadline = computed(() =>
  portal.workspace === 'admin'
    ? '把运营动作压缩成一条顺序清晰的计票链路'
    : '给投票人一条更像产品流程、而不是联调台的操作路径',
);
const currentOpsStepTitle = computed(
  () => opsSteps.find((item) => item.id === opsStep.value)?.title || '未选择',
);
const currentProposalLabel = computed(
  () =>
    portal.selectedProposal?.title ||
    portal.selectedProposalId ||
    '尚未选定提案',
);
const currentWorkflowTag = computed(() => {
  if (portal.workspace === 'admin') {
    const labels: Record<OpsStepId, string> = {
      create: '创建',
      decrypt: '解密',
      publish: '发布',
      result: '结果',
      credential: '凭证',
      anchor: '锚点',
    };
    return labels[opsStep.value];
  }
  const labels: Record<'hall' | 'identity' | 'vote', string> = {
    hall: '大厅',
    identity: '身份',
    vote: '投票',
  };
  return labels[portal.activeSection as 'hall' | 'identity' | 'vote'] || '流程';
});
const shortProposalTag = computed(() => {
  const raw = portal.selectedProposalId.trim();
  if (raw === '') {
    return '#-';
  }
  return `#${raw.slice(0, 4)}`;
});
const proposalCount = computed(() => portal.hallState.proposals.length);
const selectedProposalOptionCount = computed(
  () => portal.selectedProposal?.options.length || 0,
);
const connectionTone = computed(() => (portal.healthState.ok ? 'good' : 'bad'));
const adminTokenTone = computed(() =>
  portal.adminTokenReady ? 'good' : 'warn',
);
const proposalTone = computed(() =>
  portal.selectedProposalId ? 'good' : 'muted',
);

function setOpsStep(step: OpsStepId): void {
  opsStep.value = step;
}

async function syncGuideStepIntoView(): Promise<void> {
  await nextTick();
  const anchor = currentGuideStep.value?.anchor;
  if (!anchor) {
    return;
  }
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}

async function activateGuideStep(): Promise<void> {
  const step = currentGuideStep.value;
  if (!step) {
    return;
  }
  portal.setWorkspace(step.workspace);
  portal.setActiveSection(step.section);
  if (step.opsStep) {
    setOpsStep(step.opsStep);
  }
  await syncGuideStepIntoView();
}

async function beginGuideJourney(target: GuideJourneyId): Promise<void> {
  guidedPath.value = target;
  guidedStepIndex.value = 0;
  portal.startGuideJourney(target);
  if (target === 'admin') {
    setOpsStep('create');
  }
  await activateGuideStep();
}

async function goToCurrentGuideStep(): Promise<void> {
  await activateGuideStep();
}

async function nextGuideStep(): Promise<void> {
  if (!currentGuideStep.value) {
    return;
  }
  if (guidedStepIndex.value >= guidedSteps.value.length - 1) {
    stopGuideJourney();
    return;
  }
  guidedStepIndex.value += 1;
  await activateGuideStep();
}

async function previousGuideStep(): Promise<void> {
  if (!currentGuideStep.value || guidedStepIndex.value === 0) {
    return;
  }
  guidedStepIndex.value -= 1;
  await activateGuideStep();
}

function stopGuideJourney(): void {
  guidedPath.value = null;
}

function isGuideStepActive(stepKey: string): boolean {
  return currentGuideStep.value?.key === stepKey;
}

async function handleIssueCredential(): Promise<void> {
  const ok = await portal.issueCredential();
  if (ok) {
    portal.setActiveSection('vote');
    if (guidedPath.value === 'user' && isGuideStepActive('user-identity')) {
      guidedStepIndex.value = 2;
      await activateGuideStep();
    }
  }
}

async function handleCreateProposal(): Promise<void> {
  const ok = await portal.createProposal();
  if (ok) {
    setOpsStep('decrypt');
    if (guidedPath.value === 'admin' && isGuideStepActive('admin-create')) {
      guidedStepIndex.value = 1;
      await activateGuideStep();
    }
  }
}

async function handleSetupDecryptionSession(): Promise<void> {
  const ok = await portal.setupDecryptionSession();
  if (ok) {
    setOpsStep('publish');
    if (guidedPath.value === 'admin' && isGuideStepActive('admin-decrypt')) {
      guidedStepIndex.value = 2;
      await activateGuideStep();
    }
  }
}

async function handlePublishResult(): Promise<void> {
  const ok = await portal.publishResult();
  if (ok) {
    setOpsStep('result');
    if (guidedPath.value === 'admin' && isGuideStepActive('admin-publish')) {
      guidedStepIndex.value = 3;
      await activateGuideStep();
    }
  }
}

async function handleFetchResult(): Promise<void> {
  const ok = await portal.fetchResult();
  if (ok) {
    setOpsStep('credential');
    if (guidedPath.value === 'admin' && isGuideStepActive('admin-result')) {
      stopGuideJourney();
    }
  }
}

async function handleFetchCredentialStatuses(): Promise<void> {
  const ok = await portal.fetchCredentialStatuses();
  if (ok) {
    setOpsStep('anchor');
  }
}

function handleOpenGuideMode(): void {
  portal.openGuideMode();
}

async function startUserGuideJourney(): Promise<void> {
  await beginGuideJourney('user');
}

async function startAdminGuideJourney(): Promise<void> {
  await beginGuideJourney('admin');
}

async function handlePrepareBallotByOption(): Promise<void> {
  const ok = await portal.prepareBallotByOption();
  if (
    ok &&
    guidedPath.value === 'user' &&
    isGuideStepActive('user-vote-prepare')
  ) {
    guidedStepIndex.value = 3;
    await activateGuideStep();
  }
}

async function handleSubmitVote(): Promise<void> {
  const ok = await portal.submitVote();
  if (
    ok &&
    guidedPath.value === 'user' &&
    isGuideStepActive('user-vote-submit')
  ) {
    stopGuideJourney();
  }
}

function openVoteForProposal(id: string): void {
  if (guidedPath.value === 'user' && isGuideStepActive('user-hall')) {
    portal.adoptProposal(id, 'identity');
    guidedStepIndex.value = 1;
    void activateGuideStep();
    return;
  }
  portal.adoptProposal(id, 'vote');
}

function openOpsForProposal(id: string): void {
  portal.adoptProposal(id, 'ops');
  opsStep.value = 'decrypt';
}

function openPrimarySection(target: PrimaryNavId): void {
  if (target === 'overview' || target === 'setup' || target === 'guide') {
    portal.setActiveSection(target);
    return;
  }
  if (target === 'adminflow') {
    portal.setWorkspace('admin');
    portal.setActiveSection('ops');
    return;
  }
  portal.setWorkspace('user');
  portal.setActiveSection('hall');
}

function openWorkflowSection(target: SectionId): void {
  if (target === 'ops') {
    portal.setWorkspace('admin');
  } else if (target === 'hall' || target === 'identity' || target === 'vote') {
    portal.setWorkspace('user');
  }
  portal.setActiveSection(target);
}

function handleTabKeydown(
  event: KeyboardEvent,
  ids: string[],
  currentId: string,
  onSelect: (id: string) => void,
  prefix: string,
): void {
  const supported = [
    'ArrowRight',
    'ArrowDown',
    'ArrowLeft',
    'ArrowUp',
    'Home',
    'End',
  ];
  if (!supported.includes(event.key) || ids.length === 0) {
    return;
  }
  event.preventDefault();

  const currentIndex = Math.max(0, ids.indexOf(currentId));

  let targetIndex = currentIndex;
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    targetIndex = (currentIndex + 1) % ids.length;
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    targetIndex = (currentIndex - 1 + ids.length) % ids.length;
  } else if (event.key === 'Home') {
    targetIndex = 0;
  } else if (event.key === 'End') {
    targetIndex = ids.length - 1;
  }

  const nextId = ids[targetIndex];
  if (!nextId) {
    return;
  }

  onSelect(nextId);

  requestAnimationFrame(() => {
    const target = document.getElementById(`${prefix}${nextId}`);
    if (target instanceof HTMLButtonElement) {
      target.focus();
    }
  });
}
</script>

<template>
  <a class="skip-link" href="#main-content">跳转到主内容</a>
  <div
    v-if="portal.guideModeVisible"
    class="guide-overlay"
    role="dialog"
    aria-modal="true"
    aria-labelledby="guide-mode-title"
  >
    <section class="guide-dialog">
      <p class="guide-dialog-kicker">First Run</p>
      <h2 id="guide-mode-title">先别急着点，按引导走会顺很多</h2>
      <p class="guide-dialog-copy">
        这个系统分成“用户投票”和“运营计票”两条路径。最常见的错误，是管理员把投票先开了，结果还没来得及初始化解密。
      </p>

      <div class="guide-dialog-grid">
        <article class="guide-dialog-card">
          <strong>我是投票人</strong>
          <p>
            系统会带你按“选提案 → 领凭证 → 生成投票材料 → 提交选票”的顺序走。
          </p>
          <button class="btn" type="button" @click="startUserGuideJourney">
            从用户流程开始
          </button>
        </article>
        <article class="guide-dialog-card accent">
          <strong>我是管理员</strong>
          <p>
            系统会带你按“建提案 → 初始化解密 → 发凭证 →
            结束后发布结果”的顺序走。
          </p>
          <button
            class="btn btn-secondary"
            type="button"
            @click="startAdminGuideJourney"
          >
            从运营流程开始
          </button>
        </article>
      </div>

      <div class="guide-dialog-list">
        <p>
          <strong>你只需要记住一个顺序：</strong
          >先初始化解密，再开放投票，再发布结果。
        </p>
        <p>
          <strong>`proposal_id`：</strong
          >一场投票的唯一编号，后面所有操作都靠它关联。
        </p>
        <p>
          <strong>`shares`：</strong
          >初始化解密时生成的解密份额，最终发布结果时再提交。
        </p>
      </div>

      <div class="guide-dialog-actions">
        <button
          class="btn btn-ghost"
          type="button"
          @click="portal.closeGuideMode()"
        >
          先只看教程页
        </button>
      </div>
    </section>
  </div>
  <div class="message-stack" aria-live="polite" aria-label="全局消息">
    <article
      v-for="item in portal.notices"
      :key="item.id"
      class="message-card"
      :class="item.level"
      role="status"
    >
      <div class="message-head">
        <strong>{{ item.title }}</strong>
        <button
          class="message-close"
          type="button"
          @click="portal.dismissNotice(item.id)"
        >
          关闭
        </button>
      </div>
      <p>{{ item.detail }}</p>
      <time>{{ item.at }}</time>
    </article>
  </div>
  <aside v-if="currentGuideStep" class="journey-panel" aria-live="polite">
    <div class="journey-head">
      <p class="journey-kicker">
        {{ guidedPath === 'admin' ? '管理员引导' : '用户引导' }}
      </p>
      <button class="message-close" type="button" @click="stopGuideJourney">
        关闭
      </button>
    </div>
    <strong>{{ currentGuideStep.title }}</strong>
    <p>{{ currentGuideStep.detail }}</p>
    <div class="journey-progress">
      <span>当前进度 {{ guideProgressLabel }}</span>
      <span>{{
        currentGuideStep.workspace === 'admin' ? '运营路径' : '投票路径'
      }}</span>
    </div>
    <div class="journey-actions">
      <button
        class="btn btn-secondary"
        type="button"
        @click="goToCurrentGuideStep"
      >
        定位当前步骤
      </button>
      <button
        class="btn btn-ghost"
        type="button"
        :disabled="guidedStepIndex === 0"
        @click="previousGuideStep"
      >
        上一步
      </button>
      <button class="btn" type="button" @click="nextGuideStep">
        {{ guidedStepIndex === guidedSteps.length - 1 ? '完成引导' : '下一步' }}
      </button>
    </div>
  </aside>
  <div class="app-shell">
    <header class="shell-topbar">
      <div class="topbar-main">
        <div class="brand-block">
          <strong>匿名投票</strong>
        </div>
        <nav class="primary-nav" aria-label="一级导航">
          <button
            v-for="item in primaryNavItems"
            :id="`primary-nav-${item.id}`"
            :key="item.id"
            class="primary-nav-btn"
            :class="{ active: activePrimaryNavId === item.id }"
            :aria-current="activePrimaryNavId === item.id ? 'page' : undefined"
            @click="openPrimarySection(item.id)"
            @keydown="
              handleTabKeydown(
                $event,
                primaryNavItems.map((nav) => nav.id),
                item.id,
                (id) => openPrimarySection(id as PrimaryNavId),
                'primary-nav-',
              )
            "
          >
            <strong>{{ item.title }}</strong>
          </button>
        </nav>
        <div class="topbar-actions">
          <button
            class="topbar-link"
            :class="{ active: portal.activeSection === 'setup' }"
            type="button"
            title="打开连接配置"
            @click="openPrimarySection('setup')"
          >
            配置
          </button>
          <button
            class="topbar-link"
            :class="{ active: portal.activeSection === 'guide' }"
            type="button"
            title="打开使用教程"
            @click="openPrimarySection('guide')"
          >
            教程
          </button>
          <button
            class="topbar-link topbar-link-icon"
            type="button"
            title="打开新手引导"
            @click="handleOpenGuideMode"
          >
            ?
          </button>
        </div>
      </div>

      <div class="topbar-status-strip" aria-label="当前状态">
        <span class="topbar-chip">
          {{ portal.workspace === 'admin' ? '运营' : '投票' }}
        </span>
        <span class="topbar-chip topbar-chip-subtle">
          {{ currentWorkflowTag }}
        </span>
        <div class="topbar-dots" aria-label="系统状态">
          <span
            class="topbar-dot"
            :class="connectionTone"
            :title="portal.healthState.ok ? '服务在线' : '服务待检测'"
            :aria-label="portal.healthState.ok ? '服务在线' : '服务待检测'"
          ></span>
          <span
            class="topbar-dot"
            :class="adminTokenTone"
            :title="
              portal.adminTokenReady
                ? 'Admin Token 已配置'
                : 'Admin Token 未配置'
            "
            :aria-label="
              portal.adminTokenReady
                ? 'Admin Token 已配置'
                : 'Admin Token 未配置'
            "
          ></span>
          <span
            class="topbar-dot"
            :class="proposalTone"
            :title="portal.selectedProposalId || '未选择提案'"
            :aria-label="portal.selectedProposalId || '未选择提案'"
          ></span>
          <span
            class="topbar-mini-code"
            :class="{ empty: !portal.selectedProposalId }"
            :title="portal.selectedProposalId || '未选择提案'"
          >
            {{ shortProposalTag }}
          </span>
        </div>
      </div>
    </header>

    <main id="main-content" class="main-grid">
      <section class="main-panel" role="region" aria-label="主工作区">
        <section
          v-if="
            activePrimaryNavId === 'userflow' ||
            activePrimaryNavId === 'adminflow'
          "
          class="workspace-shell"
        >
          <nav
            class="workflow-tabs"
            :aria-label="
              portal.workspace === 'admin' ? '运营流程导航' : '用户流程导航'
            "
          >
            <button
              v-for="item in workflowNavItems"
              :id="`workflow-tab-${item.id}`"
              :key="item.id"
              class="workflow-tab-btn"
              :class="{ active: portal.activeSection === item.id }"
              :aria-current="
                portal.activeSection === item.id ? 'page' : undefined
              "
              @click="openWorkflowSection(item.id)"
              @keydown="
                handleTabKeydown(
                  $event,
                  workflowNavItems.map((entry) => entry.id),
                  item.id,
                  (id) => openWorkflowSection(id as SectionId),
                  'workflow-tab-',
                )
              "
            >
              <strong>{{ item.title }}</strong>
              <small>{{ item.description }}</small>
            </button>
          </nav>
        </section>

        <section
          v-if="portal.activeSection === 'overview'"
          id="section-panel-overview"
          class="panel overview-panel"
          role="tabpanel"
          aria-labelledby="primary-nav-overview"
        >
          <header class="panel-head">
            <div>
              <h2>系统概览</h2>
              <p class="panel-tip">先看状态、上下文和下一步入口。</p>
            </div>
            <div class="header-actions">
              <button
                class="btn btn-secondary"
                type="button"
                @click="portal.checkHealth()"
              >
                {{ portal.healthState.loading ? '检测中...' : '重新检测服务' }}
              </button>
              <button
                class="btn btn-ghost"
                type="button"
                @click="handleOpenGuideMode"
              >
                打开引导
              </button>
            </div>
          </header>

          <div class="summary-strip summary-strip-compact">
            <article class="summary-item">
              <span>连接</span>
              <strong>{{ portal.healthState.ok ? '已连通' : '待检查' }}</strong>
            </article>
            <article class="summary-item">
              <span>提案</span>
              <strong>{{ proposalCount }} 场</strong>
            </article>
            <article class="summary-item">
              <span>凭证</span>
              <strong>{{ portal.walletReady ? '已就绪' : '未加载' }}</strong>
            </article>
            <article class="summary-item">
              <span>工作区</span>
              <strong>{{
                portal.workspace === 'admin' ? '运营端' : '用户端'
              }}</strong>
            </article>
          </div>

          <div class="overview-grid">
            <article class="overview-card overview-card-hero">
              <p class="overview-kicker">Recommended Path</p>
              <h3>配置、理解、执行分层</h3>
              <div class="compact-checklist">
                <span>1. 先配置连接</span>
                <span>2. 选择用户或运营路径</span>
                <span>3. 进入具体工作页</span>
              </div>
              <div class="action-row">
                <button
                  class="btn"
                  type="button"
                  @click="openPrimarySection('setup')"
                >
                  先去配置
                </button>
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="openPrimarySection('userflow')"
                >
                  开始投票流程
                </button>
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="openPrimarySection('adminflow')"
                >
                  进入运营流程
                </button>
              </div>
            </article>

            <div class="overview-side">
              <article class="overview-card compact-card">
                <p class="card-label">当前上下文</p>
                <dl class="meta-grid dense">
                  <div>
                    <dt>后端地址</dt>
                    <dd>{{ portal.apiBaseUrl || '-' }}</dd>
                  </div>
                  <div>
                    <dt>管理能力</dt>
                    <dd>
                      {{
                        portal.adminTokenReady
                          ? 'Admin Token 已就绪'
                          : '缺少 Admin Token'
                      }}
                    </dd>
                  </div>
                  <div>
                    <dt>当前提案</dt>
                    <dd>{{ currentProposalLabel }}</dd>
                  </div>
                  <div>
                    <dt>当前步骤</dt>
                    <dd>
                      {{
                        portal.workspace === 'admin'
                          ? currentOpsStepTitle
                          : '用户流程'
                      }}
                    </dd>
                  </div>
                </dl>
              </article>

              <article class="overview-card compact-card">
                <p class="card-label">快速入口</p>
                <div class="quick-links">
                  <button
                    class="chip action-chip"
                    type="button"
                    @click="openPrimarySection('setup')"
                  >
                    连接配置
                  </button>
                  <button
                    class="chip action-chip"
                    type="button"
                    @click="openPrimarySection('userflow')"
                  >
                    用户投票
                  </button>
                  <button
                    class="chip action-chip"
                    type="button"
                    @click="openPrimarySection('adminflow')"
                  >
                    运营计票
                  </button>
                  <button
                    class="chip action-chip"
                    type="button"
                    @click="openPrimarySection('guide')"
                  >
                    术语教程
                  </button>
                </div>
              </article>
            </div>
          </div>

          <div class="launch-grid">
            <article class="launch-card compact-card">
              <strong>连接配置</strong>
              <p>填写地址、令牌并做健康检查。</p>
              <button
                class="btn btn-secondary"
                type="button"
                @click="openPrimarySection('setup')"
              >
                打开配置页
              </button>
            </article>
            <article class="launch-card compact-card">
              <strong>用户投票</strong>
              <p>选提案、领凭证、生成并提交选票。</p>
              <button
                class="btn btn-secondary"
                type="button"
                @click="openPrimarySection('userflow')"
              >
                进入用户流程
              </button>
            </article>
            <article class="launch-card compact-card">
              <strong>运营计票</strong>
              <p>建提案、初始化、发布结果、审计留痕。</p>
              <button
                class="btn btn-secondary"
                type="button"
                @click="openPrimarySection('adminflow')"
              >
                进入运营流程
              </button>
            </article>
            <article class="launch-card compact-card">
              <strong>使用教程</strong>
              <p>快速理解顺序、术语和高频误区。</p>
              <button
                class="btn btn-secondary"
                type="button"
                @click="openPrimarySection('guide')"
              >
                查看教程
              </button>
            </article>
          </div>
        </section>

        <section
          v-else-if="portal.activeSection === 'setup'"
          id="section-panel-setup"
          class="panel setup-panel"
          role="tabpanel"
          aria-labelledby="primary-nav-setup"
        >
          <header class="panel-head">
            <div>
              <h2>连接配置</h2>
              <p class="panel-tip">集中处理连接、令牌和联调前检查。</p>
            </div>
          </header>

          <div class="summary-strip summary-strip-compact">
            <article class="summary-item">
              <span>API</span>
              <strong>{{ portal.apiBaseUrl || '-' }}</strong>
            </article>
            <article class="summary-item">
              <span>连通性</span>
              <strong>{{ portal.healthState.ok ? '正常' : '未确认' }}</strong>
            </article>
            <article class="summary-item">
              <span>Admin Token</span>
              <strong>{{
                portal.adminTokenReady ? '已配置' : '未配置'
              }}</strong>
            </article>
          </div>

          <div class="setup-grid">
            <form
              class="setup-card compact-card"
              @submit.prevent="portal.checkHealth()"
            >
              <label class="field" for="api-base-url">
                后端地址
                <input
                  id="api-base-url"
                  v-model.trim="portal.apiBaseUrl"
                  type="url"
                  inputmode="url"
                  placeholder="http://localhost:8080"
                />
              </label>
              <label class="field" for="admin-bearer-token">
                Admin Token
                <input
                  id="admin-bearer-token"
                  v-model.trim="portal.adminBearerToken"
                  type="password"
                  autocomplete="off"
                  placeholder="用于运营端管理接口"
                />
              </label>
              <div class="action-row">
                <button
                  class="btn btn-secondary"
                  type="submit"
                  :disabled="portal.healthState.loading"
                >
                  {{
                    portal.healthState.loading ? '检测中...' : '检测服务连接'
                  }}
                </button>
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="openPrimarySection('overview')"
                >
                  返回概览
                </button>
              </div>
              <p
                class="health-line"
                :class="portal.healthState.ok ? 'ok' : 'bad'"
              >
                {{ portal.healthState.text }}
                <span
                  >最近检测：{{
                    portal.healthState.checkedAt || '尚未检测'
                  }}</span
                >
              </p>
            </form>

            <aside class="setup-side">
              <article class="overview-card compact-card">
                <p class="card-label">联调提示</p>
                <div class="compact-checklist">
                  <span>先检查服务连接</span>
                  <span>运营动作需要 Admin Token</span>
                  <span>只看教程时可暂不填写令牌</span>
                </div>
              </article>
              <article class="overview-card compact-card">
                <p class="card-label">当前可用能力</p>
                <div class="capability-list">
                  <span
                    :class="
                      portal.healthState.ok ? 'state-good' : 'state-muted'
                    "
                    >服务检测</span
                  >
                  <span
                    :class="
                      portal.adminTokenReady ? 'state-good' : 'state-warn'
                    "
                    >提案创建</span
                  >
                  <span
                    :class="
                      portal.adminTokenReady ? 'state-good' : 'state-warn'
                    "
                    >凭证签发</span
                  >
                  <span
                    :class="
                      portal.adminTokenReady ? 'state-good' : 'state-warn'
                    "
                    >结果发布</span
                  >
                </div>
              </article>
            </aside>
          </div>
        </section>

        <section
          v-else-if="portal.activeSection === 'guide'"
          id="section-panel-guide"
          class="panel guide-panel"
          role="tabpanel"
          aria-labelledby="primary-nav-guide"
        >
          <header class="panel-head">
            <div>
              <h2>使用教程</h2>
              <p class="panel-tip">先把流程和术语看明白，再去操作页面。</p>
            </div>
          </header>

          <div class="guide-hero">
            <div class="guide-hero-copy">
              <p class="guide-kicker">Quick Start</p>
              <h3>先初始化计票，再开放投票，再发布结果</h3>
              <p>
                这个系统最容易出错的地方，是把“开始投票”和“初始化解密”顺序搞反。对你来说，记住一个规则就够了：提案开始投票之前，先把计票准备完成。
              </p>
            </div>
            <div class="guide-hero-actions">
              <button
                class="btn"
                type="button"
                @click="
                  portal.setWorkspace('user');
                  portal.setActiveSection('hall');
                "
              >
                进入用户流程
              </button>
              <button
                class="btn btn-secondary"
                type="button"
                @click="
                  portal.setWorkspace('admin');
                  portal.setActiveSection('ops');
                "
              >
                进入运营流程
              </button>
              <button
                class="btn btn-ghost"
                type="button"
                @click="handleOpenGuideMode"
              >
                重新打开新手引导
              </button>
            </div>
          </div>

          <section class="guide-block">
            <header class="guide-block-head">
              <h3>用户怎么用</h3>
              <p>给普通投票人看的最短路径。</p>
            </header>
            <div class="guide-grid">
              <article class="guide-card">
                <span>1</span>
                <strong>先选提案</strong>
                <p>
                  在“投票大厅”中找到你要参与的那场投票，系统会自动带上它的
                  `proposal_id`。
                </p>
              </article>
              <article class="guide-card">
                <span>2</span>
                <strong>领取匿名凭证</strong>
                <p>
                  去“我的身份”领取当前提案的匿名凭证。它相当于本场投票专属的匿名入场券。
                </p>
              </article>
              <article class="guide-card">
                <span>3</span>
                <strong>按选项生成投票材料</strong>
                <p>
                  在“参与投票”里选择候选项，直接点生成即可，正常不需要手工填写
                  `proof` 和 `encrypted_vote`。
                </p>
              </article>
              <article class="guide-card">
                <span>4</span>
                <strong>提交并查询状态</strong>
                <p>
                  提交后可以查询自己是否已经投票，但别人不能靠裸 `user_id`
                  去枚举你的状态。
                </p>
              </article>
            </div>
          </section>

          <section class="guide-block">
            <header class="guide-block-head">
              <h3>管理员怎么用</h3>
              <p>按这个顺序走，不要跳步。</p>
            </header>
            <div class="guide-grid">
              <article class="guide-card accent">
                <span>1</span>
                <strong>创建未开始的提案</strong>
                <p>
                  先创建一场还没开始投票的提案。推荐直接点“填入推荐时间”，系统会自动把开始时间放到未来。
                </p>
              </article>
              <article class="guide-card accent">
                <span>2</span>
                <strong>初始化解密会话</strong>
                <p>
                  这一步会生成 `public_key` 和
                  `shares`。前者给投票加密用，后者留到最终计票时解密使用。
                </p>
              </article>
              <article class="guide-card accent">
                <span>3</span>
                <strong>签发提案内凭证</strong>
                <p>为参与者按提案发凭证。A 提案的凭证不能拿去投 B 提案。</p>
              </article>
              <article class="guide-card accent">
                <span>4</span>
                <strong>投票结束后再出结果</strong>
                <p>
                  等投票结束，再提交足够数量的
                  `shares`，系统就会解密并发布结果。
                </p>
              </article>
            </div>
          </section>

          <section class="guide-block">
            <header class="guide-block-head">
              <h3>术语字典</h3>
              <p>把页面里最常见的词翻成人话。</p>
            </header>
            <div class="glossary-grid">
              <article class="glossary-card">
                <strong>proposal_id</strong>
                <p>这场投票的唯一编号，后面所有操作都靠它指向同一场投票。</p>
              </article>
              <article class="glossary-card">
                <strong>user_id</strong>
                <p>
                  用户标识，系统会拿它校验资格，但不会直接把它和票内容绑定成一张明文表。
                </p>
              </article>
              <article class="glossary-card">
                <strong>nullifier</strong>
                <p>一次性防重号，用来防止同一张匿名票被重复提交。</p>
              </article>
              <article class="glossary-card">
                <strong>proof</strong>
                <p>
                  证明这张票合法的证明材料。联调时通常由系统生成，不需要自己写。
                </p>
              </article>
              <article class="glossary-card">
                <strong>encrypted_vote</strong>
                <p>
                  加密后的投票内容。服务端会保存它，但在最终计票前看不到你选了谁。
                </p>
              </article>
              <article class="glossary-card">
                <strong>threshold / trustees</strong>
                <p>
                  `threshold` 是至少需要几份，`trustees` 是总共拆成几份。比如
                  2/3 表示三份里任意两份可用。
                </p>
              </article>
              <article class="glossary-card">
                <strong>shares</strong>
                <p>解密份额。初始化解密时生成，发布结果时再提交回来。</p>
              </article>
              <article class="glossary-card">
                <strong>anchor_hash / merkle_root</strong>
                <p>结果校验指纹，主要给技术审计和留痕核验使用。</p>
              </article>
            </div>
          </section>
        </section>

        <section
          v-else-if="portal.activeSection === 'hall'"
          id="section-panel-hall"
          class="panel"
          :class="{ 'guide-focus': isGuideStepActive('user-hall') }"
          role="tabpanel"
          aria-labelledby="workflow-tab-hall"
        >
          <header class="panel-head">
            <div>
              <h2>投票大厅</h2>
              <p class="panel-tip">先选定提案，再进入凭证或投票页面。</p>
            </div>
            <button
              class="btn btn-secondary"
              type="button"
              :disabled="portal.hallState.loading"
              @click="portal.loadOpenProposals()"
            >
              {{ portal.hallState.loading ? '刷新中...' : '刷新提案' }}
            </button>
          </header>

          <div class="summary-strip summary-strip-compact">
            <article class="summary-item">
              <span>开放提案</span>
              <strong>{{ proposalCount }}</strong>
            </article>
            <article class="summary-item">
              <span>当前选中</span>
              <strong>{{ currentProposalLabel }}</strong>
            </article>
            <article class="summary-item">
              <span>凭证</span>
              <strong>{{ portal.walletReady ? '已就绪' : '未加载' }}</strong>
            </article>
          </div>

          <p v-if="portal.hallState.proposals.length === 0" class="empty-state">
            暂无开放提案
          </p>

          <div id="guide-anchor-user-hall" v-else class="proposal-grid">
            <article
              v-for="item in portal.hallState.proposals"
              :key="item.id"
              class="proposal-card"
            >
              <header class="proposal-head">
                <h3>{{ item.title || '未命名提案' }}</h3>
                <span class="status-badge">{{
                  item.status || '未知状态'
                }}</span>
              </header>
              <p class="card-desc">{{ item.description || '暂无描述' }}</p>
              <div class="proposal-quick-meta">
                <span>{{ item.options.length }} 个选项</span>
                <span>{{ item.id }}</span>
              </div>
              <div v-if="item.options.length > 0">
                <ul class="option-list compact">
                  <li v-for="option in item.options" :key="option">
                    {{ option }}
                  </li>
                </ul>
              </div>
              <dl class="meta-grid">
                <div>
                  <dt>开始</dt>
                  <dd>{{ item.startsAt || '-' }}</dd>
                </div>
                <div>
                  <dt>结束</dt>
                  <dd>{{ item.endsAt || '-' }}</dd>
                </div>
              </dl>
              <div class="action-row">
                <button
                  class="btn"
                  type="button"
                  @click="openVoteForProposal(item.id)"
                >
                  去投票
                </button>
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="openOpsForProposal(item.id)"
                >
                  计票管理
                </button>
              </div>
            </article>
          </div>
        </section>

        <section
          v-else-if="portal.activeSection === 'identity'"
          id="section-panel-identity"
          class="panel"
          :class="{ 'guide-focus': isGuideStepActive('user-identity') }"
          role="tabpanel"
          aria-labelledby="workflow-tab-identity"
        >
          <header class="panel-head">
            <div>
              <h2>我的身份</h2>
              <p class="panel-tip">
                凭证按提案发放，投票页会直接复用当前凭证。
              </p>
            </div>
          </header>

          <div class="summary-strip summary-strip-compact">
            <article class="summary-item">
              <span>proposal_id</span>
              <strong>{{ portal.voteForm.proposalId || '-' }}</strong>
            </article>
            <article class="summary-item">
              <span>签发权限</span>
              <strong>{{
                portal.adminTokenReady ? '已具备' : '缺少 Token'
              }}</strong>
            </article>
            <article class="summary-item">
              <span>凭证状态</span>
              <strong>{{ portal.walletReady ? '已加载' : '未加载' }}</strong>
            </article>
          </div>

          <div class="detail-split">
            <form
              id="guide-anchor-user-identity"
              class="form-grid"
              @submit.prevent="handleIssueCredential"
            >
              <label class="field" for="identity-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="identity-proposal-id"
                  v-model.trim="portal.voteForm.proposalId"
                  placeholder="先选择要发放凭证的提案"
                />
                <span class="field-help">这张凭证只属于当前投票。</span>
              </label>
              <label class="field" for="identity-user-id">
                用户标识（user_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="identity-user-id"
                  v-model.trim="portal.identityForm.userId"
                  placeholder="例如 u-1001"
                  :aria-invalid="
                    portal.identityErrors.userId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.identityErrors.userId
                      ? 'identity-user-id-error'
                      : undefined
                  "
                  autocomplete="username"
                />
                <span
                  v-if="portal.identityErrors.userId"
                  id="identity-user-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.identityErrors.userId }}</span
                >
              </label>

              <label class="switch-field">
                <input v-model="portal.useCustomPubKey" type="checkbox" />
                手动输入公钥（高级模式）
              </label>

              <label
                v-if="portal.useCustomPubKey"
                class="field"
                for="identity-pub-key"
              >
                公钥（Base64）<span class="required-mark" aria-hidden="true"
                  >*</span
                >
                <textarea
                  id="identity-pub-key"
                  v-model.trim="portal.identityForm.customPubKey"
                  rows="3"
                  :aria-invalid="
                    portal.identityErrors.customPubKey ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.identityErrors.customPubKey
                      ? 'identity-pub-key-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.identityErrors.customPubKey"
                  id="identity-pub-key-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.identityErrors.customPubKey }}</span
                >
                <span class="field-help">默认使用系统生成公钥即可。</span>
              </label>

              <article v-else class="inline-card">
                <p class="card-label">自动生成设备公钥</p>
                <code class="code">{{ portal.currentPubKey }}</code>
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="portal.regeneratePubKey"
                >
                  重新生成公钥
                </button>
              </article>

              <div class="action-row">
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="portal.openCredentialImport"
                >
                  导入凭证文件
                </button>
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="portal.exportCredentialFile"
                >
                  导出凭证文件
                </button>
              </div>

              <input
                ref="credentialFileInput"
                class="hidden-input"
                type="file"
                accept="application/json"
                @change="portal.importCredentialFile"
              />

              <button class="btn" :disabled="portal.credentialLoading">
                {{ portal.credentialLoading ? '签发中...' : '管理员代发凭证' }}
              </button>
            </form>

            <aside class="detail-side">
              <article
                class="callout-card"
                :class="portal.adminTokenReady ? 'success' : 'warn'"
              >
                <strong>{{
                  portal.adminTokenReady
                    ? '可以签发凭证'
                    : '领取前需要 Admin Token'
                }}</strong>
                <p>
                  {{
                    portal.adminTokenReady
                      ? '当前提案可直接签发或导入匿名凭证。'
                      : '未配置 Token 时，签发接口会被后端拒绝。'
                  }}
                </p>
              </article>

              <article
                class="wallet-card"
                :class="{ ready: portal.walletReady }"
              >
                <h3>凭证包</h3>
                <p v-if="!portal.walletReady" class="muted">
                  尚未领取或导入凭证。
                </p>
                <dl v-else class="wallet-list">
                  <div>
                    <dt>user_id</dt>
                    <dd>{{ portal.wallet.userId }}</dd>
                  </div>
                  <div>
                    <dt>issuer_key_version</dt>
                    <dd>{{ portal.wallet.issuerKeyVersion || '-' }}</dd>
                  </div>
                  <div>
                    <dt>issued_at</dt>
                    <dd>{{ portal.wallet.issuedAt || '-' }}</dd>
                  </div>
                  <div>
                    <dt>user_pub_key</dt>
                    <dd class="code">{{ portal.wallet.userPubKey }}</dd>
                  </div>
                  <div>
                    <dt>signature</dt>
                    <dd class="code">{{ portal.wallet.signature }}</dd>
                  </div>
                </dl>
              </article>
            </aside>
          </div>
        </section>

        <section
          v-else-if="portal.activeSection === 'vote'"
          id="section-panel-vote"
          class="panel"
          :class="{
            'guide-focus':
              isGuideStepActive('user-vote-prepare') ||
              isGuideStepActive('user-vote-submit'),
          }"
          role="tabpanel"
          aria-labelledby="workflow-tab-vote"
        >
          <header class="panel-head">
            <div>
              <h2>参与投票</h2>
              <p class="panel-tip">
                按“选提案 → 选选项 → 生成凭据 → 提交”完成投票。
              </p>
            </div>
            <div class="header-actions">
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="portal.hallState.loading"
                @click="portal.loadOpenProposals(false)"
              >
                同步提案
              </button>
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="portal.voteStatusLoading || !portal.walletReady"
                @click="portal.fetchVoteStatus()"
              >
                {{
                  portal.voteStatusLoading
                    ? '查询中...'
                    : '基于当前凭证校验状态'
                }}
              </button>
            </div>
          </header>

          <div class="status-strip">
            <p>当前提案：{{ portal.voteForm.proposalId || '-' }}</p>
            <p>凭证状态：{{ portal.walletReady ? '已就绪' : '未就绪' }}</p>
            <p>选项数：{{ selectedProposalOptionCount || '-' }}</p>
            <span
              class="status-pill"
              :class="portal.walletReady ? 'done' : 'todo'"
            >
              {{ portal.walletReady ? '可提交' : '先领取凭证' }}
            </span>
            <span
              v-if="portal.hasVoted !== null"
              class="status-pill"
              :class="portal.hasVoted ? 'done' : 'todo'"
            >
              {{ portal.hasVoted ? '你已投票' : '你还未投票' }}
            </span>
          </div>

          <div class="compact-note-row">
            <article class="compact-note">
              <strong>状态查询</strong>
              <p>必须基于当前凭证校验，不能直接枚举 `user_id`。</p>
            </article>
            <article class="compact-note">
              <strong>默认用法</strong>
              <p>通常只要选项后点击“按选项生成凭据”，无需手写 `proof`。</p>
            </article>
          </div>

          <form class="form-grid flow-form" @submit.prevent="handleSubmitVote">
            <article class="flow-step-card">
              <p class="step-kicker">步骤 1 · 选择提案</p>
              <label class="field" for="vote-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true"
                  >*</span
                >
                <input
                  id="vote-proposal-id"
                  v-model.trim="portal.voteForm.proposalId"
                  :aria-invalid="
                    portal.voteErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.voteErrors.proposalId
                      ? 'vote-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.voteErrors.proposalId"
                  id="vote-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.voteErrors.proposalId }}</span
                >
              </label>

              <div
                v-if="portal.hallState.proposals.length > 0"
                class="chip-row"
                aria-label="快速选择提案"
              >
                <button
                  v-for="item in portal.hallState.proposals"
                  :key="item.id"
                  class="chip"
                  type="button"
                  :class="{ active: portal.voteForm.proposalId === item.id }"
                  @click="portal.adoptProposal(item.id)"
                >
                  {{ item.title || item.id }}
                </button>
              </div>
            </article>

            <article
              id="guide-anchor-user-vote-prepare"
              class="flow-step-card"
              :class="{
                'guide-focus-soft': isGuideStepActive('user-vote-prepare'),
              }"
            >
              <p class="step-kicker">步骤 2 · 选择选项</p>
              <fieldset
                v-if="
                  portal.selectedProposal &&
                  portal.selectedProposal.options.length > 0
                "
                class="option-box"
                :aria-describedby="
                  portal.voteErrors.option ? 'vote-option-error' : undefined
                "
              >
                <legend>
                  投票选项<span class="required-mark" aria-hidden="true"
                    >*</span
                  >
                </legend>
                <label
                  v-for="option in portal.selectedProposal.options"
                  :key="option"
                  class="option-item"
                >
                  <input
                    v-model="portal.selectedVoteOption"
                    :value="option"
                    type="radio"
                  />
                  <span>{{ option }}</span>
                </label>
                <span
                  v-if="portal.voteErrors.option"
                  id="vote-option-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.voteErrors.option }}</span
                >
              </fieldset>
              <p v-else class="muted">当前提案没有可选项，请先切换提案。</p>
            </article>

            <article class="flow-step-card">
              <div class="step-head-inline">
                <p class="step-kicker">步骤 3 · 生成投票凭据</p>
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="showVoteTools = !showVoteTools"
                >
                  {{ showVoteTools ? '收起低频工具' : '展开低频工具' }}
                </button>
              </div>

              <button
                class="btn btn-primary"
                type="button"
                :disabled="portal.voteLoading || !portal.walletReady"
                @click="handlePrepareBallotByOption"
              >
                {{
                  portal.voteLoading ? '生成中...' : '按选项生成凭据（推荐）'
                }}
              </button>

              <label class="field" for="vote-ballot-json">
                投票凭据 JSON（可选）
                <textarea
                  id="vote-ballot-json"
                  v-model.trim="portal.voteForm.ballotJson"
                  rows="5"
                  placeholder='{"encrypted_vote":"...","proof":"...","nullifier":"...","public_witness":"..."}'
                  :aria-invalid="
                    portal.voteErrors.ballotJson ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.voteErrors.ballotJson
                      ? 'vote-ballot-json-error'
                      : undefined
                  "
                />
                <span class="field-help"
                  >常规情况下只需点击“按选项生成凭据”，无需手写 JSON。</span
                >
                <span
                  v-if="portal.voteErrors.ballotJson"
                  id="vote-ballot-json-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.voteErrors.ballotJson }}</span
                >
              </label>

              <div v-if="showVoteTools" class="advanced-tools">
                <p class="field-help">
                  低频辅助工具：仅在调试或手工修复凭据时使用。
                </p>
                <div class="action-row compact">
                  <button
                    class="btn btn-secondary"
                    type="button"
                    @click="portal.applySelectedOptionToBallot"
                  >
                    写入选项
                  </button>
                  <button
                    class="btn btn-secondary"
                    type="button"
                    @click="portal.fillBallotTemplate"
                  >
                    填充模板
                  </button>
                  <button
                    class="btn btn-secondary"
                    type="button"
                    @click="portal.importBallotJson"
                  >
                    导入 JSON
                  </button>
                  <button
                    class="btn btn-secondary"
                    type="button"
                    @click="portal.voteForm.nullifier = portal.newNullifier()"
                  >
                    新 nullifier
                  </button>
                </div>
              </div>

              <label class="switch-field">
                <input
                  v-model="portal.showAdvancedVoteFields"
                  type="checkbox"
                />
                显示高级字段（仅排障时使用）
              </label>

              <div v-if="portal.showAdvancedVoteFields" class="advanced-grid">
                <label class="field">
                  encrypted_vote<span class="required-mark" aria-hidden="true"
                    >*</span
                  >
                  <textarea
                    v-model.trim="portal.voteForm.encryptedVote"
                    rows="2"
                    :aria-invalid="
                      portal.voteErrors.encryptedVote ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.voteErrors.encryptedVote
                        ? 'vote-encrypted-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.voteErrors.encryptedVote"
                    id="vote-encrypted-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.voteErrors.encryptedVote }}</span
                  >
                </label>
                <label class="field">
                  proof<span class="required-mark" aria-hidden="true">*</span>
                  <textarea
                    v-model.trim="portal.voteForm.proof"
                    rows="2"
                    :aria-invalid="portal.voteErrors.proof ? 'true' : 'false'"
                    :aria-describedby="
                      portal.voteErrors.proof ? 'vote-proof-error' : undefined
                    "
                  />
                  <span
                    v-if="portal.voteErrors.proof"
                    id="vote-proof-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.voteErrors.proof }}</span
                  >
                </label>
                <label class="field">
                  nullifier<span class="required-mark" aria-hidden="true"
                    >*</span
                  >
                  <input
                    v-model.trim="portal.voteForm.nullifier"
                    :aria-invalid="
                      portal.voteErrors.nullifier ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.voteErrors.nullifier
                        ? 'vote-nullifier-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.voteErrors.nullifier"
                    id="vote-nullifier-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.voteErrors.nullifier }}</span
                  >
                </label>
                <label class="field">
                  public_witness（可选）
                  <textarea
                    v-model.trim="portal.voteForm.publicWitness"
                    rows="2"
                  />
                </label>
              </div>
            </article>

            <article
              id="guide-anchor-user-vote-submit"
              class="flow-step-card flow-submit"
              :class="{
                'guide-focus-soft': isGuideStepActive('user-vote-submit'),
              }"
            >
              <p class="step-kicker">步骤 4 · 提交选票</p>
              <button
                class="btn btn-primary"
                :disabled="portal.voteLoading || !portal.walletReady"
              >
                {{ portal.voteLoading ? '提交中...' : '提交匿名选票' }}
              </button>
            </article>
          </form>

          <div
            class="mobile-vote-dock"
            role="region"
            aria-label="移动端快捷操作"
          >
            <p class="mobile-vote-meta">
              <span>提案：{{ portal.voteForm.proposalId || '-' }}</span>
              <span>{{
                portal.walletReady ? '凭证已就绪' : '先领取凭证'
              }}</span>
            </p>
            <div class="mobile-vote-actions">
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="portal.voteLoading || !portal.walletReady"
                @click="portal.prepareBallotByOption"
              >
                {{ portal.voteLoading ? '生成中...' : '生成凭据' }}
              </button>
              <button
                class="btn btn-primary"
                type="button"
                :disabled="portal.voteLoading || !portal.walletReady"
                @click="portal.submitVote"
              >
                {{ portal.voteLoading ? '提交中...' : '立即提交' }}
              </button>
            </div>
          </div>
        </section>

        <section
          v-else
          id="section-panel-ops"
          class="panel"
          :class="{
            'guide-focus':
              isGuideStepActive('admin-create') ||
              isGuideStepActive('admin-decrypt') ||
              isGuideStepActive('admin-publish') ||
              isGuideStepActive('admin-result'),
          }"
          role="tabpanel"
          aria-labelledby="workflow-tab-ops"
        >
          <header class="panel-head">
            <div>
              <h2>计票与审计</h2>
              <p class="panel-tip">用一条顺序化流程完成提案、计票和审计。</p>
            </div>
          </header>

          <div class="summary-strip summary-strip-compact">
            <article class="summary-item">
              <span>当前步骤</span>
              <strong>{{ currentOpsStepTitle }}</strong>
            </article>
            <article class="summary-item">
              <span>proposal_id</span>
              <strong>{{ portal.selectedProposalId || '-' }}</strong>
            </article>
            <article class="summary-item">
              <span>Admin Token</span>
              <strong>{{
                portal.adminTokenReady ? '已配置' : '未配置'
              }}</strong>
            </article>
          </div>

          <nav class="ops-stepper" aria-label="运营步骤">
            <button
              v-for="item in opsSteps"
              :key="item.id"
              class="ops-step-btn"
              :class="{ active: opsStep === item.id }"
              :aria-current="opsStep === item.id ? 'step' : undefined"
              @click="setOpsStep(item.id)"
            >
              <span class="ops-step-order">{{ item.order }}</span>
              <span class="ops-step-content">
                <strong>{{ item.title }}</strong>
                <small>{{ item.description }}</small>
              </span>
            </button>
          </nav>

          <div class="ops-stage">
            <form
              v-if="opsStep === 'create'"
              id="guide-anchor-admin-create"
              class="ops-card ops-card-main"
              :class="{ 'guide-focus-soft': isGuideStepActive('admin-create') }"
              @submit.prevent="handleCreateProposal"
            >
              <h3>步骤 1：创建提案</h3>
              <article class="callout-card info">
                <strong>这一步是在创建一场新的投票</strong>
                <p>
                  你要填的是标题、候选项和时间范围。创建完成后，系统会给这场投票分配一个唯一的
                  `proposal_id`，后面的初始化解密、发凭证、发布结果都会用到它。
                </p>
              </article>
              <label class="field" for="ops-create-title">
                标题<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-create-title"
                  v-model.trim="portal.createProposalForm.title"
                  :aria-invalid="
                    portal.createProposalErrors.title ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.createProposalErrors.title
                      ? 'ops-create-title-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.createProposalErrors.title"
                  id="ops-create-title-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.createProposalErrors.title }}</span
                >
              </label>
              <label class="field" for="ops-create-description">
                描述
                <textarea
                  id="ops-create-description"
                  v-model.trim="portal.createProposalForm.description"
                  rows="2"
                />
              </label>
              <label class="field" for="ops-create-options">
                选项（每行一个）<span class="required-mark" aria-hidden="true"
                  >*</span
                >
                <textarea
                  id="ops-create-options"
                  v-model.trim="portal.createProposalForm.optionsText"
                  rows="4"
                  :aria-invalid="
                    portal.createProposalErrors.options ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.createProposalErrors.options
                      ? 'ops-create-options-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.createProposalErrors.options"
                  id="ops-create-options-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.createProposalErrors.options }}</span
                >
              </label>
              <div class="time-grid">
                <label class="field" for="ops-create-start">
                  开始时间（starts_at）<span
                    class="required-mark"
                    aria-hidden="true"
                    >*</span
                  >
                  <input
                    id="ops-create-start"
                    v-model.trim="portal.createProposalForm.startsAtLocal"
                    type="datetime-local"
                    :aria-invalid="
                      portal.createProposalErrors.startsAt ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.createProposalErrors.startsAt
                        ? 'ops-create-start-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.createProposalErrors.startsAt"
                    id="ops-create-start-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.createProposalErrors.startsAt }}</span
                  >
                  <span class="field-help"
                    >这场投票从什么时候开始允许用户投票。</span
                  >
                </label>
                <label class="field" for="ops-create-end">
                  结束时间（ends_at）<span
                    class="required-mark"
                    aria-hidden="true"
                    >*</span
                  >
                  <input
                    id="ops-create-end"
                    v-model.trim="portal.createProposalForm.endsAtLocal"
                    type="datetime-local"
                    :aria-invalid="
                      portal.createProposalErrors.endsAt ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.createProposalErrors.endsAt
                        ? 'ops-create-end-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.createProposalErrors.endsAt"
                    id="ops-create-end-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.createProposalErrors.endsAt }}</span
                  >
                  <span class="field-help"
                    >投票结束后，才能进入最终计票和结果发布。</span
                  >
                </label>
              </div>
              <article class="callout-card info">
                <strong>正常流程：先创建“未开始”的提案</strong>
                <p>
                  系统要求先初始化解密，再开放投票，所以开始时间请保持在未来。下面按钮会自动填入“15
                  分钟后开始、持续 1 小时”的推荐时间。
                </p>
              </article>
              <div class="action-row">
                <button
                  class="btn btn-secondary"
                  type="button"
                  @click="portal.setProposalWindow('scheduled')"
                >
                  填入推荐时间
                </button>
              </div>
              <div class="action-row">
                <button
                  class="btn btn-primary"
                  :disabled="portal.createProposalLoading"
                >
                  {{ portal.createProposalLoading ? '创建中...' : '创建提案' }}
                </button>
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="setOpsStep('decrypt')"
                >
                  下一步：初始化解密
                </button>
              </div>
            </form>

            <form
              v-else-if="opsStep === 'decrypt'"
              id="guide-anchor-admin-decrypt"
              class="ops-card ops-card-main"
              :class="{
                'guide-focus-soft': isGuideStepActive('admin-decrypt'),
              }"
              @submit.prevent="handleSetupDecryptionSession"
            >
              <h3>步骤 2：初始化解密会话</h3>
              <article class="callout-card info">
                <strong>这一步是在为计票做准备</strong>
                <p>
                  `threshold` 是“至少需要几份解密份额才能出结果”，`trustees`
                  是“总共拆成几份”。例如 `2 / 3` 表示一共 3 份，任意拿到其中 2
                  份就能完成最终解密计票。
                </p>
              </article>
              <label class="field" for="ops-decrypt-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="ops-decrypt-proposal-id"
                  v-model.trim="portal.decryptForm.proposalId"
                  :aria-invalid="
                    portal.decryptErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.decryptErrors.proposalId
                      ? 'ops-decrypt-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.decryptErrors.proposalId"
                  id="ops-decrypt-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.decryptErrors.proposalId }}</span
                >
                <span class="field-help"
                  >就是刚刚创建提案后得到的那一串编号。</span
                >
              </label>
              <div class="time-grid">
                <label class="field" for="ops-decrypt-threshold">
                  至少需要几份（threshold）<span
                    class="required-mark"
                    aria-hidden="true"
                    >*</span
                  >
                  <input
                    id="ops-decrypt-threshold"
                    v-model.number="portal.decryptForm.threshold"
                    type="number"
                    min="1"
                    inputmode="numeric"
                    :aria-invalid="
                      portal.decryptErrors.threshold ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.decryptErrors.threshold
                        ? 'ops-decrypt-threshold-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.decryptErrors.threshold"
                    id="ops-decrypt-threshold-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.decryptErrors.threshold }}</span
                  >
                  <span class="field-help"
                    >推荐默认 `2`，表示至少两份才能解密。</span
                  >
                </label>
                <label class="field" for="ops-decrypt-trustees">
                  总共分几份（trustees）<span
                    class="required-mark"
                    aria-hidden="true"
                    >*</span
                  >
                  <input
                    id="ops-decrypt-trustees"
                    v-model.number="portal.decryptForm.trustees"
                    type="number"
                    min="1"
                    inputmode="numeric"
                    :aria-invalid="
                      portal.decryptErrors.trustees ? 'true' : 'false'
                    "
                    :aria-describedby="
                      portal.decryptErrors.trustees
                        ? 'ops-decrypt-trustees-error'
                        : undefined
                    "
                  />
                  <span
                    v-if="portal.decryptErrors.trustees"
                    id="ops-decrypt-trustees-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.decryptErrors.trustees }}</span
                  >
                  <span class="field-help"
                    >推荐默认 `3`，表示系统会生成 3 份解密材料。</span
                  >
                </label>
              </div>
              <article
                class="callout-card info"
                v-if="
                  portal.decryptPublicKey || portal.decryptShares.length > 0
                "
              >
                <strong>初始化成功后你会看到什么</strong>
                <p>
                  `public_key` 是给投票加密用的公开材料；`shares`
                  是计票时要提交的解密份额。开发联调阶段可以先把 `shares`
                  留在页面里，后面发布结果时直接复用。
                </p>
              </article>
              <button class="btn btn-primary" :disabled="portal.decryptLoading">
                {{ portal.decryptLoading ? '初始化中...' : '初始化' }}
              </button>
              <p v-if="portal.decryptPublicKey" class="code block-code">
                public_key: {{ portal.decryptPublicKey }}
              </p>
              <div class="action-row">
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="setOpsStep('publish')"
                >
                  下一步：发布结果
                </button>
              </div>
            </form>

            <form
              v-else-if="opsStep === 'publish'"
              id="guide-anchor-admin-publish"
              class="ops-card ops-card-main"
              :class="{
                'guide-focus-soft': isGuideStepActive('admin-publish'),
              }"
              @submit.prevent="handlePublishResult"
            >
              <h3>步骤 3：发布结果</h3>
              <article class="callout-card info">
                <strong>这一步是在真正出结果</strong>
                <p>
                  这里的 `shares`
                  就是上一步初始化时生成的解密份额。只要提交的份额数量达到
                  `threshold`，系统就能解密所有选票并发布最终结果。
                </p>
              </article>
              <label class="field" for="ops-publish-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="ops-publish-proposal-id"
                  v-model.trim="portal.publishForm.proposalId"
                  :aria-invalid="
                    portal.publishErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.publishErrors.proposalId
                      ? 'ops-publish-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.publishErrors.proposalId"
                  id="ops-publish-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.publishErrors.proposalId }}</span
                >
              </label>
              <label class="field" for="ops-publish-shares">
                解密份额（shares）<span class="required-mark" aria-hidden="true"
                  >*</span
                >
                <textarea
                  id="ops-publish-shares"
                  v-model.trim="portal.publishForm.sharesText"
                  rows="3"
                  :aria-invalid="portal.publishErrors.shares ? 'true' : 'false'"
                  :aria-describedby="
                    portal.publishErrors.shares
                      ? 'ops-publish-shares-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.publishErrors.shares"
                  id="ops-publish-shares-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.publishErrors.shares }}</span
                >
                <span class="field-help"
                  >通常直接使用上一步生成并自动带过来的内容，不需要手工改。</span
                >
              </label>
              <button class="btn btn-primary" :disabled="portal.publishLoading">
                {{ portal.publishLoading ? '发布中...' : '发布结果' }}
              </button>
              <p class="muted">
                {{ portal.publishedSummary || '发布后将在此显示摘要' }}
              </p>
              <div class="action-row">
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="setOpsStep('result')"
                >
                  下一步：查询结果
                </button>
              </div>
            </form>

            <form
              v-else-if="opsStep === 'result'"
              id="guide-anchor-admin-result"
              class="ops-card ops-card-main"
              :class="{ 'guide-focus-soft': isGuideStepActive('admin-result') }"
              @submit.prevent="handleFetchResult"
            >
              <h3>步骤 4：查询结果</h3>
              <article class="callout-card info">
                <strong>这一步是查看最终票数</strong>
                <p>
                  查询成功后会显示每个选项的票数统计。这里看到的是已经解密并发布过的结果，不是实时投票中间态。
                </p>
              </article>
              <label class="field" for="ops-result-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="ops-result-proposal-id"
                  v-model.trim="portal.resultForm.proposalId"
                  :aria-invalid="
                    portal.resultErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.resultErrors.proposalId
                      ? 'ops-result-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.resultErrors.proposalId"
                  id="ops-result-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.resultErrors.proposalId }}</span
                >
              </label>
              <button class="btn btn-primary" :disabled="portal.resultLoading">
                {{ portal.resultLoading ? '查询中...' : '查询' }}
              </button>
              <p class="muted">
                {{ portal.resultSummary || '结果摘要将在此显示' }}
              </p>
              <ul v-if="portal.resultTally.length > 0" class="result-list">
                <li v-for="item in portal.resultTally" :key="item.option">
                  <span>{{ item.option }}</span>
                  <strong>{{ item.count }}</strong>
                </li>
              </ul>
              <div class="action-row">
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="setOpsStep('credential')"
                >
                  下一步：凭证状态
                </button>
              </div>
            </form>

            <form
              v-else-if="opsStep === 'credential'"
              class="ops-card ops-card-main"
              @submit.prevent="handleFetchCredentialStatuses"
            >
              <h3>步骤 5：凭证状态</h3>
              <article class="callout-card info">
                <strong>这一步只能看“谁投过”，看不到“投了什么”</strong>
                <p>
                  这里用于确认某个提案下已经发出去的凭证里，哪些已经被使用过。它不会泄露具体投票内容。
                </p>
              </article>
              <label class="field" for="ops-credential-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="ops-credential-proposal-id"
                  v-model.trim="portal.credentialStatusForm.proposalId"
                  :aria-invalid="
                    portal.credentialStatusErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.credentialStatusErrors.proposalId
                      ? 'ops-credential-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.credentialStatusErrors.proposalId"
                  id="ops-credential-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.credentialStatusErrors.proposalId }}</span
                >
              </label>
              <button
                class="btn btn-primary"
                :disabled="portal.credentialStatusLoading"
              >
                {{
                  portal.credentialStatusLoading ? '查询中...' : '查询凭证状态'
                }}
              </button>
              <p class="muted">
                这里只展示凭证是否已投票，不展示具体投票内容。
              </p>
              <div
                v-if="portal.credentialStatuses.length > 0"
                class="credential-status-list"
              >
                <article
                  v-for="item in portal.credentialStatuses"
                  :key="item.credentialId"
                  class="credential-status-card"
                >
                  <p>
                    <strong>{{ item.userId }}</strong>
                    <span
                      class="status-pill"
                      :class="item.hasVoted ? 'done' : 'todo'"
                      >{{ item.hasVoted ? '已投票' : '未投票' }}</span
                    >
                  </p>
                  <p class="code">credential_id: {{ item.credentialId }}</p>
                  <p class="code">user_pub_key: {{ item.userPubKey }}</p>
                  <p>issued_at: {{ item.issuedAt || '-' }}</p>
                </article>
              </div>
              <div class="action-row">
                <button
                  class="btn btn-ghost"
                  type="button"
                  @click="setOpsStep('anchor')"
                >
                  下一步：审计锚点
                </button>
              </div>
            </form>

            <form
              v-else
              class="ops-card ops-card-main"
              @submit.prevent="portal.fetchAnchors"
            >
              <h3>步骤 6：审计锚点</h3>
              <article class="callout-card info">
                <strong>这一步给的是审计材料</strong>
                <p>
                  `anchor_hash` 和 `merkle_root`
                  主要是给技术审计或后续留痕核验用的。普通使用者只需要知道：它们是本次结果对应的一组校验指纹。
                </p>
              </article>
              <label class="field" for="ops-anchor-proposal-id">
                提案编号（proposal_id）<span
                  class="required-mark"
                  aria-hidden="true"
                  >*</span
                >
                <input
                  id="ops-anchor-proposal-id"
                  v-model.trim="portal.anchorForm.proposalId"
                  :aria-invalid="
                    portal.anchorErrors.proposalId ? 'true' : 'false'
                  "
                  :aria-describedby="
                    portal.anchorErrors.proposalId
                      ? 'ops-anchor-proposal-id-error'
                      : undefined
                  "
                />
                <span
                  v-if="portal.anchorErrors.proposalId"
                  id="ops-anchor-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.anchorErrors.proposalId }}</span
                >
              </label>
              <button class="btn btn-primary" :disabled="portal.anchorLoading">
                {{ portal.anchorLoading ? '查询中...' : '查询锚点' }}
              </button>
              <div v-if="portal.anchors.length > 0" class="anchor-list">
                <article v-for="item in portal.anchors" :key="item.id">
                  <p>
                    <strong>{{ item.id }}</strong> · {{ item.createdAt }}
                  </p>
                  <p class="code">anchor_hash: {{ item.anchorHash }}</p>
                  <p class="code">merkle_root: {{ item.merkleRoot }}</p>
                </article>
              </div>
            </form>
          </div>
        </section>
      </section>
    </main>
  </div>
</template>

<style scoped>
.message-stack {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 260;
  display: grid;
  gap: 10px;
  width: min(380px, calc(100vw - 24px));
  pointer-events: none;
}

.message-card {
  pointer-events: auto;
  border-radius: 16px;
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(14px);
  box-shadow: 0 18px 36px rgba(15, 23, 42, 0.16);
  padding: 12px 14px;
  display: grid;
  gap: 6px;
  animation: message-enter 220ms ease both;
}

.message-card.success {
  border-color: rgba(22, 163, 74, 0.34);
}

.message-card.error {
  border-color: rgba(220, 74, 47, 0.42);
}

.message-card.info {
  border-color: rgba(37, 99, 235, 0.36);
}

.message-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.message-card p,
.message-card time {
  margin: 0;
  color: var(--text-soft);
}

.message-card time {
  font-size: 0.78rem;
}

.message-close {
  border: 0;
  background: transparent;
  color: var(--text-soft);
  cursor: pointer;
  padding: 0;
  font: inherit;
}

.journey-panel {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 250;
  width: min(360px, calc(100vw - 24px));
  border: 1px solid rgba(22, 163, 74, 0.24);
  border-radius: 18px;
  background: rgba(252, 255, 253, 0.96);
  backdrop-filter: blur(14px);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
  padding: 14px;
  display: grid;
  gap: 10px;
}

.journey-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.journey-kicker {
  margin: 0;
  font-size: 0.78rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  font-weight: 800;
  color: #17643a;
}

.journey-panel p {
  margin: 0;
  color: var(--text-soft);
}

.journey-progress {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 0.83rem;
  color: var(--text-soft);
}

.journey-actions {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.guide-overlay {
  position: fixed;
  inset: 0;
  z-index: 320;
  background: rgba(15, 23, 42, 0.48);
  backdrop-filter: blur(8px);
  display: grid;
  place-items: center;
  padding: 18px;
}

.guide-dialog {
  width: min(760px, 100%);
  border-radius: 22px;
  border: 1px solid rgba(226, 232, 240, 0.82);
  background:
    radial-gradient(
      circle at top right,
      rgba(34, 197, 94, 0.12),
      transparent 28%
    ),
    linear-gradient(180deg, #ffffff 0%, #f7fbff 100%);
  box-shadow: 0 28px 60px rgba(15, 23, 42, 0.24);
  padding: 24px;
  display: grid;
  gap: 16px;
}

.guide-dialog-kicker {
  margin: 0;
  font-size: 0.78rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  font-weight: 800;
  color: #17643a;
}

.guide-dialog-copy {
  margin: 0;
  color: var(--text-soft);
  max-width: 60ch;
}

.guide-dialog-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.guide-dialog-card {
  border: 1px solid var(--line);
  border-radius: 16px;
  background: #ffffff;
  padding: 16px;
  display: grid;
  gap: 10px;
}

.guide-dialog-card.accent {
  background: #f8fffb;
  border-color: rgba(22, 163, 74, 0.24);
}

.guide-dialog-card p,
.guide-dialog-list p {
  margin: 0;
  color: var(--text-soft);
}

.guide-dialog-list {
  border: 1px dashed var(--line-strong);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.72);
  padding: 14px;
  display: grid;
  gap: 8px;
}

.guide-dialog-actions {
  display: flex;
  justify-content: flex-end;
}

.skip-link {
  position: absolute;
  left: 16px;
  top: -52px;
  padding: 10px 14px;
  border-radius: 999px;
  background: var(--cta);
  color: #ffffff;
  font-weight: 700;
  text-decoration: none;
  z-index: 200;
}

.skip-link:focus {
  top: 12px;
}

.app-shell {
  width: min(1580px, 100%);
  margin: 0 auto;
  padding: 16px 16px 44px;
  display: grid;
  gap: 10px;
}

.shell-topbar {
  position: relative;
  z-index: 20;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: none;
  padding: 6px 8px;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 6px;
  align-items: center;
}

.topbar-main {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 6px;
  align-items: center;
}

.brand-block {
  display: grid;
  gap: 0;
}

.brand-block strong {
  font-size: 0.86rem;
  letter-spacing: -0.01em;
}

.primary-nav {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.primary-nav-btn,
.workflow-tab-btn {
  border: 1px solid var(--line);
  border-radius: 12px;
  background: #f8fbff;
  text-align: left;
  padding: 8px 10px;
  display: grid;
  gap: 2px;
  cursor: pointer;
  transition:
    border-color 180ms ease,
    box-shadow 180ms ease,
    background-color 180ms ease;
}

.primary-nav-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 6px;
  border-color: transparent;
  background: transparent;
  box-shadow: none;
  white-space: nowrap;
}

.primary-nav-btn strong {
  font-size: 0.76rem;
  font-weight: 700;
}

.primary-nav-btn:hover,
.workflow-tab-btn:hover {
  border-color: var(--line-strong);
  box-shadow: 0 3px 8px rgba(15, 23, 42, 0.04);
}

.primary-nav-btn.active,
.workflow-tab-btn.active {
  border-color: rgba(37, 99, 235, 0.22);
  background: #f6f9ff;
  box-shadow: inset 0 -1px 0 rgba(37, 99, 235, 0.2);
}

.workflow-tab-btn small {
  color: var(--text-soft);
  font-size: 0.74rem;
}

.workflow-tab-btn {
  background: rgba(248, 251, 255, 0.76);
  border-color: rgba(148, 163, 184, 0.18);
  padding: 10px 12px;
}

.workflow-tab-btn.active {
  background: #ffffff;
  border-color: rgba(37, 99, 235, 0.22);
  box-shadow:
    0 10px 22px rgba(15, 23, 42, 0.05),
    inset 0 0 0 1px rgba(37, 99, 235, 0.08);
}

.topbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 1px;
  justify-content: flex-end;
}

.topbar-link {
  border: 0;
  background: transparent;
  padding: 3px 5px;
  color: var(--text-soft);
  font: inherit;
  font-size: 0.72rem;
  line-height: 1;
  cursor: pointer;
  border-radius: 6px;
}

.topbar-link:hover {
  color: var(--text-main);
  background: #f8fbff;
}

.topbar-link.active {
  color: var(--text-main);
  background: #f6f9ff;
}

.topbar-link-icon {
  width: 20px;
  height: 20px;
  padding: 0;
  border-radius: 999px;
  display: inline-grid;
  place-items: center;
}

.topbar-status-strip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 3px;
  padding-top: 1px;
  opacity: 0.88;
}

.topbar-chip {
  border: 1px solid var(--line);
  border-radius: 999px;
  background: #ffffff;
  padding: 3px 6px;
  font-size: 0.7rem;
  color: var(--text-soft);
  line-height: 1.2;
  white-space: nowrap;
}

.topbar-chip-subtle {
  background: #f8fbff;
  border-color: rgba(148, 163, 184, 0.2);
}

.topbar-dots {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: 1px;
}

.topbar-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: #cbd5e1;
  box-shadow: inset 0 0 0 1px rgba(15, 23, 42, 0.08);
  flex: 0 0 auto;
}

.topbar-dot.good {
  background: #16a34a;
}

.topbar-dot.warn {
  background: #d97706;
}

.topbar-dot.bad {
  background: #dc2626;
}

.topbar-dot.muted {
  background: #94a3b8;
}

.topbar-mini-code {
  border: 1px solid var(--line);
  border-radius: 999px;
  background: #ffffff;
  padding: 2px 5px;
  font-family: var(--mono-font);
  font-size: 0.68rem;
  line-height: 1.2;
  max-width: 44px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar-mini-code.empty {
  color: var(--text-soft);
}

.workspace-shell {
  display: grid;
  gap: 8px;
}

.eyebrow,
.overview-kicker {
  margin: 0 0 6px;
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #30527c;
}

.workflow-tabs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  padding-top: 2px;
}

.summary-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
}

.summary-strip-compact {
  gap: 8px;
}

.summary-item {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 12px;
  background: rgba(248, 251, 255, 0.62);
  padding: 8px 10px;
  display: grid;
  gap: 2px;
}

.summary-item span {
  font-size: 0.72rem;
  color: var(--text-soft);
}

.summary-item strong {
  font-size: 0.86rem;
}

.overview-grid,
.setup-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(280px, 0.9fr);
  gap: 14px;
}

.overview-panel,
.setup-panel {
  gap: 16px;
}

.overview-card,
.setup-card,
.launch-card {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 18px;
  background: rgba(248, 251, 255, 0.72);
  padding: 14px;
  display: grid;
  gap: 8px;
}

.overview-card-hero {
  border-color: rgba(37, 99, 235, 0.18);
  background:
    radial-gradient(
      circle at top right,
      rgba(37, 99, 235, 0.16),
      transparent 30%
    ),
    linear-gradient(160deg, #ffffff 0%, #f7fbff 52%, #f5fbff 100%);
  padding: 18px;
  box-shadow: 0 14px 28px rgba(15, 23, 42, 0.06);
}

.overview-card p,
.setup-card p,
.launch-card p {
  margin: 0;
  color: var(--text-soft);
}

.overview-side,
.detail-side {
  display: grid;
  gap: 12px;
}

.compact-card {
  gap: 8px;
}

.compact-checklist {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.compact-checklist span,
.capability-list span {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 6px 10px;
  background: #ffffff;
  font-size: 0.8rem;
  color: var(--text-soft);
}

.quick-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.action-chip {
  font-size: 0.82rem;
}

.capability-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.state-good {
  border-color: rgba(22, 163, 74, 0.38) !important;
  color: #0a7e45 !important;
  background: #f4fff7 !important;
}

.state-warn {
  border-color: rgba(217, 119, 6, 0.34) !important;
  color: #9a5511 !important;
  background: #fffaf1 !important;
}

.state-muted {
  color: var(--text-soft) !important;
}

.launch-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
}

.setup-side {
  display: grid;
  gap: 12px;
}

.health-line {
  margin: 0;
  font-size: 0.87rem;
  display: grid;
  gap: 3px;
}

.health-line.ok {
  color: #0f8a43;
}

.health-line.bad {
  color: #a11d2b;
}

.health-line span {
  color: var(--text-soft);
}

.main-grid {
  display: grid;
}

.main-panel {
  display: grid;
  gap: 10px;
}

.guide-panel {
  gap: 18px;
}

.guide-hero {
  border: 1px solid var(--line);
  border-radius: 18px;
  padding: 18px;
  background:
    radial-gradient(
      circle at 88% 14%,
      rgba(34, 197, 94, 0.16),
      transparent 34%
    ),
    linear-gradient(140deg, #f8fffb 0%, #eefbf3 48%, #f5faff 100%);
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(240px, 0.9fr);
  gap: 16px;
  align-items: end;
}

.guide-kicker {
  margin: 0 0 8px;
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #24513c;
}

.guide-hero-copy {
  display: grid;
  gap: 8px;
}

.guide-hero-copy p {
  margin: 0;
  color: var(--text-soft);
  max-width: 60ch;
}

.guide-hero-actions {
  display: grid;
  gap: 8px;
}

.guide-block {
  display: grid;
  gap: 12px;
}

.guide-block-head {
  display: grid;
  gap: 4px;
}

.guide-block-head p {
  margin: 0;
  color: var(--text-soft);
}

.guide-grid,
.glossary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
}

.guide-card,
.glossary-card {
  border: 1px solid var(--line);
  border-radius: 15px;
  background: var(--surface-strong);
  padding: 14px;
  display: grid;
  gap: 8px;
}

.guide-card span {
  width: 30px;
  height: 30px;
  display: grid;
  place-items: center;
  border-radius: 999px;
  background: #e7eef9;
  color: #27496d;
  font-weight: 800;
  font-size: 0.85rem;
}

.guide-card.accent span {
  background: #e7f8ee;
  color: #17643a;
}

.guide-card p,
.glossary-card p {
  margin: 0;
  color: var(--text-soft);
}

.panel {
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 22px;
  background: rgba(255, 255, 255, 0.96);
  padding: 18px;
  display: grid;
  gap: 14px;
  box-shadow: 0 18px 38px rgba(15, 23, 42, 0.06);
}

.guide-focus {
  border-color: rgba(22, 163, 74, 0.46);
  box-shadow:
    0 18px 36px rgba(15, 23, 42, 0.08),
    0 0 0 4px rgba(34, 197, 94, 0.1);
}

.guide-focus-soft {
  border-color: rgba(22, 163, 74, 0.42);
  box-shadow:
    0 12px 24px rgba(15, 23, 42, 0.08),
    0 0 0 3px rgba(34, 197, 94, 0.08);
}

.panel-head {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.panel-tip {
  margin: 4px 0 0;
  color: var(--text-soft);
  font-size: 0.88rem;
}

.header-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.proposal-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 10px;
}

.proposal-card {
  border: 1px solid var(--line);
  border-radius: 15px;
  padding: 12px;
  background: #ffffff;
  display: grid;
  gap: 7px;
  transition:
    border-color 180ms ease,
    box-shadow 180ms ease;
}

.proposal-card:hover {
  border-color: var(--line-strong);
  box-shadow: 0 12px 22px rgba(15, 23, 42, 0.08);
}

.proposal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.status-badge {
  border: 1px solid var(--line-strong);
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 0.78rem;
  color: var(--text-soft);
  background: #ffffff;
}

.card-desc {
  margin: 0;
  color: var(--text-soft);
}

.card-label {
  margin: 0;
  font-size: 0.82rem;
  color: var(--text-soft);
}

.proposal-quick-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 0.79rem;
  color: var(--text-soft);
}

.option-list {
  margin: 6px 0 0;
  padding-left: 18px;
  display: grid;
  gap: 4px;
}

.option-list.compact {
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 0;
  list-style: none;
}

.option-list.compact li {
  border: 1px solid var(--line);
  border-radius: 999px;
  background: #ffffff;
  padding: 4px 9px;
  font-size: 0.8rem;
}

.meta-grid {
  margin: 0;
  display: grid;
  gap: 8px;
}

.meta-grid.dense {
  gap: 6px;
}

.meta-grid div {
  display: grid;
  gap: 2px;
}

.meta-grid dt {
  font-size: 0.78rem;
  color: var(--text-soft);
}

.meta-grid dd {
  margin: 0;
  font-size: 0.86rem;
}

.form-grid {
  display: grid;
  gap: 12px;
}

.detail-split {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(260px, 0.8fr);
  gap: 12px;
  align-items: start;
}

.field {
  display: grid;
  gap: 6px;
  color: var(--text-soft);
}

.field-help {
  font-size: 0.83rem;
  color: var(--text-soft);
}

.field-error {
  color: var(--danger);
  font-size: 0.84rem;
  font-weight: 600;
}

.field-error::before {
  content: '错误：';
  font-weight: 700;
  margin-right: 2px;
}

.required-mark {
  margin-left: 2px;
  color: var(--danger);
  font-weight: 700;
}

.switch-field {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-soft);
  font-size: 0.93rem;
}

input,
textarea {
  width: 100%;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 10px 12px;
  font: inherit;
  color: var(--text-main);
  background: #ffffff;
  transition:
    border-color 160ms ease,
    box-shadow 160ms ease,
    background-color 160ms ease;
}

textarea {
  resize: vertical;
}

input:hover,
textarea:hover {
  border-color: var(--line-strong);
}

input[aria-invalid='true'],
textarea[aria-invalid='true'] {
  border-color: rgba(220, 74, 47, 0.65);
  background: #fff9f8;
}

.inline-card,
.wallet-card,
.callout-card,
.ops-card,
.status-strip {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--surface-strong);
  padding: 12px;
}

.wallet-card.ready {
  border-color: rgba(22, 163, 74, 0.55);
  background: #f7fff9;
}

.callout-card {
  display: grid;
  gap: 4px;
}

.callout-card p {
  margin: 0;
  color: var(--text-soft);
}

.callout-card.warn {
  border-color: rgba(217, 119, 6, 0.4);
  background: #fffaf1;
}

.callout-card.success {
  border-color: rgba(22, 163, 74, 0.4);
  background: #f4fff7;
}

.callout-card.info {
  border-color: rgba(37, 99, 235, 0.28);
  background: #f7fbff;
}

.wallet-list,
.wallet-list div {
  display: grid;
  gap: 4px;
}

.wallet-list dt {
  font-size: 0.8rem;
  color: var(--text-soft);
}

.wallet-list dd {
  margin: 0;
}

.code {
  font-family: var(--mono-font);
  word-break: break-all;
}

.block-code {
  margin: 0;
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chip {
  border: 1px solid var(--line-strong);
  border-radius: 999px;
  background: #ffffff;
  padding: 7px 11px;
  color: var(--text-main);
  cursor: pointer;
  transition:
    border-color 160ms ease,
    background-color 160ms ease,
    box-shadow 160ms ease;
}

.chip:hover {
  border-color: var(--cta);
  background: #f2fff6;
}

.chip.active {
  border-color: var(--cta);
  background: #ecfdf2;
  box-shadow: inset 0 0 0 1px rgba(34, 197, 94, 0.14);
}

.option-box {
  margin: 0;
  border: 1px dashed var(--line-strong);
  border-radius: 12px;
  padding: 10px 12px;
  display: grid;
  gap: 9px;
  background: #ffffff;
}

.option-box legend {
  font-size: 0.84rem;
  color: var(--text-soft);
  padding: 0 4px;
}

.option-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 10px;
  transition: background-color 160ms ease;
}

.option-item:hover {
  background: #f5f8fc;
}

.advanced-grid {
  display: grid;
  gap: 10px;
  padding: 10px;
  border: 1px dashed var(--line-strong);
  border-radius: 12px;
  background: #fcfdff;
}

.status-strip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
}

.status-strip p {
  margin: 0;
  font-size: 0.9rem;
}

.compact-note-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
}

.compact-note {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: #f8fbff;
  padding: 10px 12px;
  display: grid;
  gap: 4px;
}

.compact-note p {
  margin: 0;
  color: var(--text-soft);
  font-size: 0.86rem;
}

.status-pill {
  border-radius: 999px;
  padding: 5px 10px;
  font-size: 0.82rem;
  font-weight: 700;
  background: #ffffff;
}

.status-pill.done {
  border: 1px solid rgba(22, 163, 74, 0.45);
  color: #0a7e45;
}

.status-pill.todo {
  border: 1px solid var(--line-strong);
  color: var(--text-soft);
}

.flow-form {
  gap: 10px;
}

.flow-step-card {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--surface-strong);
  padding: 12px;
  display: grid;
  gap: 10px;
}

.flow-submit {
  background: linear-gradient(180deg, #f8fff9 0%, #f0fff4 100%);
  border-color: rgba(22, 163, 74, 0.36);
}

.mobile-vote-dock {
  display: none;
}

.mobile-vote-meta {
  margin: 0;
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: var(--text-soft);
  font-size: 0.79rem;
}

.mobile-vote-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.step-kicker {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: #30527c;
}

.step-head-inline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.advanced-tools {
  border: 1px dashed var(--line-strong);
  border-radius: 12px;
  padding: 10px;
  background: #ffffff;
}

.action-row.compact {
  grid-template-columns: repeat(auto-fit, minmax(126px, 1fr));
}

.ops-stepper {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 8px;
}

.ops-step-btn {
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--surface-strong);
  padding: 10px;
  display: flex;
  align-items: center;
  gap: 10px;
  text-align: left;
  cursor: pointer;
  transition:
    border-color 160ms ease,
    box-shadow 160ms ease,
    background-color 160ms ease;
}

.ops-step-btn:hover {
  border-color: var(--line-strong);
  background: #ffffff;
}

.ops-step-btn.active {
  border-color: var(--cta);
  background: #f4fdf7;
  box-shadow: 0 10px 20px rgba(22, 163, 74, 0.15);
}

.ops-step-order {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: #e7eef9;
  color: #2c4363;
  font-weight: 800;
  font-size: 0.82rem;
  flex-shrink: 0;
}

.ops-step-content {
  display: grid;
  gap: 2px;
}

.ops-step-content small {
  color: var(--text-soft);
}

.ops-stage {
  display: grid;
}

.ops-card {
  display: grid;
  gap: 10px;
}

.ops-card-main {
  border-radius: 16px;
  padding: 14px;
}

.time-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.result-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}

.result-list li {
  border: 1px solid var(--line);
  border-radius: 9px;
  padding: 6px 8px;
  display: flex;
  justify-content: space-between;
}

.anchor-list {
  display: grid;
  gap: 8px;
}

.credential-status-list {
  display: grid;
  gap: 8px;
}

.credential-status-card {
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 9px;
  background: #ffffff;
  display: grid;
  gap: 4px;
}

.credential-status-card p {
  margin: 0;
  display: flex;
  gap: 8px;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
}

.anchor-list article {
  border: 1px dashed var(--line-strong);
  border-radius: 10px;
  padding: 8px;
  background: #ffffff;
}

.anchor-list p {
  margin: 0 0 4px;
}

.empty-state {
  margin: 0;
  border: 1px dashed var(--line-strong);
  border-radius: 12px;
  padding: 14px;
  color: var(--text-soft);
}

.hidden-input {
  display: none;
}

.muted {
  margin: 0;
  color: var(--text-soft);
}

.action-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
}

.btn {
  border: 1px solid transparent;
  border-radius: 11px;
  padding: 9px 12px;
  background: linear-gradient(135deg, var(--cta), var(--cta-strong));
  color: #ffffff;
  font-weight: 700;
  cursor: pointer;
  transition:
    box-shadow 160ms ease,
    opacity 160ms ease,
    filter 160ms ease,
    border-color 160ms ease;
}

.btn:hover {
  filter: brightness(1.03);
  box-shadow: 0 10px 20px rgba(22, 163, 74, 0.22);
}

.btn:disabled {
  opacity: 0.58;
  cursor: not-allowed;
  box-shadow: none;
  filter: none;
}

.btn-secondary {
  background: #ffffff;
  border: 1px solid var(--line-strong);
  color: var(--text-main);
}

.btn-secondary:hover {
  box-shadow: 0 8px 16px rgba(15, 23, 42, 0.08);
}

.btn-ghost {
  background: transparent;
  border: 1px solid var(--line);
  color: var(--text-main);
}

.btn-primary {
  background: linear-gradient(135deg, var(--cta), var(--cta-strong));
  color: #ffffff;
}

:where(button, input, textarea, a, [role='tab']):focus-visible,
.message-close:focus-visible {
  outline: 3px solid var(--focus-ring);
  outline-offset: 2px;
}

.panel {
  animation: panel-enter 260ms ease both;
}

@keyframes message-enter {
  from {
    opacity: 0;
    transform: translateY(-6px) translateX(10px);
  }

  to {
    opacity: 1;
    transform: translateY(0) translateX(0);
  }
}

@keyframes panel-enter {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 980px) {
  .topbar-main {
    grid-template-columns: auto 1fr auto;
    align-items: start;
  }

  .primary-nav {
    grid-column: 1 / -1;
  }

  .overview-grid,
  .setup-grid {
    grid-template-columns: 1fr;
  }

  .detail-split {
    grid-template-columns: 1fr;
  }

  .guide-hero {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 780px) {
  .app-shell {
    padding-inline: 12px;
    gap: 12px;
  }

  .guide-overlay {
    padding: 12px;
  }

  .guide-dialog {
    padding: 18px;
  }

  .guide-dialog-grid {
    grid-template-columns: 1fr;
  }

  .topbar-actions {
    width: auto;
    justify-content: flex-end;
  }

  .primary-nav,
  .workflow-tabs,
  .topbar-status-strip {
    display: flex;
    overflow-x: auto;
    padding-bottom: 4px;
    scrollbar-width: thin;
    flex-wrap: nowrap;
    -webkit-overflow-scrolling: touch;
  }

  .primary-nav-btn,
  .workflow-tab-btn {
    min-width: 0;
    flex: 0 0 auto;
  }

  .primary-nav-btn {
    justify-content: flex-start;
  }

  .summary-strip,
  .compact-note-row {
    grid-template-columns: 1fr;
  }

  .message-stack {
    left: 12px;
    right: 12px;
    top: 12px;
    width: auto;
  }

  .journey-panel {
    left: 12px;
    right: 12px;
    bottom: calc(110px + env(safe-area-inset-bottom));
    width: auto;
  }

  .journey-actions {
    grid-template-columns: 1fr;
  }

  .time-grid {
    grid-template-columns: 1fr;
  }

  .status-strip {
    align-items: flex-start;
    flex-direction: column;
  }

  .chip-row {
    flex-wrap: nowrap;
    overflow-x: auto;
    padding-bottom: 4px;
    -webkit-overflow-scrolling: touch;
  }

  .chip {
    flex: 0 0 auto;
  }

  #section-panel-vote {
    padding-bottom: 126px;
  }

  .flow-form {
    padding-bottom: 96px;
  }

  .flow-submit {
    display: none;
  }

  .mobile-vote-dock {
    position: fixed;
    left: 12px;
    right: 12px;
    bottom: calc(10px + env(safe-area-inset-bottom));
    z-index: 180;
    border: 1px solid var(--line-strong);
    border-radius: 14px;
    background: rgba(248, 252, 255, 0.94);
    backdrop-filter: blur(9px);
    box-shadow: 0 16px 28px rgba(15, 23, 42, 0.2);
    padding: 10px;
    display: grid;
    gap: 8px;
  }

  .btn,
  .chip,
  .primary-nav-btn,
  .workflow-tab-btn,
  .ops-step-btn {
    min-height: 44px;
  }

  .header-actions {
    width: 100%;
  }

  .header-actions > * {
    flex: 1 1 auto;
  }

  .quick-links,
  .capability-list,
  .compact-checklist {
    display: grid;
    grid-template-columns: 1fr;
  }

  .guide-grid,
  .glossary-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 520px) {
  .brand-block .eyebrow {
    display: none;
  }

  .brand-block strong {
    font-size: 0.94rem;
  }

  .primary-nav-btn,
  .workflow-tab-btn,
  .topbar-tag {
    font-size: 0.8rem;
  }

  .mobile-vote-dock {
    left: 10px;
    right: 10px;
    padding: 8px;
  }

  .mobile-vote-actions {
    grid-template-columns: 1fr;
  }

  .action-row,
  .action-row.compact {
    grid-template-columns: 1fr;
  }

  .shell-topbar,
  .panel {
    padding: 13px;
  }

  .guide-dialog {
    padding: 16px;
    border-radius: 18px;
  }

  .ops-step-btn {
    padding: 9px;
  }

  .message-stack {
    left: 10px;
    right: 10px;
  }

  .journey-panel {
    left: 10px;
    right: 10px;
    bottom: calc(104px + env(safe-area-inset-bottom));
  }
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
</style>
