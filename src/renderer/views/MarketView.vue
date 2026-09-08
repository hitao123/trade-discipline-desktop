<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import MarketHealthStrip from '@/renderer/components/market/MarketHealthStrip.vue'
import InstrumentHistoryPanel from '@/renderer/components/market/InstrumentHistoryPanel.vue'
import MarketOverviewPanel from '@/renderer/components/market/MarketOverviewPanel.vue'
import MarketTable from '@/renderer/components/market/MarketTable.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useMarketHistory } from '@/renderer/composables/useMarketHistory'
import { useRankingHistory } from '@/renderer/composables/useRankingHistory'
import { useOptionalUserProfile } from '@/renderer/composables/useUserProfile'
import { api } from '@/renderer/lib/api'
import type { MarketQuote } from '@/renderer/types'

const ranking = useRankingHistory()
const {
  comparison: rankingComparison,
  currentDate: rankingDate,
  currentDates: rankingDates,
  loading: rankingLoading,
  selectDate: selectRankingDate,
  previous: previousRankingDate,
  next: nextRankingDate,
  refreshDates: refreshRankingDates,
  error: rankingError,
} = ranking
const watchedKeys = shallowRef(new Set<string>())
const watchingKeys = shallowRef(new Set<string>())

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
const displayedSnapshot = computed(() => isLiveMode.value
  ? activeSnapshot.value
  : rankingComparison.value?.snapshot)
const rankingSummary = computed(() => {
  const comparison = rankingComparison.value
  if (!comparison || comparison.reason) return { newCount: null, continuingCount: null }
  return {
    newCount: comparison.entries.filter(entry => entry.changeState === 'new').length,
    continuingCount: comparison.entries.filter(entry => (entry.streakDays ?? 0) >= 3).length,
  }
})

async function addWatch(quote: MarketQuote) {
	const key = quote.market + '-' + quote.code
	if (watchedKeys.value.has(key) || watchingKeys.value.has(key)) return
	watchingKeys.value = new Set([...watchingKeys.value, key])
  try {
    await api.request('/api/watchlist', {
      method: 'POST',
      body: JSON.stringify({
        market: quote.market,
        code: quote.code,
        reason: watchReason(quote),
        sourceType: 'market_ranking',
      }),
    })
    notice.value = quote.name + ' 已加入观察名单'
		watchedKeys.value = new Set([...watchedKeys.value, key])
  }
  catch (cause) {
    await loadWatchlist()
    if (watchedKeys.value.has(key)) {
      notice.value = quote.name + ' 已在观察名单中'
      return
    }
    error.value = cause instanceof Error ? cause.message : '加入观察失败'
  }
	finally {
		watchingKeys.value = new Set([...watchingKeys.value].filter(item => item !== key))
	}
}

async function refreshClose() {
  await refreshAll()
  try {
    if (await refreshRankingDates(tab.value)) notice.value = '有新数据，点击“回到最新”查看；当前历史日期未切换。'
  } catch { /* refresh result remains available even if date index could not reload */ }
}

function returnToLatest() {
  const latest = rankingDates.value[0]
  if (latest) void selectRankingDate(latest.tradeDate)
}

function watchReason(quote: MarketQuote) {
	if (mode.value === 'live') return `来自实时成交榜单（${quote.tradeDate}，第 ${liveRank(quote) ?? '—'} 名），等待进一步研究`
  const entry = ranking.comparison.value?.entries.find(item => item.quote.market === quote.market && item.quote.code === quote.code)
  const change = entry?.changeState === 'new' ? '新进榜' : entry?.rankDelta === null || entry?.rankDelta === undefined ? '排名变化待确认' : entry.rankDelta > 0 ? `排名上升 ${entry.rankDelta} 位` : entry.rankDelta < 0 ? `排名下降 ${Math.abs(entry.rankDelta)} 位` : '排名持平'
  return `来自收盘成交额榜单（${ranking.currentDate.value ?? quote.tradeDate}，第 ${entry?.rank ?? '—'} 名，${change}），等待进一步研究`
}

function liveRank(quote: MarketQuote) {
	const index = activeSnapshot.value?.entries.findIndex(item => item.market === quote.market && item.code === quote.code) ?? -1
	return index >= 0 ? index + 1 : undefined
}

async function loadWatchlist() {
  try {
    const rows = await api.request<Array<{ instrument: { market: string, code: string } }>>('/api/watchlist')
    watchedKeys.value = new Set(rows.map(item => item.instrument.market + '-' + item.instrument.code))
  } catch { /* watchlist availability must not hide rankings */ }
}

function selectTab(next: 'stock' | 'etf') {
  setTab(next)
  if (mode.value === 'close') void ranking.load(next)
}

function selectMode(next: 'close' | 'live') {
  setMode(next)
  if (next === 'close') void ranking.load(tab.value)
}

onMounted(() => {
  if (!rankingEnabled.value)
    return
  if (!stockEnabled.value && etfEnabled.value)
    selectTab('etf')
  void load()
	void ranking.load(tab.value)
	void loadWatchlist()
})
</script>

<template>
  <div>
    <PageHeader
      :eyebrow="pageCopy.eyebrow"
      :title="pageCopy.title"
      :description="pageCopy.description"
    >
      <button class="button" type="button" :disabled="busy" @click="isLiveMode ? refreshLive() : refreshClose()">
        {{ busy ? '刷新中…' : pageCopy.action }}
      </button>
    </PageHeader>

    <ErrorNotice :message="error" />
	    <ErrorNotice :message="rankingError" />
    <p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <p v-if="!rankingEnabled" class="live-note">当前公开榜单仅覆盖 A 股股票与 ETF；你的工作区目前只启用了港股。</p>

    <template v-if="rankingEnabled">

    <div class="mode-tabs" role="tablist" aria-label="数据时段">
      <button type="button" :class="{ active: mode === 'close' }" @click="selectMode('close')">收盘榜单</button>
      <button type="button" :class="{ active: mode === 'live' }" @click="selectMode('live')">实时成交</button>
    </div>

    <p v-if="isLiveMode" class="live-note">实时成交额仅在 A 股连续竞价时段可更新。休市、午休和周末会显示最近一次本地快照。</p>

	<MarketHealthStrip :health="activeHealth" :mode="mode" />

    <details v-if="!isLiveMode" class="overview-details">
      <summary>市场概览</summary>
      <MarketOverviewPanel
      :overview="overview"
      :range="range"
      :loading="busy"
      @range-change="setRange"
      />
    </details>

    <div class="tabs" role="tablist" aria-label="榜单类型">
      <button v-if="stockEnabled" type="button" :class="{ active: tab === 'stock' }" @click="selectTab('stock')">
        沪深股票前 20
      </button>
      <button v-if="etfEnabled" type="button" :class="{ active: tab === 'etf' }" @click="selectTab('etf')">
        ETF 前 20
      </button>
    </div>

    <div class="snapshot-meta">
      <span>交易日 {{ displayedSnapshot?.tradeDate || '—' }}</span>
      <span>来源 {{ displayedSnapshot?.source || '—' }}</span>
      <span>{{ statusMessage }}</span>
      <span>版本 {{ displayedSnapshot?.version || '—' }}</span>
    </div>

    <div v-if="!isLiveMode" class="history-controls" aria-label="收盘榜单历史">
      <button type="button" :disabled="rankingLoading" @click="previousRankingDate">上一条记录</button>
      <select :value="rankingDate" aria-label="收盘榜单日期" @change="selectRankingDate(($event.target as HTMLSelectElement).value)">
        <option v-for="item in rankingDates" :key="item.tradeDate" :value="item.tradeDate">{{ item.tradeDate }}</option>
      </select>
      <button type="button" :disabled="rankingLoading" @click="nextRankingDate">下一条记录</button>
      <button type="button" :disabled="rankingLoading || rankingDate === rankingDates[0]?.tradeDate" @click="returnToLatest">回到最新</button>
      <span v-if="rankingComparison?.baselineDate">与 {{ rankingComparison.baselineDate }} 收盘比较</span>
      <span v-else>从首次成功保存开始积累历史</span>
    </div>
    <p v-if="!isLiveMode && rankingSummary.newCount !== null" class="ranking-summary">新进榜 {{ rankingSummary.newCount }} 只 · 连续在榜≥3日 {{ rankingSummary.continuingCount }} 只</p>
    <p v-if="!isLiveMode && rankingComparison?.reason" class="live-note">{{ rankingComparison.reason === 'missing_baseline' ? `缺少 ${rankingComparison.baselineDate ?? '上一交易日'} 的完整可比记录` : rankingComparison.reason === 'unverified' ? '此快照可回看，但未通过收盘完整性校验' : rankingComparison.reason === 'calendar_unknown' ? '交易日历覆盖不足，无法确认上一交易日' : rankingComparison.reason === 'universe_changed' ? '榜单筛选口径已变化，未做跨口径比较' : '' }}</p>

    <MarketTable
      :entries="isLiveMode ? (activeSnapshot?.entries ?? []) : (rankingComparison?.snapshot?.entries ?? [])"
      :comparison-entries="isLiveMode ? undefined : rankingComparison?.entries"
      :mode="mode"
      :watched-keys="watchedKeys"
	      :watching-keys="watchingKeys"
      :selected-key="selectedKey"
      @select-history="selectHistory"
      @watch="addWatch"
    />

    <InstrumentHistoryPanel
      :selected="selected"
      :history="history"
      :range="range"
      :loading="historyBusy"
      @range-change="setRange"
      @refresh="refreshSelected"
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

.overview-details { margin: 0 0 20px; }
.overview-details summary { margin-bottom: 12px; cursor: pointer; color: var(--ink-muted); font-weight: 650; }
.history-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 10px 0; color: var(--ink-faint); font-size: 12px; }
.history-controls button, .history-controls select { min-height: 32px; border: 1px solid var(--line); background: var(--paper); color: var(--ink); }
.ranking-summary { margin: 0 0 12px; color: var(--ink-muted); font-size: 12px; }
</style>
