import { describe, expect, it, vi } from 'vitest'

import { useRankingHistory } from './useRankingHistory'

const stockDates = { kind: 'stock' as const, latestDate: '2026-08-12', dates: [{ tradeDate: '2026-08-12', quality: 'verified_close' as const }] }
const etfDates = { kind: 'etf' as const, latestDate: '2026-08-12', dates: [{ tradeDate: '2026-08-12', quality: 'verified_close' as const }] }

function comparison(kind: 'stock' | 'etf', version: number) {
  return {
    snapshot: { id: `${kind}-${version}`, kind, tradeDate: '2026-08-12', source: 'fixture', fetchedAt: '2026-08-12T08:00:00Z', version, entries: [] },
    baselineDate: null,
    baselineSnapshotId: null,
    entries: [],
  }
}

describe('useRankingHistory', () => {
  it('clears another kind’s comparison and exposes its loading failure', async () => {
    const request = vi.fn(async (path: string) => {
      if (path.includes('dates?kind=stock')) return stockDates
      if (path.includes('compare?kind=stock')) return comparison('stock', 1)
      if (path.includes('dates?kind=etf')) return etfDates
      throw new Error('ETF 榜单暂不可用')
    })
    const ranking = useRankingHistory({ request })

    await ranking.load('stock')
    expect(ranking.comparison.value?.snapshot?.kind).toBe('stock')

    await ranking.load('etf')
    expect(ranking.comparison.value).toBeUndefined()
    expect(ranking.error.value).toBe('ETF 榜单暂不可用')
  })

  it('keeps the selected date but exposes no rows when that date cannot load', async () => {
    const request = vi.fn(async (path: string) => {
      if (path.includes('dates?kind=stock')) return {
        ...stockDates,
        dates: [stockDates.dates[0], { tradeDate: '2026-08-04', quality: 'verified_close' as const }],
      }
      if (path.includes('date=2026-08-12')) return comparison('stock', 1)
      throw new Error('09-04 榜单暂不可用')
    })
    const ranking = useRankingHistory({ request })

    await ranking.load('stock')
    await ranking.selectDate('2026-08-04')

    expect(ranking.currentDate.value).toBe('2026-08-04')
    expect(ranking.comparison.value).toBeUndefined()
    expect(ranking.error.value).toBe('09-04 榜单暂不可用')
  })

  it('reloads the selected date after the date index refreshes', async () => {
    let comparisonVersion = 1
    const request = vi.fn(async (path: string) => {
      if (path.includes('dates?kind=stock')) return stockDates
      if (path.includes('compare?kind=stock')) return comparison('stock', comparisonVersion)
      throw new Error(`unexpected request ${path}`)
    })
    const ranking = useRankingHistory({ request })

    await ranking.load('stock')
    comparisonVersion = 2
    await ranking.refreshDates('stock')

    expect(ranking.currentDate.value).toBe('2026-08-12')
    expect(ranking.comparison.value?.snapshot?.version).toBe(2)
    expect(request).toHaveBeenCalledTimes(4)
  })
})
