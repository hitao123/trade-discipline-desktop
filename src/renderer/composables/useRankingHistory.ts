import { computed, shallowRef } from 'vue'

import { api } from '@/renderer/lib/api'
import type { RankingComparison, RankingDatesResult } from '@/renderer/types'

type RankingKind = 'stock' | 'etf'
type Requester = (path: string, init?: RequestInit) => Promise<unknown>

export function useRankingHistory(options: { request?: Requester } = {}) {
  const request = options.request ?? ((path: string, init?: RequestInit) => api.request(path, init))
  const kind = shallowRef<RankingKind>('stock')
  const selectedDate = shallowRef<Record<RankingKind, string | undefined>>({ stock: undefined, etf: undefined })
  const dates = shallowRef<Record<RankingKind, RankingDatesResult | undefined>>({ stock: undefined, etf: undefined })
  const comparison = shallowRef<RankingComparison>()
  const loading = shallowRef(false)
  const error = shallowRef('')
  let requestVersion = 0
  const currentDate = computed(() => selectedDate.value[kind.value])
  const currentDates = computed(() => dates.value[kind.value]?.dates ?? [])

  async function load(nextKind = kind.value, date = selectedDate.value[nextKind]) {
    kind.value = nextKind
    const version = ++requestVersion
    loading.value = true; error.value = ''
    try {
      const dateData = await request(`/api/market/rankings/dates?kind=${nextKind}`) as RankingDatesResult
      if (version !== requestVersion) return
      dates.value = { ...dates.value, [nextKind]: dateData }
      const chosen = date ?? dateData.latestDate
      selectedDate.value = { ...selectedDate.value, [nextKind]: chosen }
      if (!chosen) { comparison.value = { snapshot: null, baselineDate: null, baselineSnapshotId: null, reason: 'no_history', entries: [] }; return }
      const compare = await request(`/api/market/rankings/compare?kind=${nextKind}&date=${encodeURIComponent(chosen)}`) as RankingComparison
      if (version === requestVersion) comparison.value = compare
    } catch (cause) {
      if (version === requestVersion) error.value = cause instanceof Error ? cause.message : '榜单历史加载失败'
    } finally { if (version === requestVersion) loading.value = false }
  }
  async function selectDate(date: string) { await load(kind.value, date) }
  async function previous() { const index = currentDates.value.findIndex(item => item.tradeDate === currentDate.value); if (index >= 0 && index < currentDates.value.length - 1) await selectDate(currentDates.value[index + 1].tradeDate) }
  async function next() { const index = currentDates.value.findIndex(item => item.tradeDate === currentDate.value); if (index > 0) await selectDate(currentDates.value[index - 1].tradeDate) }
  return { kind, comparison, currentDate, currentDates, loading, error, load, selectDate, previous, next }
}
