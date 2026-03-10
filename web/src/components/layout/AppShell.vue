<script setup lang="ts">
defineProps<{
  title: string;
  subtitle?: string;
}>();
</script>

<template>
  <a class="skip-link" href="#main-content">跳转到主内容</a>
  <div class="app-shell">
    <header class="hero">
      <div class="hero-main">
        <p class="eyebrow">Nullifier Vote Console</p>
        <h1>{{ title }}</h1>
        <p v-if="subtitle" class="hero-copy">{{ subtitle }}</p>
        <slot name="hero" />
      </div>

      <slot name="aside" />
    </header>

    <main id="main-content" class="main-grid" tabindex="-1">
      <slot />
    </main>
  </div>
</template>

