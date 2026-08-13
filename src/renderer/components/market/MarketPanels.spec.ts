import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import InstrumentHistoryPanel from './InstrumentHistoryPanel.vue'
import MarketOverviewPanel from './MarketOverviewPanel.vue'

describe('market history panels', () => {
  it('shows both market metrics, cache status, and emits a range change', async () => {
    const view = render(MarketOverviewPanel, {
      props: {
        range: '3m',
        loading: false,
        overview: {
          range: '3m',
          aShareTurnover: [{ tradeDate: '2026-08-11', metric: 'ashare_turnover', valueFen: 150_000_000_000_000, source: 'fixture', sourceTime: '2026-08-11T07:00:00Z' }],
          southboundNetBuy: [{ tradeDate: '2026-08-11', metric: 'southbound_net_buy', valueFen: -7_000_000_000, source: 'fixture', sourceTime: '2026-08-11T08:00:00Z' }],
          lastSuccessfulAt: '2026-08-12T08:00:00Z',
          cached: true,
          errors: { southbound_net_buy: '来源暂时不可用' },
        },
      },
    })

    expect(screen.getByText('沪深A股成交额')).toBeTruthy()
    expect(screen.getByText('南向资金成交净买额')).toBeTruthy()
    expect(screen.getAllByText(/正在显示本地缓存/).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: /2026-08-11.*-0.70 亿/ })).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: '近 1 个月' }))
    expect(view.emitted().rangeChange).toEqual([['1m']])
  })

  it('orients the user before a ranking security is selected', () => {
    render(InstrumentHistoryPanel, {
      props: { selected: undefined, history: undefined, range: '3m', loading: false },
    })
    expect(screen.getByText('从榜单选择一只证券查看收盘走势')).toBeTruthy()
  })
})
