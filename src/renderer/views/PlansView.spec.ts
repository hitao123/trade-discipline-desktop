import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PlansView from './PlansView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('PlansView', () => {
  beforeEach(() => {
    window.scrollTo = vi.fn()
    request.mockReset()
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100, currency: 'HKD' }])
      if (path === '/api/plans' && !init) return Promise.resolve([])
      if (path === '/api/plans' && init?.method === 'POST') {
        return Promise.resolve({
          id: 'plan-1', status: 'rejected', ruleVersionId: 'rule-1',
          draft: JSON.parse(String(init.body)),
          validation: { qualified: false, savable: true, findings: [{ code: 'BOARD_LOT_REQUIRED', field: 'quantity', severity: 'hard', message: '数量必须是当前交易单位的整数倍' }], metrics: {} },
        })
      }
      throw new Error(`unexpected ${path}`)
    })
  })

  it('saves a 150-share HK plan as rejected and explains board lots', async () => {
    render(PlansView)
    await screen.findByLabelText('证券')
    await fireEvent.update(screen.getByLabelText('计划股数'), '150')
    await fireEvent.click(screen.getByRole('button', { name: '保存并校验' }))
    expect(await screen.findByText('数量必须是当前交易单位的整数倍')).toBeTruthy()
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans', expect.objectContaining({ method: 'POST' })))
  })

  it('revises a plan with a reason instead of replacing history silently', async () => {
    const plan = {
      id: 'plan-1', status: 'qualified', ruleVersionId: 'rule-1',
      draft: {
        instrumentId: 'hk-9988', code: '9988.HK', thesis: '云业务利润率改善', falsification: '云收入降速', pricedExpectation: '温和复苏',
        evidence: ['下季云收入'], breakCondition: '同比转负', exitCondition: '逻辑破坏', entryLowMinor: 11000, entryHighMinor: 12000,
        riskExitMinor: 9000, quantity: 100, estimatedCostFen: 1200000, maxPlanLossFen: 360000, stressDropBP: 3000,
        referencePriceAt: '2026-08-12T07:00:00.000Z', validUntil: '2026-08-19T08:00:00.000Z', fearScore: 0, greedScore: 0, revengeScore: 0,
      },
      validation: { qualified: true, savable: true, findings: [], metrics: {} },
    }
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100, currency: 'HKD' }])
      if (path === '/api/plans' && !init) return Promise.resolve([plan])
      if (path === '/api/plans/plan-1' && init?.method === 'PUT') return Promise.resolve({ ...plan, draft: JSON.parse(String(init.body)).draft })
      throw new Error(`unexpected ${path}`)
    })
    render(PlansView)
    await fireEvent.click(await screen.findByRole('button', { name: '修订（保留原记录）' }))
    await fireEvent.update(screen.getByLabelText('修改原因'), '补充最新财报证据')
    await fireEvent.click(screen.getByRole('button', { name: '保存修订并重新校验' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans/plan-1', expect.objectContaining({ method: 'PUT' })))
    const call = request.mock.calls.find(([path]) => path === '/api/plans/plan-1')
    expect(JSON.parse(String(call?.[1]?.body)).reason).toBe('补充最新财报证据')
  })
})
