<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useVotePortal } from './composables/useVotePortal';
import type { SectionId, WorkspaceId } from './types/portal';

const portalState = useVotePortal();
const portal = reactive(portalState);
const credentialFileInput = portalState.credentialFileInput;
const showVoteTools = ref(false);

type OpsStepId = 'create' | 'decrypt' | 'publish' | 'result' | 'credential' | 'anchor';
const opsStep = ref<OpsStepId>('create');
const opsSteps: Array<{ id: OpsStepId; order: number; title: string; description: string }> = [
  { id: 'create', order: 1, title: '创建提案', description: '定义标题、选项和时间窗口' },
  { id: 'decrypt', order: 2, title: '初始化解密', description: '生成门限会话和份额' },
  { id: 'publish', order: 3, title: '发布结果', description: '提交 shares 并上链锚定' },
  { id: 'result', order: 4, title: '查询结果', description: '查看 tally 与总票数' },
  { id: 'credential', order: 5, title: '凭证状态', description: '查看哪些凭证已投票' },
  { id: 'anchor', order: 6, title: '审计锚点', description: '核验 anchor_hash 与 merkle_root' },
];

function setOpsStep(step: OpsStepId): void {
  opsStep.value = step;
}

function openVoteForProposal(id: string): void {
  portal.adoptProposal(id, 'vote');
}

function openOpsForProposal(id: string): void {
  portal.adoptProposal(id, 'ops');
  opsStep.value = 'decrypt';
}

function handleTabKeydown(event: KeyboardEvent, group: 'workspace' | 'section', currentId: string): void {
  const supported = ['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'Home', 'End'];
  if (!supported.includes(event.key)) {
    return;
  }
  event.preventDefault();

  const ids =
    group === 'workspace'
      ? portal.workspaces.map((item) => item.id)
      : portal.visibleSections.filter((item) => item && item.id).map((item) => item.id);
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

  if (group === 'workspace') {
    portal.setWorkspace(nextId as WorkspaceId);
  } else {
    portal.setActiveSection(nextId as SectionId);
  }

  const prefix = group === 'workspace' ? 'workspace-tab-' : 'section-tab-';
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
  <div class="app-shell">
    <header class="hero">
      <div class="hero-main">
        <p class="eyebrow">Nullifier Vote Console</p>
        <h1>匿名投票控制台</h1>
        <p class="hero-copy">将“用户投票流程”与“运营计票流程”分区，减少误操作与认知负担。</p>
        <div class="workspace-tabs" role="tablist" aria-label="工作区切换">
          <button
            v-for="item in portal.workspaces"
            :id="`workspace-tab-${item.id}`"
            :key="item.id"
            class="workspace-btn"
            role="tab"
            :aria-controls="`workspace-panel-${item.id}`"
            :aria-selected="portal.workspace === item.id"
            :tabindex="portal.workspace === item.id ? 0 : -1"
            :class="{ active: portal.workspace === item.id }"
            @click="portal.setWorkspace(item.id)"
            @keydown="handleTabKeydown($event, 'workspace', item.id)"
          >
            <strong>{{ item.title }}</strong>
            <small>{{ item.description }}</small>
          </button>
        </div>
      </div>

      <form class="endpoint-card" @submit.prevent="portal.checkHealth()">
        <label class="field" for="api-base-url">
          后端地址
          <input id="api-base-url" v-model.trim="portal.apiBaseUrl" type="url" inputmode="url" placeholder="http://localhost:8080" />
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
          <button class="btn btn-secondary" type="submit" :disabled="portal.healthState.loading">
            {{ portal.healthState.loading ? '检测中...' : '检测服务连接' }}
          </button>
        </div>
        <p class="health-line" :class="portal.healthState.ok ? 'ok' : 'bad'">
          {{ portal.healthState.text }}
          <span>最近检测：{{ portal.healthState.checkedAt || '尚未检测' }}</span>
        </p>
      </form>
    </header>

    <div v-if="portal.activeAlert" class="alert-banner" :class="portal.activeAlert.level" role="status" aria-live="polite">
      <div>
        <strong>{{ portal.activeAlert.title }}</strong>
        <p>{{ portal.activeAlert.detail }}</p>
      </div>
      <button class="btn btn-ghost" type="button" @click="portal.dismissActiveAlert">关闭</button>
    </div>

    <nav class="section-tabs" role="tablist" aria-label="功能分区导航">
      <button
        v-for="item in portal.visibleSections"
        :id="`section-tab-${item.id}`"
        :key="item.id"
        class="section-btn"
        role="tab"
        :aria-controls="`section-panel-${item.id}`"
        :aria-selected="portal.activeSection === item.id"
        :tabindex="portal.activeSection === item.id ? 0 : -1"
        :class="{ active: portal.activeSection === item.id }"
        @click="portal.setActiveSection(item.id)"
        @keydown="handleTabKeydown($event, 'section', item.id)"
      >
        <strong>{{ item.title }}</strong>
        <small>{{ item.description }}</small>
      </button>
    </nav>

    <main id="main-content" class="main-grid">
      <section :id="`workspace-panel-${portal.workspace}`" class="main-panel" role="region" :aria-labelledby="`workspace-tab-${portal.workspace}`">
        <section v-if="portal.activeSection === 'hall'" id="section-panel-hall" class="panel" role="tabpanel" aria-labelledby="section-tab-hall">
          <header class="panel-head">
            <div>
              <h2>投票大厅</h2>
              <p class="panel-tip">先选提案，再进入“参与投票”。</p>
            </div>
            <button class="btn btn-secondary" type="button" :disabled="portal.hallState.loading" @click="portal.loadOpenProposals()">
              {{ portal.hallState.loading ? '刷新中...' : '刷新提案' }}
            </button>
          </header>

          <p v-if="portal.hallState.proposals.length === 0" class="empty-state">暂无开放提案</p>

          <div v-else class="proposal-grid">
            <article v-for="item in portal.hallState.proposals" :key="item.id" class="proposal-card">
              <header class="proposal-head">
                <h3>{{ item.title || '未命名提案' }}</h3>
                <span class="status-badge">{{ item.status || '未知状态' }}</span>
              </header>
              <p class="card-desc">{{ item.description || '暂无描述' }}</p>
              <div v-if="item.options.length > 0">
                <p class="card-label">可选项</p>
                <ul class="option-list">
                  <li v-for="option in item.options" :key="option">{{ option }}</li>
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
                <button class="btn" type="button" @click="openVoteForProposal(item.id)">去投票</button>
                <button class="btn btn-secondary" type="button" @click="openOpsForProposal(item.id)">计票管理</button>
              </div>
            </article>
          </div>
        </section>

        <section
          v-else-if="portal.activeSection === 'identity'"
          id="section-panel-identity"
          class="panel"
          role="tabpanel"
          aria-labelledby="section-tab-identity"
        >
          <header class="panel-head">
            <div>
              <h2>我的身份</h2>
              <p class="panel-tip">这里展示的是提案内匿名凭证。当前版本需要管理员令牌代为签发，投票步骤会自动复用该凭证。</p>
            </div>
          </header>

          <article class="callout-card" :class="portal.adminTokenReady ? 'success' : 'warn'">
            <strong>{{ portal.adminTokenReady ? 'Admin Token 已就绪' : '领取前需要 Admin Token' }}</strong>
            <p>
              {{ portal.adminTokenReady ? '可以为当前 proposal 申领凭证。' : '请先在页面顶部填写 Admin Token，否则凭证签发会被后端拒绝。' }}
            </p>
          </article>

          <form class="form-grid" @submit.prevent="portal.issueCredential">
            <label class="field" for="identity-proposal-id">
              proposal_id<span class="required-mark" aria-hidden="true">*</span>
              <input id="identity-proposal-id" v-model.trim="portal.voteForm.proposalId" placeholder="先选择要发放凭证的提案" />
              <span class="field-help">提案内凭证只能用于对应 proposal 投票。</span>
            </label>
            <label class="field" for="identity-user-id">
              用户标识（user_id）<span class="required-mark" aria-hidden="true">*</span>
              <input
                id="identity-user-id"
                v-model.trim="portal.identityForm.userId"
                placeholder="例如 u-1001"
                :aria-invalid="portal.identityErrors.userId ? 'true' : 'false'"
                :aria-describedby="portal.identityErrors.userId ? 'identity-user-id-error' : undefined"
                autocomplete="username"
              />
              <span v-if="portal.identityErrors.userId" id="identity-user-id-error" class="field-error" role="alert" aria-live="assertive">{{
                portal.identityErrors.userId
              }}</span>
            </label>

            <label class="switch-field">
              <input v-model="portal.useCustomPubKey" type="checkbox" />
              手动输入公钥（高级模式）
            </label>

            <label v-if="portal.useCustomPubKey" class="field" for="identity-pub-key">
              公钥（Base64）<span class="required-mark" aria-hidden="true">*</span>
              <textarea
                id="identity-pub-key"
                v-model.trim="portal.identityForm.customPubKey"
                rows="3"
                :aria-invalid="portal.identityErrors.customPubKey ? 'true' : 'false'"
                :aria-describedby="portal.identityErrors.customPubKey ? 'identity-pub-key-error' : undefined"
              />
              <span
                v-if="portal.identityErrors.customPubKey"
                id="identity-pub-key-error"
                class="field-error"
                role="alert"
                aria-live="assertive"
                >{{ portal.identityErrors.customPubKey }}</span
              >
            </label>

            <article v-else class="inline-card">
              <p class="card-label">自动生成设备公钥</p>
              <code class="code">{{ portal.currentPubKey }}</code>
              <button class="btn btn-secondary" type="button" @click="portal.regeneratePubKey">重新生成公钥</button>
            </article>

            <div class="action-row">
              <button class="btn btn-secondary" type="button" @click="portal.openCredentialImport">导入凭证文件</button>
              <button class="btn btn-secondary" type="button" @click="portal.exportCredentialFile">导出凭证文件</button>
            </div>

            <input ref="credentialFileInput" class="hidden-input" type="file" accept="application/json" @change="portal.importCredentialFile" />

            <button class="btn" :disabled="portal.credentialLoading">
              {{ portal.credentialLoading ? '签发中...' : '管理员代发凭证' }}
            </button>
          </form>

          <article class="wallet-card" :class="{ ready: portal.walletReady }">
            <h3>凭证包</h3>
            <p v-if="!portal.walletReady" class="muted">尚未领取或导入凭证。</p>
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
        </section>

        <section v-else-if="portal.activeSection === 'vote'" id="section-panel-vote" class="panel" role="tabpanel" aria-labelledby="section-tab-vote">
          <header class="panel-head">
            <div>
              <h2>参与投票</h2>
              <p class="panel-tip">按步骤完成：先选提案和选项，再生成凭据，最后提交匿名选票。</p>
            </div>
            <div class="header-actions">
              <button class="btn btn-secondary" type="button" :disabled="portal.hallState.loading" @click="portal.loadOpenProposals(false)">
                同步提案
              </button>
              <button class="btn btn-secondary" type="button" :disabled="portal.voteStatusLoading || !portal.walletReady" @click="portal.fetchVoteStatus()">
                {{ portal.voteStatusLoading ? '查询中...' : '基于当前凭证校验状态' }}
              </button>
            </div>
          </header>

          <article class="callout-card info">
            <strong>状态查询不接受裸 user_id 枚举</strong>
            <p>系统会使用当前已加载的凭证材料校验归属后再返回 `has_voted`，只有持有这份凭证的人才能查询自己的状态。</p>
          </article>

          <div class="status-strip">
            <p>当前提案：{{ portal.voteForm.proposalId || '-' }}</p>
            <p>凭证状态：{{ portal.walletReady ? '已就绪' : '未就绪' }}</p>
            <span class="status-pill" :class="portal.walletReady ? 'done' : 'todo'">
              {{ portal.walletReady ? '可提交' : '先领取凭证' }}
            </span>
            <span v-if="portal.hasVoted !== null" class="status-pill" :class="portal.hasVoted ? 'done' : 'todo'">
              {{ portal.hasVoted ? '你已投票' : '你还未投票' }}
            </span>
          </div>

          <form class="form-grid flow-form" @submit.prevent="portal.submitVote">
            <article class="flow-step-card">
              <p class="step-kicker">步骤 1 · 选择提案</p>
              <label class="field" for="vote-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="vote-proposal-id"
                  v-model.trim="portal.voteForm.proposalId"
                  :aria-invalid="portal.voteErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.voteErrors.proposalId ? 'vote-proposal-id-error' : undefined"
                />
                <span v-if="portal.voteErrors.proposalId" id="vote-proposal-id-error" class="field-error" role="alert" aria-live="assertive">{{
                  portal.voteErrors.proposalId
                }}</span>
              </label>

              <div v-if="portal.hallState.proposals.length > 0" class="chip-row" aria-label="快速选择提案">
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

            <article class="flow-step-card">
              <p class="step-kicker">步骤 2 · 选择选项</p>
              <fieldset
                v-if="portal.selectedProposal && portal.selectedProposal.options.length > 0"
                class="option-box"
                :aria-describedby="portal.voteErrors.option ? 'vote-option-error' : undefined"
              >
                <legend>投票选项<span class="required-mark" aria-hidden="true">*</span></legend>
                <label v-for="option in portal.selectedProposal.options" :key="option" class="option-item">
                  <input v-model="portal.selectedVoteOption" :value="option" type="radio" />
                  <span>{{ option }}</span>
                </label>
                <span v-if="portal.voteErrors.option" id="vote-option-error" class="field-error" role="alert" aria-live="assertive">{{
                  portal.voteErrors.option
                }}</span>
              </fieldset>
              <p v-else class="muted">当前提案没有可选项，请先切换提案。</p>
            </article>

            <article class="flow-step-card">
              <div class="step-head-inline">
                <p class="step-kicker">步骤 3 · 生成投票凭据</p>
                <button class="btn btn-ghost" type="button" @click="showVoteTools = !showVoteTools">
                  {{ showVoteTools ? '收起低频工具' : '展开低频工具' }}
                </button>
              </div>

              <button class="btn btn-primary" type="button" :disabled="portal.voteLoading || !portal.walletReady" @click="portal.prepareBallotByOption">
                {{ portal.voteLoading ? '生成中...' : '按选项生成凭据（推荐）' }}
              </button>

              <label class="field" for="vote-ballot-json">
                投票凭据 JSON（可选）
                <textarea
                  id="vote-ballot-json"
                  v-model.trim="portal.voteForm.ballotJson"
                  rows="5"
                  placeholder='{"encrypted_vote":"...","proof":"...","nullifier":"...","public_witness":"..."}'
                  :aria-invalid="portal.voteErrors.ballotJson ? 'true' : 'false'"
                  :aria-describedby="portal.voteErrors.ballotJson ? 'vote-ballot-json-error' : undefined"
                />
                <span class="field-help">常规情况下只需点击“按选项生成凭据”，无需手写 JSON。</span>
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
                <p class="field-help">低频辅助工具：仅在调试或手工修复凭据时使用。</p>
                <div class="action-row compact">
                  <button class="btn btn-secondary" type="button" @click="portal.applySelectedOptionToBallot">写入选项</button>
                  <button class="btn btn-secondary" type="button" @click="portal.fillBallotTemplate">填充模板</button>
                  <button class="btn btn-secondary" type="button" @click="portal.importBallotJson">导入 JSON</button>
                  <button class="btn btn-secondary" type="button" @click="portal.voteForm.nullifier = portal.newNullifier()">新 nullifier</button>
                </div>
              </div>

              <label class="switch-field">
                <input v-model="portal.showAdvancedVoteFields" type="checkbox" />
                显示高级字段（仅排障时使用）
              </label>

              <div v-if="portal.showAdvancedVoteFields" class="advanced-grid">
                <label class="field">
                  encrypted_vote<span class="required-mark" aria-hidden="true">*</span>
                  <textarea
                    v-model.trim="portal.voteForm.encryptedVote"
                    rows="2"
                    :aria-invalid="portal.voteErrors.encryptedVote ? 'true' : 'false'"
                    :aria-describedby="portal.voteErrors.encryptedVote ? 'vote-encrypted-error' : undefined"
                  />
                  <span v-if="portal.voteErrors.encryptedVote" id="vote-encrypted-error" class="field-error" role="alert" aria-live="assertive">{{
                    portal.voteErrors.encryptedVote
                  }}</span>
                </label>
                <label class="field">
                  proof<span class="required-mark" aria-hidden="true">*</span>
                  <textarea
                    v-model.trim="portal.voteForm.proof"
                    rows="2"
                    :aria-invalid="portal.voteErrors.proof ? 'true' : 'false'"
                    :aria-describedby="portal.voteErrors.proof ? 'vote-proof-error' : undefined"
                  />
                  <span v-if="portal.voteErrors.proof" id="vote-proof-error" class="field-error" role="alert" aria-live="assertive">{{
                    portal.voteErrors.proof
                  }}</span>
                </label>
                <label class="field">
                  nullifier<span class="required-mark" aria-hidden="true">*</span>
                  <input
                    v-model.trim="portal.voteForm.nullifier"
                    :aria-invalid="portal.voteErrors.nullifier ? 'true' : 'false'"
                    :aria-describedby="portal.voteErrors.nullifier ? 'vote-nullifier-error' : undefined"
                  />
                  <span v-if="portal.voteErrors.nullifier" id="vote-nullifier-error" class="field-error" role="alert" aria-live="assertive">{{
                    portal.voteErrors.nullifier
                  }}</span>
                </label>
                <label class="field">
                  public_witness（可选）
                  <textarea v-model.trim="portal.voteForm.publicWitness" rows="2" />
                </label>
              </div>
            </article>

            <article class="flow-step-card flow-submit">
              <p class="step-kicker">步骤 4 · 提交选票</p>
              <button class="btn btn-primary" :disabled="portal.voteLoading || !portal.walletReady">
                {{ portal.voteLoading ? '提交中...' : '提交匿名选票' }}
              </button>
            </article>
          </form>

          <div class="mobile-vote-dock" role="region" aria-label="移动端快捷操作">
            <p class="mobile-vote-meta">
              <span>提案：{{ portal.voteForm.proposalId || '-' }}</span>
              <span>{{ portal.walletReady ? '凭证已就绪' : '先领取凭证' }}</span>
            </p>
            <div class="mobile-vote-actions">
              <button class="btn btn-secondary" type="button" :disabled="portal.voteLoading || !portal.walletReady" @click="portal.prepareBallotByOption">
                {{ portal.voteLoading ? '生成中...' : '生成凭据' }}
              </button>
              <button class="btn btn-primary" type="button" :disabled="portal.voteLoading || !portal.walletReady" @click="portal.submitVote">
                {{ portal.voteLoading ? '提交中...' : '立即提交' }}
              </button>
            </div>
          </div>
        </section>

        <section v-else id="section-panel-ops" class="panel" role="tabpanel" aria-labelledby="section-tab-ops">
          <header class="panel-head">
            <div>
              <h2>计票与审计</h2>
              <p class="panel-tip">按固定步骤完成运营流程，减少并行操作导致的误配置。</p>
            </div>
          </header>

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
            <form v-if="opsStep === 'create'" class="ops-card ops-card-main" @submit.prevent="portal.createProposal">
              <h3>步骤 1：创建提案</h3>
              <label class="field" for="ops-create-title">
                标题<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-create-title"
                  v-model.trim="portal.createProposalForm.title"
                  :aria-invalid="portal.createProposalErrors.title ? 'true' : 'false'"
                  :aria-describedby="portal.createProposalErrors.title ? 'ops-create-title-error' : undefined"
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
                <textarea id="ops-create-description" v-model.trim="portal.createProposalForm.description" rows="2" />
              </label>
              <label class="field" for="ops-create-options">
                选项（每行一个）<span class="required-mark" aria-hidden="true">*</span>
                <textarea
                  id="ops-create-options"
                  v-model.trim="portal.createProposalForm.optionsText"
                  rows="4"
                  :aria-invalid="portal.createProposalErrors.options ? 'true' : 'false'"
                  :aria-describedby="portal.createProposalErrors.options ? 'ops-create-options-error' : undefined"
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
                  starts_at<span class="required-mark" aria-hidden="true">*</span>
                  <input
                    id="ops-create-start"
                    v-model.trim="portal.createProposalForm.startsAtLocal"
                    type="datetime-local"
                    :aria-invalid="portal.createProposalErrors.startsAt ? 'true' : 'false'"
                    :aria-describedby="portal.createProposalErrors.startsAt ? 'ops-create-start-error' : undefined"
                  />
                  <span
                    v-if="portal.createProposalErrors.startsAt"
                    id="ops-create-start-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.createProposalErrors.startsAt }}</span
                  >
                </label>
                <label class="field" for="ops-create-end">
                  ends_at<span class="required-mark" aria-hidden="true">*</span>
                  <input
                    id="ops-create-end"
                    v-model.trim="portal.createProposalForm.endsAtLocal"
                    type="datetime-local"
                    :aria-invalid="portal.createProposalErrors.endsAt ? 'true' : 'false'"
                    :aria-describedby="portal.createProposalErrors.endsAt ? 'ops-create-end-error' : undefined"
                  />
                  <span
                    v-if="portal.createProposalErrors.endsAt"
                    id="ops-create-end-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.createProposalErrors.endsAt }}</span
                  >
                </label>
              </div>
              <div class="action-row">
                <button class="btn btn-secondary" type="button" @click="portal.setProposalWindow('active')">开放窗口</button>
                <button class="btn btn-secondary" type="button" @click="portal.setProposalWindow('closed')">结束窗口</button>
              </div>
              <div class="action-row">
                <button class="btn btn-primary" :disabled="portal.createProposalLoading">
                  {{ portal.createProposalLoading ? '创建中...' : '创建提案' }}
                </button>
                <button class="btn btn-ghost" type="button" @click="setOpsStep('decrypt')">下一步：初始化解密</button>
              </div>
            </form>

            <form v-else-if="opsStep === 'decrypt'" class="ops-card ops-card-main" @submit.prevent="portal.setupDecryptionSession">
              <h3>步骤 2：初始化解密会话</h3>
              <label class="field" for="ops-decrypt-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-decrypt-proposal-id"
                  v-model.trim="portal.decryptForm.proposalId"
                  :aria-invalid="portal.decryptErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.decryptErrors.proposalId ? 'ops-decrypt-proposal-id-error' : undefined"
                />
                <span
                  v-if="portal.decryptErrors.proposalId"
                  id="ops-decrypt-proposal-id-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.decryptErrors.proposalId }}</span
                >
              </label>
              <div class="time-grid">
                <label class="field" for="ops-decrypt-threshold">
                  threshold<span class="required-mark" aria-hidden="true">*</span>
                  <input
                    id="ops-decrypt-threshold"
                    v-model.number="portal.decryptForm.threshold"
                    type="number"
                    min="1"
                    inputmode="numeric"
                    :aria-invalid="portal.decryptErrors.threshold ? 'true' : 'false'"
                    :aria-describedby="portal.decryptErrors.threshold ? 'ops-decrypt-threshold-error' : undefined"
                  />
                  <span
                    v-if="portal.decryptErrors.threshold"
                    id="ops-decrypt-threshold-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.decryptErrors.threshold }}</span
                  >
                </label>
                <label class="field" for="ops-decrypt-trustees">
                  trustees<span class="required-mark" aria-hidden="true">*</span>
                  <input
                    id="ops-decrypt-trustees"
                    v-model.number="portal.decryptForm.trustees"
                    type="number"
                    min="1"
                    inputmode="numeric"
                    :aria-invalid="portal.decryptErrors.trustees ? 'true' : 'false'"
                    :aria-describedby="portal.decryptErrors.trustees ? 'ops-decrypt-trustees-error' : undefined"
                  />
                  <span
                    v-if="portal.decryptErrors.trustees"
                    id="ops-decrypt-trustees-error"
                    class="field-error"
                    role="alert"
                    aria-live="assertive"
                    >{{ portal.decryptErrors.trustees }}</span
                  >
                </label>
              </div>
              <button class="btn btn-primary" :disabled="portal.decryptLoading">{{ portal.decryptLoading ? '初始化中...' : '初始化' }}</button>
              <p v-if="portal.decryptPublicKey" class="code block-code">public_key: {{ portal.decryptPublicKey }}</p>
              <div class="action-row">
                <button class="btn btn-ghost" type="button" @click="setOpsStep('publish')">下一步：发布结果</button>
              </div>
            </form>

            <form v-else-if="opsStep === 'publish'" class="ops-card ops-card-main" @submit.prevent="portal.publishResult">
              <h3>步骤 3：发布结果</h3>
              <label class="field" for="ops-publish-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-publish-proposal-id"
                  v-model.trim="portal.publishForm.proposalId"
                  :aria-invalid="portal.publishErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.publishErrors.proposalId ? 'ops-publish-proposal-id-error' : undefined"
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
                shares<span class="required-mark" aria-hidden="true">*</span>
                <textarea
                  id="ops-publish-shares"
                  v-model.trim="portal.publishForm.sharesText"
                  rows="3"
                  :aria-invalid="portal.publishErrors.shares ? 'true' : 'false'"
                  :aria-describedby="portal.publishErrors.shares ? 'ops-publish-shares-error' : undefined"
                />
                <span
                  v-if="portal.publishErrors.shares"
                  id="ops-publish-shares-error"
                  class="field-error"
                  role="alert"
                  aria-live="assertive"
                  >{{ portal.publishErrors.shares }}</span
                >
              </label>
              <button class="btn btn-primary" :disabled="portal.publishLoading">{{ portal.publishLoading ? '发布中...' : '发布结果' }}</button>
              <p class="muted">{{ portal.publishedSummary || '发布后将在此显示摘要' }}</p>
              <div class="action-row">
                <button class="btn btn-ghost" type="button" @click="setOpsStep('result')">下一步：查询结果</button>
              </div>
            </form>

            <form v-else-if="opsStep === 'result'" class="ops-card ops-card-main" @submit.prevent="portal.fetchResult">
              <h3>步骤 4：查询结果</h3>
              <label class="field" for="ops-result-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-result-proposal-id"
                  v-model.trim="portal.resultForm.proposalId"
                  :aria-invalid="portal.resultErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.resultErrors.proposalId ? 'ops-result-proposal-id-error' : undefined"
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
              <button class="btn btn-primary" :disabled="portal.resultLoading">{{ portal.resultLoading ? '查询中...' : '查询' }}</button>
              <p class="muted">{{ portal.resultSummary || '结果摘要将在此显示' }}</p>
              <ul v-if="portal.resultTally.length > 0" class="result-list">
                <li v-for="item in portal.resultTally" :key="item.option">
                  <span>{{ item.option }}</span>
                  <strong>{{ item.count }}</strong>
                </li>
              </ul>
              <div class="action-row">
                <button class="btn btn-ghost" type="button" @click="setOpsStep('credential')">下一步：凭证状态</button>
              </div>
            </form>

            <form v-else-if="opsStep === 'credential'" class="ops-card ops-card-main" @submit.prevent="portal.fetchCredentialStatuses">
              <h3>步骤 5：凭证状态</h3>
              <label class="field" for="ops-credential-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-credential-proposal-id"
                  v-model.trim="portal.credentialStatusForm.proposalId"
                  :aria-invalid="portal.credentialStatusErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.credentialStatusErrors.proposalId ? 'ops-credential-proposal-id-error' : undefined"
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
              <button class="btn btn-primary" :disabled="portal.credentialStatusLoading">
                {{ portal.credentialStatusLoading ? '查询中...' : '查询凭证状态' }}
              </button>
              <p class="muted">这里只展示凭证是否已投票，不展示具体投票内容。</p>
              <div v-if="portal.credentialStatuses.length > 0" class="credential-status-list">
                <article v-for="item in portal.credentialStatuses" :key="item.credentialId" class="credential-status-card">
                  <p>
                    <strong>{{ item.userId }}</strong>
                    <span class="status-pill" :class="item.hasVoted ? 'done' : 'todo'">{{ item.hasVoted ? '已投票' : '未投票' }}</span>
                  </p>
                  <p class="code">credential_id: {{ item.credentialId }}</p>
                  <p class="code">user_pub_key: {{ item.userPubKey }}</p>
                  <p>issued_at: {{ item.issuedAt || '-' }}</p>
                </article>
              </div>
              <div class="action-row">
                <button class="btn btn-ghost" type="button" @click="setOpsStep('anchor')">下一步：审计锚点</button>
              </div>
            </form>

            <form v-else class="ops-card ops-card-main" @submit.prevent="portal.fetchAnchors">
              <h3>步骤 6：审计锚点</h3>
              <label class="field" for="ops-anchor-proposal-id">
                proposal_id<span class="required-mark" aria-hidden="true">*</span>
                <input
                  id="ops-anchor-proposal-id"
                  v-model.trim="portal.anchorForm.proposalId"
                  :aria-invalid="portal.anchorErrors.proposalId ? 'true' : 'false'"
                  :aria-describedby="portal.anchorErrors.proposalId ? 'ops-anchor-proposal-id-error' : undefined"
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
              <button class="btn btn-primary" :disabled="portal.anchorLoading">{{ portal.anchorLoading ? '查询中...' : '查询锚点' }}</button>
              <div v-if="portal.anchors.length > 0" class="anchor-list">
                <article v-for="item in portal.anchors" :key="item.id">
                  <p><strong>{{ item.id }}</strong> · {{ item.createdAt }}</p>
                  <p class="code">anchor_hash: {{ item.anchorHash }}</p>
                  <p class="code">merkle_root: {{ item.merkleRoot }}</p>
                </article>
              </div>
            </form>
          </div>
        </section>
      </section>

      <aside class="activity-panel" aria-label="系统动态">
        <header>
          <h2>系统动态</h2>
          <p class="muted">操作结果和接口反馈会同步显示在这里。</p>
        </header>

        <article v-if="portal.latestNotice" class="notice-card" :class="portal.latestNotice.level">
          <h3>{{ portal.latestNotice.title }}</h3>
          <p>{{ portal.latestNotice.detail }}</p>
          <time>{{ portal.latestNotice.at }}</time>
        </article>

        <p v-else class="muted">暂无动态</p>

        <div class="history-list">
          <article
            v-for="item in portal.noticeHistory.filter(Boolean)"
            :key="`${item.at}-${item.title}`"
            class="notice-card"
            :class="item.level"
          >
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
  width: min(1380px, 100%);
  margin: 0 auto;
  padding: 20px 18px 48px;
  display: grid;
  gap: 16px;
}

.hero {
  border: 1px solid var(--line);
  border-radius: 22px;
  padding: 20px;
  background:
    radial-gradient(circle at 88% 14%, rgba(33, 197, 94, 0.2), transparent 34%),
    linear-gradient(138deg, #0f172a 0%, #16243d 58%, #1f365f 100%);
  color: #f9fbff;
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(280px, 1fr);
  gap: 16px;
  box-shadow: 0 16px 32px rgba(2, 7, 23, 0.24);
}

.hero-main {
  display: grid;
  gap: 14px;
}

.eyebrow {
  margin: 0;
  font-size: 0.76rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: rgba(242, 247, 255, 0.78);
  font-weight: 700;
}

h1,
h2,
h3 {
  margin: 0;
}

h1 {
  font-size: clamp(1.56rem, 2.2vw, 2.38rem);
  line-height: 1.14;
  letter-spacing: -0.012em;
}

.hero-copy {
  margin: 0;
  max-width: 54ch;
  color: rgba(241, 247, 255, 0.9);
}

.workspace-tabs {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.workspace-btn {
  border: 1px solid rgba(210, 224, 244, 0.35);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.07);
  color: inherit;
  text-align: left;
  padding: 11px 12px;
  display: grid;
  gap: 5px;
  cursor: pointer;
  transition: border-color 180ms ease, background-color 180ms ease, box-shadow 180ms ease;
}

.workspace-btn:hover {
  border-color: rgba(248, 252, 255, 0.78);
  box-shadow: 0 8px 16px rgba(3, 8, 24, 0.22);
}

.workspace-btn.active {
  background: rgba(240, 249, 255, 0.2);
  border-color: rgba(253, 255, 255, 0.92);
}

.workspace-btn small {
  color: rgba(241, 247, 255, 0.82);
}

.endpoint-card {
  border: 1px solid rgba(211, 224, 245, 0.32);
  border-radius: 16px;
  padding: 12px;
  background: rgba(251, 254, 255, 0.1);
  display: grid;
  gap: 10px;
}

.health-line {
  margin: 0;
  font-size: 0.87rem;
  display: grid;
  gap: 3px;
}

.health-line.ok {
  color: #ddffe9;
}

.health-line.bad {
  color: #ffd9d3;
}

.health-line span {
  color: rgba(241, 247, 255, 0.82);
}

.alert-banner {
  border-radius: 15px;
  border: 1px solid var(--line);
  background: var(--surface);
  padding: 11px 14px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.alert-banner strong {
  display: block;
}

.alert-banner p {
  margin: 4px 0 0;
}

.alert-banner.success {
  border-color: rgba(22, 163, 74, 0.42);
}

.alert-banner.error {
  border-color: rgba(220, 74, 47, 0.48);
}

.alert-banner.info {
  border-color: rgba(59, 130, 246, 0.44);
}

.section-tabs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 10px;
}

.section-btn {
  border: 1px solid var(--line);
  border-radius: 13px;
  background: var(--surface);
  text-align: left;
  padding: 10px 12px;
  display: grid;
  gap: 4px;
  cursor: pointer;
  transition: border-color 180ms ease, box-shadow 180ms ease, background-color 180ms ease;
}

.section-btn small {
  color: var(--text-soft);
}

.section-btn:hover {
  border-color: var(--line-strong);
  background: var(--surface-strong);
}

.section-btn.active {
  border-color: var(--cta);
  box-shadow: 0 12px 26px rgba(22, 163, 74, 0.18);
  background: #f8fffb;
}

.main-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.55fr) minmax(290px, 0.95fr);
  gap: 14px;
  align-items: start;
}

.main-panel {
  display: grid;
}

.panel {
  border: 1px solid var(--line);
  border-radius: 18px;
  background: var(--surface);
  padding: 16px;
  display: grid;
  gap: 14px;
  box-shadow: 0 14px 28px rgba(15, 23, 42, 0.05);
}

.panel-head {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.panel-tip {
  margin: 6px 0 0;
  color: var(--text-soft);
}

.header-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.proposal-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 12px;
}

.proposal-card {
  border: 1px solid var(--line);
  border-radius: 15px;
  padding: 12px;
  background: var(--surface-strong);
  display: grid;
  gap: 9px;
  transition: border-color 180ms ease, box-shadow 180ms ease;
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

.option-list {
  margin: 6px 0 0;
  padding-left: 18px;
  display: grid;
  gap: 4px;
}

.meta-grid {
  margin: 0;
  display: grid;
  gap: 8px;
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
  transition: border-color 160ms ease, box-shadow 160ms ease, background-color 160ms ease;
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
.activity-panel,
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
  transition: border-color 160ms ease, background-color 160ms ease, box-shadow 160ms ease;
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
}

.status-strip p {
  margin: 0;
  font-size: 0.9rem;
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
  transition: border-color 160ms ease, box-shadow 160ms ease, background-color 160ms ease;
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

.activity-panel {
  display: grid;
  gap: 10px;
  position: sticky;
  top: 16px;
}

.activity-panel h2 {
  font-size: 1.06rem;
}

.notice-card {
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 10px;
  background: #ffffff;
  display: grid;
  gap: 5px;
}

.notice-card p {
  margin: 0;
  color: var(--text-soft);
}

.notice-card time {
  font-size: 0.78rem;
  color: var(--text-soft);
}

.notice-card.success {
  border-color: rgba(22, 163, 74, 0.42);
}

.notice-card.error {
  border-color: rgba(220, 74, 47, 0.45);
}

.notice-card.info {
  border-color: rgba(37, 99, 235, 0.42);
}

.history-list {
  display: grid;
  gap: 8px;
  max-height: 52vh;
  overflow: auto;
  padding-right: 2px;
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
  transition: box-shadow 160ms ease, opacity 160ms ease, filter 160ms ease, border-color 160ms ease;
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

:where(button, input, textarea, a, [role='tab']):focus-visible {
  outline: 3px solid var(--focus-ring);
  outline-offset: 2px;
}

.panel,
.activity-panel,
.alert-banner {
  animation: panel-enter 260ms ease both;
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

@media (max-width: 1240px) {
  .hero {
    grid-template-columns: 1fr;
  }

  .main-grid {
    grid-template-columns: 1fr;
  }

  .activity-panel {
    position: static;
  }

  .history-list {
    max-height: none;
  }
}

@media (max-width: 780px) {
  .app-shell {
    padding-inline: 12px;
    gap: 14px;
  }

  .workspace-tabs {
    grid-template-columns: 1fr;
  }

  .section-tabs {
    display: flex;
    overflow-x: auto;
    padding-bottom: 4px;
    scrollbar-width: thin;
  }

  .section-tabs {
    grid-template-columns: none;
  }

  .section-btn {
    min-width: 220px;
    flex: 0 0 auto;
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
  .section-btn,
  .workspace-btn,
  .ops-step-btn {
    min-height: 44px;
  }

  .header-actions {
    width: 100%;
  }

  .header-actions > * {
    flex: 1 1 auto;
  }
}

@media (max-width: 520px) {
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

  .panel {
    padding: 13px;
  }

  .ops-step-btn {
    padding: 9px;
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
