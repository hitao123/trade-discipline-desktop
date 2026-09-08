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

  function noHistoryComparison(): RankingComparison {
    return { snapshot: null, baselineDate: null, baselineSnapshotId: null, reason: 'no_history', entries: [] }
  }

  async function loadComparison(nextKind: RankingKind, chosen: string | undefined, version: number) {
    comparison.value = undefined
    if (!chosen) {
      comparison.value = noHistoryComparison()
      return
    }
    const compare = await request(`/api/market/rankings/compare?kind=${nextKind}&date=${encodeURIComponent(chosen)}`) as RankingComparison
    if (version === requestVersion) comparison.value = compare
  }

  async function load(nextKind = kind.value, date = selectedDate.value[nextKind]) {
    kind.value = nextKind
    const version = ++requestVersion
    loading.value = true
    error.value = ''
    comparison.value = undefined
    try {
      const dateData = await request(`/api/market/rankings/dates?kind=${nextKind}`) as RankingDatesResult
      if (version !== requestVersion) return
      dates.value = { ...dates.value, [nextKind]: dateData }
      const chosen = date ?? dateData.latestDate
      selectedDate.value = { ...selectedDate.value, [nextKind]: chosen }
      await loadComparison(nextKind, chosen, version)
    } catch (cause) {
      if (version === requestVersion) {
        comparison.value = undefined
        error.value = cause instanceof Error ? cause.message : '榜单历史加载失败'
      }
    } finally { if (version === requestVersion) loading.value = false }
  }
  async function selectDate(date: string) { await load(kind.value, date) }
  async function refreshDates(nextKind = kind.value) {
    const version = ++requestVersion
    loading.value = true
    error.value = ''
    comparison.value = undefined
    try {
      const dateData = await request(`/api/market/rankings/dates?kind=${nextKind}`) as RankingDatesResult
      if (version !== requestVersion) return false
      dates.value = { ...dates.value, [nextKind]: dateData }
      const current = selectedDate.value[nextKind]
      const chosen = current && dateData.dates.some(item => item.tradeDate === current) ? current : dateData.latestDate
      selectedDate.value = { ...selectedDate.value, [nextKind]: chosen }
      await loadComparison(nextKind, chosen, version)
      return Boolean(dateData.latestDate && dateData.latestDate !== chosen)
    } catch (cause) {
      if (version === requestVersion) {
        comparison.value = undefined
        error.value = cause instanceof Error ? cause.message : '榜单历史加载失败'
      }
      return false
    } finally {
      if (version === requestVersion) loading.value = false
    }
  }
  async function previous() { const index = currentDates.value.findIndex(item => item.tradeDate === currentDate.value); const target = index >= 0 ? currentDates.value[index + 1] : undefined; if (target) await selectDate(target.tradeDate) }
  async function next() { const index = currentDates.value.findIndex(item => item.tradeDate === currentDate.value); const target = index > 0 ? currentDates.value[index - 1] : undefined; if (target) await selectDate(target.tradeDate) }
  return { kind, comparison, currentDate, currentDates, loading, error, load, refreshDates, selectDate, previous, next }
}
