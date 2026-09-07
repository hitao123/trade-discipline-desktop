import { describe, expect, it } from 'vitest'

import { parseExecutionScreenshot } from './execution-screenshot'

describe('parseExecutionScreenshot', () => {
  it('extracts the supplied ETF execution without rounding its three-decimal price', () => {
    const result = parseExecutionScreenshot([
      { text: '515880', confidence: 0.99 },
      { text: '通信ETF国泰', confidence: 0.99 },
      { text: '买入，委托数量8000股', confidence: 0.98 },
      { text: '已成交8,000股，已全部成交', confidence: 0.98 },
      { text: '5,216.00元（成交价格：0.652元）', confidence: 0.98 },
      { text: '2026-08-24 10:34:21', confidence: 0.99 },
    ])

    expect(result).toMatchObject({
      code: '515880',
      market: 'SH',
      name: '通信ETF国泰',
      side: 'buy',
      quantity: 8000,
      localPrice: 0.652,
      settlementYuan: 5216,
      grossYuan: 5216,
      executedAt: '2026-08-24T10:34:21',
      warnings: [],
    })
  })

  it('recognizes a Hong Kong broker screenshot with a 4-digit code', () => {
    const result = parseExecutionScreenshot([
      { text: '0700.HK', confidence: 0.99 },
      { text: '腾讯控股', confidence: 0.99 },
      { text: '买入，委托数量100股', confidence: 0.98 },
      { text: '已成交100股，已全部成交', confidence: 0.98 },
      { text: '48,020.00港币（成交价格：480.20元）', confidence: 0.98 },
      { text: '2026-08-24 10:34:21', confidence: 0.99 },
    ])

    expect(result).toMatchObject({
      code: '0700.HK',
      market: 'HK',
      name: '腾讯控股',
      side: 'buy',
      quantity: 100,
      localPrice: 480.2,
      executedAt: '2026-08-24T10:34:21',
    })
  })
})
