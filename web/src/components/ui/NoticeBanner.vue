<script setup lang="ts">
withDefaults(
  defineProps<{
    tone?: 'brand' | 'info' | 'warn' | 'danger';
    title: string;
    text?: string;
  }>(),
  { tone: 'brand', text: undefined },
);
</script>

<template>
  <div class="banner" :class="`banner--${tone}`">
    <div class="banner-body">
      <span class="banner-title">{{ title }}</span>
      <span v-if="text" class="banner-text">{{ text }}</span>
    </div>
    <div v-if="$slots.default" class="row">
      <slot />
    </div>
  </div>
</template>
