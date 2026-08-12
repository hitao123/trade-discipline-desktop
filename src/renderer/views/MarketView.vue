<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import MarketTable from '@/renderer/components/market/MarketTable.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { api } from '@/renderer/lib/api'
import type { MarketQuote, MarketResult } from '@/renderer/types'

const market = shallowRef<MarketResult>()
const tab = shallowRef<'stock' | 'etf'>('stock')
const busy = shallowRef(false)
const error = shallowRef('')
const notice = shallowRef('')
const active = computed(() => tab.value === 'stock' ? market.value?.stock : market.value?.etf)

async function load() {
  try { market.value = await api.request<MarketResult>('/api/market/snapshots/latest') }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '榜单加载失败' }
}
async function refresh() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    market.value = await api.request<MarketResult>('/api/market/refresh', { method: 'POST' })
    const failed = Object.keys(market.value.errors ?? {})
    notice.value = failed.length ? `部分刷新成功；${failed.join('、')} 暂时沿用最近成功数据` : '收盘榜单刷新完成'
  }
  catch (cause) { error.value = cause instanceof Error ? `${cause.message}。最近成功快照仍会保留，可在规则与设置导入 CSV。` : '刷新失败' }
  finally { busy.value = false }
}
async function addWatch(quote: MarketQuote) {
  try {
    await api.request('/api/watchlist', { method: 'POST', body: JSON.stringify({ market: quote.market, code: quote.code, reason: '来自收盘成交额榜单，等待进一步研究', sourceType: 'market_ranking' }) })
    notice.value = `${quote.name} 已加入观察名单`
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '加入观察失败' }
}
onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="CLOSE RANKING" title="收盘后再筛选" description="Go 后端独立按成交额排序。榜单只能进入观察名单，不能直接变成合规成交。">
      <button class="button" type="button" :disabled="busy" @click="refresh">{{ busy ? '刷新中…' : '刷新收盘榜单' }}</button>
    </PageHeader>
    <ErrorNotice :message="error" /><p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <div class="tabs" role="tablist" aria-label="榜单类型">
      <button type="button" :class="{ active: tab === 'stock' }" @click="tab = 'stock'">沪深股票前 20</button>
      <button type="button" :class="{ active: tab === 'etf' }" @click="tab = 'etf'">ETF 前 10</button>
    </div>
    <div class="snapshot-meta"><span>交易日 {{ active?.tradeDate || '—' }}</span><span>来源 {{ active?.source || '—' }}</span><span>版本 {{ active?.version || '—' }}</span></div>
    <MarketTable :entries="active?.entries ?? []" @watch="addWatch" />
  </div>
</template>

<style scoped>
.tabs { display: flex; gap: 20px; margin-bottom: 12px; border-bottom: 1px solid var(--line); }
.tabs button { padding: 10px 1px; color: var(--ink-faint); border: 0; border-bottom: 2px solid transparent; background: transparent; cursor: pointer; }
.tabs button.active { color: var(--ink); border-bottom-color: var(--accent); font-weight: 700; }
.snapshot-meta { display: flex; gap: 22px; margin: 12px 0; color: var(--ink-faint); font-size: 11px; }
.success-notice { color: #506448; font-size: 13px; }
</style>
