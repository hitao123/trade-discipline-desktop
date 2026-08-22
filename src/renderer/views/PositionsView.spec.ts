import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PositionsView from './PositionsView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('PositionsView', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/portfolio') return Promise.resolve({ availableCashFen: 18_000_000, positions: {}, chinaTechExposureFen: 0, cumulativeLossFen: 0, alibabaObservationTradingDays: 0, disciplineScoreBP: 0 })
      if (path === '/api/monitor/status') return Promise.resolve({ enabled: true, interval: '10m', lastSuccessfulAt: '2026-08-19T12:00:00.000Z' })
      if (path === '/api/monitor/alerts') return Promise.resolve([{ id: 'alert-1', planId: 'plan-1', instrumentId: 'hk-9988', kind: 'risk_exit', triggerPriceMinor: 8900, thresholdMinor: 9000, source: 'fixture', sourceTime: '2026-08-19T12:00:00.000Z', triggeredAt: '2026-08-19T12:00:00.000Z' }])
      if (path === '/api/monitor/alerts/alert-1/reviews' && init?.method === 'POST') return Promise.resolve({ id: 'review-1' })
      throw new Error(`unexpected ${path}`)
    })
  })

  it('shows monitor status and saves a review for an outstanding key-price alert', async () => {
    render(PositionsView)
    expect(await screen.findByText('已触及风险退出线')).toBeTruthy()
    expect(screen.getByText('每 10 分钟检查一次')).toBeTruthy()
    await fireEvent.click(screen.getByLabelText('卖出'))
    await fireEvent.update(screen.getByLabelText('本次决定理由'), '原始逻辑已被最新数据推翻')
    await fireEvent.click(screen.getByRole('button', { name: '保存复核记录' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/monitor/alerts/alert-1/reviews', expect.objectContaining({ method: 'POST' })))
    expect(screen.queryByText('已触及风险退出线')).toBeNull()
  })
})
