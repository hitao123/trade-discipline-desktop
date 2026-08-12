package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func openExecutionService(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(db, func() time.Time { return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC) })
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
}

func TestPortfolioDerivesTencentGateProgressFromCloseDaysAndLatestReview(t *testing.T) {
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
	if portfolio.AlibabaObservationTradingDays != 20 || portfolio.DisciplineScoreBP != 10_000 {
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
	for table, expected := range map[string]int{"execution_events": 1, "violation_events": 1, "cooldown_periods": 1, "audit_events": 1} {
		var count int
		if err := svc.store.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s count=%d err=%v", table, count, err)
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
