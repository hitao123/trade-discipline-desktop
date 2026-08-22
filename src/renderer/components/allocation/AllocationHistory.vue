<script setup lang="ts">
import type { DeepReadonly } from 'vue'

import type { AllocationVersion } from '@/renderer/types'

defineProps<{ versions: readonly DeepReadonly<AllocationVersion>[] }>()
</script>

<template>
  <section class="history-section">
    <div class="section-heading"><p>版本历史</p><h2>为什么改变计划</h2></div>
    <ol class="history-list"><li v-for="version in versions" :key="version.id"><div><strong>v{{ version.version }}</strong><time>{{ new Date(version.createdAt).toLocaleString('zh-CN') }}</time></div><p>{{ version.reason }}</p><span>基准 {{ (version.draft.initialCapitalFen / 100).toLocaleString('zh-CN') }} 元 · 目标收益 {{ (version.draft.targetReturnBP / 100).toFixed(2) }}% · {{ version.draft.targetDeadline }}</span></li><li v-if="versions.length === 0">尚无配置版本记录</li></ol>
  </section>
</template>

<style scoped>
.history-section { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; padding: 28px 0; border-top: 1px solid var(--line); }.section-heading p { margin: 0 0 6px; color: var(--accent); font-size: 10px; letter-spacing: .12em; }.section-heading h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.history-list { display: grid; gap: 1px; margin: 0; padding: 0; background: var(--line); list-style: none; }.history-list li { display: grid; grid-template-columns: 120px 1fr; gap: 5px 16px; padding: 14px; background: var(--paper); }.history-list li div { grid-row: span 2; display: grid; gap: 4px; }.history-list strong { font-family: var(--font-serif); font-size: 20px; font-weight: 500; }.history-list time, .history-list span { color: var(--ink-faint); font-size: 10px; }.history-list p { margin: 0; font-size: 12px; }
</style>
