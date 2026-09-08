import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PlansView from './PlansView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('PlansView', () => {
  afterEach(() => vi.useRealTimers())

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

  async function openNewPlan() {
    await fireEvent.click(await screen.findByRole('button', { name: '新建计划' }))
    await screen.findByLabelText('证券')
  }

  async function advanceToConfirmation() {
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
  }

  it('saves a 150-share HK plan as rejected and explains board lots', async () => {
    render(PlansView)
    await openNewPlan()
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    await fireEvent.update(screen.getByLabelText('计划股数'), '150')
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    await fireEvent.click(screen.getByRole('button', { name: '保存并校验' }))
    expect(await screen.findByText('数量必须是当前交易单位的整数倍')).toBeTruthy()
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans', expect.objectContaining({ method: 'POST' })))
  })

  it('includes a key target price range when saving a plan', async () => {
    render(PlansView)
    await openNewPlan()
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    await fireEvent.update(screen.getByLabelText(/目标退出下限/), '130')
    await fireEvent.update(screen.getByLabelText(/目标退出上限/), '140')
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    await fireEvent.click(screen.getByRole('button', { name: '保存并校验' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path, init]) => path === '/api/plans' && init?.method === 'POST')
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ targetExitLowMinor: 13_000, targetExitHighMinor: 14_000 })
  })

  it('records the three-item pre-trade confirmation for a qualified plan after the pause', async () => {
    const plan = {
      id: 'plan-1', status: 'qualified', ruleVersionId: 'rule-1',
      draft: { instrumentId: 'hk-9988', code: '9988.HK', quantity: 100, thesis: '云业务利润率改善', riskExitMinor: 9000, targetExitLowMinor: 13000, targetExitHighMinor: 14000 },
      validation: { qualified: true, savable: true, findings: [], metrics: {} },
    }
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100, currency: 'HKD' }])
      if (path === '/api/plans' && !init) return Promise.resolve([plan])
      if (path === '/api/plans/plan-1/pre-trade-confirmations' && init?.method === 'POST') return Promise.resolve({ id: 'pretrade-1' })
      throw new Error(`unexpected ${path}`)
    })

    render(PlansView)
    const startButton = await screen.findByRole('button', { name: '开始开仓前确认' })
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-19T12:00:00.000Z'))
    await fireEvent.click(startButton)
    await fireEvent.click(screen.getByLabelText('我不是因为害怕错过而买入'))
    await fireEvent.click(screen.getByLabelText('我不是为了扳回亏损而买入'))
    await fireEvent.click(screen.getByLabelText('我不会在浮亏时继续加仓'))
    await vi.advanceTimersByTimeAsync(30_000)
    await fireEvent.click(screen.getByRole('button', { name: '确认后去券商下单' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans/plan-1/pre-trade-confirmations', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path]) => path === '/api/plans/plan-1/pre-trade-confirmations')
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ noFomo: true, noLossRecovery: true, noAveragingDown: true })
    expect(screen.getByText('已准备去券商执行；开仓前确认已写入本地记录。')).toBeTruthy()
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
    await advanceToConfirmation()
    await fireEvent.click(screen.getByRole('button', { name: '保存修订并重新校验' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/plans/plan-1', expect.objectContaining({ method: 'PUT' })))
    const call = request.mock.calls.find(([path]) => path === '/api/plans/plan-1')
    expect(JSON.parse(String(call?.[1]?.body)).reason).toBe('补充最新财报证据')
  })

  it('does not show a board-lot error for a 100-share revision', async () => {
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
    request.mockImplementation((path: string) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100, currency: 'HKD' }])
      if (path === '/api/plans') return Promise.resolve([plan])
      throw new Error(`unexpected ${path}`)
    })

    render(PlansView)
    await fireEvent.click(await screen.findByRole('button', { name: '修订（保留原记录）' }))
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect((screen.getByLabelText('计划股数') as HTMLInputElement).value).toBe('100')
    expect(screen.queryByText('必须是 100 的整数倍')).toBeNull()
  })

  it('does not present a missing revision reason as active validation', async () => {
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
    request.mockImplementation((path: string) => {
      if (path === '/api/instruments') return Promise.resolve([{ id: 'hk-9988', code: '9988.HK', name: '阿里巴巴-W', lotSize: 100, currency: 'HKD' }])
      if (path === '/api/plans') return Promise.resolve([plan])
      throw new Error(`unexpected ${path}`)
    })

    render(PlansView)
    await fireEvent.click(await screen.findByRole('button', { name: '修订（保留原记录）' }))
    await advanceToConfirmation()

    const submit = screen.getByRole('button', { name: '保存修订并重新校验' }) as HTMLButtonElement
    expect(submit.disabled).toBe(true)
    expect(screen.queryByRole('button', { name: '正在校验…' })).toBeNull()
  })
})
