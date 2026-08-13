import { fireEvent, render, screen } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MarketView from './MarketView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('MarketView', () => {
  beforeEach(() => {
    request.mockReset()
    const overview = {
      range: '3m', cached: false, lastSuccessfulAt: '2026-08-12T08:00:00Z',
      aShareTurnover: [{ tradeDate: '2026-08-11', metric: 'ashare_turnover', valueFen: 150_000_000_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }],
      southboundNetBuy: [{ tradeDate: '2026-08-11', metric: 'southbound_net_buy', valueFen: -7_000_000_000, source: 'fixture', sourceTime: '2026-08-11T08:00:00Z' }],
    }
    const market = {
      stock: { id: 'stock-1', tradeDate: '2026-08-11', kind: 'stock', source: 'fixture', fetchedAt: '2026-08-12T08:00:00Z', version: 1, entries: [{ code: '600001', name: '示例股票', market: 'SH', closeMinor: 1000, changeBP: 120, turnoverFen: 900000000, assetType: 'stock', tradeDate: '2026-08-11', source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }] },
      etf: { id: 'etf-1', tradeDate: '2026-08-11', kind: 'etf', source: 'fixture', fetchedAt: '2026-08-12T08:00:00Z', version: 1, entries: [{ code: '510300', name: '沪深300ETF', market: 'SH', closeMinor: 420, changeBP: 15, turnoverFen: 800000000, assetType: 'etf', tradeDate: '2026-08-11', source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }] },
      overview,
    }
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/market/snapshots/latest') return market
      if (path.startsWith('/api/market/overview')) return overview
      if (path.startsWith('/api/market/history?')) return { market: 'SH', code: '600001', range: '3m', points: [], cached: false }
      if (path === '/api/market/history/refresh') return {
        market: 'SH', code: '600001', range: '3m', cached: false, lastSuccessfulAt: '2026-08-12T08:00:00Z',
        points: [{ tradeDate: '2026-08-11', market: 'SH', code: '600001', closeMinor: 1000, turnoverFen: 900000000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }],
      }
      if (path === '/api/watchlist' && init?.method === 'POST') return { id: 'watch-1' }
      throw new Error(`unexpected request ${path}`)
    })
  })

  it('shows market charts and selected security history while keeping watch-only actions', async () => {
    render(MarketView)
    expect(await screen.findByText('示例股票')).toBeTruthy()
    expect(screen.getByText('沪深A股成交额')).toBeTruthy()
    expect(screen.getByText('南向资金成交净买额')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /买入|下单/ })).toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: '查看走势' }))
    expect(await screen.findByRole('heading', { name: '示例股票' })).toBeTruthy()
    expect(await screen.findByRole('img', { name: /示例股票 收盘价/ })).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: 'ETF 前 10' }))
    expect(await screen.findByText('沪深300ETF')).toBeTruthy()
    expect(screen.getByRole('button', { name: '加入观察' })).toBeTruthy()
  })
})
