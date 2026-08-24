import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import QuickExecutionForm from './QuickExecutionForm.vue'

const hkInstrument = { id: 'hk-0700', market: 'HK', code: '0700.HK', name: '腾讯控股', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true } as const
const cnyInstrument = { id: 'sh-510300', market: 'SH', code: '510300', name: '沪深300ETF', assetType: 'etf', currency: 'CNY', lotSize: 100, isChinaTech: false } as const

describe('QuickExecutionForm', () => {
  it('calculates the local amount and emits an honest HK execution', async () => {
    const rendered = render(QuickExecutionForm, { props: { instruments: [hkInstrument], plans: [], busy: false } })
    expect(screen.getByText('数量').closest('.field-heading')).toHaveTextContent('数量一手 100 股')
    expect(screen.getByText('成交均价').closest('.field-heading')).toHaveTextContent('成交均价HKD')
    await fireEvent.update(screen.getByLabelText('成交均价'), '480')
	  expect(screen.getByText('HK$48,000.00')).toBeTruthy()
    expect(screen.getByText('无计划也可以如实保存，系统会记入纪律记录。')).toBeTruthy()
    await fireEvent.update(screen.getByLabelText('券商实际人民币扣款/到账（元）'), '44160')
    await fireEvent.click(screen.getByRole('button', { name: '立即如实入账' }))
    const submissions = rendered.emitted('submit') as unknown as Array<[Record<string, unknown>]>
    expect(submissions[0]?.[0]).toEqual(expect.objectContaining({
      instrumentId: 'hk-0700', side: 'buy', quantity: 100, localPriceMinor: 48_000, settlementFen: -4_416_000,
    }))
  })

  it('prefills CNY settlement but never invents an HK RMB settlement', async () => {
    const rendered = render(QuickExecutionForm, { props: { instruments: [cnyInstrument, hkInstrument], plans: [], busy: false } })
    await fireEvent.update(screen.getByLabelText('成交均价'), '4.2')
    expect(screen.getByLabelText('券商实际人民币扣款/到账（元）')).toHaveValue(420)
    await fireEvent.update(screen.getByLabelText('证券'), 'hk-0700')
    expect(screen.getByLabelText('券商实际人民币扣款/到账（元）')).toHaveValue(null)
    expect(rendered.getByRole('button', { name: '立即如实入账' })).toBeEnabled()
  })

  it('applies screenshot facts and keeps ETF price precision with user-entered emotion', async () => {
    const communicationETF = { ...cnyInstrument, id: 'sh-515880', code: '515880', name: '通信ETF国泰' }
    const rendered = render(QuickExecutionForm, {
      props: {
        instruments: [communicationETF],
        plans: [],
        busy: false,
        prefill: {
          code: '515880', side: 'buy', quantity: 8000, localPrice: 0.652,
          settlementYuan: 5216, executedAt: '2026-08-24T10:34:21', warnings: [],
        },
      },
    })

    expect(screen.getByLabelText('成交均价')).toHaveValue(0.652)
    expect(screen.getByLabelText('券商实际人民币扣款/到账（元）')).toHaveValue(5216)
    await fireEvent.update(screen.getByLabelText('恐惧 0–10'), '3')
    await fireEvent.update(screen.getByLabelText('贪婪 0–10'), '1')
    await fireEvent.update(screen.getByLabelText('回本/报复性冲动 0–10'), '0')
    await fireEvent.click(screen.getByRole('button', { name: '立即如实入账' }))

    const submissions = rendered.emitted('submit') as unknown as Array<[Record<string, unknown>]>
    expect(submissions[0]?.[0]).toEqual(expect.objectContaining({
      instrumentId: 'sh-515880', side: 'buy', quantity: 8000,
      localPriceMinor: 65, localPriceTenThousandth: 6520, settlementFen: -521_600,
      emotion: { fearScore: 3, greedScore: 1, revengeScore: 0 },
    }))
  })
})
