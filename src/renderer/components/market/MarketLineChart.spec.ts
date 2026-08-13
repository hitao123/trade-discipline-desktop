import { render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import MarketLineChart from './MarketLineChart.vue'

describe('MarketLineChart', () => {
  it('renders an accessible zero axis and focusable values for a signed series', () => {
    render(MarketLineChart, {
      props: {
        label: '南向资金成交净买额',
        points: [
          { date: '2026-08-11', value: -7_000_000_000 },
          { date: '2026-08-12', value: 3_000_000_000 },
        ],
        formatValue: (value: number) => `${value >= 0 ? '+' : ''}${(value / 10_000_000_000).toFixed(2)} 亿`,
      },
    })

    expect(screen.getByRole('img', { name: /南向资金成交净买额/ })).toBeTruthy()
    expect(screen.getByLabelText('零轴')).toBeTruthy()
    expect(screen.getByRole('button', { name: /2026-08-11.*-0.70 亿/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: /2026-08-12.*\+0.30 亿/ })).toBeTruthy()
    expect(screen.getByText('08-11')).toBeTruthy()
    expect(screen.getByText('08-12')).toBeTruthy()
  })

  it('shows a clear empty state instead of an invalid SVG path', () => {
    render(MarketLineChart, {
      props: {
        label: '收盘价',
        points: [],
        formatValue: (value: number) => String(value),
        emptyText: '选择榜单证券后获取走势',
      },
    })

    expect(screen.getByText('选择榜单证券后获取走势')).toBeTruthy()
    expect(screen.queryByRole('img')).toBeNull()
  })
})
