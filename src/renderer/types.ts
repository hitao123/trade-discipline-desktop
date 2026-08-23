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

export interface PostTradeReview {
  id: string
  executionId: string
  status: 'pending' | 'completed'
  note: string
  createdAt: string
  completedAt?: string
  instrumentId: string
  side: 'buy' | 'sell'
  quantity: number
  localPriceMinor: number
  settlementFen: number
  executedAt: string
}

export type MonitorInterval = 'off' | '10m' | '15m' | '30m'

export interface MonitorSettings {
  interval: MonitorInterval
}

export interface MonitorStatus {
  enabled: boolean
  interval: MonitorInterval
  lastAttemptAt?: string
  lastSuccessfulAt?: string
  lastError?: string
}

export interface PriceAlert {
  id: string
  planId: string
  instrumentId: string
  kind: 'risk_exit' | 'target_zone'
  triggerPriceMinor: number
  thresholdMinor: number
  source: string
  sourceTime: string
  triggeredAt: string
  notifiedAt?: string
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
	mode?: 'close' | 'live'
  source: string
  fetchedAt: string
  version: number
  entries: MarketQuote[]
}

export type MarketHealthState = 'live' | 'delayed' | 'cached' | 'unavailable'

export interface MarketComponentHealth {
  state: MarketHealthState
  source?: string
  sourceTime?: string
  lastSuccessfulAt?: string
  message?: string
  detailCode?: string
}

export interface MarketRefreshStatus {
	mode: 'close' | 'live'
	lastAttemptAt?: string
	lastSuccessfulAt?: string
	errors?: Record<string, string>
	components?: Record<string, MarketComponentHealth>
}

export interface MarketResult {
  stock: MarketSnapshot
  etf: MarketSnapshot
	overview: MarketOverviewResult
	errors?: Record<string, string>
	health?: Record<string, MarketComponentHealth>
	status?: MarketRefreshStatus
}

export interface LiveMarketResult extends MarketResult {
	isLive: boolean
	state: 'live' | 'market_closed' | 'degraded' | 'unavailable'
}

export type MarketRange = '1m' | '3m'

export type MarketMetricKind =
  | 'ashare_turnover'
  | 'southbound_net_buy'
  | 'sh_turnover'
  | 'sz_turnover'
  | 'southbound_sh_net_buy'
  | 'southbound_sz_net_buy'

export interface MarketMetricPoint {
  tradeDate: string
  metric: MarketMetricKind
  valueFen: number
  source: string
  sourceTime: string
}

export interface MarketOverviewResult {
  range: MarketRange
  aShareTurnover: MarketMetricPoint[]
  southboundNetBuy: MarketMetricPoint[]
  lastSuccessfulAt?: string
  cached: boolean
  errors?: Record<string, string>
}

export interface MarketDailyBar {
  tradeDate: string
  market: string
  code: string
  closeMinor: number
  turnoverFen: number
  source: string
  sourceTime: string
}

export interface MarketHistoryResult {
  market: string
  code: string
  range: MarketRange
  points: MarketDailyBar[]
  lastSuccessfulAt?: string
  cached: boolean
  error?: string
}

export interface RuleVersion {
  id: string
  version: number
  snapshot: Record<string, unknown> & {
    initialCapitalFen: number
    lossCautionFen: number
    lossRedLineFen: number
    chinaTechLimitFen: number
    enforceTencentSequenceGate?: boolean
    tencentObservationDays?: number
    minimumDisciplineScoreBP?: number
  }
  reason: string
  createdAt: string
}

export interface AllocationItem {
  key: string
  name: string
  targetWeightBP: number
  targetShares: number
  buyRule: string
  sellRule: string
  note: string
}

export interface AllocationDraft {
  initialCapitalFen: number
  targetReturnBP: number
  targetDeadline: string
  items: AllocationItem[]
}

export interface AllocationVersion {
  id: string
  profileId: string
  version: number
  draft: AllocationDraft
  reason: string
  createdAt: string
}

export interface AllocationValueEvent {
  id: string
  profileId?: string
  itemKey: string
  valueFen: number
  source: 'initial_import' | 'manual'
  observedAt: string
  createdAt: string
}

export interface AllocationItemProgress {
  item: AllocationItem
  currentValueFen: number
  currentWeightBP: number
  buildTargetFen: number
  buildGapFen: number
  rebalanceTargetFen: number
  rebalanceGapFen: number
  latestValueRecorded?: AllocationValueEvent
}

export interface AllocationOverview {
  profileId: string
  version: AllocationVersion
  currentTotalFen: number
  targetTotalFen: number
  returnBP: number
  goalGapFen: number
  items: AllocationItemProgress[]
}
