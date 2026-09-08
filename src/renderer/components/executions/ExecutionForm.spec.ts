import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import ExecutionForm from './ExecutionForm.vue'

const instruments = [
  { id: 'hk-0700', market: 'HK' as const, code: '0700.HK', name: '腾讯控股', assetType: 'stock' as const, currency: 'HKD' as const, lotSize: 100, isChinaTech: true },
]
const plans = [
  { id: 'expired', status: 'qualified' as const, ruleVersionId: 'rule-1', draft: { instrumentId: 'hk-0700', code: '0700.HK', quantity: 100, validUntil: '2020-01-01T00:00:00.000Z' }, validation: { savable: true, qualified: true, findings: [], metrics: {} } },
  { id: 'draft', status: 'draft' as const, ruleVersionId: 'rule-1', draft: { instrumentId: 'hk-0700', code: '0700.HK', quantity: 100, validUntil: '2099-01-01T00:00:00.000Z' }, validation: { savable: true, qualified: false, findings: [], metrics: {} } },
  { id: 'ok', status: 'qualified' as const, ruleVersionId: 'rule-1', draft: { instrumentId: 'hk-0700', code: '0700.HK', quantity: 100, validUntil: '2099-01-01T00:00:00.000Z' }, validation: { savable: true, qualified: true, findings: [], metrics: {} } },
]

describe('ExecutionForm', () => {
  it('lists only qualified and unexpired plans and records emotion', async () => {
    const rendered = render(ExecutionForm, { props: { instruments, plans, busy: false } })
    const planSelect = screen.getByLabelText('对应计划（请明确选择）') as HTMLSelectElement
    const values = [...planSelect.options].map(option => option.value)
    expect(values).toEqual(['', 'ok'])
    await fireEvent.update(screen.getByLabelText('本币成交价'), '480')
    await fireEvent.update(screen.getByLabelText('本币成交金额'), '48000')
    await fireEvent.update(screen.getByLabelText(/实际人民币扣款\/到账/), '44160')
    await fireEvent.click(screen.getByLabelText('如实评分'))
    await fireEvent.update(screen.getByLabelText('害怕 0–10'), '2')
    await fireEvent.click(screen.getByRole('button', { name: '确认如实记录' }))
    const submissions = rendered.emitted('submit') as unknown as Array<[Record<string, unknown>]>
    expect(submissions[0]?.[0]).toEqual(expect.objectContaining({
      instrumentId: 'hk-0700',
      emotion: { state: 'recorded', fearScore: 2, greedScore: 0, revengeScore: 0 },
    }))
  })
})
