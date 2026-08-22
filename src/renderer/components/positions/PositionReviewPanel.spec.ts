import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'

import PositionReviewPanel from './PositionReviewPanel.vue'

describe('PositionReviewPanel', () => {
  it('requires a reason and records the selected decision after a key-price alert', async () => {
    const onSubmit = vi.fn()
    render(PositionReviewPanel, {
      props: {
        alert: {
          id: 'alert-1',
          planId: 'plan-1',
          instrumentId: 'hk-9988',
          kind: 'risk_exit',
          triggerPriceMinor: 8900,
          thresholdMinor: 9000,
          source: 'fixture',
          sourceTime: '2026-08-19T12:00:00.000Z',
          triggeredAt: '2026-08-19T12:00:00.000Z',
        },
        busy: false,
        onSubmit,
      },
    })

    expect(screen.getByText('已触及风险退出线')).toBeTruthy()
    expect(screen.getByRole('button', { name: '保存复核记录' })).toBeDisabled()
    await fireEvent.click(screen.getByLabelText('继续持有'))
    await fireEvent.update(screen.getByLabelText('本次决定理由'), '等待下一份运营数据验证逻辑')
    await fireEvent.click(screen.getByRole('button', { name: '保存复核记录' }))

    expect(onSubmit).toHaveBeenCalledWith({ alertId: 'alert-1', decision: 'hold', reason: '等待下一份运营数据验证逻辑' })
  })
})
