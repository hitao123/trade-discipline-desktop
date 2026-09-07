import { render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import App from './App.vue'
import { api } from './lib/api'
import { createAppRouter } from './router'

vi.mock('./lib/api', () => ({
  api: { request: vi.fn() },
}))

const completedProfile = {
  id: 'local-user',
  mode: 'generic',
  onboardingStatus: 'completed',
  investableCapitalFen: 20_000_000,
  maxLossFen: 2_000_000,
  holdingHorizon: '6_to_12m',
  enabledMarkets: ['ashare_etf'],
  updatedAt: '2026-08-25T08:00:00Z',
} as const

describe('App', () => {
  beforeEach(() => vi.mocked(api.request).mockReset())

  it('shows only onboarding for a pending fresh workspace', async () => {
    vi.mocked(api.request).mockResolvedValue({ ...completedProfile, onboardingStatus: 'pending' })
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()
    render(App, { global: { plugins: [router], stubs: { RouterView: true } } })

    expect(await screen.findByRole('heading', { name: '先建立你的纪律底线' })).toBeTruthy()
    expect(screen.queryByRole('navigation', { name: '主要功能' })).toBeNull()
  })

  it('renders the nine primary navigation entries after onboarding', async () => {
    vi.mocked(api.request).mockResolvedValue(completedProfile)
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()
    render(App, { global: { plugins: [router], stubs: { RouterView: true } } })

    expect(await screen.findByRole('link', { name: '今日' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '观察名单' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '交易计划' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '成交补录' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '持仓' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '资产配置' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '市场榜单' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '每周复盘' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '规则与设置' })).toBeTruthy()
  })

  it('deep-links a monitor alert to the positions page', async () => {
    let handler: ((alertID: string) => void) | undefined
    Object.defineProperty(window, 'discipline', {
      configurable: true,
      value: {
        onMonitorAlert: (callback: (alertID: string) => void) => {
          handler = callback
          return () => { handler = undefined }
        },
      },
    })
    vi.mocked(api.request).mockResolvedValue(completedProfile)
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()
    render(App, { global: { plugins: [router], stubs: { RouterView: true } } })
    await screen.findByRole('link', { name: '持仓' })
    handler?.('alert-9')
    await waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/positions?alert=alert-9'))
  })

  it('offers a retry when the local service cannot be checked', async () => {
    vi.mocked(api.request).mockImplementation((path) => {
      if (path === '/api/onboarding')
        throw new Error('offline')
      return Promise.resolve([])
    })
    const router = createAppRouter()
    await router.push('/')
    await router.isReady()
    render(App, { global: { plugins: [router], stubs: { RouterView: true } } })

    expect(await screen.findByRole('button', { name: '重新检查本地服务' })).toBeTruthy()
  })

})
