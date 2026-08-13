<script setup lang="ts">
import { computed } from 'vue'

import MarketLineChart from '@/renderer/components/market/MarketLineChart.vue'
import { formatPrice } from '@/renderer/lib/format'
import type { MarketHistoryResult, MarketQuote, MarketRange } from '@/renderer/types'

interface Props {
  selected: MarketQuote | undefined
  history: MarketHistoryResult | undefined
  range: MarketRange
  loading: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  rangeChange: [range: MarketRange]
  refresh: []
}>()

const pricePoints = computed(() => (props.history?.points ?? []).map(point => ({ date: point.tradeDate, value: point.closeMinor })))
const historyStatus = computed(() => {
  if (!props.selected) return ''
  if (!props.history?.lastSuccessfulAt) return '还没有保存这只证券的历史数据'
  const timestamp = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(props.history.lastSuccessfulAt))
  return (props.history.cached ? '本地缓存 · ' : '最近更新 · ') + timestamp
})
</script>

<template>
  <section class="history" aria-labelledby="instrument-history-title">
    <div v-if="!selected" class="history-empty">
      <p class="history-eyebrow">SELECTED SECURITY</p>
      <h2 id="instrument-history-title" class="history-title">从榜单选择一只证券查看收盘走势</h2>
      <p>只为当前选择补齐历史，不会同时请求榜单中的全部证券。</p>
    </div>
    <template v-else>
      <div class="history-header">
        <div>
          <p class="history-eyebrow">SELECTED SECURITY</p>
          <h2 id="instrument-history-title" class="history-title">{{ selected.name }}</h2>
          <p class="history-status">{{ selected.market }} · {{ selected.code }} · {{ historyStatus }}</p>
        </div>
        <div class="history-actions">
          <div class="range-switch" aria-label="证券走势范围">
            <button type="button" :aria-pressed="range === '1m'" @click="emit('rangeChange', '1m')">近 1 个月</button>
            <button type="button" :aria-pressed="range === '3m'" @click="emit('rangeChange', '3m')">近 3 个月</button>
          </div>
          <button class="text-button" type="button" :disabled="loading" @click="emit('refresh')">{{ loading ? '获取中…' : '重新获取走势' }}</button>
        </div>
      </div>
      <p v-if="history?.error" class="history-warning">{{ history.error }}；继续显示最后一次成功缓存。</p>
      <MarketLineChart
        :label="selected.name + ' 收盘价'"
        :points="pricePoints"
        :format-value="formatPrice"
        :loading="loading"
        empty-text="这只证券还没有本地历史，点击重新获取走势"
      />
    </template>
  </section>
</template>

<style scoped>
.history { margin: 4px 0 24px; padding: 18px 0 22px; border-block: 1px solid var(--line); }
.history-empty { min-height: 120px; color: var(--ink-faint); }
.history-empty p:last-child { margin: 8px 0 0; font-size: 11px; }
.history-header { display: flex; align-items: end; justify-content: space-between; gap: 24px; margin-bottom: 10px; }
.history-eyebrow { margin: 0 0 4px; color: var(--accent); font-size: 9px; font-weight: 750; letter-spacing: .14em; }
.history-title { margin: 0; font-family: var(--font-serif); font-size: 20px; font-weight: 650; }
.history-status { margin: 5px 0 0; color: var(--ink-faint); font-size: 10px; }
.history-actions { display: flex; align-items: center; gap: 18px; }
.range-switch { display: flex; border-bottom: 1px solid var(--line); }
.range-switch button { padding: 8px 10px; color: var(--ink-faint); border: 0; border-bottom: 2px solid transparent; background: transparent; font-size: 11px; cursor: pointer; }
.range-switch button[aria-pressed="true"] { color: var(--ink); border-bottom-color: var(--accent); font-weight: 700; }
.history-warning { margin: 4px 0 10px; color: #506448; font-size: 11px; }
</style>
