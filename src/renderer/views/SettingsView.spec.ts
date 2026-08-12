import { fireEvent, render, screen } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsView from './SettingsView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

describe('SettingsView', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/rules' && !init) return Promise.resolve([{ id: 'rule-1', version: 1, reason: '初始规则', createdAt: '2026-08-12T00:00:00Z', snapshot: { initialCapitalFen: 20000000, lossCautionFen: 1500000, lossRedLineFen: 2000000, chinaTechLimitFen: 8000000 } }])
      if (path === '/api/audit') return Promise.resolve([])
      if (path === '/api/rules/versions') return Promise.resolve({ id: 'rule-2', version: 2 })
      throw new Error(`unexpected ${path}`)
    })
  })

  it('requires a reason before creating a new rule version', async () => {
    render(SettingsView)
    await screen.findByDisplayValue('20000')
    expect(screen.getByRole('button', { name: '创建规则新版本' })).toBeDisabled()
    await fireEvent.update(screen.getByLabelText('修改原因'), '降低未来风险额度')
    expect(screen.getByRole('button', { name: '创建规则新版本' })).toBeEnabled()
  })
})
