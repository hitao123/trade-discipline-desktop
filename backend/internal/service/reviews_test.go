package service

import (
	"context"
	"testing"
	"time"
)

func TestPeriodDisciplineScorePenalizesViolations(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	first := ExecutionDraft{PlanID: plan.ID, InstrumentID: "hk-9988", Side: "buy", Quantity: 100, SettlementFen: -1_205_000, ExecutedAt: base}
	if _, err := svc.RecordExecution(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ExecutedAt = base.Add(time.Minute)
	// 第二笔超出计划数量，产生 PLAN_QUANTITY_EXCEEDED 违规。
	if _, err := svc.RecordExecution(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	score, err := svc.periodDisciplineScore(context.Background(),
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if score >= 10_000 {
		t.Fatalf("有未撤销违规的周期纪律分应低于满分，got %d", score)
	}
}

func TestWeeklyReviewSavesAutomaticMetricsAndUserJudgment(t *testing.T) {
	svc := openExecutionService(t)
	review, err := svc.SaveWeeklyReview(context.Background(), WeeklyReviewDraft{
		PeriodStart: "2026-08-10", PeriodEnd: "2026-08-16", ImpulseNotes: "本周没有追涨",
		NextAllowedAction: "只观察阿里，不新增标的",
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.DisciplineScoreBP != 10_000 || review.UserContent.NextAllowedAction == "" || review.RuleVersionID == "" {
		t.Fatalf("unexpected review: %#v", review)
	}
	latest, err := svc.LatestWeeklyReview(context.Background())
	if err != nil || latest == nil || latest.ID != review.ID {
		t.Fatalf("latest=%#v err=%v", latest, err)
	}
	loaded, err := svc.WeeklyReview(context.Background(), "2026-08-10", "2026-08-16")
	if err != nil || loaded == nil || loaded.ID != review.ID || loaded.UserContent.ImpulseNotes != "本周没有追涨" {
		t.Fatalf("period review=%#v err=%v", loaded, err)
	}
	_, err = svc.SaveWeeklyReview(context.Background(), WeeklyReviewDraft{
		PeriodStart: "2026-08-10", PeriodEnd: "2026-08-16", NextAllowedAction: "再次提交应被拒绝",
	})
	if code, ok := ErrorCode(err); !ok || code != "REVIEW_PERIOD_EXISTS" {
		t.Fatalf("expected REVIEW_PERIOD_EXISTS, got %v", err)
	}
}
