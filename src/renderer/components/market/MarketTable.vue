<script setup lang="ts">
import { formatPercentBP, formatPrice, formatTurnover } from '@/renderer/lib/format'
import type { MarketQuote } from '@/renderer/types'

defineProps<{ entries: MarketQuote[], selectedKey?: string }>()
const emit = defineEmits<{
  watch: [quote: MarketQuote]
  selectHistory: [quote: MarketQuote]
}>()

function quoteKey(quote: MarketQuote) {
  return quote.market + '-' + quote.code
}
</script>

<template>
  <div class="table-frame">
    <table>
      <thead><tr><th>排名</th><th>证券</th><th>收盘价</th><th>涨跌</th><th>成交额</th><th>交易日</th><th><span class="sr-only">操作</span></th></tr></thead>
      <tbody>
        <tr
          v-for="(quote, index) in entries"
          :key="quoteKey(quote)"
          :class="{ selected: quoteKey(quote) === selectedKey }"
          :aria-selected="quoteKey(quote) === selectedKey"
        >
          <td class="rank">{{ String(index + 1).padStart(2, '0') }}</td>
          <td><strong>{{ quote.name }}</strong><small>{{ quote.market }} · {{ quote.code }}</small></td>
          <td>{{ formatPrice(quote.closeMinor) }}</td>
          <td :class="quote.changeBP < 0 ? 'negative' : 'positive'">{{ formatPercentBP(quote.changeBP) }}</td>
          <td>{{ formatTurnover(quote.turnoverFen) }}</td>
          <td>{{ quote.tradeDate }}</td>
          <td>
            <div class="row-actions">
              <button class="text-button text-button--history" type="button" @click="emit('selectHistory', quote)">查看走势</button>
              <button class="text-button" type="button" @click="emit('watch', quote)">加入观察</button>
            </div>
          </td>
        </tr>
        <tr v-if="entries.length === 0"><td colspan="7" class="empty-cell">还没有成功的收盘快照</td></tr>
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
.rank { color: var(--ink-faint); font-variant-numeric: tabular-nums; }.positive { color: #a43b2e; }.negative { color: #55715c; }
.row-actions { display: flex; gap: 14px; justify-content: flex-end; }
.text-button--history { color: var(--ink-muted); }
.empty-cell { padding: 42px; color: var(--ink-faint); text-align: center; }

@media (prefers-reduced-motion: reduce) {
  tr { transition: none; }
}
</style>
