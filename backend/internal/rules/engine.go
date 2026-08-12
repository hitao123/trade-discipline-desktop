package rules

import (
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

type Severity string

const (
	SeverityHard    Severity = "hard"
	SeverityWarning Severity = "warning"
)

type Finding struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
}

type Metrics struct {
	EstimatedCostFen           int64 `json:"estimatedCostFen"`
	AvailableCashAfterFen      int64 `json:"availableCashAfterFen"`
	ChinaTechExposureAfterFen  int64 `json:"chinaTechExposureAfterFen"`
	PressureLossAfterFen       int64 `json:"pressureLossAfterFen"`
	MaximumPlannedLossAfterFen int64 `json:"maximumPlannedLossAfterFen"`
}

type Decision struct {
	Savable   bool      `json:"savable"`
	Qualified bool      `json:"qualified"`
	Findings  []Finding `json:"findings"`
	Metrics   Metrics   `json:"metrics"`
}

func EvaluatePlan(now time.Time, rule Snapshot, portfolio domain.PortfolioState, plan domain.TradePlanDraft) Decision {
	decision := Decision{Savable: true, Findings: make([]Finding, 0)}
	add := func(code, field, message string) {
		decision.Findings = append(decision.Findings, Finding{Code: code, Severity: SeverityHard, Field: field, Message: message})
	}

	if strings.TrimSpace(plan.Thesis) == "" {
		add("THESIS_REQUIRED", "thesis", "请用一句话写清买入逻辑")
	}
	if strings.TrimSpace(plan.Falsification) == "" || strings.TrimSpace(plan.BreakCondition) == "" {
		add("FALSIFICATION_REQUIRED", "falsification", "请写明市场可能错在哪里和逻辑破坏条件")
	}
	if strings.TrimSpace(plan.PricedExpectation) == "" {
		add("PRICED_EXPECTATION_REQUIRED", "pricedExpectation", "请写明当前价格已反映的预期")
	}
	if len(plan.Evidence) == 0 || len(plan.Evidence) > 2 {
		add("EVIDENCE_REQUIRED", "evidence", "请填写一到两个未来可验证证据")
	}
	if strings.TrimSpace(plan.ExitCondition) == "" {
		add("EXIT_CONDITION_REQUIRED", "exitCondition", "请写明目标或估值退出条件")
	}
	if plan.EntryLowMinor <= 0 || plan.EntryHighMinor < plan.EntryLowMinor {
		add("ENTRY_RANGE_INVALID", "entryLowMinor", "计划买入区间必须有效")
	}
	if plan.RiskExitMinor <= 0 || (plan.EntryLowMinor > 0 && plan.RiskExitMinor >= plan.EntryLowMinor) {
		add("RISK_EXIT_INVALID", "riskExitMinor", "风险退出价必须大于零且低于计划买入区间")
	}
	if plan.FearScore < 0 || plan.FearScore > 10 || plan.GreedScore < 0 || plan.GreedScore > 10 || plan.RevengeScore < 0 || plan.RevengeScore > 10 {
		add("EMOTION_SCORE_INVALID", "revengeScore", "情绪评分必须在 0 到 10 之间")
	}
	if plan.Quantity <= 0 {
		add("QUANTITY_REQUIRED", "quantity", "计划数量必须大于零")
	}
	if plan.LotSize <= 0 || plan.Quantity%plan.LotSize != 0 {
		add("BOARD_LOT_REQUIRED", "quantity", "数量必须是当前交易单位的整数倍")
	}
	if plan.EstimatedCostFen <= 0 {
		add("ESTIMATED_COST_REQUIRED", "estimatedCostFen", "预计人民币资金必须大于零")
	}
	minimumEstimatedCost := plan.EntryLowMinor * int64(plan.Quantity)
	if strings.HasSuffix(strings.ToUpper(plan.Code), ".HK") {
		minimumEstimatedCost = minimumEstimatedCost * int64(rule.HKDCNYRateBP) / 10_000
	}
	if plan.EstimatedCostFen > 0 && minimumEstimatedCost > 0 && plan.EstimatedCostFen < minimumEstimatedCost {
		add("ESTIMATED_COST_UNDERSTATED", "estimatedCostFen", "预计人民币资金不能低于计划下限价对应的保守折算金额")
	}
	if plan.EstimatedCostFen > portfolio.AvailableCashFen {
		add("INSUFFICIENT_CASH", "estimatedCostFen", "预计资金超过统一账户可用现金")
	}
	if plan.MaxPlanLossFen <= 0 {
		add("MAX_LOSS_REQUIRED", "maxPlanLossFen", "最大计划损失必须大于零")
	}
	if plan.StressDropBP <= 0 || plan.StressDropBP > 10_000 {
		add("STRESS_DROP_INVALID", "stressDropBP", "压力情景跌幅必须在 0% 到 100% 之间")
	}
	if plan.ReferencePriceAt.IsZero() || now.Sub(plan.ReferencePriceAt) > 48*time.Hour || plan.ReferencePriceAt.After(now) {
		add("STALE_REFERENCE_PRICE", "referencePriceAt", "参考价格已过期或时间无效")
	}
	if !plan.ValidUntil.After(now) {
		add("PLAN_EXPIRED", "validUntil", "计划有效期必须晚于当前时间")
	}
	if !portfolio.ActiveCooldownUntil.IsZero() && now.Before(portfolio.ActiveCooldownUntil) {
		add("ACTIVE_COOLDOWN", "validUntil", "当前冷静期禁止主动开仓")
	}

	existingQuantity := 0
	if existing, ok := portfolio.Positions[plan.InstrumentID]; ok {
		existingQuantity = existing.Quantity
		if existing.UnrealizedPnLFen < 0 && rule.NoAddToLosingInstrument {
			add("NO_ADD_TO_LOSER", "quantity", "同一标的浮亏时禁止继续加仓")
		}
	}
	maxShares := 0
	switch plan.Code {
	case "0700.HK":
		maxShares = rule.TencentMaxShares
	case "9988.HK":
		maxShares = rule.AlibabaMaxShares
	}
	if maxShares > 0 && existingQuantity+plan.Quantity > maxShares {
		add("INSTRUMENT_SHARE_LIMIT", "quantity", "计划后数量超过当前规则的单一证券上限")
	}
	if plan.IsChinaTech && portfolio.ChinaTechUnrealizedPnLFen < 0 && rule.NoCrossInstrumentAveraging {
		add("NO_CROSS_INSTRUMENT_AVERAGING", "instrumentId", "中国科技仓整体浮亏时禁止新增相关风险")
	}

	pressureLoss := portfolio.CurrentPressureLossFen + plan.EstimatedCostFen*int64(plan.StressDropBP)/10_000
	maximumLoss := portfolio.CumulativeLossFen + plan.MaxPlanLossFen
	chinaExposure := portfolio.ChinaTechExposureFen
	if plan.IsChinaTech {
		chinaExposure += plan.EstimatedCostFen
	}
	decision.Metrics = Metrics{
		EstimatedCostFen:           plan.EstimatedCostFen,
		AvailableCashAfterFen:      portfolio.AvailableCashFen - plan.EstimatedCostFen,
		ChinaTechExposureAfterFen:  chinaExposure,
		PressureLossAfterFen:       pressureLoss,
		MaximumPlannedLossAfterFen: maximumLoss,
	}
	if maximumLoss > rule.LossRedLineFen {
		add("PORTFOLIO_LOSS_RED_LINE", "maxPlanLossFen", "计划后最大损失超过 20,000 元组合红线")
	}
	if pressureLoss > rule.LossRedLineFen {
		add("STRESS_LOSS_RED_LINE", "stressDropBP", "计划后压力损失超过 20,000 元组合红线")
	}
	if chinaExposure > rule.ChinaTechLimitFen {
		add("CHINA_TECH_EXPOSURE_LIMIT", "estimatedCostFen", "计划后中国科技敞口超过 80,000 元")
	}
	if plan.Code == "0700.HK" && (portfolio.AlibabaObservationTradingDays < rule.TencentObservationDays || portfolio.DisciplineScoreBP < rule.MinimumDisciplineScoreBP) {
		add("TENCENT_SEQUENCE_GATE", "instrumentId", "腾讯计划需先完成阿里 20 个交易日观察且纪律分不低于 90")
	}

	decision.Qualified = len(decision.Findings) == 0
	return decision
}
