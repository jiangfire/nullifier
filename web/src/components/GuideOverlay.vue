<script setup lang="ts">
defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  start: [role: 'user' | 'admin'];
  dismiss: [];
}>();
</script>

<template>
  <div v-if="open" class="guide-overlay" @click.self="emit('dismiss')">
    <div
      class="guide-dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="guide-dialog-title"
    >
      <h2 id="guide-dialog-title" class="guide-dialog-title">选择你的角色</h2>
      <p class="guide-dialog-text">两条路径的操作顺序不同，选一个就开始。</p>

      <div class="guide-roles">
        <button class="guide-role" type="button" @click="emit('start', 'user')">
          <span class="guide-role-name">我要投票</span>
          <span class="guide-role-desc"> 选提案 → 领凭证 → 选选项 → 提交 </span>
        </button>
        <button
          class="guide-role"
          type="button"
          @click="emit('start', 'admin')"
        >
          <span class="guide-role-name">我要管理投票</span>
          <span class="guide-role-desc">
            建提案 → 初始化解密 → 结束后发布结果
          </span>
        </button>
      </div>

      <div class="guide-dialog-foot">
        <button class="btn btn--ghost" type="button" @click="emit('dismiss')">
          暂不开始
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.guide-overlay {
  position: fixed;
  inset: 0;
  z-index: 300;
  background: rgba(16, 24, 40, 0.5);
  display: grid;
  place-items: center;
  padding: var(--s-4);
}

.guide-dialog {
  width: min(520px, 100%);
  background: var(--c-surface);
  border-radius: var(--r-xl);
  box-shadow: var(--shadow-lg);
  padding: var(--s-6);
  display: grid;
  gap: var(--s-4);
}

.guide-dialog-title {
  font-size: var(--t-page);
  font-weight: 600;
  line-height: var(--lh-tight);
}

.guide-dialog-text {
  font-size: var(--t-label);
  color: var(--c-text-muted);
}

.guide-roles {
  display: grid;
  gap: var(--s-3);
}

.guide-role {
  display: grid;
  gap: var(--s-1);
  text-align: left;
  border: 1px solid var(--c-border-strong);
  border-radius: var(--r-lg);
  background: var(--c-surface);
  padding: var(--s-4);
  cursor: pointer;
  transition:
    border-color 140ms ease,
    background-color 140ms ease;
}

.guide-role:hover {
  border-color: var(--c-brand);
  background: var(--c-brand-soft);
}

.guide-role-name {
  font-size: var(--t-section);
  font-weight: 600;
}

.guide-role-desc {
  font-size: var(--t-label);
  color: var(--c-text-muted);
}

.guide-dialog-foot {
  display: flex;
  justify-content: flex-end;
}
</style>
