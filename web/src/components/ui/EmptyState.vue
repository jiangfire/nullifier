<script setup lang="ts">
defineProps<{
  title: string;
  text?: string;
}>();
</script>

<template>
  <div class="empty">
    <p class="empty-title">{{ title }}</p>
    <p v-if="text" class="empty-text">{{ text }}</p>
    <slot />
  </div>
</template>
