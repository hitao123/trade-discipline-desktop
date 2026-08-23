import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/renderer/lib/api'

import WeeklyReviewView from './WeeklyReviewView.vue'

describe('WeeklyReviewView', () => {
  it('loads and completes this week pending execution reviews before the weekly review', async () => {
    let completed = false
    const request = vi.spyOn(api, 'request').mockImplementation((path, init) => {
      if (path === '/api/reviews/current') return Promise.resolve(null)
      if (path === '/api/instruments') return Promise.resolve([
        { id: 'hk-0700', code: '0700.HK', name: '腾讯控股', market: 'HK', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true },
      ])
      if (path.startsWith('/api/post-trade-reviews?')) return Promise.resolve(completed ? [] : [{
        id: 'review-1', executionId: 'execution-1', status: 'pending', note: '', instrumentId: 'hk-0700',
        side: 'buy', quantity: 100, localPriceMinor: 48_000, settlementFen: -4_416_000,
        executedAt: '2026-08-21T07:30:00Z', createdAt: '2026-08-21T07:31:00Z',
      }])
      if (path === '/api/post-trade-reviews/execution-1/complete' && init?.method === 'POST') {
        completed = true
        return Promise.resolve({ id: 'review-1', executionId: 'execution-1', status: 'completed' })
      }
      return Promise.resolve(null)
    })

    render(WeeklyReviewView)
    expect(await screen.findByText('0700.HK · 腾讯控股')).toBeTruthy()
    await fireEvent.update(screen.getByLabelText('这笔成交当时发生了什么？'), '这是一次追涨，今后先确认计划。')
    await fireEvent.click(screen.getByRole('button', { name: '完成这笔复盘' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/post-trade-reviews/execution-1/complete', {
      method: 'POST', body: JSON.stringify({ note: '这是一次追涨，今后先确认计划。' }),
    }))
    expect(await screen.findByText('本周没有待补的成交复盘')).toBeTruthy()
  })
})
