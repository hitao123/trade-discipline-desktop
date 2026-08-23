package service

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func TestRunMonitorOnceCreatesRiskExitAlertOnlyOnCrossing(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 8_900, Source: "fixture", SourceTime: svc.now()}}})
	first, err := svc.RunMonitorOnce(context.Background())
	if err != nil || first.Triggered != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := svc.RunMonitorOnce(context.Background())
	if err != nil || second.Triggered != 0 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestRunMonitorOnceUsesStoredMarketForShanghaiETF(t *testing.T) {
	svc := openExecutionService(t)
	if _, err := svc.store.DB().Exec(`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status)
		VALUES ('sh-510300', 'SH', '510300', '沪深300ETF', 'etf', 'CNY', 100, 'fixture', 0, 0, 'active')`); err != nil {
		t.Fatal(err)
	}
	draft := validPlanDraft()
	draft.InstrumentID = "sh-510300"
	draft.EntryLowMinor, draft.EntryHighMinor, draft.RiskExitMinor = 400, 450, 350
	draft.EstimatedCostFen, draft.MaxPlanLossFen = 45_000, 5_000
	plan, err := svc.CreatePlan(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "qualified" {
		t.Fatalf("plan=%#v", plan)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "sh-510300", Side: "buy", Quantity: 100, SettlementFen: -45_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	var requested []market.InstrumentKey
	svc.SetMarketProvider(fakeMarketProvider{
		quotes:   []market.Quote{{Market: "SH", Code: "510300", CloseMinor: 420, Source: "fixture", SourceTime: svc.now()}},
		seenKeys: &requested,
	})
	if _, err := svc.RunMonitorOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 1 || requested[0] != (market.InstrumentKey{Market: "SH", Code: "510300"}) {
		t.Fatalf("requested=%#v", requested)
	}
}

func TestMonitorStatusRetainsTheLastSuccessfulCheck(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 9_500, Source: "fixture", SourceTime: svc.now()}}})
	if _, err := svc.RunMonitorOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := svc.MonitorStatus(context.Background())
	if err != nil || status.LastSuccessfulAt == nil || !status.LastSuccessfulAt.Equal(svc.now()) {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestRunMonitorOnceSkipsStaleQuote(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 8_900, Source: "fixture", SourceTime: svc.now().Add(-21 * time.Minute)}}})
	result, err := svc.RunMonitorOnce(context.Background())
	if err != nil || result.Triggered != 0 || result.Status.LastError == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRunMonitorOnceSkipsQuoteFromTheFuture(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{Market: "HK", Code: "9988.HK", CloseMinor: 8_900, Source: "fixture", SourceTime: svc.now().Add(time.Minute)}}})
	result, err := svc.RunMonitorOnce(context.Background())
	if err != nil || result.Triggered != 0 || result.Status.LastError == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRunMonitorOnceSkipsQuoteWithPreviousTradeDate(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(context.Background(), ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{TradeDate: "2026-08-11", Market: "HK", Code: "9988.HK", CloseMinor: 8_900, Source: "fixture", SourceTime: svc.now()}}})
	result, err := svc.RunMonitorOnce(context.Background())
	if err != nil || result.Triggered != 0 || result.Checked != 0 || result.Status.LastError == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMonitorIntervalSupportsConfiguredValues(t *testing.T) {
	if got := monitorInterval("15m"); got != 15*time.Minute {
		t.Fatalf("15m interval=%s", got)
	}
	if got := monitorInterval("off"); got != 0 {
		t.Fatalf("off interval=%s", got)
	}
}
