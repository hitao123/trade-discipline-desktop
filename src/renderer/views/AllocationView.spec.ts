import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AllocationView from './AllocationView.vue'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/renderer/lib/api', () => ({ api: { request } }))

const allocation = {
  profileId: 'allocation-main',
  version: {
    id: 'allocation-version-1', profileId: 'allocation-main', version: 1, reason: '初始基准', createdAt: '2026-08-22T08:00:00Z',
    draft: {
      initialCapitalFen: 20_000_000, targetReturnBP: 1_000, targetDeadline: '2026-12-31',
      items: [
        { key: 'tencent', name: '腾讯', targetWeightBP: 3_900, targetShares: 200, instrumentCodes: ['0700.HK'], buyRule: '分批建仓', sellRule: '按再平衡', note: '核心成长' },
        { key: 'semiconductor-equipment-etf-159558', name: '半导体设备 ETF 159558', targetWeightBP: 1_250, targetShares: 0, instrumentCodes: ['159558'], buyRule: '回撤分批', sellRule: '逻辑转弱', note: '已有一万元' },
        { key: 'cash-fund', name: '现金/货基', targetWeightBP: 4_850, targetShares: 0, instrumentCodes: [], buyRule: '保留流动性', sellRule: '再平衡', note: '示例补齐' },
        { key: 'csi300-etf', name: '沪深300 ETF', targetWeightBP: 0, targetShares: 0, instrumentCodes: [], buyRule: '分批', sellRule: '再平衡', note: '' },
        { key: 'gold-etf', name: '黄金 ETF', targetWeightBP: 0, targetShares: 0, instrumentCodes: [], buyRule: '回撤', sellRule: '再平衡', note: '' },
        { key: 'communication-etf', name: '通信 ETF', targetWeightBP: 0, targetShares: 0, instrumentCodes: ['515880'], buyRule: '观察', sellRule: '逻辑转弱', note: '' },
        { key: 'china-internet-etf', name: '中概互联网 ETF', targetWeightBP: 0, targetShares: 0, instrumentCodes: ['513050'], buyRule: '不配置', sellRule: '不配置', note: '' },
      ],
    },
  },
  currentTotalFen: 20_250_000,
  targetTotalFen: 22_000_000,
  returnBP: 125,
  goalGapFen: 1_750_000,
  items: [
    { item: { key: 'tencent', name: '腾讯', targetWeightBP: 3_900, targetShares: 200, instrumentCodes: ['0700.HK'], buyRule: '分批建仓', sellRule: '按再平衡', note: '核心成长' }, currentValueFen: 0, linkedValueFen: 0, manualAdjustmentFen: 0, linkedPositions: [], currentWeightBP: 0, buildTargetFen: 7_800_000, buildGapFen: 7_800_000, rebalanceTargetFen: 7_897_500, rebalanceGapFen: 7_897_500 },
    { item: { key: 'semiconductor-equipment-etf-159558', name: '半导体设备 ETF 159558', targetWeightBP: 1_250, targetShares: 0, instrumentCodes: ['159558'], buyRule: '回撤分批', sellRule: '逻辑转弱', note: '已有一万元' }, currentValueFen: 1_250_000, linkedValueFen: 1_140_000, manualAdjustmentFen: 110_000, linkedPositions: [{ instrumentId: 'sz-159558', code: '159558', name: '半导体设备ETF', quantity: 10000, marketValueFen: 1_140_000 }], currentWeightBP: 617, buildTargetFen: 2_500_000, buildGapFen: 1_250_000, rebalanceTargetFen: 2_531_250, rebalanceGapFen: 1_281_250 },
  ],
  unassignedPositions: [{ instrumentId: 'sh-510050', code: '510050', name: '上证50ETF', quantity: 100, marketValueFen: 30_000 }],
}

const allocationAfterRefresh = structuredClone(allocation)
allocationAfterRefresh.version.draft.items[0]!.targetWeightBP = '3900' as unknown as number

describe('AllocationView', () => {
  beforeEach(() => {
    request.mockReset()
    let allocationLoads = 0
    request.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/allocation' && !init) {
        allocationLoads += 1
        return Promise.resolve({ configured: true, suggestedCapitalFen: 20_000_000, overview: allocationLoads === 1 ? allocation : allocationAfterRefresh })
      }
      if (path === '/api/allocation/versions') return Promise.resolve([allocation.version])
      if (path === '/api/instruments') return Promise.resolve([{ id: 'sz-159558', code: '159558', name: '半导体设备ETF', market: 'SZ', currency: 'CNY', assetType: 'etf', lotSize: 100 }])
      if (path === '/api/allocation/items/semiconductor-equipment-etf-159558/adjustments' && init?.method === 'POST') return Promise.resolve({ id: 'adjustment-2' })
      if (path === '/api/allocation' && init?.method === 'PUT') return Promise.resolve({ id: 'allocation-version-2' })
      throw new Error(`unexpected request: ${path}`)
    })
  })

  it('shows linked holdings and records a signed external adjustment without trading actions', async () => {
    render(AllocationView)
    expect(await screen.findByRole('heading', { name: '资产配置与年末目标' })).toBeTruthy()
    expect(await screen.findByText('¥202,500.00')).toBeTruthy()
    expect(screen.getByText('¥220,000.00')).toBeTruthy()
    expect(screen.getByText('+1.25%')).toBeTruthy()
    expect(screen.getByText('持仓 ¥11,400.00 + 调整 ¥1,100.00')).toBeTruthy()
    expect(screen.getByText('上证50ETF · 510050')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /买入|卖出|下单/ })).toBeNull()

    await fireEvent.update(screen.getByLabelText('资产项目'), 'semiconductor-equipment-etf-159558')
    await fireEvent.update(screen.getByLabelText('外部调整（元）'), '200')
    await fireEvent.click(screen.getByRole('button', { name: '保存调整' }))

    await waitFor(() => expect(request).toHaveBeenCalledWith('/api/allocation/items/semiconductor-equipment-etf-159558/adjustments', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(request.mock.calls.filter(([path]) => path === '/api/allocation')).toHaveLength(2))
    expect(screen.getByText('目标权重合计：100.00%')).toBeTruthy()
  })
})
