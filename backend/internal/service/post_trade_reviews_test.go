package service

import (
	"context"
	"strings"
	"testing"
)

func TestRecordQuickExecutionImmediatelyUpdatesPortfolioAndCreatesPendingReview(t *testing.T) {
	svc := openExecutionService(t)
	receipt, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{
		InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: 12_000, SettlementFen: -1_205_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Position.Quantity != 100 || receipt.CashFen != 18_795_000 || !receipt.PendingReview {
		t.Fatalf("receipt=%#v", receipt)
	}
	if receipt.Classification != "serious_violation" || receipt.ViolationCode != "UNPLANNED_EXECUTION" || receipt.Cooldown == nil {
		t.Fatalf("discipline receipt=%#v", receipt)
	}
	items, err := svc.PendingPostTradeReviews(context.Background(), "2026-08-10", "2026-08-16")
	if err != nil || len(items) != 1 || items[0].ExecutionID != receipt.ID || items[0].InstrumentID != "hk-9988" || items[0].Side != "buy" || items[0].Quantity != 100 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestRecordQuickExecutionRequiresActualHKSettlementAndPrefillsCNYSettlement(t *testing.T) {
	svc := openExecutionService(t)
	if _, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: 12_000}); err == nil || !strings.Contains(err.Error(), "港股") {
		t.Fatalf("missing HK settlement err=%v", err)
	}
	if _, err := svc.store.DB().Exec(`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES ('sh-510300','SH','510300','沪深300ETF','etf','CNY',100,'fixture',0,0,'active')`); err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{InstrumentID: "sh-510300", Side: "buy", Quantity: 100, LocalPriceMinor: 420})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CashFen != 19_958_000 || receipt.Position.Quantity != 100 {
		t.Fatalf("CNY receipt=%#v", receipt)
	}
}

func TestWeeklyReviewWaitsForPendingQuickTradeReviews(t *testing.T) {
	svc := openExecutionService(t)
	receipt, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: 12_000, SettlementFen: -1_205_000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveWeeklyReview(context.Background(), WeeklyReviewDraft{PeriodStart: "2026-08-10", PeriodEnd: "2026-08-16", NextAllowedAction: "只按计划行动"})
	if code, ok := ErrorCode(err); !ok || code != "PENDING_POST_TRADE_REVIEW" {
		t.Fatalf("weekly gate err=%v code=%q", err, code)
	}
	if _, err := svc.CompletePostTradeReview(context.Background(), receipt.ID, "追涨后补录，今后先完成计划确认"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveWeeklyReview(context.Background(), WeeklyReviewDraft{PeriodStart: "2026-08-10", PeriodEnd: "2026-08-16", NextAllowedAction: "只按计划行动"}); err != nil {
		t.Fatal(err)
	}
}

func TestQuickLocalAmountRejectsOverflow(t *testing.T) {
	svc := openExecutionService(t)
	_, err := svc.RecordQuickExecution(context.Background(), QuickExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: int64(^uint64(0) >> 1), SettlementFen: -1})
	if err == nil {
		t.Fatal("expected overflow rejection")
	}
	var count int
	if queryErr := svc.store.DB().QueryRow(`SELECT count(*) FROM execution_events`).Scan(&count); queryErr != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, queryErr)
	}
}
