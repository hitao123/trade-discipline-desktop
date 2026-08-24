import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ExecutionsView from './ExecutionsView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('ExecutionsView', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockImplementation((path: string) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100 }])
      if (path === '/api/plans') return Promise.resolve([])
      throw new Error(`unexpected ${path}`)
    })
    Object.defineProperty(window, 'discipline', {
      configurable: true,
      value: {
        recognizeExecutionScreenshot: vi.fn().mockResolvedValue({
          name: '成交截图.png',
          lines: [
            { text: '515880', confidence: 1 },
            { text: '通信ETF国泰', confidence: 0.9 },
            { text: '买入，委托数量8000股', confidence: 1 },
            { text: '5,216.00元（成交价格：0.652元）', confidence: 0.9 },
            { text: '2026-08-24 10:34:21', confidence: 1 },
          ],
        }),
      },
    })
  })

  it('allows honest recording after a serious violation warning', async () => {
    render(ExecutionsView)
	  expect(await screen.findByText('无计划也可以如实保存，系统会记入纪律记录。')).toBeTruthy()
	  expect(screen.getByRole('button', { name: '立即如实入账' })).toBeEnabled()
	  expect(screen.getByText('只写事实，不连接券商，也不会下单。')).toBeTruthy()
  })

  it('can append a reversal with a required reason', async () => {
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100 }])
      if (path === '/api/plans') return Promise.resolve([])
      if (path === '/api/executions' && init?.method === 'POST') return Promise.resolve({ id: 'execution-1', classification: 'serious_violation', violationCode: 'UNPLANNED_EXECUTION', position: { quantity: 100 }, cashFen: 18_795_000 })
      if (path === '/api/executions/execution-1/reverse' && init?.method === 'POST') return Promise.resolve({ id: 'execution-2', classification: 'reversed', position: { quantity: 0 }, cashFen: 20_000_000 })
      throw new Error(`unexpected ${path}`)
    })
    render(ExecutionsView)
	await fireEvent.click(await screen.findByRole('button', { name: '完整补录' }))
    await screen.findByLabelText('证券')
    await fireEvent.update(screen.getByLabelText('本币成交价'), '120')
    await fireEvent.update(screen.getByLabelText('本币成交金额'), '12000')
    await fireEvent.update(screen.getByLabelText(/实际人民币扣款\/到账/), '12050')
    await fireEvent.click(screen.getByRole('button', { name: '确认如实记录' }))
    await fireEvent.update(await screen.findByLabelText('若本次录错，填写冲正原因'), '结算金额录错')
    await fireEvent.click(screen.getByRole('button', { name: '追加冲正' }))
    expect(await screen.findByText('原成交已追加冲正')).toBeTruthy()
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/executions/execution-1/reverse', expect.objectContaining({ method: 'POST' })))
  })

  it('uses the quick endpoint and confirms immediate portfolio update', async () => {
    request.mockImplementation((path: string, init?: RequestInit) => {
	  if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', market: 'HK', code: '9988.HK', name: '阿里巴巴-W', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true }])
	  if (path === '/api/plans') return Promise.resolve([])
	  if (path === '/api/executions/quick' && init?.method === 'POST') return Promise.resolve({ id: 'execution-quick', classification: 'serious_violation', violationCode: 'UNPLANNED_EXECUTION', position: { quantity: 100 }, cashFen: 18_795_000, pendingReview: true })
	  throw new Error(`unexpected ${path}`)
	})
	render(ExecutionsView)
	await fireEvent.update(await screen.findByLabelText('成交均价'), '120')
	await fireEvent.update(screen.getByLabelText('券商实际人民币扣款/到账（元）'), '12050')
	await fireEvent.click(screen.getByRole('button', { name: '立即如实入账' }))
	expect(await screen.findByText('已加入待复盘')).toBeTruthy()
	await waitFor(() => expect(request).toHaveBeenCalledWith('/api/executions/quick', expect.objectContaining({ method: 'POST' })))
  })

  it('prefills from a local screenshot without recording before confirmation', async () => {
    request.mockImplementation((path: string) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'sh-515880', market: 'SH', code: '515880', name: '通信ETF国泰', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false }])
      if (path === '/api/plans') return Promise.resolve([])
      throw new Error(`unexpected ${path}`)
    })

    render(ExecutionsView)
    await fireEvent.click(await screen.findByRole('button', { name: '从成交截图识别' }))
    expect(await screen.findByText('已识别：515880 · 买入 · 8000 股 · 0.652 元')).toBeTruthy()
    expect(screen.getByLabelText('成交均价')).toHaveValue(0.652)
    expect(request).not.toHaveBeenCalledWith('/api/executions/quick', expect.anything())
  })
})
