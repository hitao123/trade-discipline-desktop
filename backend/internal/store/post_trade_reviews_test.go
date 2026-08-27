package store

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

func TestQuickExecutionCreatesPendingReviewAtomically(t *testing.T) {
	db := openLegacyTestStore(t)
	ctx := context.Background()
	ruleID, err := db.CurrentRuleVersionID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	executedAt := time.Date(2026, 8, 22, 8, 30, 0, 0, time.UTC)
	result, err := db.AppendExecution(ctx, AppendExecutionInput{
		Event:          domain.ExecutionEvent{EventType: domain.ExecutionBuy, InstrumentID: "hk-9988", Quantity: 100, LocalPriceMinor: 12_000, LocalAmountMinor: 1_200_000, SettlementFen: -1_205_000, ExecutedAt: executedAt},
		RuleVersionID:  ruleID,
		Classification: "violation",
		QuickRecord:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := db.PostTradeReview(ctx, result.ExecutionID)
	if err != nil || review.Status != "pending" || review.ExecutionID != result.ExecutionID {
		t.Fatalf("review=%#v err=%v", review, err)
	}
	items, err := db.PendingPostTradeReviews(ctx, executedAt.Add(-time.Hour), executedAt.Add(time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestPostTradeReviewCompletionRequiresNoteAndFXObservationDoesNotChangeSettlement(t *testing.T) {
	db := openLegacyTestStore(t)
	ctx := context.Background()
	ruleID, err := db.CurrentRuleVersionID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	executedAt := time.Date(2026, 8, 22, 8, 30, 0, 0, time.UTC)
	result, err := db.AppendExecution(ctx, AppendExecutionInput{
		Event:          domain.ExecutionEvent{EventType: domain.ExecutionBuy, InstrumentID: "hk-9988", Quantity: 100, LocalPriceMinor: 12_000, LocalAmountMinor: 1_200_000, SettlementFen: -1_205_000, ExecutedAt: executedAt},
		RuleVersionID:  ruleID,
		Classification: "violation",
		QuickRecord:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CompletePostTradeReview(ctx, result.ExecutionID, "", executedAt.Add(time.Hour)); err == nil {
		t.Fatal("empty completion note should fail")
	}
	review, err := db.CompletePostTradeReview(ctx, result.ExecutionID, "冲动追涨，之后必须先等计划确认", executedAt.Add(time.Hour))
	if err != nil || review.Status != "completed" || review.Note == "" {
		t.Fatalf("review=%#v err=%v", review, err)
	}
	var auditCount int
	if err := db.DB().QueryRow(`SELECT count(*) FROM audit_events WHERE entity_type='post_trade_review' AND entity_id=? AND action='completed'`, review.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("completion audit count=%d err=%v", auditCount, err)
	}
	observation, err := db.SaveExecutionFXObservation(ctx, ExecutionFXObservation{
		ExecutionID: result.ExecutionID, BaseCurrency: "HKD", QuoteCurrency: "CNY", RateMinor: 915_000,
		Source: "public-reference", SourceTime: executedAt, ObservedAt: executedAt.Add(time.Minute),
	})
	if err != nil || observation.ID == "" {
		t.Fatalf("observation=%#v err=%v", observation, err)
	}
	var settlementFen int64
	if err := db.DB().QueryRow(`SELECT settlement_fen FROM execution_events WHERE id=?`, result.ExecutionID).Scan(&settlementFen); err != nil || settlementFen != -1_205_000 {
		t.Fatalf("settlement=%d err=%v", settlementFen, err)
	}
}
