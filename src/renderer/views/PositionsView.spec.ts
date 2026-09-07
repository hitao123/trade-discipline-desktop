import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import PositionsView from './PositionsView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

async function renderPositions(query = '') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/positions', component: PositionsView }],
  })
  await router.push(`/positions${query}`)
  await router.isReady()
  return render(PositionsView, { global: { plugins: [router] } })
}

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
    await renderPositions()
    expect(await screen.findByText('已触及风险退出线')).toBeTruthy()
    expect(screen.getByText('每 10 分钟检查一次')).toBeTruthy()
    await fireEvent.click(screen.getByLabelText('卖出'))
    await fireEvent.update(screen.getByLabelText('本次决定理由'), '原始逻辑已被最新数据推翻')
    await fireEvent.click(screen.getByRole('button', { name: '保存复核记录' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/monitor/alerts/alert-1/reviews', expect.objectContaining({ method: 'POST' })))
    expect(screen.queryByText('已触及风险退出线')).toBeNull()
  })

  it('shows the security name and submits a one-step execution correction', async () => {
	request.mockImplementation((path: string, init?: RequestInit) => {
	  if (path === '/api/portfolio') return Promise.resolve({
		availableCashFen: 19_480_000,
		positions: {
		  'sh-515880': { instrumentId: 'sh-515880', code: '515880', name: '通信ETF国泰', market: 'SH', currency: 'CNY', lotSize: 100, quantity: 800, costFen: 52_000, marketValueFen: 52_000, referencePriceMinor: 65, unrealizedPnLFen: 0, realizedPnLFen: 0, isChinaTech: false },
		},
		chinaTechExposureFen: 0, cumulativeLossFen: 0, alibabaObservationTradingDays: 0, disciplineScoreBP: 0,
	  })
	  if (path === '/api/monitor/status') return Promise.resolve({ enabled: false, interval: 'off' })
	  if (path === '/api/monitor/alerts') return Promise.resolve([])
	  if (path === '/api/instruments') return Promise.resolve([{ id: 'sh-515880', market: 'SH', code: '515880', name: '通信ETF国泰', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false }])
	  if (path === '/api/executions?instrumentId=sh-515880') return Promise.resolve([{ id: 'execution-1', instrumentId: 'sh-515880', code: '515880', name: '通信ETF国泰', side: 'buy', quantity: 800, localPriceMinor: 65, localPriceTenThousandth: 6500, localAmountMinor: 52_000, settlementFen: -52_000, executedAt: '2026-08-24T02:34:21Z', emotion: { fearScore: 2, greedScore: 0, revengeScore: 0 }, quickRecord: true }])
	  if (path === '/api/executions/execution-1/correct' && init?.method === 'POST') return Promise.resolve({ id: 'execution-2', classification: 'serious_violation', position: { quantity: 80 }, cashFen: 19_948_000, pendingReview: true })
	  throw new Error(`unexpected ${path}`)
	})

	await renderPositions()
	expect(await screen.findByText('通信ETF国泰')).toBeTruthy()
	expect(screen.getByText('515880')).toBeTruthy()
	await fireEvent.click(screen.getByRole('button', { name: '修正成交' }))
	await fireEvent.click(await screen.findByRole('button', { name: '修正这条' }))
	await fireEvent.update(screen.getByLabelText('修正后数量'), '80')
	await fireEvent.update(screen.getByLabelText('修正原因'), '原成交数量录错')
	await fireEvent.click(screen.getByRole('button', { name: '保存修正' }))

	await waitFor(() => expect(request).toHaveBeenCalledWith('/api/executions/execution-1/correct', expect.objectContaining({ method: 'POST' })))
	const call = request.mock.calls.find(([path]) => path === '/api/executions/execution-1/correct')
	expect(JSON.parse(String(call?.[1]?.body))).toEqual(expect.objectContaining({ reason: '原成交数量录错', draft: expect.objectContaining({ quantity: 80 }) }))
  })

  it('opens the matching alert when arriving from a notification deep link', async () => {
    await renderPositions('?alert=alert-1')
    expect(await screen.findByText('已触及风险退出线')).toBeTruthy()
  })
})
