<script setup lang="ts">
defineProps<{ eyebrow: string; title: string; description: string }>()
</script>

<template>
  <header class="page-header">
    <div>
      <p class="page-header__eyebrow">{{ eyebrow }}</p>
      <h1 class="page-header__title">{{ title }}</h1>
      <p class="page-header__description">{{ description }}</p>
    </div>
    <div class="page-header__actions"><slot /></div>
  </header>
</template>

<style scoped>
.page-header { display: flex; align-items: end; justify-content: space-between; gap: 32px; margin-bottom: 30px; padding-bottom: 24px; border-bottom: 1px solid var(--line); }
.page-header__eyebrow { margin: 0 0 10px; color: var(--accent); font-size: 11px; font-weight: 750; letter-spacing: .16em; }
.page-header__title { margin: 0; font-family: var(--font-serif); font-size: 36px; font-weight: 500; letter-spacing: -.03em; }
.page-header__description { max-width: 650px; margin: 10px 0 0; color: var(--ink-muted); font-size: 14px; line-height: 1.7; }
.page-header__actions { flex: none; }
</style>
