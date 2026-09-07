export interface OCRLine {
  text: string
  confidence: number
}

export interface ExecutionScreenshotDraft {
  code?: string | undefined
  market?: 'HK' | 'SH' | 'SZ' | undefined
  name?: string | undefined
  side?: 'buy' | 'sell' | undefined
  quantity?: number | undefined
  localPrice?: number | undefined
  settlementYuan?: number | undefined
  grossYuan?: number | undefined
  executedAt?: string | undefined
  warnings: string[]
}

export interface ExecutionScreenshotPrefill extends ExecutionScreenshotDraft {
  instrumentId?: string | undefined
}

function numberFrom(raw?: string) {
  if (!raw) return undefined
  const value = Number(raw.replaceAll(',', ''))
  return Number.isFinite(value) ? value : undefined
}

export function parseExecutionScreenshot(lines: OCRLine[]): ExecutionScreenshotDraft {
  const texts = lines.map(line => line.text.trim()).filter(Boolean)
  const joined = texts.join('\n')
  const compact = texts.join(' ')
  const aShareCode = compact.match(/\b([013568]\d{5})\b/)?.[1]
  const hkExplicit = compact.match(/\b(\d{4,5})\.HK\b/i)?.[1]
  const hkHinted = /港股|港币|HKD|香港/.test(compact) ? compact.match(/\b(0?\d{4})\b/)?.[1] : undefined
  const hkBare = !aShareCode ? compact.match(/\b(0\d{3})\b/)?.[1] : undefined
  const rawHK = hkExplicit ?? hkHinted ?? hkBare
  const market = aShareCode ? (aShareCode.startsWith('6') || aShareCode.startsWith('5') ? 'SH' : 'SZ') : rawHK ? 'HK' : undefined
  const code = aShareCode ?? (rawHK ? normalizeHKCode(rawHK) : undefined)
  const side = /买入/.test(compact) ? 'buy' : /卖出/.test(compact) ? 'sell' : undefined
  const quantity = numberFrom(compact.match(/(?:已成交|委托数量)\s*([\d,]+)\s*股/)?.[1])
  const localPrice = numberFrom(compact.match(/成交价格\s*[:：]?\s*([\d,]+(?:\.\d+)?)\s*元/)?.[1])
  const settlementYuan = numberFrom(compact.match(/([\d,]+\.\d{2})\s*元\s*[（(]/)?.[1])
  const timestamp = compact.match(/(20\d{2}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})/)
  const executedAt = timestamp ? `${timestamp[1]}T${timestamp[2]}` : undefined
  const codeIndex = code ? texts.findIndex(text => text.includes(code)) : -1
  const nextLine = codeIndex >= 0 ? texts[codeIndex + 1] : undefined
  const name = nextLine && !/买入|卖出|成交|委托|\d{4}-\d{2}-\d{2}/.test(nextLine) ? nextLine : undefined
  const grossYuan = quantity !== undefined && localPrice !== undefined
    ? Math.round(quantity * localPrice * 100) / 100
    : undefined
  const warnings: string[] = []

  if (!code) warnings.push('未识别到证券代码，请手动选择证券')
  if (!side) warnings.push('未识别到买卖方向，请手动确认')
  if (quantity === undefined) warnings.push('未识别到成交数量，请手动填写')
  if (localPrice === undefined) warnings.push('未识别到成交均价，请手动填写')
  if (settlementYuan === undefined) warnings.push('未识别到成交金额，请按券商实际扣款或到账填写')
  if (!executedAt) warnings.push('未识别到成交时间，请手动确认')
  if (grossYuan !== undefined && settlementYuan !== undefined) {
    const tolerance = Math.max(1, grossYuan * 0.005)
    if (Math.abs(grossYuan - settlementYuan) > tolerance) {
      warnings.push('成交金额与数量乘均价差异较大，请检查券商费用或识别结果')
    }
  }

  return { code, market, name, side, quantity, localPrice, settlementYuan, grossYuan, executedAt, warnings }
}

function normalizeHKCode(raw: string) {
  const digits = raw.replace(/\.HK$/i, '').replace(/^0+/, '')
  return `${digits.padStart(4, '0')}.HK`
}
