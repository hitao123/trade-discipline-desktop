import { describe, expect, it } from 'vitest'

import { formatMonitorNotification, processMonitorAlerts } from './monitor-notifications'

describe('formatMonitorNotification', () => {
  it('uses a calm but actionable risk-exit message', () => {
    expect(formatMonitorNotification({ kind: 'risk_exit', triggerPriceMinor: 8900, thresholdMinor: 9000, sourceTime: '2026-08-19T12:00:00.000Z' })).toEqual({
      title: '持仓关键线提醒',
      body: '已触及风险退出线（触发价 89.00，关键线 90.00）。请打开 Plain Rule 完成复核。',
    })
  })

  it('uses a target-zone message without treating it as a sell instruction', () => {
    expect(formatMonitorNotification({ kind: 'target_zone', triggerPriceMinor: 13200, thresholdMinor: 13000, sourceTime: '2026-08-19T12:00:00.000Z' })).toEqual({
      title: '持仓关键线提醒',
      body: '已进入目标退出区间（触发价 132.00）。请打开 Plain Rule 完成复核。',
    })
  })

  it('shows each alert before marking it notified', async () => {
    const calls: string[] = []
    await processMonitorAlerts(
      [{ id: 'alert-1', kind: 'risk_exit', triggerPriceMinor: 8900, thresholdMinor: 9000, sourceTime: '2026-08-19T12:00:00.000Z' }],
      () => calls.push('show'),
      async id => { calls.push(`mark:${id}`) },
    )
    expect(calls).toEqual(['show', 'mark:alert-1'])
  })
})
