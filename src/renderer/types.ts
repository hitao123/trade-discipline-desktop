export interface Instrument {
  id: string
  market: 'HK' | 'SH' | 'SZ'
  code: string
  name: string
  assetType: 'stock' | 'etf'
  currency: 'CNY' | 'HKD'
  lotSize: number
  isChinaTech: boolean
}

export interface RuleFinding {
  code: string
  severity: 'hard' | 'warning'
  field?: string
  message: string
}

export interface PlanRecord {
  id: string
  ruleVersionId: string
  status: 'draft' | 'qualified' | 'rejected' | 'expired' | 'executed' | 'cancelled'
  draft: Record<string, unknown> & { code?: string; quantity?: number; thesis?: string }
  validation: { savable: boolean; qualified: boolean; findings: RuleFinding[]; metrics: Record<string, number> }
}

export interface Position {
  instrumentId: string
  code: string
  quantity: number
  costFen: number
  marketValueFen: number
  referencePriceMinor: number
  referencePriceAt?: string
  referencePriceSource?: string
  unrealizedPnLFen: number
  realizedPnLFen: number
  isChinaTech: boolean
}

export interface Portfolio {
  availableCashFen: number
  positions: Record<string, Position>
  chinaTechExposureFen: number
  cumulativeLossFen: number
  alibabaObservationTradingDays: number
  disciplineScoreBP: number
}

export interface Dashboard {
  initialCapitalFen: number
  portfolio: Portfolio
  lossCautionFen: number
  lossRedLineFen: number
  lossUsedFen: number
  chinaTechLimitFen: number
  violationCount: number
  allowedAction: string
  cooldown?: { reason: string; expectedEndsAt: string }
  lastMarketFetch?: string
}

export interface MarketQuote {
  tradeDate: string
  market: string
  code: string
  name: string
  assetType: 'stock' | 'etf'
  closeMinor: number
  changeBP: number
  turnoverFen: number
  source: string
  sourceTime: string
}

export interface MarketSnapshot {
  id: string
  tradeDate: string
  kind: 'stock' | 'etf'
  source: string
  fetchedAt: string
  version: number
  entries: MarketQuote[]
}

export interface MarketResult {
  stock: MarketSnapshot
  etf: MarketSnapshot
  errors?: Record<string, string>
}

export interface RuleVersion {
  id: string
  version: number
  snapshot: Record<string, unknown> & {
    initialCapitalFen: number
    lossCautionFen: number
    lossRedLineFen: number
    chinaTechLimitFen: number
  }
  reason: string
  createdAt: string
}
