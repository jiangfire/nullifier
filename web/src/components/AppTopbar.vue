<script setup lang="ts">
defineProps<{
  title: string;
}>();

const emit = defineEmits<{
  toggleNav: [];
}>();
</script>

<template>
  <header class="shell-topbar">
    <div class="row" style="--row-gap: var(--s-3)">
      <button
        class="btn btn--secondary btn--sm shell-nav-toggle"
        type="button"
        aria-label="打开导航"
        @click="emit('toggleNav')"
      >
        ☰
      </button>
      <h1 class="shell-page-title">{{ title }}</h1>
    </div>

    <div class="shell-topbar-right">
      <slot name="meta" />
      <slot name="actions" />
    </div>
  </header>
</template>
