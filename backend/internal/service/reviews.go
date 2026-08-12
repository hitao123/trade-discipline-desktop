package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type WeeklyReviewDraft struct {
	PeriodStart       string `json:"periodStart"`
	PeriodEnd         string `json:"periodEnd"`
	ImpulseNotes      string `json:"impulseNotes"`
	NextAllowedAction string `json:"nextAllowedAction"`
}

func (s *Service) SaveWeeklyReview(ctx context.Context, draft WeeklyReviewDraft) (store.WeeklyReviewRow, error) {
	start, err := time.Parse("2006-01-02", draft.PeriodStart)
	if err != nil {
		return store.WeeklyReviewRow{}, fmt.Errorf("复盘开始日期无效")
	}
	end, err := time.Parse("2006-01-02", draft.PeriodEnd)
	if err != nil || end.Before(start) {
		return store.WeeklyReviewRow{}, fmt.Errorf("复盘结束日期无效")
	}
	if strings.TrimSpace(draft.NextAllowedAction) == "" {
		return store.WeeklyReviewRow{}, fmt.Errorf("请写明下周唯一允许动作")
	}
	dashboard, err := s.Dashboard(ctx)
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	score := 10_000 - dashboard.ViolationCount*2_000
	if score < 0 {
		score = 0
	}
	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	return s.store.SaveWeeklyReview(ctx, store.WeeklyReviewRow{
		PeriodStart: draft.PeriodStart, PeriodEnd: draft.PeriodEnd,
		Metrics:           store.ReviewMetrics{CashFen: dashboard.Portfolio.AvailableCashFen, ChinaTechExposureFen: dashboard.Portfolio.ChinaTechExposureFen, CumulativeLossFen: dashboard.Portfolio.CumulativeLossFen, ViolationCount: dashboard.ViolationCount},
		UserContent:       store.ReviewContent{ImpulseNotes: strings.TrimSpace(draft.ImpulseNotes), NextAllowedAction: strings.TrimSpace(draft.NextAllowedAction)},
		DisciplineScoreBP: score, RuleVersionID: ruleID, SubmittedAt: s.now(),
	})
}

func (s *Service) LatestWeeklyReview(ctx context.Context) (*store.WeeklyReviewRow, error) {
	return s.store.LatestWeeklyReview(ctx)
}
