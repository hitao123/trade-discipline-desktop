import { fireEvent, render, screen } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/renderer/lib/api'
import OnboardingView from './OnboardingView.vue'

vi.mock('@/renderer/lib/api', () => ({
  api: { request: vi.fn() },
}))

describe('OnboardingView', () => {
  it('completes the three-step local setup with integer fen amounts', async () => {
    const profile = {
      id: 'local-user', mode: 'generic', onboardingStatus: 'completed',
      investableCapitalFen: 20_000_000, maxLossFen: 2_000_000,
      holdingHorizon: '6_to_12m', enabledMarkets: ['ashare_etf'], updatedAt: '2026-08-25T08:00:00Z',
    }
    vi.mocked(api.request).mockResolvedValue(profile)
    const { emitted } = render(OnboardingView)

    expect(screen.getByRole('button', { name: '下一步' })).toBeDisabled()
    await fireEvent.update(screen.getByLabelText('可投资总资金（元）'), '200000')
    await fireEvent.update(screen.getByLabelText('最大可承受损失（元）'), '20000')
    expect(screen.getByText('占总资金 10.0%')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    await fireEvent.click(screen.getByLabelText('半年到一年'))
    await fireEvent.click(screen.getByLabelText('A 股 ETF'))
    await fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    for (const label of ['数据只保存在本机', '不会连接券商或自动下单', '新工作区没有任何预设持仓和资产配置'])
      await fireEvent.click(screen.getByLabelText(label))
    await fireEvent.click(screen.getByRole('button', { name: '创建空白工作区' }))

    expect(api.request).toHaveBeenCalledWith('/api/onboarding/complete', {
      method: 'POST',
      body: JSON.stringify({
        investableCapitalFen: 20_000_000,
        maxLossFen: 2_000_000,
        holdingHorizon: '6_to_12m',
        enabledMarkets: ['ashare_etf'],
      }),
    })
    expect(emitted().completed?.[0]).toEqual([profile])
  })
})
