import type { PlanRecord } from '@/renderer/types'

export function activeQualifiedPlans(plans: PlanRecord[], now = Date.now()) {
  return qualifiedPlansAt(plans, now)
}

export function qualifiedPlansAt(plans: PlanRecord[], at: number | Date) {
  const timestamp = at instanceof Date ? at.getTime() : at
  return plans.filter((plan) => {
    if (plan.status !== 'qualified')
      return false
    const until = plan.draft.validUntil
    if (!until)
      return true
    return new Date(String(until)).getTime() > timestamp
  })
}

export function preferredInstrumentId<T extends { id: string }>(items: readonly T[], lastId = '') {
  return items.find(item => item.id === lastId)?.id ?? items[0]?.id ?? ''
}
