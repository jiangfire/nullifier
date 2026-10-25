<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue';
import { useVotePortal } from './composables/useVotePortal';
import {
  formatProposalTag,
  PAGE_TITLES,
  resolveNextGuideIndex,
  resolveWorkspaceEntry,
  resolveWorkspaceForSection,
} from './lib/portal-view';
import AppSidebar from './components/AppSidebar.vue';
import AppTopbar from './components/AppTopbar.vue';
import GuideOverlay from './components/GuideOverlay.vue';
import JourneyBar from './components/JourneyBar.vue';
import NoticeStack from './components/NoticeStack.vue';
import HallPanel from './components/panels/HallPanel.vue';
import IdentityPanel from './components/panels/IdentityPanel.vue';
import OpsPanel from './components/panels/OpsPanel.vue';
import OverviewPanel from './components/panels/OverviewPanel.vue';
import ReferencePanel from './components/panels/ReferencePanel.vue';
import SetupPanel from './components/panels/SetupPanel.vue';
import VotePanel from './components/panels/VotePanel.vue';
import type { OpsStepId, SectionId, WorkspaceId } from './types/portal';

const portalState = useVotePortal();
const portal = reactive(portalState);
const credentialFileInput = portalState.credentialFileInput;
const eligibilityFileInput = portalState.eligibilityFileInput;

type JourneyId = 'user' | 'admin';

type GuideStep = {
  key: string;
  title: string;
  detail: string;
  workspace: WorkspaceId;
  section: SectionId;
  opsStep?: OpsStepId;
  anchor: string;
};

const navOpen = ref(false);
const opsStep = ref<OpsStepId>('create');
const guidedPath = ref<JourneyId | null>(null);
const guidedStepIndex = ref(0);

const pageTitles: Record<SectionId, string> = PAGE_TITLES;

const userGuideSteps: GuideStep[] = [
  {
    key: 'user-hall',
    title: '选一场提案',
    detail: '在投票大厅里选中要参与的提案。',
    workspace: 'user',
    section: 'hall',
    anchor: 'guide-anchor-user-hall',
  },
  {
    key: 'user-identity',
    title: '领取匿名凭证',
    detail: '凭证按提案发放，只对当前提案有效。',
    workspace: 'user',
    section: 'identity',
    anchor: 'guide-anchor-user-identity',
  },
  {
    key: 'user-vote-prepare',
    title: '选择选项并生成凭据',
    detail: '选好候选项后点「按选项生成凭据」。',
    workspace: 'user',
    section: 'vote',
    anchor: 'guide-anchor-user-vote-prepare',
  },
  {
    key: 'user-vote-submit',
    title: '提交匿名选票',
    detail: '确认凭据已生成后提交。',
    workspace: 'user',
    section: 'vote',
    anchor: 'guide-anchor-user-vote-submit',
  },
];

const adminGuideSteps: GuideStep[] = [
  {
    key: 'admin-create',
    title: '创建未开始的提案',
    detail: '开始时间放在未来，先拿到 proposal_id。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'create',
    anchor: 'guide-anchor-admin-create',
  },
  {
    key: 'admin-decrypt',
    title: '初始化解密会话',
    detail: '投票开始前完成，生成 public_key 与 shares。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'decrypt',
    anchor: 'guide-anchor-admin-decrypt',
  },
  {
    key: 'admin-publish',
    title: '投票结束后发布结果',
    detail: '提交上一步的 shares，数量达到 threshold 即可出结果。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'publish',
    anchor: 'guide-anchor-admin-publish',
  },
  {
    key: 'admin-result',
    title: '查询最终票数',
    detail: '确认整轮计票已完成。',
    workspace: 'admin',
    section: 'ops',
    opsStep: 'result',
    anchor: 'guide-anchor-admin-result',
  },
];

const guideSteps = computed<GuideStep[]>(() =>
  guidedPath.value === 'admin'
    ? adminGuideSteps
    : guidedPath.value === 'user'
      ? userGuideSteps
      : [],
);

const currentStep = computed<GuideStep | null>(
  () => guideSteps.value[guidedStepIndex.value] ?? null,
);

const pageTitle = computed(() => pageTitles[portal.activeSection]);

const shortProposalTag = computed(() =>
  formatProposalTag(portal.selectedProposalId),
);

function focusAnchor(anchor: string): void {
  document
    .querySelectorAll('.is-guide-focus')
    .forEach((element) => element.classList.remove('is-guide-focus'));
  const target = document.getElementById(anchor);
  if (target) {
    target.classList.add('is-guide-focus');
    target.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}

async function activateStep(): Promise<void> {
  const step = currentStep.value;
  if (!step) {
    return;
  }
  navOpen.value = false;
  portal.setWorkspace(step.workspace);
  portal.setActiveSection(step.section);
  if (step.opsStep) {
    opsStep.value = step.opsStep;
  }
  await nextTick();
  focusAnchor(step.anchor);
}

async function beginJourney(role: JourneyId): Promise<void> {
  portal.closeGuideMode();
  guidedPath.value = role;
  guidedStepIndex.value = 0;
  await activateStep();
}

function stopJourney(): void {
  guidedPath.value = null;
}

async function nextStep(): Promise<void> {
  if (!currentStep.value) {
    return;
  }
  const nextIndex = resolveNextGuideIndex(
    guidedStepIndex.value,
    guideSteps.value.length,
  );
  if (nextIndex === null) {
    stopJourney();
    return;
  }
  guidedStepIndex.value = nextIndex;
  await activateStep();
}

async function prevStep(): Promise<void> {
  if (!currentStep.value || guidedStepIndex.value === 0) {
    return;
  }
  guidedStepIndex.value -= 1;
  await activateStep();
}

function navigate(section: SectionId): void {
  const owner = resolveWorkspaceForSection(section);
  if (owner) {
    portal.setWorkspace(owner);
  }
  portal.setActiveSection(section);
  navOpen.value = false;
}

function switchWorkspace(next: WorkspaceId): void {
  portal.setWorkspace(next);
  portal.setActiveSection(resolveWorkspaceEntry(next));
  navOpen.value = false;
}

function handleIssued(): void {
  if (
    guidedPath.value === 'user' &&
    currentStep.value?.key === 'user-identity'
  ) {
    guidedStepIndex.value = 2;
    void activateStep();
  }
}

function handleBallotReady(): void {
  if (
    guidedPath.value === 'user' &&
    currentStep.value?.key === 'user-vote-prepare'
  ) {
    guidedStepIndex.value = 3;
    void activateStep();
  }
}

function handleSubmitted(): void {
  if (guidedPath.value === 'user') {
    stopJourney();
  }
}

function handleProposalCreated(): void {
  if (
    guidedPath.value === 'admin' &&
    currentStep.value?.key === 'admin-create'
  ) {
    guidedStepIndex.value = 1;
    void activateStep();
  }
}

function handleDecryptReady(): void {
  if (
    guidedPath.value === 'admin' &&
    currentStep.value?.key === 'admin-decrypt'
  ) {
    guidedStepIndex.value = 2;
    void activateStep();
  }
}

function handlePublished(): void {
  if (
    guidedPath.value === 'admin' &&
    currentStep.value?.key === 'admin-publish'
  ) {
    guidedStepIndex.value = 3;
    void activateStep();
  }
}

function handleResultLoaded(): void {
  if (guidedPath.value === 'admin') {
    stopJourney();
  }
}
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主内容</a>

  <GuideOverlay
    :open="portal.guideModeVisible"
    @start="beginJourney"
    @dismiss="portal.closeGuideMode()"
  />

  <NoticeStack :notices="portal.notices" @dismiss="portal.dismissNotice" />

  <div class="shell">
    <AppSidebar
      :workspace="portal.workspace"
      :active-section="portal.activeSection"
      :open="navOpen"
      :health-ok="portal.healthState.ok"
      :health-text="portal.healthState.text"
      :admin-token-ready="portal.adminTokenReady"
      @navigate="navigate"
      @switch="switchWorkspace"
      @close="navOpen = false"
      @guide="portal.guideModeVisible = true"
    />

    <button
      v-if="navOpen"
      class="shell-scrim"
      type="button"
      aria-label="关闭导航"
      @click="navOpen = false"
    ></button>

    <div class="shell-main">
      <AppTopbar :title="pageTitle" @toggle-nav="navOpen = !navOpen">
        <template #meta>
          <span
            class="pill"
            :class="portal.selectedProposalId ? 'pill--info' : ''"
          >
            {{ shortProposalTag }}
          </span>
          <span v-if="portal.walletReady" class="pill pill--ok"
            >凭证已就绪</span
          >
          <span v-if="portal.adminTokenReady" class="pill">管理权限</span>
        </template>
      </AppTopbar>

      <main id="main-content" class="shell-content">
        <JourneyBar
          v-if="currentStep"
          :path-label="guidedPath === 'admin' ? '运营路径' : '投票路径'"
          :step-title="currentStep.title"
          :step-detail="currentStep.detail"
          :index="guidedStepIndex"
          :total="guideSteps.length"
          :can-go-back="guidedStepIndex > 0"
          @prev="prevStep"
          @next="nextStep"
          @exit="stopJourney"
        />

        <OverviewPanel
          v-if="portal.activeSection === 'overview'"
          :portal="portal"
        />
        <SetupPanel
          v-else-if="portal.activeSection === 'setup'"
          :portal="portal"
        />
        <ReferencePanel v-else-if="portal.activeSection === 'guide'" />
        <HallPanel
          v-else-if="portal.activeSection === 'hall'"
          :portal="portal"
        />
        <IdentityPanel
          v-else-if="portal.activeSection === 'identity'"
          :portal="portal"
          @issued="handleIssued"
        />
        <VotePanel
          v-else-if="portal.activeSection === 'vote'"
          :portal="portal"
          @ballot-ready="handleBallotReady"
          @submitted="handleSubmitted"
        />
        <OpsPanel
          v-else
          :portal="portal"
          :step="opsStep"
          @update:step="opsStep = $event"
          @proposal-created="handleProposalCreated"
          @decrypt-ready="handleDecryptReady"
          @published="handlePublished"
          @result-loaded="handleResultLoaded"
        />
      </main>
    </div>
  </div>

  <input
    ref="credentialFileInput"
    class="hidden-input"
    type="file"
    accept="application/json"
    @change="portal.importCredentialFile"
  />
  <input
    ref="eligibilityFileInput"
    class="hidden-input"
    type="file"
    accept="application/json"
    @change="portal.importEligibilityPackage"
  />
</template>
