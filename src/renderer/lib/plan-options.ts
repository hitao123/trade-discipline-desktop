import type { PlanRecord } from '@/renderer/types'

export function activeQualifiedPlans(plans: PlanRecord[], now = Date.now()) {
  return plans.filter((plan) => {
    if (plan.status !== 'qualified')
      return false
    const until = plan.draft.validUntil
    if (!until)
      return true
    return new Date(String(until)).getTime() > now
  })
}

export function preferredInstrumentId<T extends { id: string }>(items: readonly T[], lastId = '') {
  return items.find(item => item.id === lastId)?.id ?? items[0]?.id ?? ''
}
