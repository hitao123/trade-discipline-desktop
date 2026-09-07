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
})
