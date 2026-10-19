<script setup lang="ts">
import type { Notice } from '../types/portal';

defineProps<{
  notices: Notice[];
}>();

const emit = defineEmits<{
  dismiss: [id: string];
}>();

const levelTone: Record<Notice['level'], 'brand' | 'danger' | 'info'> = {
  success: 'brand',
  error: 'danger',
  info: 'info',
};
</script>

<template>
  <div class="notice-stack" aria-live="polite" aria-label="全局消息">
    <div
      v-for="item in notices"
      :key="item.id"
      class="notice"
      :class="`notice--${levelTone[item.level]}`"
      role="status"
    >
      <div class="notice-body">
        <strong class="notice-title">{{ item.title }}</strong>
        <span class="notice-text">{{ item.detail }}</span>
      </div>
      <button
        class="notice-close"
        type="button"
        aria-label="关闭消息"
        @click="emit('dismiss', item.id)"
      >
        ✕
      </button>
    </div>
  </div>
</template>

<style scoped>
.notice-stack {
  position: fixed;
  top: var(--s-4);
  right: var(--s-4);
  z-index: 200;
  width: min(340px, calc(100vw - var(--s-6)));
  display: grid;
  gap: var(--s-2);
  pointer-events: none;
}

.notice {
  pointer-events: auto;
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  border: 1px solid var(--c-border);
  border-left-width: 3px;
  border-radius: var(--r-md);
  background: var(--c-surface);
  box-shadow: var(--shadow-md);
  padding: var(--s-3);
  animation: notice-enter 180ms ease both;
}

.notice--brand {
  border-left-color: var(--c-brand);
}

.notice--danger {
  border-left-color: var(--c-danger);
}

.notice--info {
  border-left-color: var(--c-info);
}

.notice-body {
  display: grid;
  gap: 2px;
  flex: 1 1 auto;
  min-width: 0;
}

.notice-title {
  font-size: var(--t-label);
  font-weight: 600;
}

.notice-text {
  font-size: var(--t-caption);
  color: var(--c-text-muted);
  overflow-wrap: anywhere;
}

.notice-close {
  border: 0;
  background: transparent;
  color: var(--c-text-subtle);
  cursor: pointer;
  padding: 0;
  width: 20px;
  height: 20px;
  flex: 0 0 auto;
  border-radius: var(--r-xs);
}

.notice-close:hover {
  color: var(--c-text);
  background: var(--c-surface-3);
}

@keyframes notice-enter {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 560px) {
  .notice-stack {
    left: var(--s-3);
    right: var(--s-3);
    width: auto;
  }
}
</style>
