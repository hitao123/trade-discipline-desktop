<script setup lang="ts">
import { onMounted } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import InstrumentHistoryPanel from '@/renderer/components/market/InstrumentHistoryPanel.vue'
import MarketOverviewPanel from '@/renderer/components/market/MarketOverviewPanel.vue'
import MarketTable from '@/renderer/components/market/MarketTable.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useMarketHistory } from '@/renderer/composables/useMarketHistory'
import { api } from '@/renderer/lib/api'
import type { MarketQuote } from '@/renderer/types'

const {
  overview,
  history,
  selected,
  tab,
  range,
  busy,
  historyBusy,
  error,
  notice,
  activeSnapshot,
  selectedKey,
  load,
  refreshAll,
  selectHistory,
  refreshSelected,
  setRange,
  setTab,
} = useMarketHistory()

async function addWatch(quote: MarketQuote) {
  try {
    await api.request('/api/watchlist', {
      method: 'POST',
      body: JSON.stringify({
        market: quote.market,
        code: quote.code,
        reason: '来自收盘成交额榜单，等待进一步研究',
        sourceType: 'market_ranking',
      }),
    })
    notice.value = quote.name + ' 已加入观察名单'
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '加入观察失败'
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader
      eyebrow="CLOSE RANKING"
      title="收盘后再筛选"
      description="榜单、市场成交额和南向资金只在刷新时获取并保存在本机；榜单仍只能进入观察名单。"
    >
      <button class="button" type="button" :disabled="busy" @click="refreshAll">
        {{ busy ? '刷新中…' : '刷新收盘数据' }}
      </button>
    </PageHeader>

    <ErrorNotice :message="error" />
    <p v-if="notice" class="success-notice" role="status">{{ notice }}</p>

    <MarketOverviewPanel
      :overview="overview"
      :range="range"
      :loading="busy"
      @range-change="setRange"
    />

    <div class="tabs" role="tablist" aria-label="榜单类型">
      <button type="button" :class="{ active: tab === 'stock' }" @click="setTab('stock')">
        沪深股票前 20
      </button>
      <button type="button" :class="{ active: tab === 'etf' }" @click="setTab('etf')">
        ETF 前 10
      </button>
    </div>

    <div class="snapshot-meta">
      <span>交易日 {{ activeSnapshot?.tradeDate || '—' }}</span>
      <span>来源 {{ activeSnapshot?.source || '—' }}</span>
      <span>版本 {{ activeSnapshot?.version || '—' }}</span>
    </div>

    <InstrumentHistoryPanel
      :selected="selected"
      :history="history"
      :range="range"
      :loading="historyBusy"
      @range-change="setRange"
      @refresh="refreshSelected"
    />

    <MarketTable
      :entries="activeSnapshot?.entries ?? []"
      :selected-key="selectedKey"
      @select-history="selectHistory"
      @watch="addWatch"
    />
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: 20px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.tabs button {
  padding: 10px 1px;
  color: var(--ink-faint);
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  cursor: pointer;
}

.tabs button.active {
  color: var(--ink);
  border-bottom-color: var(--accent);
  font-weight: 700;
}

.snapshot-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 22px;
  margin: 12px 0;
  color: var(--ink-faint);
  font-size: 11px;
}

.success-notice {
  color: #506448;
  font-size: 13px;
}
</style>
