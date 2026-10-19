<script setup lang="ts">
import type { PortalApi } from '../../types/portal-api';
import NoticeBanner from '../ui/NoticeBanner.vue';

const props = defineProps<{ portal: PortalApi }>();
const portal = props.portal;
</script>

<template>
  <div class="stack" style="--stack-gap: var(--s-5)">
    <NoticeBanner
      v-if="!portal.adminTokenReady"
      tone="warn"
      title="未配置 Admin Token"
      text="创建提案、签发凭证、发布结果会被后端拒绝。只查看结果和教程时不需要令牌。"
    />

    <form class="card" @submit.prevent="portal.checkHealth()">
      <div class="card-head">
        <h2 class="card-title">后端连接</h2>
        <span
          class="pill"
          :class="portal.healthState.ok ? 'pill--ok' : 'pill--warn'"
        >
          {{ portal.healthState.ok ? '已连通' : '未确认' }}
        </span>
      </div>

      <div class="grid-2">
        <label class="field" for="setup-api-base-url">
          <span class="field-label">后端地址</span>
          <input
            id="setup-api-base-url"
            v-model.trim="portal.apiBaseUrl"
            type="url"
            inputmode="url"
            placeholder="http://localhost:8080"
          />
        </label>

        <label class="field" for="setup-admin-token">
          <span class="field-label">Admin Token</span>
          <input
            id="setup-admin-token"
            v-model.trim="portal.adminBearerToken"
            type="password"
            autocomplete="off"
            placeholder="运营端管理接口的 Bearer 令牌"
          />
        </label>
      </div>

      <div class="row-between">
        <p class="card-sub">
          {{ portal.healthState.text }}
          <span v-if="portal.healthState.checkedAt" class="subtle">
            · 最近检测 {{ portal.healthState.checkedAt }}
          </span>
        </p>
        <button
          class="btn btn--primary"
          type="submit"
          :disabled="portal.healthState.loading"
        >
          {{ portal.healthState.loading ? '检测中…' : '检测连接' }}
        </button>
      </div>
    </form>
  </div>
</template>
