import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import PostTradeReviewQueue from './PostTradeReviewQueue.vue'

describe('PostTradeReviewQueue', () => {
  it('requires a factual note before completing a pending trade review', async () => {
    const rendered = render(PostTradeReviewQueue, {
      props: {
        reviews: [{
          id: 'review-1', executionId: 'execution-1', status: 'pending', note: '',
          instrumentId: 'hk-0700', side: 'buy', quantity: 100, localPriceMinor: 48_000,
          settlementFen: -4_416_000, executedAt: '2026-08-21T07:30:00Z', createdAt: '2026-08-21T07:31:00Z',
        }],
        instrumentNames: { 'hk-0700': '0700.HK · 腾讯控股' },
        busyExecutionId: '',
      },
    })

    expect(screen.getByText('0700.HK · 腾讯控股')).toBeTruthy()
    expect(screen.getByText('买入 100 股')).toBeTruthy()
    expect(screen.getByText('实际结算')).toBeTruthy()
    expect(screen.getByText('−¥44,160.00')).toBeTruthy()

    const button = screen.getByRole('button', { name: '完成这笔复盘' })
    expect(button).toBeDisabled()
    await fireEvent.update(screen.getByLabelText('这笔成交当时发生了什么？'), '看到上涨后冲动下单，今后先完成计划确认。')
    expect(button).toBeEnabled()
    await fireEvent.click(button)

    const events = rendered.emitted('complete') as unknown as Array<[string, string]>
    expect(events[0]).toEqual(['execution-1', '看到上涨后冲动下单，今后先完成计划确认。'])
  })

  it('shows a clear empty state when the week has no pending trades', () => {
    render(PostTradeReviewQueue, { props: { reviews: [], instrumentNames: {}, busyExecutionId: '' } })
    expect(screen.getByText('本周没有待补的成交复盘')).toBeTruthy()
  })
})
