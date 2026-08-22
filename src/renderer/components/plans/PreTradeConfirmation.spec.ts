import { fireEvent, render, screen } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PreTradeConfirmation from './PreTradeConfirmation.vue'

const plan = {
  id: 'plan-1',
  ruleVersionId: 'rule-1',
  status: 'qualified' as const,
  draft: {
    code: '9988.HK',
    quantity: 100,
    thesis: '云业务利润率改善',
    riskExitMinor: 9000,
    targetExitLowMinor: 13000,
    targetExitHighMinor: 14000,
    estimatedCostFen: 1_200_000,
    maxPlanLossFen: 360_000,
    breakCondition: '云收入同比转负',
    validUntil: '2026-08-26T12:00:00.000Z',
  },
  validation: { savable: true, qualified: true, findings: [], metrics: {} },
}

afterEach(() => vi.useRealTimers())

describe('PreTradeConfirmation', () => {
  it('shows the planned funds, loss limit, thesis break and validity before confirmation', () => {
    render(PreTradeConfirmation, {
      props: { plan, startedAt: '2026-08-19T11:59:31.000Z', busy: false },
    })

    expect(screen.getByText('预计人民币资金：¥12,000.00')).toBeTruthy()
    expect(screen.getByText('最大计划损失：¥3,600.00')).toBeTruthy()
    expect(screen.getByText('逻辑破坏：云收入同比转负')).toBeTruthy()
    expect(screen.getByText(/计划有效至：2026/)).toBeTruthy()
  })

  it('waits 30 seconds and records all three pre-trade attestations', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-19T12:00:00.000Z'))
    const onConfirm = vi.fn()
    render(PreTradeConfirmation, {
      props: {
        plan,
        startedAt: '2026-08-19T11:59:31.000Z',
        busy: false,
        onConfirm,
      },
    })

    await fireEvent.click(screen.getByLabelText('我不是因为害怕错过而买入'))
    await fireEvent.click(screen.getByLabelText('我不是为了扳回亏损而买入'))
    await fireEvent.click(screen.getByLabelText('我不会在浮亏时继续加仓'))
    expect(screen.getByRole('button', { name: '确认后去券商下单' })).toBeDisabled()
    expect(screen.getByText('请再等待 1 秒')).toBeTruthy()

    await vi.advanceTimersByTimeAsync(1_000)
    expect(screen.getByRole('button', { name: '确认后去券商下单' })).toBeEnabled()

    await fireEvent.click(screen.getByRole('button', { name: '确认后去券商下单' }))
    expect(onConfirm).toHaveBeenCalledWith({
      planId: 'plan-1',
      startedAt: '2026-08-19T11:59:31.000Z',
      noFomo: true,
      noLossRecovery: true,
      noAveragingDown: true,
    })
  })
})
