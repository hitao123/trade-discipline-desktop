<script setup lang="ts">
import { computed } from 'vue'

import { formatCNY } from '@/renderer/lib/format'

const props = defineProps<{ used: number; caution: number; limit: number; label: string }>()
const progress = computed(() => Math.min(100, Math.max(0, props.used / props.limit * 100)))
const tone = computed(() => props.used >= props.limit ? 'danger' : props.used >= props.caution ? 'caution' : 'safe')
</script>

<template>
  <section class="risk-meter" :data-tone="tone">
    <div class="risk-meter__top"><span>{{ label }}</span><strong>{{ formatCNY(used) }} / {{ formatCNY(limit) }}</strong></div>
    <div class="risk-meter__track"><span class="risk-meter__fill" :style="{ width: `${progress}%` }" /></div>
    <p>警戒线 {{ formatCNY(caution) }}</p>
  </section>
</template>

<style scoped>
.risk-meter__top { display: flex; justify-content: space-between; gap: 16px; margin-bottom: 12px; font-size: 13px; }
.risk-meter__top strong { font-variant-numeric: tabular-nums; }
.risk-meter__track { height: 7px; overflow: hidden; background: #dedbd2; }
.risk-meter__fill { display: block; height: 100%; background: #6e7b63; transition: width .25s ease; }
.risk-meter[data-tone="caution"] .risk-meter__fill { background: #ad7a31; }
.risk-meter[data-tone="danger"] .risk-meter__fill { background: var(--accent); }
.risk-meter p { margin: 8px 0 0; color: var(--ink-faint); font-size: 11px; }
</style>
