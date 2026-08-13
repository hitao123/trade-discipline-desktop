import { describe, expect, it, vi } from 'vitest'

import type { MarketHistoryResult, MarketOverviewResult, MarketQuote, MarketResult } from '@/renderer/types'

import { useMarketHistory } from './useMarketHistory'

const quote: MarketQuote = {
  tradeDate: '2026-08-11', market: 'SH', code: '600001', name: '示例股票', assetType: 'stock',
  closeMinor: 1_020, changeBP: 120, turnoverFen: 900_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z',
}

function overview(range: '1m' | '3m'): MarketOverviewResult {
  return {
    range,
    aShareTurnover: [{ tradeDate: '2026-08-11', metric: 'ashare_turnover', valueFen: 150_000_000_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }],
    southboundNetBuy: [{ tradeDate: '2026-08-11', metric: 'southbound_net_buy', valueFen: -7_000_000_000, source: 'fixture', sourceTime: '2026-08-11T08:00:00Z' }],
    lastSuccessfulAt: '2026-08-12T08:00:00Z', cached: false,
  }
}

function marketResult(): MarketResult {
  return {
    stock: { id: 'stock-1', tradeDate: '2026-08-11', kind: 'stock', source: 'fixture', fetchedAt: '2026-08-12T08:00:00Z', version: 1, entries: [quote] },
    etf: { id: 'etf-1', tradeDate: '2026-08-11', kind: 'etf', source: 'fixture', fetchedAt: '2026-08-12T08:00:00Z', version: 1, entries: [] },
    overview: overview('3m'),
  }
}

describe('useMarketHistory', () => {
  it('loads the ranking and overview, then refreshes only the selected security once per session', async () => {
    let historySaved = false
    const request = vi.fn(async (path: string, init?: RequestInit) => {
      if (path === '/api/market/snapshots/latest') return marketResult()
      if (path.startsWith('/api/market/overview')) return overview(path.includes('range=1m') ? '1m' : '3m')
      if (path.startsWith('/api/market/history?')) {
        const range = path.includes('range=1m') ? '1m' : '3m'
        return {
          market: 'SH', code: '600001', range, cached: false,
          points: historySaved ? [{ tradeDate: '2026-08-11', market: 'SH', code: '600001', closeMinor: 1_020, turnoverFen: 900_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }] : [],
        } satisfies MarketHistoryResult
      }
      if (path === '/api/market/history/refresh' && init?.method === 'POST') {
        historySaved = true
        return {
          market: 'SH', code: '600001', range: '3m', cached: false, lastSuccessfulAt: '2026-08-12T08:00:00Z',
          points: [{ tradeDate: '2026-08-11', market: 'SH', code: '600001', closeMinor: 1_020, turnoverFen: 900_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }],
        } satisfies MarketHistoryResult
      }
      throw new Error('unexpected request ' + path)
    })
    const state = useMarketHistory({ request })

    await state.load()
    expect(state.market.value?.stock.entries[0]?.code).toBe('600001')
    expect(state.overview.value?.aShareTurnover).toHaveLength(1)

    await state.selectHistory(quote)
    await state.selectHistory(quote)
    const refreshCalls = request.mock.calls.filter(call => call[0] === '/api/market/history/refresh')
    expect(refreshCalls).toHaveLength(1)
    expect(JSON.parse(String(refreshCalls[0]?.[1]?.body))).toEqual({ market: 'SH', code: '600001' })
    expect(state.history.value?.points[0]?.closeMinor).toBe(1_020)

    await state.setRange('1m')
    expect(state.range.value).toBe('1m')
    expect(request.mock.calls.some(call => call[0] === '/api/market/overview?range=1m')).toBe(true)
    expect(request.mock.calls.some(call => String(call[0]).includes('/api/market/history?market=SH&code=600001&range=1m'))).toBe(true)

    await state.refreshSelected()
    expect(state.history.value?.range).toBe('1m')
  })
})
