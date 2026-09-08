import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/renderer/lib/api'

import WeeklyReviewView from './WeeklyReviewView.vue'

describe('WeeklyReviewView', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })
  it('loads and completes this week pending execution reviews before the weekly review', async () => {
    let completed = false
    const request = vi.spyOn(api, 'request').mockImplementation((path, init) => {
      if (path.startsWith('/api/reviews')) return Promise.resolve(null)
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

  it('loads an existing weekly review when the period changes', async () => {
    const request = vi.spyOn(api, 'request').mockImplementation((path) => {
      if (path.startsWith('/api/reviews?periodStart=2026-08-10')) {
        return Promise.resolve({
          id: 'review-week', periodStart: '2026-08-10', periodEnd: '2026-08-16', disciplineScoreBP: 8000,
          metrics: { cashFen: 18_000_000, chinaTechExposureFen: 0, cumulativeLossFen: 200_000, violationCount: 1 },
          userContent: { impulseNotes: '当时差点追涨', nextAllowedAction: '只跟踪一项证据' },
        })
      }
      if (path.startsWith('/api/reviews')) return Promise.resolve(null)
      if (path === '/api/instruments') return Promise.resolve([])
      if (path.startsWith('/api/post-trade-reviews?')) return Promise.resolve([])
      return Promise.resolve(null)
    })

    render(WeeklyReviewView)
    await fireEvent.update(await screen.findByLabelText('周期开始'), '2026-08-10')
    await fireEvent.update(screen.getByLabelText('周期结束'), '2026-08-16')
    await fireEvent.change(screen.getByLabelText('周期结束'))

    await waitFor(() => expect(request).toHaveBeenCalledWith(expect.stringContaining('/api/reviews?periodStart=2026-08-10')))
    expect(await screen.findByDisplayValue('当时差点追涨')).toBeTruthy()
    expect(screen.getByDisplayValue('只跟踪一项证据')).toBeTruthy()
    expect(screen.getByText('2026-08-10 至 2026-08-16')).toBeTruthy()
  })

  it('keeps each period draft isolated while switching and returns to today for 本周', async () => {
    const drafts = new Map<string, string>()
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => drafts.get(key) ?? null,
        setItem: (key: string, value: string) => { drafts.set(key, value) },
        removeItem: (key: string) => { drafts.delete(key) },
      },
    })
    const request = vi.spyOn(api, 'request').mockImplementation((path) => {
      if (path.startsWith('/api/reviews')) return Promise.resolve(null)
      if (path === '/api/instruments' || path === '/api/executions') return Promise.resolve([])
      if (path.startsWith('/api/post-trade-reviews')) return Promise.resolve([])
      return Promise.resolve(null)
    })
    render(WeeklyReviewView)
    await new Promise(resolve => window.setTimeout(resolve, 0))
    const notes = screen.getByPlaceholderText('写事实：当时想做什么，最后按什么规则处理？')
    const thisWeekStart = (screen.getByLabelText('周期开始') as HTMLInputElement).value
    await fireEvent.update(notes, '本周草稿 A')
    await fireEvent.click(screen.getByRole('button', { name: '上周' }))
    await fireEvent.update(notes, '上周草稿 B')
    await fireEvent.click(screen.getByRole('button', { name: '本周' }))
    await waitFor(() => expect(screen.getByDisplayValue('本周草稿 A')).toBeTruthy())
    expect(screen.getByLabelText('周期开始')).toHaveValue(thisWeekStart)
    expect(request).toHaveBeenCalledWith(expect.stringContaining(`periodStart=${thisWeekStart}`))
  })

  it('does not transfer draft ownership when loading the target period fails', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-19T12:00:00'))
    const drafts = new Map<string, string>([
      ['plain-rule:weekly-review-draft:2026-08-17:2026-08-23', JSON.stringify({ impulseNotes: '本周草稿 A', nextAllowedAction: '' })],
      ['plain-rule:weekly-review-draft:2026-08-10:2026-08-16', JSON.stringify({ impulseNotes: '上周草稿 B', nextAllowedAction: '' })],
    ])
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => drafts.get(key) ?? null,
        setItem: (key: string, value: string) => { drafts.set(key, value) },
        removeItem: (key: string) => { drafts.delete(key) },
      },
    })
    vi.spyOn(api, 'request').mockImplementation((path) => {
      if (path.startsWith('/api/reviews?periodStart=2026-08-10')) return Promise.reject(new Error('复盘读取失败'))
      if (path.startsWith('/api/reviews')) return Promise.resolve(null)
      if (path === '/api/instruments' || path === '/api/executions') return Promise.resolve([])
      if (path.startsWith('/api/post-trade-reviews')) return Promise.resolve([])
      return Promise.resolve(null)
    })

    render(WeeklyReviewView)
    await Promise.resolve()
    await Promise.resolve()
    await fireEvent.click(screen.getByRole('button', { name: '上周' }))
    await Promise.resolve()
    await Promise.resolve()
    await fireEvent.update(screen.getByPlaceholderText('写事实：当时想做什么，最后按什么规则处理？'), '加载失败后继续编辑 A')

    expect(JSON.parse(drafts.get('plain-rule:weekly-review-draft:2026-08-10:2026-08-16') ?? '{}')).toMatchObject({ impulseNotes: '上周草稿 B' })
    expect(JSON.parse(drafts.get('plain-rule:weekly-review-draft:2026-08-17:2026-08-23') ?? '{}')).toMatchObject({ impulseNotes: '加载失败后继续编辑 A' })
  })
})
