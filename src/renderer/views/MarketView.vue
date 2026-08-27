<script setup lang="ts">
import { computed, onMounted } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import MarketHealthStrip from '@/renderer/components/market/MarketHealthStrip.vue'
import InstrumentHistoryPanel from '@/renderer/components/market/InstrumentHistoryPanel.vue'
import MarketOverviewPanel from '@/renderer/components/market/MarketOverviewPanel.vue'
import MarketTable from '@/renderer/components/market/MarketTable.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useMarketHistory } from '@/renderer/composables/useMarketHistory'
import { useOptionalUserProfile } from '@/renderer/composables/useUserProfile'
import { api } from '@/renderer/lib/api'
import type { MarketQuote } from '@/renderer/types'

const {
  overview,
  history,
  selected,
  tab,
  mode,
  range,
  busy,
  historyBusy,
  error,
  notice,
  activeSnapshot,
  activeStatus,
  activeHealth,
  selectedKey,
  load,
  refreshAll,
  refreshLive,
  selectHistory,
  refreshSelected,
  setRange,
  setTab,
  setMode,
} = useMarketHistory()

const isLiveMode = computed(() => mode.value === 'live')
const userProfile = useOptionalUserProfile()
const stockEnabled = computed(() => userProfile?.profile.value?.mode !== 'generic' || userProfile.profile.value.enabledMarkets.includes('ashare_stock'))
const etfEnabled = computed(() => userProfile?.profile.value?.mode !== 'generic' || userProfile.profile.value.enabledMarkets.includes('ashare_etf'))
const rankingEnabled = computed(() => stockEnabled.value || etfEnabled.value)
const pageCopy = computed(() => isLiveMode.value
  ? {
      eyebrow: 'LIVE TURNOVER',
      title: '交易时段内再观察',
      description: '仅在 A 股交易时段按需读取公开成交额排行；不自动轮询、不连接券商，也不会发出交易指令。',
      action: '刷新实时数据',
    }
  : {
      eyebrow: 'CLOSE RANKING',
      title: '收盘后再筛选',
      description: '榜单、市场成交额和南向资金只在刷新时获取并保存在本机；榜单仍只能进入观察名单。',
      action: '刷新收盘数据',
    })

const statusMessage = computed(() => {
  const status = activeStatus.value
	if (status?.lastAttemptAt) return '最近检查 ' + new Date(status.lastAttemptAt).toLocaleString('zh-CN', { hour12: false })
	if (status?.lastSuccessfulAt) return '最近成功刷新 ' + new Date(status.lastSuccessfulAt).toLocaleString('zh-CN', { hour12: false })
  return '尚未手动刷新'
})

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

onMounted(() => {
  if (!rankingEnabled.value)
    return
  if (!stockEnabled.value && etfEnabled.value)
    setTab('etf')
  void load()
})
</script>

<template>
  <div>
    <PageHeader
      :eyebrow="pageCopy.eyebrow"
      :title="pageCopy.title"
      :description="pageCopy.description"
    >
      <button class="button" type="button" :disabled="busy" @click="isLiveMode ? refreshLive() : refreshAll()">
        {{ busy ? '刷新中…' : pageCopy.action }}
      </button>
    </PageHeader>

    <ErrorNotice :message="error" />
    <p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <p v-if="!rankingEnabled" class="live-note">当前公开榜单仅覆盖 A 股股票与 ETF；你的工作区目前只启用了港股。</p>

    <template v-if="rankingEnabled">

    <div class="mode-tabs" role="tablist" aria-label="数据时段">
      <button type="button" :class="{ active: mode === 'close' }" @click="setMode('close')">收盘榜单</button>
      <button type="button" :class="{ active: mode === 'live' }" @click="setMode('live')">实时成交</button>
    </div>

    <p v-if="isLiveMode" class="live-note">实时成交额仅在 A 股连续竞价时段可更新。休市、午休和周末会显示最近一次本地快照。</p>

	<MarketHealthStrip :health="activeHealth" :mode="mode" />

    <MarketOverviewPanel
      v-if="!isLiveMode"
      :overview="overview"
      :range="range"
      :loading="busy"
      @range-change="setRange"
    />

    <div class="tabs" role="tablist" aria-label="榜单类型">
      <button v-if="stockEnabled" type="button" :class="{ active: tab === 'stock' }" @click="setTab('stock')">
        沪深股票前 20
      </button>
      <button v-if="etfEnabled" type="button" :class="{ active: tab === 'etf' }" @click="setTab('etf')">
        ETF 前 20
      </button>
    </div>

    <div class="snapshot-meta">
      <span>交易日 {{ activeSnapshot?.tradeDate || '—' }}</span>
      <span>来源 {{ activeSnapshot?.source || '—' }}</span>
      <span>{{ statusMessage }}</span>
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
    </template>
  </div>
</template>

<style scoped>
.mode-tabs,
.tabs {
  display: flex;
  gap: 20px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.mode-tabs button,
.tabs button {
  padding: 10px 1px;
  color: var(--ink-faint);
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  cursor: pointer;
}

.mode-tabs button.active,
.tabs button.active {
  color: var(--ink);
  border-bottom-color: var(--accent);
  font-weight: 700;
}

.mode-tabs {
  margin-bottom: 26px;
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

.live-note {
  margin: 0 0 16px;
  color: var(--ink-faint);
  font-size: 13px;
}
</style>
