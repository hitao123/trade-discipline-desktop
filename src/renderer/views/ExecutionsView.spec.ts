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
  })

  it('allows honest recording after a serious violation warning', async () => {
    render(ExecutionsView)
    expect(await screen.findByText('无计划成交将记为严重违规，但不会阻止保存。')).toBeTruthy()
    expect(screen.getByRole('button', { name: '确认如实记录' })).toBeEnabled()
    expect(screen.getByText('这里只补录，不会向券商发送订单')).toBeTruthy()
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
})
