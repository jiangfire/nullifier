<script setup lang="ts">
import type { PortalApi } from '../../types/portal-api';
import StatTile from '../ui/StatTile.vue';
import NoticeBanner from '../ui/NoticeBanner.vue';

const props = defineProps<{ portal: PortalApi }>();
const portal = props.portal;

function goUser(): void {
  portal.setWorkspace('user');
  portal.setActiveSection('hall');
}

function goAdmin(): void {
  portal.setWorkspace('admin');
  portal.setActiveSection('ops');
}
</script>

<template>
  <div class="stack">
    <NoticeBanner
      v-if="!portal.healthState.ok"
      tone="warn"
      title="尚未连接后端服务"
      text="多数操作都需要后端在线。先在「连接配置」确认地址并做一次健康检查。"
    >
      <button
        class="btn btn--secondary btn--sm"
        type="button"
        @click="portal.checkHealth()"
      >
        检测连接
      </button>
    </NoticeBanner>

    <div class="tile-row">
      <StatTile
        label="服务连接"
        :value="portal.healthState.ok ? '已连通' : '未连通'"
        :tone="portal.healthState.ok ? 'ok' : 'warn'"
      />
      <StatTile
        label="开放提案"
        :value="`${portal.hallState.proposals.length} 场`"
      />
      <StatTile
        label="匿名凭证"
        :value="portal.walletReady ? '已就绪' : '未领取'"
        :tone="portal.walletReady ? 'ok' : 'default'"
      />
      <StatTile
        label="管理权限"
        :value="portal.adminTokenReady ? '已配置' : '未配置'"
        :tone="portal.adminTokenReady ? 'ok' : 'warn'"
      />
    </div>

    <div class="grid-2">
      <article class="card card--accent">
        <div class="card-head">
          <h2 class="card-title">我要投票</h2>
          <span class="pill pill--ok">投票人</span>
        </div>
        <p class="card-sub">
          选一场提案，领取本场专属的匿名凭证，然后按选项生成凭据并提交。
        </p>
        <div class="row">
          <button class="btn btn--primary" type="button" @click="goUser">
            进入投票大厅
          </button>
        </div>
      </article>

      <article class="card">
        <div class="card-head">
          <h2 class="card-title">我要管理投票</h2>
          <span class="pill">运营</span>
        </div>
        <p class="card-sub">
          创建提案，先初始化解密会话，投票结束后再提交解密份额发布结果。
        </p>
        <div class="row">
          <button class="btn btn--secondary" type="button" @click="goAdmin">
            进入计票与审计
          </button>
        </div>
      </article>
    </div>

    <article v-if="portal.selectedProposal" class="card card--pad-sm">
      <div class="row-between">
        <div class="stack stack-sm">
          <span class="label-caps">当前选中提案</span>
          <strong>{{ portal.selectedProposal.title || '未命名提案' }}</strong>
          <span class="mono subtle">{{ portal.selectedProposal.id }}</span>
        </div>
        <div class="row">
          <button
            class="btn btn--secondary btn--sm"
            type="button"
            @click="goUser"
          >
            去投票
          </button>
          <button class="btn btn--ghost btn--sm" type="button" @click="goAdmin">
            计票管理
          </button>
        </div>
      </div>
    </article>
  </div>
</template>
