import { computed, shallowRef } from 'vue'

import { api } from '@/renderer/lib/api'
import type {
  MarketHistoryResult,
  MarketOverviewResult,
  MarketQuote,
  MarketRange,
  MarketResult,
  LiveMarketResult,
} from '@/renderer/types'

type Requester = (path: string, init?: RequestInit) => Promise<unknown>

interface UseMarketHistoryOptions {
  request?: Requester
}

export function useMarketHistory(options: UseMarketHistoryOptions = {}) {
  const request: Requester = options.request ?? ((path, init) => api.request<unknown>(path, init))
  const market = shallowRef<MarketResult>()
  const liveMarket = shallowRef<LiveMarketResult>()
  const overview = shallowRef<MarketOverviewResult>()
  const history = shallowRef<MarketHistoryResult>()
  const selected = shallowRef<MarketQuote>()
  const tab = shallowRef<'stock' | 'etf'>('stock')
  const mode = shallowRef<'close' | 'live'>('close')
  const range = shallowRef<MarketRange>('3m')
  const busy = shallowRef(false)
  const historyBusy = shallowRef(false)
  const error = shallowRef('')
  const notice = shallowRef('')
  const refreshedThisSession = new Set<string>()

  const activeMarket = computed<MarketResult | LiveMarketResult | undefined>(() => mode.value === 'close' ? market.value : liveMarket.value)
  const activeSnapshot = computed(() => tab.value === 'stock' ? activeMarket.value?.stock : activeMarket.value?.etf)
  const activeStatus = computed(() => activeMarket.value?.status)
  const selectedKey = computed(() => selected.value ? selected.value.market + '-' + selected.value.code : '')

  async function requestAs<T>(path: string, init?: RequestInit) {
    return await request(path, init) as T
  }

  async function loadOverview() {
    overview.value = await requestAs<MarketOverviewResult>('/api/market/overview?range=' + range.value)
  }

  async function load() {
    busy.value = true
    error.value = ''
    const [marketResult, overviewResult, liveResult] = await Promise.allSettled([
      requestAs<MarketResult>('/api/market/snapshots/latest'),
      requestAs<MarketOverviewResult>('/api/market/overview?range=' + range.value),
      requestAs<LiveMarketResult>('/api/market/live/latest'),
    ])
    if (marketResult.status === 'fulfilled') {
      market.value = marketResult.value
    }
    if (overviewResult.status === 'fulfilled') {
      overview.value = overviewResult.value
    }
    if (liveResult.status === 'fulfilled') {
      liveMarket.value = liveResult.value
    }
    const failures = [marketResult, overviewResult]
      .filter((result): result is PromiseRejectedResult => result.status === 'rejected')
      .map(result => result.reason instanceof Error ? result.reason.message : '市场数据加载失败')
    error.value = failures.join('；')
    busy.value = false
  }

  async function refreshAll() {
    busy.value = true
    error.value = ''
    notice.value = ''
    try {
      market.value = await requestAs<MarketResult>('/api/market/refresh', { method: 'POST' })
      await loadOverview()
      const failed = Object.keys(market.value.errors ?? {})
      notice.value = failed.length > 0
        ? '部分数据源暂时不可用，失败部分继续显示本地缓存'
        : '收盘榜单与市场概览刷新完成'
    }
    catch (cause) {
      error.value = cause instanceof Error ? cause.message + '。最近成功数据仍保留在本机。' : '市场刷新失败'
    }
    finally {
      busy.value = false
    }
  }

  async function refreshLive() {
    busy.value = true
    error.value = ''
    notice.value = ''
    try {
      liveMarket.value = await requestAs<LiveMarketResult>('/api/market/live/refresh', { method: 'POST' })
      notice.value = liveMarket.value.state === 'market_closed'
        ? '当前不在 A 股交易时段，未请求实时行情；将保留最近一次本地快照。'
        : Object.keys(liveMarket.value.errors ?? {}).length > 0
          ? '部分实时来源暂时不可用，失败部分继续显示本地快照。'
          : '实时成交额数据已更新；仅供观察，不会发出交易指令。'
    }
    catch (cause) {
      error.value = cause instanceof Error ? cause.message + '。最近成功数据仍保留在本机。' : '实时行情刷新失败'
    }
    finally {
      busy.value = false
    }
  }

  async function loadHistoryCache(quote: MarketQuote) {
    const query = new URLSearchParams({ market: quote.market, code: quote.code, range: range.value })
    return requestAs<MarketHistoryResult>('/api/market/history?' + query.toString())
  }

  function refreshedKey(quote: MarketQuote) {
    return quote.market + '-' + quote.code
  }

  async function selectHistory(quote: MarketQuote, force = false) {
    selected.value = quote
    historyBusy.value = true
    error.value = ''
    try {
      const cached = await loadHistoryCache(quote)
      history.value = cached
      const key = refreshedKey(quote)
      const fetchedToday = cached.lastSuccessfulAt
        ? new Date(cached.lastSuccessfulAt).toDateString() === new Date().toDateString()
        : false
      if (force || ((!cached.points.length || !fetchedToday) && !refreshedThisSession.has(key))) {
        const refreshed = await requestAs<MarketHistoryResult>('/api/market/history/refresh', {
          method: 'POST',
          body: JSON.stringify({ market: quote.market, code: quote.code }),
        })
        refreshedThisSession.add(key)
        if (range.value === '3m') {
          history.value = refreshed
        }
        else {
          const ranged = await loadHistoryCache(quote)
          const lastSuccessfulAt = refreshed.lastSuccessfulAt ?? ranged.lastSuccessfulAt
          history.value = {
            ...ranged,
            cached: refreshed.cached,
            ...(lastSuccessfulAt ? { lastSuccessfulAt } : {}),
            ...(refreshed.error ? { error: refreshed.error } : {}),
          }
        }
      }
    }
    catch (cause) {
      error.value = cause instanceof Error ? cause.message : '证券走势加载失败'
    }
    finally {
      historyBusy.value = false
    }
  }

  async function refreshSelected() {
    if (selected.value) await selectHistory(selected.value, true)
  }

  async function setRange(nextRange: MarketRange) {
    if (range.value === nextRange) return
    range.value = nextRange
    error.value = ''
    const tasks: Promise<unknown>[] = [loadOverview()]
    if (selected.value) {
      tasks.push(loadHistoryCache(selected.value).then(result => {
        history.value = result
      }))
    }
    try {
      await Promise.all(tasks)
    }
    catch (cause) {
      error.value = cause instanceof Error ? cause.message : '历史范围切换失败'
    }
  }

  function setTab(nextTab: 'stock' | 'etf') {
    tab.value = nextTab
  }

  function setMode(nextMode: 'close' | 'live') {
    mode.value = nextMode
    notice.value = ''
  }

  return {
    market,
    liveMarket,
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
    selectedKey,
    load,
    refreshAll,
    refreshLive,
    selectHistory,
    refreshSelected,
    setRange,
    setTab,
    setMode,
  }
}
