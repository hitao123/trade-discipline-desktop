export type UserMode = 'legacy' | 'generic'
export type OnboardingStatus = 'pending' | 'completed'
export type HoldingHorizon = 'under_6m' | '6_to_12m' | '1_to_3y' | 'over_3y' | 'legacy_unspecified'
export type MarketScope = 'ashare_stock' | 'ashare_etf' | 'hk'

export interface UserProfile {
  id: string
  mode: UserMode
  onboardingStatus: OnboardingStatus
  investableCapitalFen: number
  maxLossFen: number
  holdingHorizon: HoldingHorizon
  enabledMarkets: MarketScope[]
  completedAt?: string
  updatedAt: string
}

export interface CompleteOnboardingInput {
  investableCapitalFen: number
  maxLossFen: number
  holdingHorizon: Exclude<HoldingHorizon, 'legacy_unspecified'>
  enabledMarkets: MarketScope[]
}

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
  draft: Record<string, unknown> & { code?: string; quantity?: number; thesis?: string; validUntil?: string }
  validation: { savable: boolean; qualified: boolean; findings: RuleFinding[]; metrics: Record<string, number> }
}

export interface Position {
  instrumentId: string
  code: string
  name: string
  market: 'HK' | 'SH' | 'SZ'
  currency: 'CNY' | 'HKD'
  lotSize: number
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

export interface ExecutionRecord {
  id: string
  planId?: string
  instrumentId: string
  code: string
  name: string
  side: 'buy' | 'sell'
  quantity: number
  localPriceMinor: number
  localPriceTenThousandth: number
  localAmountMinor: number
  settlementFen: number
  exitCode?: string
  evidence?: string
  brokerReference?: string
  emotion: { fearScore: number; greedScore: number; revengeScore: number; state?: 'recorded' | 'unfilled' | 'unknown' }
  executedAt: string
  quickRecord: boolean
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
  localPriceTenThousandth?: number
  settlementFen: number
  emotion?: { fearScore: number; greedScore: number; revengeScore: number; state?: 'recorded' | 'unfilled' | 'unknown' }
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

export type CashEventType = 'deposit' | 'withdrawal' | 'dividend' | 'reversal'

export interface CashEvent {
  id: string
  eventType: CashEventType
  amountFen: number
  reason: string
  occurredAt: string
  originalEventId?: string
}

export interface Dashboard {
  profileMode: UserMode
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
	quality?: 'verified_close' | 'legacy_unverified' | 'manual_unverified' | 'incomplete'
	universeVersion?: string
	qualityReason?: string
}

export interface RankingDate { tradeDate: string, quality: NonNullable<MarketSnapshot['quality']> }
export interface RankingComparisonEntry {
  quote: MarketQuote
  rank: number
  previousRank: number | null
  rankDelta: number | null
  changeState: 'up' | 'down' | 'unchanged' | 'new' | 'unknown'
  streakDays: number | null
  streakExact: boolean
  streakReason?: string
	etfLabel: { trackingIndexId: string | null, trackingIndexName: string | null, assetCategory: string, sourceURL: string, verifiedAt: string } | null
}
export interface RankingComparison {
  snapshot: MarketSnapshot | null
  baselineDate: string | null
  baselineSnapshotId: string | null
  quality?: NonNullable<MarketSnapshot['quality']>
  universeVersion?: string
  reason?: 'no_history' | 'unverified' | 'missing_baseline' | 'calendar_unknown' | 'universe_changed'
  entries: RankingComparisonEntry[]
}
export interface RankingDatesResult { kind: 'stock' | 'etf', dates: RankingDate[], latestDate?: string }

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
    chinaTechLimitFen?: number
    currencyRatesBP?: Record<string, number>
    hkdCnyRateBP?: number
    noAddToLosingInstrument?: boolean
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
  instrumentCodes: string[]
  buyRule: string
  sellRule: string
  note: string
  role?: 'holding' | 'cash'
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
  linkedValueFen: number
  manualAdjustmentFen: number
  linkedPositions: Position[]
  latestAdjustment?: {
    id: string
    itemKey: string
    adjustmentFen: number
    source: 'manual_adjustment'
    observedAt: string
    createdAt: string
  }
}

export interface AllocationOverview {
  profileId: string
  version: AllocationVersion
  currentTotalFen: number
  targetTotalFen: number
  returnBP: number
  goalGapFen: number
  items: AllocationItemProgress[]
  unassignedPositions: Position[]
}

export interface AllocationState {
  configured: boolean
  suggestedCapitalFen: number
  overview?: AllocationOverview
}
