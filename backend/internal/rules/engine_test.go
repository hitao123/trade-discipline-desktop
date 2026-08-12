package rules

import (
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

func TestEvaluatePlanRejectsNonBoardLotButKeepsDraftSavable(t *testing.T) {
	p := validAlibabaPlan()
	p.Quantity = 150
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	if !got.Savable || got.Qualified {
		t.Fatalf("unexpected decision: %#v", got)
	}
	assertFinding(t, got, "BOARD_LOT_REQUIRED", SeverityHard)
}

func TestEvaluatePlanRejectsFirstPositionAboveShareLimit(t *testing.T) {
	p := validAlibabaPlan()
	p.Quantity = 200
	p.EstimatedCostFen = 2_400_000
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "INSTRUMENT_SHARE_LIMIT", SeverityHard)
}

func TestEvaluatePlanRejectsUnderstatedEstimatedCost(t *testing.T) {
	p := validAlibabaPlan()
	p.EstimatedCostFen = 100_000
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "ESTIMATED_COST_UNDERSTATED", SeverityHard)
}

func TestEvaluatePlanRejectsAveragingDown(t *testing.T) {
	state := emptyPortfolio()
	state.Positions["hk-9988"] = domain.PositionState{InstrumentID: "hk-9988", Quantity: 100, UnrealizedPnLFen: -120_000}
	got := EvaluatePlan(fixedNow, initialRule(), state, validAlibabaPlan())
	assertFinding(t, got, "NO_ADD_TO_LOSER", SeverityHard)
}

func TestEvaluatePlanRejectsCrossInstrumentChinaTechAveraging(t *testing.T) {
	state := emptyPortfolio()
	state.ChinaTechUnrealizedPnLFen = -20_000
	got := EvaluatePlan(fixedNow, initialRule(), state, validTencentPlan())
	assertFinding(t, got, "NO_CROSS_INSTRUMENT_AVERAGING", SeverityHard)
}

func TestTencentRequiresTwentyTradingDaysAndScoreNinety(t *testing.T) {
	state := emptyPortfolio()
	state.AlibabaObservationTradingDays = 19
	state.DisciplineScoreBP = 9_100
	got := EvaluatePlan(fixedNow, initialRule(), state, validTencentPlan())
	assertFinding(t, got, "TENCENT_SEQUENCE_GATE", SeverityHard)
}

func TestQualifiedAlibabaOneLot(t *testing.T) {
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), validAlibabaPlan())
	if !got.Qualified || !got.Savable || len(got.Findings) != 0 {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestQualifiedTencentAfterDisciplineGate(t *testing.T) {
	state := emptyPortfolio()
	state.AlibabaObservationTradingDays = 20
	state.DisciplineScoreBP = 9_000
	got := EvaluatePlan(fixedNow, initialRule(), state, validTencentPlan())
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
