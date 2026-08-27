<script setup lang="ts">
import { computed } from 'vue'

import MarketLineChart from '@/renderer/components/market/MarketLineChart.vue'
import { formatTurnover } from '@/renderer/lib/format'
import type { MarketOverviewResult, MarketRange } from '@/renderer/types'

interface Props {
  overview: MarketOverviewResult | undefined
  range: MarketRange
  loading: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{ rangeChange: [range: MarketRange] }>()

const aShareLowTurnoverThresholdFen = 200_000_000_000_000
const turnoverPoints = computed(() => (props.overview?.aShareTurnover ?? []).map(point => ({ date: point.tradeDate, value: point.valueFen })))
const southboundPoints = computed(() => (props.overview?.southboundNetBuy ?? []).map(point => ({ date: point.tradeDate, value: point.valueFen })))
const sourceStatus = computed(() => {
  if (!props.overview?.lastSuccessfulAt) return '尚未保存市场历史'
  const timestamp = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(props.overview.lastSuccessfulAt))
  return (props.overview.cached ? '正在显示本地缓存 · ' : '最近更新 · ') + timestamp
})

function formatSignedTurnover(valueFen: number) {
  const valueYi = Math.abs(valueFen) / 100 / 100_000_000
  return (valueFen > 0 ? '+' : valueFen < 0 ? '-' : '') + valueYi.toFixed(2) + ' 亿'
}
</script>

<template>
  <section class="overview" aria-labelledby="market-overview-title">
    <div class="overview-header">
      <div>
        <p class="overview-eyebrow">MARKET CONTEXT</p>
        <h2 id="market-overview-title" class="overview-title">市场收盘概览</h2>
        <p class="overview-status">{{ sourceStatus }}</p>
      </div>
      <div class="range-switch" aria-label="市场概览范围">
        <button type="button" :aria-pressed="range === '1m'" @click="emit('rangeChange', '1m')">近 1 个月</button>
        <button type="button" :aria-pressed="range === '3m'" @click="emit('rangeChange', '3m')">近 3 个月</button>
      </div>
    </div>
    <p v-if="overview?.cached" class="cache-note">公开数据源暂时不可用，正在显示本地缓存；历史记录没有被覆盖。</p>
    <div class="overview-grid">
      <article class="metric">
        <div class="metric-heading">
          <h3 class="metric-title">沪深A股成交额</h3>
          <span class="metric-unit">低于 2 万亿元以绿色标注</span>
        </div>
        <MarketLineChart
          label="沪深A股成交额"
          :points="turnoverPoints"
          :format-value="formatTurnover"
          :low-threshold="aShareLowTurnoverThresholdFen"
          :loading="loading"
          empty-text="刷新收盘榜单后保存市场成交额"
        />
      </article>
      <article class="metric metric--second">
        <div class="metric-heading">
          <h3 class="metric-title">南向资金成交净买额</h3>
          <span class="metric-unit">正为净买入 · 负为净卖出</span>
        </div>
        <MarketLineChart
          label="南向资金成交净买额"
          :points="southboundPoints"
          :format-value="formatSignedTurnover"
          :loading="loading"
          empty-text="收盘刷新后保存南向资金净买额"
        />
      </article>
    </div>
  </section>
</template>

<style scoped>
.overview { margin: 8px 0 30px; border-block: 1px solid var(--line); }
.overview-header { display: flex; align-items: end; justify-content: space-between; gap: 24px; padding: 18px 0 16px; }
.overview-eyebrow { margin: 0 0 4px; color: var(--accent); font-size: 9px; font-weight: 750; letter-spacing: .14em; }
.overview-title { margin: 0; font-family: var(--font-serif); font-size: 21px; font-weight: 650; }
.overview-status { margin: 5px 0 0; color: var(--ink-faint); font-size: 10px; }
.range-switch { display: flex; border-bottom: 1px solid var(--line); }
.range-switch button { padding: 8px 10px; color: var(--ink-faint); border: 0; border-bottom: 2px solid transparent; background: transparent; font-size: 11px; cursor: pointer; }
.range-switch button[aria-pressed="true"] { color: var(--ink); border-bottom-color: var(--accent); font-weight: 700; }
.cache-note { margin: 0; padding: 9px 0; color: #506448; border-top: 1px solid var(--line); font-size: 11px; }
.overview-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); border-top: 1px solid var(--line); }
.metric { min-width: 0; padding: 18px 26px 20px 0; }
.metric--second { padding-right: 0; padding-left: 26px; border-left: 1px solid var(--line); }
.metric-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 6px; }
.metric-title { margin: 0; font-size: 13px; font-weight: 720; }
.metric-unit { color: var(--ink-faint); font-size: 9px; }
</style>
