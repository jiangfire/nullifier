<script setup lang="ts">
import type { SectionId, WorkspaceId } from '../types/portal';

defineProps<{
  workspace: WorkspaceId;
  activeSection: SectionId;
  open: boolean;
  healthOk: boolean;
  healthText: string;
  adminTokenReady: boolean;
}>();

const emit = defineEmits<{
  navigate: [section: SectionId];
  switch: [workspace: WorkspaceId];
  close: [];
  guide: [];
}>();

const sharedItems: Array<{ id: SectionId; title: string }> = [
  { id: 'overview', title: '概览' },
  { id: 'setup', title: '连接配置' },
  { id: 'guide', title: '术语参考' },
];

const workspaceItems: Record<
  WorkspaceId,
  Array<{ id: SectionId; title: string }>
> = {
  user: [
    { id: 'hall', title: '投票大厅' },
    { id: 'identity', title: '我的身份' },
    { id: 'vote', title: '参与投票' },
  ],
  admin: [{ id: 'ops', title: '计票与审计' }],
};

function select(section: SectionId): void {
  emit('navigate', section);
  emit('close');
}
</script>

<template>
  <nav class="shell-nav" :class="{ 'is-open': open }" aria-label="主导航">
    <div class="shell-brand">
      <span class="shell-brand-mark" aria-hidden="true">匿</span>
      <span>
        <span class="shell-brand-name">匿名投票</span>
        <span class="shell-brand-sub">Nullifier Portal</span>
      </span>
    </div>

    <div class="shell-switch" role="group" aria-label="切换工作区">
      <button
        class="shell-switch-btn"
        :class="{ 'is-active': workspace === 'user' }"
        type="button"
        :aria-pressed="workspace === 'user'"
        @click="emit('switch', 'user')"
      >
        投票人
      </button>
      <button
        class="shell-switch-btn"
        :class="{ 'is-active': workspace === 'admin' }"
        type="button"
        :aria-pressed="workspace === 'admin'"
        @click="emit('switch', 'admin')"
      >
        运营
      </button>
    </div>

    <div class="shell-nav-group">
      <button
        v-for="item in sharedItems"
        :key="item.id"
        class="shell-nav-item"
        :class="{ 'is-active': activeSection === item.id }"
        type="button"
        :aria-current="activeSection === item.id ? 'page' : undefined"
        @click="select(item.id)"
      >
        <span class="shell-nav-marker" aria-hidden="true"></span>
        {{ item.title }}
      </button>
    </div>

    <div class="shell-nav-group">
      <p class="shell-nav-heading">
        {{ workspace === 'admin' ? '运营路径' : '投票路径' }}
      </p>
      <button
        v-for="item in workspaceItems[workspace]"
        :key="item.id"
        class="shell-nav-item"
        :class="{ 'is-active': activeSection === item.id }"
        type="button"
        :aria-current="activeSection === item.id ? 'page' : undefined"
        @click="select(item.id)"
      >
        <span class="shell-nav-marker" aria-hidden="true"></span>
        {{ item.title }}
      </button>
    </div>

    <div class="shell-nav-footer">
      <span class="shell-status">
        <span
          class="shell-status-dot"
          :class="healthOk ? 'is-ok' : 'is-bad'"
          aria-hidden="true"
        ></span>
        {{ healthText }}
      </span>
      <span class="shell-status">
        <span
          class="shell-status-dot"
          :class="adminTokenReady ? 'is-ok' : 'is-warn'"
          aria-hidden="true"
        ></span>
        {{ adminTokenReady ? '管理权限可用' : '缺少 Admin Token' }}
      </span>
      <button
        class="btn btn--secondary btn--sm btn--block"
        type="button"
        @click="emit('guide')"
      >
        新手引导
      </button>
    </div>
  </nav>
</template>
