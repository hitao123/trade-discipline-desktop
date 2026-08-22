export interface MonitorNotificationAlert {
  kind: 'risk_exit' | 'target_zone'
  triggerPriceMinor: number
  thresholdMinor: number
  sourceTime: string
}

export interface PendingMonitorNotificationAlert extends MonitorNotificationAlert {
  id: string
}

export function formatMonitorNotification(alert: MonitorNotificationAlert) {
  const triggerPrice = (alert.triggerPriceMinor / 100).toFixed(2)
  if (alert.kind === 'risk_exit') {
    const threshold = (alert.thresholdMinor / 100).toFixed(2)
    return {
      title: '持仓关键线提醒',
      body: `已触及风险退出线（触发价 ${triggerPrice}，关键线 ${threshold}）。请打开 Plain Rule 完成复核。`,
    }
  }
  return {
    title: '持仓关键线提醒',
    body: `已进入目标退出区间（触发价 ${triggerPrice}）。请打开 Plain Rule 完成复核。`,
  }
}

export async function processMonitorAlerts(
  alerts: PendingMonitorNotificationAlert[],
  show: (alert: PendingMonitorNotificationAlert, notification: ReturnType<typeof formatMonitorNotification>) => void,
  markNotified: (alertID: string) => Promise<void>,
) {
  for (const alert of alerts) {
    show(alert, formatMonitorNotification(alert))
    await markNotified(alert.id)
  }
}
