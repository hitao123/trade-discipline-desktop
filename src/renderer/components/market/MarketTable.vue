<script setup lang="ts">
import { computed } from 'vue'

import { formatPercentBP, formatTurnover } from '@/renderer/lib/format'
import type { MarketQuote, RankingComparisonEntry } from '@/renderer/types'

const props = defineProps<{ entries: MarketQuote[], comparisonEntries?: RankingComparisonEntry[] | undefined, mode?: 'close' | undefined | 'live', watchedKeys?: Set<string> | undefined, watchingKeys?: Set<string> | undefined, selectedKey?: string | undefined }>()
const emit = defineEmits<{
  watch: [quote: MarketQuote]
  selectHistory: [quote: MarketQuote]
}>()

function quoteKey(quote: MarketQuote) {
  return quote.market + '-' + quote.code
}

function isWatchPending(quote: MarketQuote) {
  return props.watchingKeys?.has(quoteKey(quote)) ?? false
}

function isWatched(quote: MarketQuote) {
  return props.watchedKeys?.has(quoteKey(quote)) ?? false
}

function changeText(entry: RankingComparisonEntry) {
  if (entry.changeState === 'new') return '新进榜'
  if (entry.rankDelta === null) return '—'
  if (entry.rankDelta > 0) return `↑${entry.rankDelta}`
  if (entry.rankDelta < 0) return `↓${Math.abs(entry.rankDelta)}`
  return '持平'
}

function streakText(entry: RankingComparisonEntry) {
  if (entry.streakDays === null) return '—'
  return entry.streakExact ? `${entry.streakDays}天` : `≥${entry.streakDays}天`
}

const displayEntries = computed(() => props.comparisonEntries ?? props.entries.map((quote, index) => ({ quote, rank: index + 1, previousRank: null, rankDelta: null, changeState: 'unknown' as const, streakDays: null, streakExact: false, etfLabel: null })))
</script>

<template>
  <div class="table-frame">
    <table>
      <thead><tr><th>排名/证券</th><th>{{ mode === 'live' ? '截至当前成交额' : '成交额' }}</th><th>涨跌</th><th v-if="mode !== 'live'">较上交易日收盘</th><th v-if="mode !== 'live'">连续在榜</th><th><span class="sr-only">操作</span></th></tr></thead>
      <tbody>
        <tr
          v-for="entry in displayEntries"
          :key="quoteKey(entry.quote)"
          :class="{ selected: quoteKey(entry.quote) === selectedKey }"
          :aria-selected="quoteKey(entry.quote) === selectedKey"
        >
          <td class="security"><span class="rank">{{ String(entry.rank).padStart(2, '0') }}</span><strong>{{ entry.quote.name }}</strong><small>{{ entry.quote.market }} · {{ entry.quote.code }}</small><small v-if="entry.etfLabel">{{ entry.etfLabel.trackingIndexName || entry.etfLabel.assetCategory }} · <a :href="entry.etfLabel.sourceURL" target="_blank" rel="noreferrer">标签来源</a></small><small v-else-if="entry.quote.assetType === 'etf'">标签待补充</small></td>
          <td>{{ formatTurnover(entry.quote.turnoverFen) }}</td>
          <td :class="entry.quote.changeBP < 0 ? 'negative' : 'positive'">{{ formatPercentBP(entry.quote.changeBP) }}</td>
          <td v-if="mode !== 'live'" :aria-label="changeText(entry)">{{ changeText(entry) }}</td>
          <td v-if="mode !== 'live'">{{ streakText(entry) }}</td>
          <td>
            <div class="row-actions">
              <button class="text-button text-button--history" type="button" @click="emit('selectHistory', entry.quote)">查看走势</button>
              <button class="text-button" type="button" :disabled="isWatched(entry.quote) || isWatchPending(entry.quote)" @click="emit('watch', entry.quote)">{{ isWatched(entry.quote) ? '已观察' : isWatchPending(entry.quote) ? '加入中…' : '加入观察' }}</button>
            </div>
          </td>
        </tr>
        <tr v-if="displayEntries.length === 0"><td :colspan="mode === 'live' ? 4 : 6" class="empty-cell">还没有成功的榜单快照</td></tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.table-frame { overflow: auto; border: 1px solid var(--line); }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { padding: 12px 14px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; font-weight: 650; letter-spacing: .08em; text-align: left; }
td { padding: 14px; border-top: 1px solid var(--line); white-space: nowrap; }
tr { transition: background-color 140ms ease; }
tr.selected { background: rgba(178, 59, 43, .055); }
tr.selected .rank { box-shadow: inset 2px 0 0 var(--accent); }
td strong, td small { display: block; } td small { margin-top: 3px; color: var(--ink-faint); }
.security { min-width: 180px; }.rank { display: inline-block; width: 28px; color: var(--ink-faint); font-variant-numeric: tabular-nums; }.positive { color: #a43b2e; }.negative { color: #55715c; }
.row-actions { display: flex; gap: 14px; justify-content: flex-end; }
.text-button--history { color: var(--ink-muted); }
.empty-cell { padding: 42px; color: var(--ink-faint); text-align: center; }

@media (prefers-reduced-motion: reduce) {
  tr { transition: none; }
}
</style>
