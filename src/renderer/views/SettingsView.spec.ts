import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
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
      if (path === '/api/monitor/settings' && !init) return Promise.resolve({ interval: '10m' })
      if (path === '/api/monitor/settings' && init?.method === 'PUT') return Promise.resolve({ interval: '15m' })
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

  it('creates a new rule version with the Tencent sequence gate disabled by default', async () => {
    render(SettingsView)
    await fireEvent.update(await screen.findByLabelText('修改原因'), '腾讯顺序门槛改为可选提醒')
    await fireEvent.click(screen.getByRole('button', { name: '创建规则新版本' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/rules/versions', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path]) => path === '/api/rules/versions')
    expect(JSON.parse(String(call?.[1]?.body)).snapshot.enforceTencentSequenceGate).toBe(false)
  })

  it('persists enabled Tencent sequence thresholds in a new rule version', async () => {
    render(SettingsView)
    const gate = await screen.findByRole('checkbox', { name: '启用腾讯顺序门槛' })
    await fireEvent.click(gate)
    await fireEvent.update(screen.getByLabelText('阿里观察交易日'), '15')
    await fireEvent.update(screen.getByLabelText('最低纪律分'), '85')
    await fireEvent.update(screen.getByLabelText('修改原因'), '将腾讯顺序门槛改为个人观察规则')
    await fireEvent.click(screen.getByRole('button', { name: '创建规则新版本' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/rules/versions', expect.objectContaining({ method: 'POST' })))
    const call = request.mock.calls.find(([path]) => path === '/api/rules/versions')
    const snapshot = JSON.parse(String(call?.[1]?.body)).snapshot
    expect(snapshot.enforceTencentSequenceGate).toBe(true)
    expect(snapshot.tencentObservationDays).toBe(15)
    expect(snapshot.minimumDisciplineScoreBP).toBe(8_500)
  })

  it('saves the local key-price reminder interval independently from rule versions', async () => {
    render(SettingsView)
    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/monitor/settings'))
    await fireEvent.update(await screen.findByLabelText('提醒检查间隔'), '15m')
    await fireEvent.click(screen.getByRole('button', { name: '保存价格提醒设置' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/monitor/settings', expect.objectContaining({ method: 'PUT' })))
    const call = request.mock.calls.find(([path, init]) => path === '/api/monitor/settings' && init?.method === 'PUT')
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ interval: '15m' })
  })
})
