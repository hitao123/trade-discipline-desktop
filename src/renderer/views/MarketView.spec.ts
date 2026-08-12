import { fireEvent, render, screen } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MarketView from './MarketView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('MarketView', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({
      stock: { tradeDate: '2026-08-11', entries: [{ code: '600001', name: '示例股票', market: 'SH', closeMinor: 1000, changeBP: 120, turnoverFen: 900000000, assetType: 'stock' }] },
      etf: { tradeDate: '2026-08-11', entries: [{ code: '510300', name: '沪深300ETF', market: 'SH', closeMinor: 420, changeBP: 15, turnoverFen: 800000000, assetType: 'etf' }] },
    })
  })

  it('keeps stock top 20 and ETF top 10 as separate tabs with watch-only actions', async () => {
    render(MarketView)
    expect(await screen.findByText('示例股票')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /买入|下单/ })).toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: 'ETF 前 10' }))
    expect(await screen.findByText('沪深300ETF')).toBeTruthy()
    expect(screen.getByRole('button', { name: '加入观察' })).toBeTruthy()
  })
})
