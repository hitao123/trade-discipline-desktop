package rules

import (
	"strings"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

var fixedNow = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

func validAlibabaPlan() domain.TradePlanDraft {
	return domain.TradePlanDraft{
		InstrumentID: "hk-9988", Code: "9988.HK", IsChinaTech: true, LotSize: 100,
		Thesis: "云业务利润率改善", Falsification: "云收入连续两个季度降速", PricedExpectation: "市场只计入温和复苏",
		Evidence: []string{"下季云收入增速", "客户留存率"}, BreakCondition: "云收入同比转负", ExitCondition: "估值进入历史高位或逻辑破坏",
		EntryLowMinor: 11_000, EntryHighMinor: 12_000, RiskExitMinor: 9_000,
		Quantity: 100, EstimatedCostFen: 1_200_000, MaxPlanLossFen: 360_000, StressDropBP: 3_000,
		ReferencePriceAt: fixedNow.Add(-2 * time.Hour), ValidUntil: fixedNow.Add(7 * 24 * time.Hour),
	}
}

func validTencentPlan() domain.TradePlanDraft {
	p := validAlibabaPlan()
	p.InstrumentID = "hk-0700"
	p.Code = "0700.HK"
	p.EstimatedCostFen = 4_800_000
	p.MaxPlanLossFen = 1_440_000
	return p
}

func emptyPortfolio() domain.PortfolioState {
	return domain.PortfolioState{
		AvailableCashFen: 20_000_000,
		Positions:        map[string]domain.PositionState{},
	}
}

func initialRule() Snapshot { return InitialSnapshot() }

func assertFinding(t *testing.T, decision Decision, code string, severity Severity) {
	t.Helper()
	for _, finding := range decision.Findings {
		if finding.Code == code && finding.Severity == severity {
			return
		}
	}
	t.Fatalf("finding %s/%s missing from %#v", code, severity, decision.Findings)
}

func hasFinding(decision Decision, code string) bool {
	for _, finding := range decision.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func TestEvaluatePlanRejectsNonBoardLotButKeepsDraftSavable(t *testing.T) {
	p := validAlibabaPlan()
	p.Quantity = 150
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	if !got.Savable || got.Qualified {
		t.Fatalf("unexpected decision: %#v", got)
	}
	assertFinding(t, got, "BOARD_LOT_REQUIRED", SeverityHard)
}

func TestEvaluatePlanRejectsUnderstatedEstimatedCost(t *testing.T) {
	p := validAlibabaPlan()
	p.EstimatedCostFen = 100_000
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "ESTIMATED_COST_UNDERSTATED", SeverityHard)
}

func TestEvaluatePlanRejectsIncompleteTargetExitRange(t *testing.T) {
	p := validAlibabaPlan()
	p.TargetExitLowMinor = 15_000
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "TARGET_EXIT_RANGE_INVALID", SeverityHard)
}

func TestEvaluatePlanRejectsAveragingDown(t *testing.T) {
	state := emptyPortfolio()
	state.Positions["hk-9988"] = domain.PositionState{InstrumentID: "hk-9988", Quantity: 100, UnrealizedPnLFen: -120_000}
	rule := initialRule()
	rule.NoAddToLosingInstrument = true
	got := EvaluatePlan(fixedNow, rule, state, validAlibabaPlan())
	assertFinding(t, got, "NO_ADD_TO_LOSER", SeverityHard)
}

func TestDefaultRuleDoesNotBlockTencentSequence(t *testing.T) {
	state := emptyPortfolio()
	got := EvaluatePlan(fixedNow, initialRule(), state, validTencentPlan())
	if !got.Qualified {
		t.Fatalf("generic Tencent plan should qualify: %#v", got)
	}
}

func TestQualifiedAlibabaOneLot(t *testing.T) {
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), validAlibabaPlan())
	if !got.Qualified || !got.Savable || len(got.Findings) != 0 {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestQualifiedTencentAfterDisciplineGate(t *testing.T) {
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), validTencentPlan())
	if !got.Qualified {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestEvaluatePlanRejectsMissingReasonsAndStalePrice(t *testing.T) {
	p := validAlibabaPlan()
	p.Thesis = ""
	p.ReferencePriceAt = fixedNow.Add(-72 * time.Hour)
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "THESIS_REQUIRED", SeverityHard)
	assertFinding(t, got, "STALE_REFERENCE_PRICE", SeverityHard)
}

func TestGenericRuleSkipsPersonalGatesButKeepsCommonDiscipline(t *testing.T) {
	rule := GenericSnapshot(20_000_000, 2_000_000)
	state := emptyPortfolio()
	state.ChinaTechUnrealizedPnLFen = -20_000
	state.AlibabaObservationTradingDays = 0
	state.DisciplineScoreBP = 0
	plan := validTencentPlan()
	plan.Quantity = 200
	plan.EstimatedCostFen = 9_600_000
	plan.StressDropBP = 1_000

	got := EvaluatePlan(fixedNow, rule, state, plan)
	for _, code := range []string{
		"INSTRUMENT_SHARE_LIMIT", "TENCENT_SEQUENCE_GATE",
		"CHINA_TECH_EXPOSURE_LIMIT", "NO_CROSS_INSTRUMENT_AVERAGING",
	} {
		if hasFinding(got, code) {
			t.Fatalf("generic rule must not emit %s: %#v", code, got.Findings)
		}
	}
	if !got.Qualified {
		t.Fatalf("generic plan should qualify: %#v", got.Findings)
	}

	state.Positions[plan.InstrumentID] = domain.PositionState{
		InstrumentID:     plan.InstrumentID,
		Quantity:         100,
		UnrealizedPnLFen: -10_000,
	}
	rule.NoAddToLosingInstrument = true
	got = EvaluatePlan(fixedNow, rule, state, plan)
	assertFinding(t, got, "NO_ADD_TO_LOSER", SeverityHard)

	plan.MaxPlanLossFen = 2_000_001
	got = EvaluatePlan(fixedNow, rule, emptyPortfolio(), plan)
	assertFinding(t, got, "PORTFOLIO_LOSS_RED_LINE", SeverityHard)
	for _, finding := range got.Findings {
		if finding.Code == "PORTFOLIO_LOSS_RED_LINE" && strings.Contains(finding.Message, "20,000") {
			t.Fatalf("loss message must use configured amount rather than a hard-coded label: %q", finding.Message)
		}
	}
}

func TestEvaluatePlanRejectsStressLossWhenCurrentDrawdownIsHigh(t *testing.T) {
	state := emptyPortfolio()
	state.CurrentPressureLossFen = 1_800_000
	state.CumulativeLossFen = 1_800_000
	plan := validAlibabaPlan()
	plan.EstimatedCostFen = 1_200_000
	plan.StressDropBP = 3_000
	got := EvaluatePlan(fixedNow, initialRule(), state, plan)
	assertFinding(t, got, "STRESS_LOSS_RED_LINE", SeverityHard)
}
