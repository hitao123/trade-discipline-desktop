import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import ExecutionCorrectionPanel from './ExecutionCorrectionPanel.vue'

describe('ExecutionCorrectionPanel', () => {
  it('omits an empty planId instead of sending a blank string', async () => {
    const rendered = render(ExecutionCorrectionPanel, {
      props: {
        busy: false,
        instruments: [{ id: 'hk-0700', market: 'HK', code: '0700.HK', name: '腾讯控股', assetType: 'stock', currency: 'HKD', lotSize: 100, isChinaTech: true }],
        record: {
          id: 'execution-1', instrumentId: 'hk-0700', code: '0700.HK', name: '腾讯控股', side: 'buy', quantity: 100,
          localPriceMinor: 48_000, localPriceTenThousandth: 4_800_000, localAmountMinor: 4_800_000, settlementFen: -4_416_000,
          executedAt: '2026-08-12T08:00:00.000Z', emotion: { fearScore: 0, greedScore: 0, revengeScore: 0 }, quickRecord: true,
        },
      },
    })
    await fireEvent.update(screen.getByLabelText('修正原因'), '数量录错')
    await fireEvent.click(screen.getByRole('button', { name: '保存修正' }))
    const submissions = rendered.emitted('submit') as unknown as Array<[{ draft: Record<string, unknown> }]>
    expect(submissions[0]?.[0].draft.planId).toBeUndefined()
  })
})
