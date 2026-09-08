export type Market = 'HK' | 'SH' | 'SZ'
export type AssetType = 'stock' | 'etf'
export type Currency = 'CNY' | 'HKD'
export type MoneyFen = number

export interface ExecutionEmotion {
  fearScore: number
  greedScore: number
  revengeScore: number
  /** Empty is a legacy record; it must not be displayed as an explicit zero. */
  state?: 'recorded' | 'unfilled' | 'unknown'
}

export interface ApiFieldError {
  code: string
  message: string
  fields?: Record<string, string>
}

export type ApiEnvelope<T> =
  | { ok: true; data: T }
  | { ok: false; error: ApiFieldError }

export interface TradePlanDraft {
  instrumentId: string
  thesis: string
  falsification: string
  pricedExpectation: string
  evidence: string[]
  breakCondition: string
  entryLowMinor: number
  entryHighMinor: number
  riskExitMinor: number
  stressDropBP: number
  exitCondition: string
  quantity: number
  fearScore: number
  greedScore: number
  revengeScore: number
  validUntil: string
}

export interface ExecutionDraft {
  planId?: string
  instrumentId: string
  side: 'buy' | 'sell'
  executedAt: string
  quantity: number
  localPriceMinor: number
  localPriceTenThousandth?: number
  localAmountMinor: number
  settlementFen: MoneyFen
  exitCode?: 'T' | 'B' | 'R' | 'C'
  evidence?: string
  brokerReference?: string
  emotion?: ExecutionEmotion
}
