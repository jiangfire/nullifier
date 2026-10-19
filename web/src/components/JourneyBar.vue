<script setup lang="ts">
defineProps<{
  pathLabel: string;
  stepTitle: string;
  stepDetail: string;
  index: number;
  total: number;
  canGoBack: boolean;
}>();

const emit = defineEmits<{
  prev: [];
  next: [];
  exit: [];
}>();
</script>

<template>
  <div class="journey">
    <div class="journey-main">
      <div class="journey-meta">
        <span class="journey-path">{{ pathLabel }}</span>
        <span class="journey-count">{{ index + 1 }} / {{ total }}</span>
      </div>
      <strong class="journey-title">{{ stepTitle }}</strong>
      <p class="journey-detail">{{ stepDetail }}</p>
    </div>
    <div class="journey-actions">
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        :disabled="!canGoBack"
        @click="emit('prev')"
      >
        上一步
      </button>
      <button
        class="btn btn--primary btn--sm"
        type="button"
        @click="emit('next')"
      >
        {{ index + 1 >= total ? '完成' : '下一步' }}
      </button>
      <button
        class="btn btn--ghost btn--sm"
        type="button"
        @click="emit('exit')"
      >
        退出
      </button>
    </div>
  </div>
</template>

<style scoped>
.journey {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-4);
  flex-wrap: wrap;
  border: 1px solid var(--c-brand-border);
  border-radius: var(--r-lg);
  background: var(--c-brand-soft);
  padding: var(--s-3) var(--s-4);
}

.journey-main {
  display: grid;
  gap: 2px;
  min-width: 0;
  flex: 1 1 320px;
}

.journey-meta {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.journey-path {
  font-size: var(--t-caption);
  font-weight: 600;
  color: var(--c-brand);
}

.journey-count {
  font-size: var(--t-caption);
  color: var(--c-text-muted);
}

.journey-title {
  font-size: var(--t-section);
  font-weight: 600;
  line-height: var(--lh-tight);
}

.journey-detail {
  font-size: var(--t-label);
  color: var(--c-text-muted);
}

.journey-actions {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  flex-wrap: wrap;
}
</style>
