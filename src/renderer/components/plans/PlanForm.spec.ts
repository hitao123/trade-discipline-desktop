import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import PlanForm from './PlanForm.vue'

const alibaba = { id: 'hk-9988', market: 'HK', code: '9988.HK', name: '阿里巴巴-W', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true } as const
const tencent = { id: 'hk-0700', market: 'HK', code: '0700.HK', name: '腾讯控股', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true } as const

describe('PlanForm', () => {
  it('defaults to the first listed instrument without baking in personal prices', async () => {
    render(PlanForm, { props: { instruments: [alibaba, tencent], busy: false, fieldErrors: {}, initialDraft: undefined } })
    expect(await screen.findByLabelText('证券')).toHaveValue('hk-9988')
    expect(screen.getByLabelText('买入下限（本币）')).toHaveValue(0)
    expect(screen.getByLabelText('买入上限（本币）')).toHaveValue(0)
  })

  it('clears price fields when the instrument changes', async () => {
    render(PlanForm, { props: { instruments: [alibaba, tencent], busy: false, fieldErrors: {}, initialDraft: undefined } })
    await fireEvent.update(await screen.findByLabelText('买入下限（本币）'), '110')
    await fireEvent.update(screen.getByLabelText('买入上限（本币）'), '120')
    await fireEvent.update(screen.getByLabelText('证券'), 'hk-0700')
    expect(screen.getByLabelText('买入下限（本币）')).toHaveValue(0)
    expect(screen.getByLabelText('买入上限（本币）')).toHaveValue(0)
    expect(screen.getByLabelText('计划股数')).toHaveValue(100)
  })

  it('restores every saved field when the draft instrument is not first in the list', async () => {
    render(PlanForm, {
      props: {
        instruments: [alibaba, tencent], busy: false, fieldErrors: {},
        initialDraft: { instrumentId: 'hk-0700', quantity: 500, entryLowMinor: 11_000, entryHighMinor: 12_000, riskExitMinor: 9_000 },
      },
    })
    expect(await screen.findByLabelText('证券')).toHaveValue('hk-0700')
    expect(screen.getByLabelText('计划股数')).toHaveValue(500)
    expect(screen.getByLabelText('买入下限（本币）')).toHaveValue(110)
    expect(screen.getByLabelText('买入上限（本币）')).toHaveValue(120)
    expect(screen.getByLabelText('风险退出价（本币）')).toHaveValue(90)
  })

  it('persists an incomplete date while the user continues editing the draft', async () => {
    const drafts = new Map<string, string>()
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => drafts.get(key) ?? null,
        setItem: (key: string, value: string) => { drafts.set(key, value) },
        removeItem: (key: string) => { drafts.delete(key) },
      },
    })
    render(PlanForm, {
      props: { instruments: [alibaba], busy: false, fieldErrors: {}, initialDraft: undefined, draftStorageKey: 'plan-draft' },
    })

    await fireEvent.update(await screen.findByLabelText('参考价时间'), '')
    await fireEvent.update(screen.getByLabelText('一句话买入逻辑'), '日期稍后补全')

    expect(JSON.parse(drafts.get('plan-draft') ?? '{}')).toMatchObject({
      thesis: '日期稍后补全',
      referencePriceAt: '',
    })
  })
})
