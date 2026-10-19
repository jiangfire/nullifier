<script setup lang="ts">
import { formatDateTimeRange } from '../../lib/datetime';
import type { PortalApi } from '../../types/portal-api';
import EmptyState from '../ui/EmptyState.vue';

const props = defineProps<{ portal: PortalApi }>();
const portal = props.portal;

function openVote(id: string): void {
  portal.adoptProposal(id, 'vote');
}

function openOps(id: string): void {
  portal.adoptProposal(id, 'ops');
}
</script>

<template>
  <div id="guide-anchor-user-hall" class="stack">
    <div class="row-between">
      <p class="card-sub">
        共 {{ portal.hallState.proposals.length }} 场开放中的投票
      </p>
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        :disabled="portal.hallState.loading"
        @click="portal.loadOpenProposals()"
      >
        {{ portal.hallState.loading ? '刷新中…' : '刷新' }}
      </button>
    </div>

    <EmptyState
      v-if="
        portal.hallState.proposals.length === 0 && !portal.hallState.loading
      "
      title="暂无开放中的提案"
      text="后端还没有处于开放状态的投票。可以先在「计票与审计」里创建一场。"
    >
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        @click="portal.loadOpenProposals()"
      >
        重新加载
      </button>
    </EmptyState>

    <div v-else class="grid-auto">
      <article
        v-for="item in portal.hallState.proposals"
        :key="item.id"
        class="card card--pad-sm"
      >
        <div class="card-head">
          <h2 class="card-title">{{ item.title || '未命名提案' }}</h2>
          <span class="pill">{{ item.status || '未知状态' }}</span>
        </div>

        <p v-if="item.description" class="card-sub">{{ item.description }}</p>

        <ul v-if="item.options.length > 0" class="option-tags">
          <li v-for="option in item.options" :key="option" class="option-tag">
            {{ option }}
          </li>
        </ul>

        <p class="card-sub subtle">
          {{ formatDateTimeRange(item.startsAt, item.endsAt) }}
        </p>

        <div class="row">
          <button
            class="btn btn--primary btn--sm"
            type="button"
            @click="openVote(item.id)"
          >
            去投票
          </button>
          <button
            class="btn btn--secondary btn--sm"
            type="button"
            @click="openOps(item.id)"
          >
            计票管理
          </button>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.option-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
  list-style: none;
}

.option-tag {
  border: 1px solid var(--c-border);
  border-radius: var(--r-sm);
  background: var(--c-surface-2);
  color: var(--c-text-muted);
  padding: 2px 8px;
  font-size: var(--t-caption);
}
</style>
