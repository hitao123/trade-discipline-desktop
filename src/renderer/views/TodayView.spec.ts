import { render, screen } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'

import TodayView from './TodayView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('TodayView', () => {
  it('shows generic cash and loss meters without personal share gates', async () => {
    request.mockResolvedValue({
      profileMode: 'generic',
      initialCapitalFen: 20_000_000,
      portfolio: { availableCashFen: 18_000_000, positions: {}, chinaTechExposureFen: 0, cumulativeLossFen: 200_000, alibabaObservationTradingDays: 0, disciplineScoreBP: 8000 },
      lossCautionFen: 1_500_000,
      lossRedLineFen: 2_000_000,
      lossUsedFen: 200_000,
      chinaTechLimitFen: 0,
      violationCount: 1,
      allowedAction: '先写完整计划，再决定是否行动',
    })
    render(TodayView)
    expect(await screen.findByText('先写完整计划，再决定是否行动')).toBeTruthy()
    expect(screen.getByText('损失红线已使用')).toBeTruthy()
    expect(screen.queryByText('中国科技敞口')).toBeNull()
    expect(screen.queryByText('腾讯顺序门槛')).toBeNull()
  })
})
