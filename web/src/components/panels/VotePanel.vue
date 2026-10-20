<script setup lang="ts">
import type { PortalApi } from '../../types/portal-api';
import NoticeBanner from '../ui/NoticeBanner.vue';

const props = defineProps<{ portal: PortalApi }>();
const portal = props.portal;

const emit = defineEmits<{
  ballotReady: [];
  submitted: [];
}>();

async function prepare(): Promise<void> {
  const ok = await portal.prepareBallotByOption();
  if (ok) {
    emit('ballotReady');
  }
}

async function submit(): Promise<void> {
  const ok = await portal.submitVote();
  if (ok) {
    emit('submitted');
  }
}

async function checkStatus(): Promise<void> {
  await portal.fetchVoteStatus();
}
</script>

<template>
  <div class="stack" style="--stack-gap: var(--s-5)">
    <NoticeBanner
      v-if="!portal.walletReady"
      tone="warn"
      title="尚未领取匿名凭证"
      text="凭证按提案发放，领取后才能生成投票凭据。"
    >
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        @click="portal.setActiveSection('identity')"
      >
        去领取
      </button>
    </NoticeBanner>

    <NoticeBanner
      v-else-if="portal.hasVoted !== null"
      :tone="portal.hasVoted ? 'brand' : 'info'"
      :title="portal.hasVoted ? '你已投过这一票' : '你还没有投票'"
      :text="
        portal.hasVoted
          ? '同一张凭证只能提交一次。'
          : '选好选项后生成凭据并提交。'
      "
    />

    <ol class="flow">
      <li class="flow-step">
        <span class="flow-index" aria-hidden="true">1</span>
        <div class="flow-body">
          <h2 class="card-title">选择提案</h2>
          <label class="field" for="vote-proposal-id">
            <span class="field-label">
              proposal_id
              <span class="field-required" aria-hidden="true">*</span>
            </span>
            <input
              id="vote-proposal-id"
              v-model.trim="portal.voteForm.proposalId"
              :aria-invalid="portal.voteErrors.proposalId ? 'true' : 'false'"
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
            >
              {{ portal.voteErrors.proposalId }}
            </span>
          </label>
          <div
            v-if="portal.hallState.proposals.length > 0"
            class="row"
            aria-label="快速选择提案"
          >
            <button
              v-for="item in portal.hallState.proposals"
              :key="item.id"
              class="chip"
              type="button"
              :aria-pressed="portal.voteForm.proposalId === item.id"
              @click="portal.adoptProposal(item.id)"
            >
              {{ item.title || item.id }}
            </button>
          </div>
          <div class="row">
            <button
              class="btn btn--secondary btn--sm"
              type="button"
              :disabled="
                portal.voteStatusLoading ||
                !portal.walletReady ||
                portal.voteForm.proposalId.trim() === ''
              "
              @click="checkStatus"
            >
              {{ portal.voteStatusLoading ? '查询中…' : '查询投票状态' }}
            </button>
          </div>
          <p
            v-if="portal.voteStatusError"
            class="field-error"
            role="alert"
          >
            {{ portal.voteStatusError }}
          </p>
        </div>
      </li>

      <li class="flow-step">
        <span class="flow-index" aria-hidden="true">2</span>
        <div class="flow-body">
          <h2 class="card-title">选择选项</h2>
          <fieldset
            v-if="
              portal.selectedProposal &&
              portal.selectedProposal.options.length > 0
            "
            class="stack stack-sm"
            :aria-describedby="
              portal.voteErrors.option ? 'vote-option-error' : undefined
            "
          >
            <legend class="field-label">
              投票选项
              <span class="field-required" aria-hidden="true">*</span>
            </legend>
            <label
              v-for="option in portal.selectedProposal.options"
              :key="option"
              class="option-radio"
            >
              <input
                v-model="portal.selectedVoteOption"
                :value="option"
                type="radio"
                name="vote-option"
              />
              <span>{{ option }}</span>
            </label>
            <span
              v-if="portal.voteErrors.option"
              id="vote-option-error"
              class="field-error"
              role="alert"
            >
              {{ portal.voteErrors.option }}
            </span>
          </fieldset>
          <p v-else class="card-sub">当前提案没有可选项，请先切换提案。</p>
        </div>
      </li>

      <li class="flow-step">
        <span class="flow-index" aria-hidden="true">3</span>
        <div id="guide-anchor-user-vote-prepare" class="flow-body">
          <h2 class="card-title">生成投票凭据</h2>
          <p class="card-sub">
            凭据由服务端按你选中的选项生成，不需要手写 proof 或密文。
            改选项会让已生成的凭据失效，需要重新生成。
          </p>
          <button
            class="btn btn--primary"
            type="button"
            :disabled="portal.voteLoading || !portal.walletReady"
            @click="prepare"
          >
            {{ portal.voteLoading ? '生成中…' : '按选项生成凭据' }}
          </button>

          <details class="disclosure">
            <summary>低频工具与高级字段</summary>
            <label class="field" for="vote-ballot-json">
              <span class="field-label">投票凭据 JSON</span>
              <textarea
                id="vote-ballot-json"
                v-model.trim="portal.voteForm.ballotJson"
                rows="4"
                placeholder='{"encrypted_vote":"...","proof":"...","nullifier":"..."}'
                :aria-invalid="portal.voteErrors.ballotJson ? 'true' : 'false'"
              />
            </label>
            <div class="row">
              <button
                class="btn btn--secondary btn--sm"
                type="button"
                @click="portal.applySelectedOptionToBallot()"
              >
                写入选项
              </button>
              <button
                class="btn btn--secondary btn--sm"
                type="button"
                @click="portal.fillBallotTemplate()"
              >
                填充模板
              </button>
              <button
                class="btn btn--secondary btn--sm"
                type="button"
                @click="portal.importBallotJson()"
              >
                导入 JSON
              </button>
              <button
                class="btn btn--secondary btn--sm"
                type="button"
                @click="portal.voteForm.nullifier = portal.newNullifier()"
              >
                新 nullifier
              </button>
            </div>
            <label class="field" for="vote-encrypted-vote">
              <span class="field-label">encrypted_vote</span>
              <textarea
                id="vote-encrypted-vote"
                v-model.trim="portal.voteForm.encryptedVote"
                rows="2"
                :aria-invalid="
                  portal.voteErrors.encryptedVote ? 'true' : 'false'
                "
              />
            </label>
            <label class="field" for="vote-proof">
              <span class="field-label">proof</span>
              <textarea
                id="vote-proof"
                v-model.trim="portal.voteForm.proof"
                rows="2"
                :aria-invalid="portal.voteErrors.proof ? 'true' : 'false'"
              />
            </label>
            <label class="field" for="vote-nullifier">
              <span class="field-label">nullifier</span>
              <input
                id="vote-nullifier"
                v-model.trim="portal.voteForm.nullifier"
                :aria-invalid="portal.voteErrors.nullifier ? 'true' : 'false'"
              />
            </label>
            <label class="field" for="vote-public-witness">
              <span class="field-label">public_witness（可选）</span>
              <textarea
                id="vote-public-witness"
                v-model.trim="portal.voteForm.publicWitness"
                rows="2"
              />
            </label>
          </details>
        </div>
      </li>

      <li class="flow-step">
        <span class="flow-index" aria-hidden="true">4</span>
        <div id="guide-anchor-user-vote-submit" class="flow-body">
          <h2 class="card-title">提交选票</h2>
          <p class="card-sub">
            提交后服务端保存的是加密选票，计票需要凑齐门限份额才能还原。
            但当前版本密文由服务端生成，生成时服务端能看到你选的选项 ——
            客户端本地加密是下一版本的目标。
          </p>
          <button
            class="btn btn--primary"
            type="button"
            :disabled="
              portal.voteLoading || !portal.walletReady || !portal.ballotTargetCurrent
            "
            @click="submit"
          >
            {{ portal.voteLoading ? '提交中…' : '提交匿名选票' }}
          </button>
          <p v-if="!portal.ballotTargetCurrent" class="field-error" role="alert">
            投票凭据对应的选项与当前选择不一致，请重新生成凭据后再提交。
          </p>
        </div>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.flow {
  display: grid;
  gap: var(--s-3);
  list-style: none;
}

.flow-step {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: var(--s-4);
  align-items: start;
}

.flow-index {
  width: 28px;
  height: 28px;
  border-radius: var(--r-pill);
  background: var(--c-surface-3);
  color: var(--c-text-muted);
  display: grid;
  place-items: center;
  font-size: var(--t-caption);
  font-weight: 700;
}

.flow-body {
  display: grid;
  gap: var(--s-3);
  border: 1px solid var(--c-border);
  border-radius: var(--r-lg);
  background: var(--c-surface);
  padding: var(--s-4);
  min-width: 0;
}
</style>
