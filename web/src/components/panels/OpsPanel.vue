<script setup lang="ts">
import { nextTick, ref } from 'vue';
import type { OpsStepId } from '../../types/portal';
import type { PortalApi } from '../../types/portal-api';
import { OPS_STEPS } from '../../lib/portal-view';
import NoticeBanner from '../ui/NoticeBanner.vue';

const props = defineProps<{
  portal: PortalApi;
  step: OpsStepId;
}>();
const portal = props.portal;

const emit = defineEmits<{
  'update:step': [step: OpsStepId];
  proposalCreated: [];
  decryptReady: [];
  published: [];
  resultLoaded: [];
}>();

const steps = OPS_STEPS;

const optionRowsRef = ref<HTMLElement | null>(null);

const tallyMax = (): number =>
  portal.resultTally.reduce((max, item) => Math.max(max, item.count), 0);

function go(step: OpsStepId): void {
  emit('update:step', step);
}

/** 把焦点落到指定行的输入框上；行号越界时落到最后一行。 */
function focusOptionRow(index: number): void {
  const inputs =
    optionRowsRef.value?.querySelectorAll<HTMLInputElement>('input');
  if (!inputs || inputs.length === 0) {
    return;
  }
  const target = inputs[Math.min(index, inputs.length - 1)];
  target?.focus();
}

async function addOption(): Promise<void> {
  portal.addProposalOption();
  await nextTick();
  focusOptionRow(portal.createProposalForm.options.length - 1);
}

async function removeOption(index: number): Promise<void> {
  portal.removeProposalOption(index);
  await nextTick();
  focusOptionRow(index);
}

async function createProposal(): Promise<void> {
  const ok = await portal.createProposal();
  if (ok) {
    emit('proposalCreated');
    go('decrypt');
  }
}

async function setupDecryption(): Promise<void> {
  const ok = await portal.setupDecryptionSession();
  if (ok) {
    emit('decryptReady');
    go('publish');
  }
}

async function publish(): Promise<void> {
  const ok = await portal.publishResult();
  if (ok) {
    emit('published');
    go('result');
  }
}

async function loadResult(): Promise<void> {
  const ok = await portal.fetchResult();
  if (ok) {
    emit('resultLoaded');
  }
}
</script>

<template>
  <div class="stack" style="--stack-gap: var(--s-5)">
    <nav class="stepper" aria-label="运营步骤">
      <button
        v-for="(item, index) in steps"
        :key="item.id"
        class="stepper-item"
        :class="{ 'is-active': step === item.id }"
        type="button"
        :aria-current="step === item.id ? 'step' : undefined"
        @click="go(item.id)"
      >
        <span class="stepper-index" aria-hidden="true">{{ index + 1 }}</span>
        <span class="stepper-label">{{ item.title }}</span>
      </button>
    </nav>

    <NoticeBanner
      v-if="!portal.adminTokenReady"
      tone="warn"
      title="未配置 Admin Token"
      text="创建提案、初始化解密、发布结果、查询凭证状态都属于管理面。"
    >
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        @click="portal.setActiveSection('setup')"
      >
        去配置
      </button>
    </NoticeBanner>

    <!-- 步骤 1 -->
    <form
      v-if="step === 'create'"
      id="guide-anchor-admin-create"
      class="card"
      @submit.prevent="createProposal"
    >
      <div class="card-head">
        <h2 class="card-title">创建提案</h2>
        <span class="pill">开始时间需在将来</span>
      </div>

      <label class="field" for="ops-create-title">
        <span class="field-label">
          标题
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-create-title"
          v-model.trim="portal.createProposalForm.title"
          :aria-invalid="portal.createProposalErrors.title ? 'true' : 'false'"
        />
        <span
          v-if="portal.createProposalErrors.title"
          class="field-error"
          role="alert"
        >
          {{ portal.createProposalErrors.title }}
        </span>
      </label>

      <label class="field" for="ops-create-description">
        <span class="field-label">描述</span>
        <textarea
          id="ops-create-description"
          v-model.trim="portal.createProposalForm.description"
          rows="2"
        />
      </label>

      <div
        class="field"
        role="group"
        aria-labelledby="ops-create-options-label"
      >
        <span class="field-label" id="ops-create-options-label">
          选项
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <div ref="optionRowsRef" class="option-rows">
          <div
            v-for="(option, index) in portal.createProposalForm.options"
            :key="index"
            class="option-row"
          >
            <input
              :id="`ops-create-option-${index}`"
              v-model="portal.createProposalForm.options[index]"
              type="text"
              :placeholder="`选项 ${index + 1}`"
              :aria-label="`选项 ${index + 1}`"
              :aria-invalid="
                portal.createProposalErrors.options ? 'true' : 'false'
              "
            />
            <button
              class="btn btn--ghost btn--sm"
              type="button"
              :aria-label="`移除选项 ${index + 1}`"
              :disabled="!portal.canRemoveProposalOption()"
              @click="removeOption(index)"
            >
              移除
            </button>
          </div>
        </div>
        <button
          class="btn btn--secondary btn--sm option-add"
          type="button"
          @click="addOption"
        >
          + 添加选项
        </button>
        <span
          v-if="portal.createProposalErrors.options"
          class="field-error"
          role="alert"
        >
          {{ portal.createProposalErrors.options }}
        </span>
      </div>

      <div class="grid-2">
        <label class="field" for="ops-create-start">
          <span class="field-label">
            starts_at
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="ops-create-start"
            v-model.trim="portal.createProposalForm.startsAtLocal"
            type="datetime-local"
            :aria-invalid="
              portal.createProposalErrors.startsAt ? 'true' : 'false'
            "
          />
          <span
            v-if="portal.createProposalErrors.startsAt"
            class="field-error"
            role="alert"
          >
            {{ portal.createProposalErrors.startsAt }}
          </span>
        </label>

        <label class="field" for="ops-create-end">
          <span class="field-label">
            ends_at
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="ops-create-end"
            v-model.trim="portal.createProposalForm.endsAtLocal"
            type="datetime-local"
            :aria-invalid="
              portal.createProposalErrors.endsAt ? 'true' : 'false'
            "
          />
          <span
            v-if="portal.createProposalErrors.endsAt"
            class="field-error"
            role="alert"
          >
            {{ portal.createProposalErrors.endsAt }}
          </span>
        </label>
      </div>

      <div class="row-between">
        <button
          class="btn btn--secondary btn--sm"
          type="button"
          @click="portal.setProposalWindow('scheduled')"
        >
          填入推荐时间
        </button>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.createProposalLoading"
        >
          {{ portal.createProposalLoading ? '创建中…' : '创建提案' }}
        </button>
      </div>
    </form>

    <!-- 步骤 2 -->
    <form
      v-else-if="step === 'decrypt'"
      id="guide-anchor-admin-decrypt"
      class="card"
      @submit.prevent="setupDecryption"
    >
      <div class="card-head">
        <h2 class="card-title">初始化解密会话</h2>
        <span class="pill">投票开始前完成</span>
      </div>

      <label class="field" for="ops-decrypt-proposal-id">
        <span class="field-label">
          proposal_id
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-decrypt-proposal-id"
          v-model.trim="portal.decryptForm.proposalId"
          list="ops-proposal-options"
          :aria-invalid="portal.decryptErrors.proposalId ? 'true' : 'false'"
        />
        <span
          v-if="portal.decryptErrors.proposalId"
          class="field-error"
          role="alert"
        >
          {{ portal.decryptErrors.proposalId }}
        </span>
      </label>

      <div class="grid-2">
        <label class="field" for="ops-decrypt-threshold">
          <span class="field-label">
            threshold
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="ops-decrypt-threshold"
            v-model.number="portal.decryptForm.threshold"
            type="number"
            min="1"
            inputmode="numeric"
            :aria-invalid="portal.decryptErrors.threshold ? 'true' : 'false'"
          />
          <span
            v-if="portal.decryptErrors.threshold"
            class="field-error"
            role="alert"
          >
            {{ portal.decryptErrors.threshold }}
          </span>
        </label>

        <label class="field" for="ops-decrypt-trustees">
          <span class="field-label">
            trustees
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="ops-decrypt-trustees"
            v-model.number="portal.decryptForm.trustees"
            type="number"
            min="1"
            inputmode="numeric"
            :aria-invalid="portal.decryptErrors.trustees ? 'true' : 'false'"
          />
          <span
            v-if="portal.decryptErrors.trustees"
            class="field-error"
            role="alert"
          >
            {{ portal.decryptErrors.trustees }}
          </span>
        </label>
      </div>

      <p class="field-hint">
        threshold 是至少需要几份份额才能出结果，trustees 是总共拆成几份。
      </p>

      <div class="row-between">
        <span class="card-sub subtle">
          {{
            portal.decryptShares.length > 0
              ? `已生成 ${portal.decryptShares.length} 份份额`
              : '尚未生成份额'
          }}
        </span>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.decryptLoading"
        >
          {{ portal.decryptLoading ? '初始化中…' : '初始化' }}
        </button>
      </div>

      <p v-if="portal.decryptPublicKey" class="mono">
        public_key: {{ portal.decryptPublicKey }}
      </p>
    </form>

    <!-- 步骤 3 -->
    <form
      v-else-if="step === 'publish'"
      id="guide-anchor-admin-publish"
      class="card"
      @submit.prevent="publish"
    >
      <div class="card-head">
        <h2 class="card-title">发布结果</h2>
        <span class="pill">投票结束后执行</span>
      </div>

      <NoticeBanner
        v-if="portal.decryptShares.length === 0"
        tone="warn"
        title="还没有解密份额"
        text="shares 由「初始化解密」生成，请先完成上一步。"
      >
        <button
          class="btn btn--secondary btn--sm"
          type="button"
          @click="go('decrypt')"
        >
          去初始化
        </button>
      </NoticeBanner>

      <label class="field" for="ops-publish-proposal-id">
        <span class="field-label">
          proposal_id
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-publish-proposal-id"
          v-model.trim="portal.publishForm.proposalId"
          list="ops-proposal-options"
          :aria-invalid="portal.publishErrors.proposalId ? 'true' : 'false'"
        />
        <span
          v-if="portal.publishErrors.proposalId"
          class="field-error"
          role="alert"
        >
          {{ portal.publishErrors.proposalId }}
        </span>
      </label>

      <label class="field" for="ops-publish-shares">
        <span class="field-label">
          shares
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <textarea
          id="ops-publish-shares"
          v-model.trim="portal.publishForm.sharesText"
          rows="3"
          :aria-invalid="portal.publishErrors.shares ? 'true' : 'false'"
        />
        <span
          v-if="portal.publishErrors.shares"
          class="field-error"
          role="alert"
        >
          {{ portal.publishErrors.shares }}
        </span>
      </label>

      <div class="row-between">
        <span class="card-sub subtle">
          {{ portal.publishedSummary || '发布后显示摘要' }}
        </span>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.publishLoading"
        >
          {{ portal.publishLoading ? '发布中…' : '发布结果' }}
        </button>
      </div>
    </form>

    <!-- 步骤 4 -->
    <form
      v-else-if="step === 'result'"
      id="guide-anchor-admin-result"
      class="card"
      @submit.prevent="loadResult"
    >
      <div class="card-head">
        <h2 class="card-title">查询结果</h2>
        <span class="pill">已发布的最终票数</span>
      </div>

      <label class="field" for="ops-result-proposal-id">
        <span class="field-label">
          proposal_id
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-result-proposal-id"
          v-model.trim="portal.resultForm.proposalId"
          list="ops-proposal-options"
          :aria-invalid="portal.resultErrors.proposalId ? 'true' : 'false'"
        />
        <span
          v-if="portal.resultErrors.proposalId"
          class="field-error"
          role="alert"
        >
          {{ portal.resultErrors.proposalId }}
        </span>
      </label>

      <div class="row-between">
        <span class="card-sub subtle">
          {{ portal.resultSummary || '查询后显示摘要' }}
        </span>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.resultLoading"
        >
          {{ portal.resultLoading ? '查询中…' : '查询' }}
        </button>
      </div>

      <ul v-if="portal.resultTally.length > 0" class="tally">
        <li
          v-for="item in portal.resultTally"
          :key="item.option"
          class="tally-row"
        >
          <div class="row-between">
            <span class="tally-option">{{ item.option }}</span>
            <strong class="tally-count">{{ item.count }}</strong>
          </div>
          <span
            class="tally-bar"
            :style="{
              width: `${tallyMax() > 0 ? (item.count / tallyMax()) * 100 : 0}%`,
            }"
          ></span>
        </li>
      </ul>
    </form>

    <!-- 步骤 5 -->
    <form
      v-else-if="step === 'credential'"
      class="card"
      @submit.prevent="portal.fetchCredentialStatuses()"
    >
      <div class="card-head">
        <h2 class="card-title">凭证状态</h2>
        <span class="pill">只看是否投票</span>
      </div>

      <label class="field" for="ops-credential-proposal-id">
        <span class="field-label">
          proposal_id
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-credential-proposal-id"
          v-model.trim="portal.credentialStatusForm.proposalId"
          list="ops-proposal-options"
          :aria-invalid="
            portal.credentialStatusErrors.proposalId ? 'true' : 'false'
          "
        />
        <span
          v-if="portal.credentialStatusErrors.proposalId"
          class="field-error"
          role="alert"
        >
          {{ portal.credentialStatusErrors.proposalId }}
        </span>
      </label>

      <div class="row-between">
        <span class="card-sub subtle">
          共 {{ portal.credentialStatuses.length }} 张凭证
        </span>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.credentialStatusLoading"
        >
          {{ portal.credentialStatusLoading ? '查询中…' : '查询' }}
        </button>
      </div>

      <ul v-if="portal.credentialStatuses.length > 0" class="credential-list">
        <li
          v-for="item in portal.credentialStatuses"
          :key="item.credentialId"
          class="credential-row"
        >
          <div class="row-between">
            <strong>{{ item.userId }}</strong>
            <span
              class="pill"
              :class="item.hasVoted ? 'pill--ok' : 'pill--warn'"
            >
              {{ item.hasVoted ? '已投票' : '未投票' }}
            </span>
          </div>
          <span class="mono subtle"
            >credential_id: {{ item.credentialId }}</span
          >
          <span class="card-sub subtle"
            >issued_at: {{ item.issuedAt || '-' }}</span
          >
        </li>
      </ul>
    </form>

    <!-- 步骤 6 -->
    <form v-else class="card" @submit.prevent="portal.fetchAnchors()">
      <div class="card-head">
        <h2 class="card-title">审计锚点</h2>
        <span class="pill">结果校验指纹</span>
      </div>

      <label class="field" for="ops-anchor-proposal-id">
        <span class="field-label">
          proposal_id
          <span class="field-required" aria-hidden="true">*</span>
        </span>
        <input
          id="ops-anchor-proposal-id"
          v-model.trim="portal.anchorForm.proposalId"
          list="ops-proposal-options"
          :aria-invalid="portal.anchorErrors.proposalId ? 'true' : 'false'"
        />
        <span
          v-if="portal.anchorErrors.proposalId"
          class="field-error"
          role="alert"
        >
          {{ portal.anchorErrors.proposalId }}
        </span>
      </label>

      <div class="row-between">
        <span class="card-sub subtle">
          共 {{ portal.anchors.length }} 个锚点
        </span>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.anchorLoading"
        >
          {{ portal.anchorLoading ? '查询中…' : '查询' }}
        </button>
      </div>

      <ul v-if="portal.anchors.length > 0" class="credential-list">
        <li
          v-for="item in portal.anchors"
          :key="item.id"
          class="credential-row"
        >
          <div class="row-between">
            <strong>{{ item.id }}</strong>
            <span class="card-sub subtle">{{ item.createdAt }}</span>
          </div>
          <span class="mono subtle">anchor_hash: {{ item.anchorHash }}</span>
          <span class="mono subtle">merkle_root: {{ item.merkleRoot }}</span>
        </li>
      </ul>
    </form>

    <datalist id="ops-proposal-options">
      <option
        v-for="item in portal.opsProposalsState.proposals"
        :key="item.id"
        :value="item.id"
      >
        {{ item.title || item.id }}
      </option>
    </datalist>
  </div>
</template>

<style scoped>
.tally {
  display: grid;
  gap: var(--s-3);
  list-style: none;
}

.tally-row {
  display: grid;
  gap: var(--s-1);
}

.tally-option {
  font-size: var(--t-label);
  font-weight: 600;
}

.tally-count {
  font-size: var(--t-label);
}

.tally-bar {
  display: block;
  height: 6px;
  border-radius: var(--r-pill);
  background: var(--c-brand);
  min-width: 2px;
}

.credential-list {
  display: grid;
  gap: var(--s-2);
  list-style: none;
}

.credential-row {
  display: grid;
  gap: var(--s-1);
  border: 1px solid var(--c-border);
  border-radius: var(--r-md);
  background: var(--c-surface-2);
  padding: var(--s-3);
}
</style>
