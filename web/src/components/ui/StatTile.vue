<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string;
    value: string;
    tone?: 'default' | 'ok' | 'warn' | 'danger';
  }>(),
  { tone: 'default' },
);
</script>

<template>
  <div class="tile" :class="tone === 'default' ? undefined : `tile--${tone}`">
    <span class="tile-label">{{ label }}</span>
    <strong class="tile-value">{{ value }}</strong>
  </div>
</template>
