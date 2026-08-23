<script setup lang="ts">
import { computed } from 'vue'

import type { MarketComponentHealth } from '@/renderer/types'

interface Props {
  health: Record<string, MarketComponentHealth> | undefined
  mode: 'close' | 'live'
}

const props = defineProps<Props>()

const closeComponents = [
  ['stock', '股票榜单'],
  ['etf', 'ETF 榜单'],
  ['quotes', '持仓参考价'],
  ['ashare_turnover', 'A 股成交额'],
  ['southbound_net_buy', '南向资金'],
] as const

const liveComponents = [
  ['live_stock', '实时股票榜单'],
  ['live_etf', '实时 ETF 榜单'],
] as const

const stateLabels: Record<MarketComponentHealth['state'], string> = {
  live: '已更新',
  delayed: '延迟',
  cached: '本地缓存',
  unavailable: '暂不可用',
}

const detailLabels: Record<string, string> = {
  PRIMARY_TEMPORARY_FAILURE: '公开来源暂时未响应',
  FALLBACK_FAILURE: '主来源和备用来源均暂不可用',
  SOURCE_FETCH_FAILED: '公开来源暂时未响应',
  SOURCE_EMPTY: '公开来源未返回有效数据',
  LOCAL_QUERY_FAILED: '本地数据读取失败',
  LOCAL_SAVE_FAILED: '本地数据保存失败',
  NO_LOCAL_CACHE: '本机还没有成功缓存',
}

const items = computed(() => {
  const definitions = props.mode === 'live' ? liveComponents : closeComponents
  return definitions.flatMap(([key, label]) => {
    const health = props.health?.[key]
    return health ? [{ key, label, health }] : []
  })
})

function stateLabel(health: MarketComponentHealth) {
  return stateLabels[health.state]
}

function detailLabel(code?: string) {
  return code ? (detailLabels[code] ?? '数据来源状态异常') : ''
}

function formatTime(value?: string) {
  if (!value) return ''
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) return ''
  return parsed.toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <section v-if="items.length" class="health-strip" aria-label="数据健康状态">
    <article v-for="item in items" :key="item.key" class="health-item">
      <div class="health-item__heading">
        <strong>{{ item.label }}</strong>
        <span class="health-state" :class="`health-state--${item.health.state}`">{{ stateLabel(item.health) }}</span>
      </div>
      <p v-if="formatTime(item.health.sourceTime)" class="health-item__time">数据时间 {{ formatTime(item.health.sourceTime) }}</p>
      <p v-else-if="formatTime(item.health.lastSuccessfulAt)" class="health-item__time">上次成功 {{ formatTime(item.health.lastSuccessfulAt) }}</p>
      <details v-if="item.health.detailCode" class="health-item__detail">
        <summary>原因</summary>
        <span>{{ detailLabel(item.health.detailCode) }}</span>
      </details>
    </article>
  </section>
</template>

<style scoped>
.health-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 1px;
  margin: 0 0 22px;
  padding: 1px;
  border: 1px solid var(--line);
  background: var(--line);
}

.health-item {
  min-width: 0;
  padding: 12px 14px;
  background: var(--paper);
}

.health-item__heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
}

.health-item__heading strong {
  overflow: hidden;
  color: var(--ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.health-state {
  flex: 0 0 auto;
  color: var(--ink-faint);
  font-size: 11px;
}

.health-state--live { color: var(--success); }
.health-state--delayed,
.health-state--cached { color: #8a6a32; }
.health-state--unavailable { color: var(--accent); }

.health-item__time {
  margin: 7px 0 0;
  color: var(--ink-faint);
  font-size: 10px;
}

.health-item__detail {
  margin-top: 7px;
  color: var(--ink-faint);
  font-size: 10px;
}

.health-item__detail summary {
  cursor: pointer;
}

.health-item__detail span {
  display: block;
  margin-top: 4px;
}
</style>
