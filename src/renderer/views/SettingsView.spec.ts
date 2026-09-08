import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsView from './SettingsView.vue'

const { request, profile, replaceProfile } = vi.hoisted(() => ({
  request: vi.fn(),
  profile: { value: {
    id: 'local-user', mode: 'generic', onboardingStatus: 'completed', investableCapitalFen: 20_000_000,
    maxLossFen: 2_000_000, holdingHorizon: '6_to_12m', enabledMarkets: ['ashare_stock', 'ashare_etf', 'hk'], updatedAt: '2026-08-25T00:00:00Z',
  } },
  replaceProfile: vi.fn(),
}))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))
vi.mock('@/renderer/composables/useUserProfile', () => ({ useUserProfile: () => ({ profile, replaceProfile }) }))

describe('SettingsView', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/rules' && !init) return Promise.resolve([{ id: 'rule-1', version: 1, reason: '初始规则', createdAt: '2026-08-12T00:00:00Z', snapshot: { initialCapitalFen: 20000000, lossCautionFen: 1500000, lossRedLineFen: 2000000, currencyRatesBP: { CNY: 10000, HKD: 9500 } } }])
      if (path === '/api/audit') return Promise.resolve([])
      if (path === '/api/rules/versions') return Promise.resolve({ id: 'rule-2', version: 2 })
      if (path === '/api/monitor/settings' && !init) return Promise.resolve({ interval: '10m' })
      if (path === '/api/monitor/settings' && init?.method === 'PUT') return Promise.resolve({ interval: '15m' })
      if (path === '/api/profile' && init?.method === 'PUT') return Promise.resolve({ ...profile.value })
      if (path === '/api/cash-events' && init?.method === 'POST') return Promise.resolve({ id: 'cash-1', eventType: 'deposit', amountFen: 500_000, reason: '券商转入' })
      if (path === '/api/cash-events') return Promise.resolve([])
      if (path === '/api/dashboard') return Promise.resolve({ initialCapitalFen: 20_000_000, portfolio: { availableCashFen: 20_000_000, positions: {} } })
      throw new Error(`unexpected ${path}`)
    })
  })

  it('shows cash movements instead of personal share limits', async () => {
    render(SettingsView)
    expect(await screen.findByLabelText('资金变动金额（元）')).toBeTruthy()
    expect(screen.getByText(/当前资金池/)).toBeTruthy()
    expect(screen.queryByText('中国科技敞口上限（元）')).toBeNull()
    expect(screen.queryByText('腾讯顺序门槛（可选）')).toBeNull()
  })

  it('keeps cash movement controls in the settings content column', async () => {
    const { container } = render(SettingsView)
    await screen.findByLabelText('资金变动金额（元）')

    const contentColumn = container.querySelector('.cash-management')
    expect(contentColumn).toContainElement(container.querySelector('.cash-form'))
    expect(contentColumn).toContainElement(container.querySelector('.cash-history'))
  })

  it('requires a reason before creating a new rule version', async () => {
    render(SettingsView)
    const submit = await screen.findByRole('button', { name: '创建规则新版本' })
    expect(submit).toBeDisabled()
    await fireEvent.update(screen.getByLabelText('修改原因'), '降低未来风险额度')
    expect(screen.getByRole('button', { name: '创建规则新版本' })).toBeEnabled()
  })

  it('creates a new rule version with the edited FX table', async () => {
    render(SettingsView)
    await fireEvent.update(await screen.findByLabelText('HKD汇率'), '0.88')
    await fireEvent.update(screen.getByLabelText('修改原因'), '按最新结汇价调整')
    await fireEvent.click(screen.getByRole('button', { name: '创建规则新版本' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/rules/versions', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path]) => path === '/api/rules/versions')
    const snapshot = JSON.parse(String(call?.[1]?.body)).snapshot
    expect(snapshot.currencyRatesBP.HKD).toBe(8800)
    expect(snapshot.hkdCnyRateBP).toBe(8800)
  })

  it('records a cash movement instead of rewriting investable capital', async () => {
    render(SettingsView)
    await fireEvent.update(await screen.findByLabelText('资金变动金额（元）'), '5000')
    await fireEvent.update(screen.getByLabelText('资金变动原因'), '券商转入')
    await fireEvent.click(screen.getByRole('button', { name: '保存资金变动' }))
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/cash-events', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path, init]) => path === '/api/cash-events' && init?.method === 'POST')
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ eventType: 'deposit', amountFen: 500_000, reason: '券商转入' })
  })

  it('saves the local key-price reminder interval independently from rule versions', async () => {
    render(SettingsView)
    await screen.findByLabelText('HKD汇率')
    await fireEvent.update(screen.getByLabelText('提醒检查间隔'), '15m')
    await fireEvent.click(screen.getByRole('button', { name: '保存价格提醒设置' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/monitor/settings', expect.objectContaining({ method: 'PUT' })))
    const call = request.mock.calls.find(([path, init]) => path === '/api/monitor/settings' && init?.method === 'PUT')
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ interval: '15m' })
  })
})
