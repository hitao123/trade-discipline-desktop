package service

import (
	"context"
	"testing"
)

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
}
