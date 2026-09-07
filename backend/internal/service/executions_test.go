package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func openExecutionService(t *testing.T) *Service {
	t.Helper()
	svc := openExecutionServiceWithoutSeed(t)
	seedLegacyTestData(t, svc.store)
	return svc
}

func openExecutionServiceWithoutSeed(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := New(db, func() time.Time { return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC) })
	svc.SetMarketRankingFallback(fakeMarketProvider{})
	svc.SetMarketQuoteFallback(nil)
	return svc
}

func TestPortfolioUsesLatestHKCloseWithConservativeRMBRate(t *testing.T) {
	svc := openExecutionService(t)
	_, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.store.UpdateQuotes(context.Background(), []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 12_200, Source: "fixture", SourceTime: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	position := portfolio.Positions["hk-9988"]
	if position.MarketValueFen != 1_159_000 || position.UnrealizedPnLFen != -46_000 || portfolio.CumulativeLossFen != 46_000 {
		t.Fatalf("portfolio=%#v position=%#v", portfolio, position)
	}
	if position.Name != "阿里巴巴-W" || position.Market != "HK" || position.Currency != "HKD" || position.LotSize != 100 {
		t.Fatalf("missing instrument metadata: %#v", position)
	}
}

func TestExecutionUsesGenericRuleWithoutPersonalShareLimit(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	if _, err := svc.store.DB().Exec(`INSERT INTO instruments(
		id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status
	) VALUES('hk-0700','HK','0700.HK','测试港股','stock','HKD',100,'test',1,0,'active')`); err != nil {
		t.Fatal(err)
	}
	draft := validPlanDraft()
	draft.InstrumentID = "hk-0700"
	draft.EntryLowMinor = 47_000
	draft.EntryHighMinor = 49_000
	draft.RiskExitMinor = 40_000
	draft.Quantity = 200
	draft.EstimatedCostFen = 9_600_000
	draft.MaxPlanLossFen = 960_000
	draft.StressDropBP = 1_000
	plan, err := svc.CreatePlan(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "qualified" {
		t.Fatalf("generic plan should not use the legacy Tencent cap: %#v", plan.Validation.Findings)
	}
	receipt, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		PlanID: plan.ID, InstrumentID: "hk-0700", Side: "buy", Quantity: 200,
		SettlementFen: -9_600_000, ExecutedAt: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Classification != "compliant" || receipt.Position.Quantity != 200 {
		t.Fatalf("receipt=%#v", receipt)
	}
}

func TestGenericPortfolioCashUsesAccountInsteadOfLatestRuleReferenceCapital(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	changed := rules.GenericSnapshot(30_000_000, 3_000_000)
	if _, err := svc.store.CreateRuleVersion(context.Background(), "测试风险基准变化", changed, time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.AvailableCashFen != 20_000_000 {
		t.Fatalf("cash must replay from immutable account funding, got %d", portfolio.AvailableCashFen)
	}
}

func TestPortfolioDisciplineScoreFromLatestReview(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RecordExecution(context.Background(), ExecutionDraft{
		PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000,
		ExecutedAt: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	inserted := 0
	for inserted < 20 {
		if date.Weekday() != time.Saturday && date.Weekday() != time.Sunday {
			_, err := svc.store.DB().Exec(`INSERT INTO market_snapshots(id, trade_date, ranking_kind, source, fetched_at, status, version) VALUES(?,?,?,?,?,'success',1)`, store.NewID("market"), date.Format("2006-01-02"), market.KindStock, "fixture", date.Format(time.RFC3339Nano))
			if err != nil {
				t.Fatal(err)
			}
			inserted++
		}
		date = date.AddDate(0, 0, 1)
	}
	svc.now = func() time.Time { return date }
	if _, err := svc.SaveWeeklyReview(context.Background(), WeeklyReviewDraft{PeriodStart: "2026-08-10", PeriodEnd: date.Format("2006-01-02"), NextAllowedAction: "只按已合格计划行动"}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.DisciplineScoreBP != 10_000 {
		t.Fatalf("unexpected discipline progress: %#v", portfolio)
	}
}

func TestUnplannedExecutionUpdatesHoldingAndCreatesCooldown(t *testing.T) {
	svc := openExecutionService(t)
	receipt, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
		Quantity: 100, LocalPriceMinor: 12_000, LocalAmountMinor: 1_200_000, SettlementFen: -1_205_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Classification != "serious_violation" || receipt.Position.Quantity != 100 || receipt.Cooldown == nil || receipt.Cooldown.ExpectedEndsAt.IsZero() {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	for query, expected := range map[string]int{
		"SELECT count(*) FROM execution_events":                           1,
		"SELECT count(*) FROM violation_events":                           1,
		"SELECT count(*) FROM cooldown_periods":                           1,
		"SELECT count(*) FROM audit_events WHERE entity_type='execution'": 1,
		"SELECT count(*) FROM post_trade_reviews WHERE status='pending'":  1,
	} {
		var count int
		if err := svc.store.DB().QueryRow(query).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s count=%d err=%v", query, count, err)
		}
	}
}

func TestSeriousViolationCooldownEscalatesOnSecondAndThirdOccurrence(t *testing.T) {
	svc := openExecutionService(t)
	base := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	first, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_200_000, ExecutedAt: base})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_200_000, ExecutedAt: base.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_200_000, ExecutedAt: base.AddDate(0, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Cooldown == nil || second.Cooldown == nil || third.Cooldown == nil {
		t.Fatal("all serious violations must create cooldowns")
	}
	if second.Cooldown.ExpectedEndsAt.Sub(base.AddDate(0, 0, 1)) < 25*24*time.Hour {
		t.Fatalf("second cooldown did not escalate: %#v", second.Cooldown)
	}
	if got := third.Cooldown.ExpectedEndsAt.Sub(base.AddDate(0, 0, 2)); got != 30*24*time.Hour {
		t.Fatalf("third cooldown=%s", got)
	}
}

func TestInvalidSettlementWritesNothing(t *testing.T) {
	svc := openExecutionService(t)
	_, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: 1_205_000})
	if err == nil {
		t.Fatal("expected sign validation failure")
	}
	var count int
	if err := svc.store.DB().QueryRow("SELECT count(*) FROM execution_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestRejectedPlanExecutionIsRecordedAsSeriousViolation(t *testing.T) {
	svc := openExecutionService(t)
	draft := validPlanDraft()
	draft.Quantity = 150
	plan, err := svc.CreatePlan(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100,
		SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Classification != "serious_violation" || receipt.ViolationCode != "PLAN_NOT_QUALIFIED" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestRepeatedUseOfSameBuyPlanIsSeriousViolation(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	first := ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)}
	if _, err := svc.RecordExecution(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ExecutedAt = second.ExecutedAt.Add(time.Minute)
	receipt, err := svc.RecordExecution(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Classification != "serious_violation" || receipt.ViolationCode != "PLAN_QUANTITY_EXCEEDED" || receipt.Position.Quantity != 200 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestOversellIsRejectedBeforeItCanPoisonLedger(t *testing.T) {
	svc := openExecutionService(t)
	_, err := svc.RecordExecution(context.Background(), ExecutionDraft{InstrumentID: "hk-9988", Side: "sell", Quantity: 100, SettlementFen: 1_000_000, ExitCode: "R"})
	if err == nil {
		t.Fatal("expected oversell rejection")
	}
	var count int
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM execution_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReverseExecutionRestoresCashAndHoldingWithoutDeletingHistory(t *testing.T) {
	svc := openExecutionService(t)
	recorded, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000,
		ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := svc.ReverseExecution(context.Background(), recorded.ID, "人民币结算金额录错")
	if err != nil {
		t.Fatal(err)
	}
	if reversed.CashFen != 20_000_000 || reversed.Position.Quantity != 0 {
		t.Fatalf("unexpected reversal: %#v", reversed)
	}
	var executions, audits int
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM execution_events`).Scan(&executions); err != nil || executions != 2 {
		t.Fatalf("executions=%d err=%v", executions, err)
	}
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM audit_events WHERE action='reversed'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
}

func TestCorrectExecutionAtomicallyReplacesFactAndCurrentDisciplineState(t *testing.T) {
	svc := openExecutionService(t)
	recorded, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: 12_000,
		SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := svc.CorrectExecution(context.Background(), recorded.ID, "成交数量录错", ExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 200, LocalPriceMinor: 12_000,
		LocalPriceTenThousandth: 1_200_000, LocalAmountMinor: 2_400_000, SettlementFen: -2_410_000,
		ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Position.Quantity != 200 || corrected.CashFen != 17_590_000 || !corrected.PendingReview {
		t.Fatalf("corrected receipt=%#v", corrected)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM execution_events`:                                                  3,
		`SELECT count(*) FROM execution_corrections`:                                             1,
		`SELECT count(*) FROM violation_events WHERE acknowledged_at IS NULL`:                    1,
		`SELECT count(*) FROM cooldown_periods WHERE actual_ends_at IS NULL`:                     1,
		`SELECT count(*) FROM post_trade_reviews WHERE status='pending'`:                         1,
		`SELECT count(*) FROM audit_events WHERE entity_type='execution' AND action='corrected'`: 1,
	} {
		var got int
		if err := svc.store.DB().QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("query=%s got=%d want=%d err=%v", query, got, want, err)
		}
	}
}

func TestOddLotExecutionIsRejected(t *testing.T) {
	svc := openExecutionService(t)
	_, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 80, LocalPriceMinor: 12_000, LocalAmountMinor: 960_000, SettlementFen: -964_000,
	})
	if err == nil || !strings.Contains(err.Error(), "整数倍") {
		t.Fatalf("expected lot-size rejection, got %v", err)
	}
}

func TestCNYSettlementMismatchIsRejected(t *testing.T) {
	svc := openExecutionService(t)
	if _, err := svc.store.DB().Exec(`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status)
		VALUES('sh-510300','SH','510300','沪深300ETF','etf','CNY',100,'test',0,0,'active')`); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		InstrumentID: "sh-510300", Side: "buy", Quantity: 100, LocalPriceMinor: 100, LocalAmountMinor: 10_000, SettlementFen: -50_000,
	})
	if err == nil || !strings.Contains(err.Error(), "偏差过大") {
		t.Fatalf("expected settlement mismatch, got %v", err)
	}
}

func TestPressureLossBlocksNewPlanWhenUnrealizedDrawdownIsHigh(t *testing.T) {
	svc := openExecutionService(t)
	first, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		PlanID: first.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.UpdateQuotes(context.Background(), []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 100, Source: "fixture", SourceTime: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatal(err)
	}
	draft := validPlanDraft()
	draft.StressDropBP = 8_000
	plan, err := svc.CreatePlan(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status == "qualified" {
		t.Fatalf("expected stress rejection: %#v", plan.Validation)
	}
	found := false
	for _, finding := range plan.Validation.Findings {
		if finding.Code == "STRESS_LOSS_RED_LINE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing STRESS_LOSS_RED_LINE: %#v", plan.Validation.Findings)
	}
}

func TestCurrencyRateTableConvertsMarketValue(t *testing.T) {
	svc := openExecutionService(t)
	current, err := svc.store.CurrentRule(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.CurrencyRatesBP = map[string]int{"CNY": 10_000, "HKD": 8_000}
	if _, err := svc.store.CreateRuleVersion(context.Background(), "下调港币折算", current, time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.UpdateQuotes(context.Background(), []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 12_200, Source: "fixture", SourceTime: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	position := portfolio.Positions["hk-9988"]
	if position.MarketValueFen != 976_000 {
		t.Fatalf("expected 12200*100*0.8=976000, got %#v", position)
	}
}
