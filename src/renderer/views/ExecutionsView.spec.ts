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
      if (path === '/api/executions') return Promise.resolve([])
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
        recognizeExecutionClipboard: vi.fn().mockResolvedValue({
          name: '剪贴板截图',
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
	  expect(await screen.findByText('该证券没有当前合格且未过期的计划；无计划成交仍可保存。')).toBeTruthy()
	  expect(screen.getByLabelText('我已核对计划关联；未选即为真实无计划成交。')).toBeTruthy()
	  expect(screen.getByLabelText('我已核对成交日期与时间，不把历史成交记成今天。')).toBeTruthy()
	  expect(screen.getByRole('button', { name: '立即如实入账' })).toBeEnabled()
	  expect(screen.getByText('只写事实，不连接券商，也不会下单。')).toBeTruthy()
  })

  it('repairs a stored corrupted instrument name when the page opens', async () => {
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'sz-159361', market: 'SZ', code: '159361', name: 'A500ETF� ���', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false }])
      if (path === '/api/plans') return Promise.resolve([])
      if (path === '/api/executions') return Promise.resolve([])
      if (path === '/api/instruments/resolve' && init?.method === 'POST') return Promise.resolve({ id: 'sz-159361', market: 'SZ', code: '159361', name: 'A500ETF易方达', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false })
      throw new Error(`unexpected ${path}`)
    })

    render(ExecutionsView)

    expect(await screen.findByRole('option', { name: '159361 · A500ETF易方达' })).toBeTruthy()
    expect(screen.queryByRole('option', { name: '159361 · A500ETF� ���' })).toBeNull()
  })

  it('can append a reversal with a required reason', async () => {
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100 }])
      if (path === '/api/plans') return Promise.resolve([])
      if (path === '/api/executions' && init?.method === 'POST') return Promise.resolve({ id: 'execution-1', classification: 'serious_violation', violationCode: 'UNPLANNED_EXECUTION', position: { quantity: 100 }, cashFen: 18_795_000 })
      if (path === '/api/executions') return Promise.resolve([])
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
	  if (path === '/api/executions') return Promise.resolve([])
	  if (path === '/api/executions/quick' && init?.method === 'POST') return Promise.resolve({ id: 'execution-quick', classification: 'serious_violation', violationCode: 'UNPLANNED_EXECUTION', position: { quantity: 100 }, cashFen: 18_795_000, pendingReview: true })
	  throw new Error(`unexpected ${path}`)
	})
	render(ExecutionsView)
	await screen.findByRole('option', { name: '9988.HK · 阿里巴巴-W' })
	await fireEvent.update(screen.getByLabelText('成交均价'), '120')
	await fireEvent.update(screen.getByLabelText('券商实际人民币扣款/到账（元）'), '12050')
	await fireEvent.click(screen.getByLabelText('我已核对计划关联；未选即为真实无计划成交。'))
	await fireEvent.click(screen.getByLabelText('我已核对成交日期与时间，不把历史成交记成今天。'))
	await fireEvent.click(screen.getByRole('button', { name: '立即如实入账' }))
	expect(await screen.findByText('已加入待复盘')).toBeTruthy()
	await waitFor(() => expect(request).toHaveBeenCalledWith('/api/executions/quick', expect.objectContaining({ method: 'POST' })))
	expect(screen.getByLabelText('成交均价')).toHaveValue(0)
  })

  it('prefills from a local screenshot without recording before confirmation', async () => {
    request.mockImplementation((path: string) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'sh-515880', market: 'SH', code: '515880', name: '通信ETF国泰', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false }])
      if (path === '/api/plans') return Promise.resolve([])
      if (path === '/api/executions') return Promise.resolve([])
      throw new Error(`unexpected ${path}`)
    })

    render(ExecutionsView)
    await fireEvent.click(await screen.findByRole('button', { name: '从成交截图识别' }))
    expect(await screen.findByText('已识别：515880 · 买入 · 8000 股 · 0.652 元')).toBeTruthy()
    expect(screen.getByLabelText('成交均价')).toHaveValue(0.652)
    expect(request).not.toHaveBeenCalledWith('/api/executions/quick', expect.anything())
  })

  it('prefills directly when a screenshot image is pasted from the clipboard', async () => {
    window.discipline!.recognizeExecutionClipboard = vi.fn().mockResolvedValue({
      name: '剪贴板截图',
      lines: [
        { text: '159361', confidence: 1 },
        { text: '示例ETF', confidence: 0.9 },
        { text: '买入，委托数量9800股', confidence: 1 },
        { text: '11,956.00元（成交价格：1.22元）', confidence: 0.9 },
        { text: '2026-08-25 10:34:21', confidence: 1 },
      ],
    })
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'sz-159361', market: 'SZ', code: '159361', name: 'A500ETF� ���', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false }])
      if (path === '/api/plans') return Promise.resolve([])
      if (path === '/api/executions') return Promise.resolve([])
      if (path === '/api/instruments/resolve' && init?.method === 'POST') return Promise.resolve({ id: 'sz-159361', market: 'SZ', code: '159361', name: 'A500ETF易方达', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false })
      throw new Error(`unexpected ${path}`)
    })

    render(ExecutionsView)
    await screen.findByRole('button', { name: '粘贴截图识别' })
    const paste = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(paste, 'clipboardData', { value: { items: [{ type: 'image/png' }] } })
    window.dispatchEvent(paste)

    expect(await screen.findByText('已识别：159361 · 买入 · 9800 股 · 1.22 元')).toBeTruthy()
    expect(screen.getByLabelText('证券')).toHaveValue('sz-159361')
    expect(screen.getByRole('option', { name: '159361 · A500ETF易方达' })).toBeTruthy()
    expect(screen.getByLabelText('成交均价')).toHaveValue(1.22)
    expect(window.discipline?.recognizeExecutionClipboard).toHaveBeenCalledOnce()
    expect(request).toHaveBeenCalledWith('/api/instruments/resolve', expect.objectContaining({ method: 'POST' }))
    expect(request).not.toHaveBeenCalledWith('/api/executions/quick', expect.anything())
  })
})
