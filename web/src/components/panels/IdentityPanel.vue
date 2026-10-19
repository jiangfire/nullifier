<script setup lang="ts">
import type { PortalApi } from '../../types/portal-api';
import NoticeBanner from '../ui/NoticeBanner.vue';
import EmptyState from '../ui/EmptyState.vue';

const props = defineProps<{ portal: PortalApi }>();
const portal = props.portal;

const emit = defineEmits<{
  issued: [];
}>();

async function issue(): Promise<void> {
  const ok = await portal.issueCredential();
  if (ok) {
    emit('issued');
  }
}
</script>

<template>
  <div
    id="guide-anchor-user-identity"
    class="stack"
    style="--stack-gap: var(--s-5)"
  >
    <NoticeBanner
      v-if="!portal.adminTokenReady"
      tone="warn"
      title="签发凭证需要 Admin Token"
      text="凭证按提案发放，签发接口属于管理面。"
    >
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        @click="portal.setActiveSection('setup')"
      >
        去配置
      </button>
    </NoticeBanner>

    <div class="split">
      <form class="card" @submit.prevent="issue">
        <div class="card-head">
          <h2 class="card-title">领取凭证</h2>
          <span
            class="pill"
            :class="portal.walletReady ? 'pill--ok' : 'pill--warn'"
          >
            {{ portal.walletReady ? '已就绪' : '未就绪' }}
          </span>
        </div>

        <label class="field" for="identity-proposal-id">
          <span class="field-label">
            proposal_id
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="identity-proposal-id"
            v-model.trim="portal.voteForm.proposalId"
            placeholder="先在投票大厅选择一场提案"
          />
        </label>

        <label class="field" for="identity-user-id">
          <span class="field-label">
            user_id
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <input
            id="identity-user-id"
            v-model.trim="portal.identityForm.userId"
            placeholder="例如 u-1001"
            autocomplete="username"
            :aria-invalid="portal.identityErrors.userId ? 'true' : 'false'"
            :aria-describedby="
              portal.identityErrors.userId
                ? 'identity-user-id-error'
                : undefined
            "
          />
          <span
            v-if="portal.identityErrors.userId"
            id="identity-user-id-error"
            class="field-error"
            role="alert"
          >
            {{ portal.identityErrors.userId }}
          </span>
        </label>

        <label class="switch">
          <input v-model="portal.useCustomPubKey" type="checkbox" />
          手动指定公钥
        </label>

        <label
          v-if="portal.useCustomPubKey"
          class="field"
          for="identity-pub-key"
        >
          <span class="field-label">
            user_pub_key
            <span class="field-required" aria-hidden="true">*</span>
          </span>
          <textarea
            id="identity-pub-key"
            v-model.trim="portal.identityForm.customPubKey"
            rows="3"
            :aria-invalid="
              portal.identityErrors.customPubKey ? 'true' : 'false'
            "
          />
          <span
            v-if="portal.identityErrors.customPubKey"
            class="field-error"
            role="alert"
          >
            {{ portal.identityErrors.customPubKey }}
          </span>
        </label>

        <div v-else class="row-between">
          <span class="mono truncate grow" :title="portal.currentPubKey">
            {{ portal.currentPubKey }}
          </span>
          <button
            class="btn btn--ghost btn--sm"
            type="button"
            @click="portal.regeneratePubKey()"
          >
            换一个公钥
          </button>
        </div>

        <div class="row">
          <button
            class="btn btn--secondary btn--sm"
            type="button"
            @click="portal.openCredentialImport()"
          >
            导入凭证文件
          </button>
          <button
            class="btn btn--secondary btn--sm"
            type="button"
            :disabled="!portal.walletReady"
            @click="portal.exportCredentialFile()"
          >
            导出凭证文件
          </button>
        </div>

        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.credentialLoading"
        >
          {{ portal.credentialLoading ? '签发中…' : '签发凭证' }}
        </button>
      </form>

      <aside class="card card--pad-sm">
        <h2 class="card-title">凭证包</h2>
        <EmptyState
          v-if="!portal.walletReady"
          title="尚未领取凭证"
          text="凭证是本场投票的匿名入场券，只能用于当前 proposal_id。"
        />
        <dl v-else class="kv">
          <div class="kv-row">
            <dt class="kv-key">user_id</dt>
            <dd class="kv-value">{{ portal.wallet.userId }}</dd>
          </div>
          <div class="kv-row">
            <dt class="kv-key">issuer_key_version</dt>
            <dd class="kv-value">
              {{ portal.wallet.issuerKeyVersion || '-' }}
            </dd>
          </div>
          <div class="kv-row">
            <dt class="kv-key">issued_at</dt>
            <dd class="kv-value">{{ portal.wallet.issuedAt || '-' }}</dd>
          </div>
          <div class="kv-row">
            <dt class="kv-key">user_pub_key</dt>
            <dd class="kv-value mono">{{ portal.wallet.userPubKey }}</dd>
          </div>
          <div class="kv-row">
            <dt class="kv-key">signature</dt>
            <dd class="kv-value mono">{{ portal.wallet.signature }}</dd>
          </div>
        </dl>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.split {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
  gap: var(--s-4);
  align-items: start;
}

@media (max-width: 900px) {
  .split {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
