package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type WeeklyReviewDraft struct {
	PeriodStart       string `json:"periodStart"`
	PeriodEnd         string `json:"periodEnd"`
	ImpulseNotes      string `json:"impulseNotes"`
	NextAllowedAction string `json:"nextAllowedAction"`
}

func (s *Service) SaveWeeklyReview(ctx context.Context, draft WeeklyReviewDraft) (store.WeeklyReviewRow, error) {
	start, err := parseShanghaiDate(draft.PeriodStart)
	if err != nil {
		return store.WeeklyReviewRow{}, fmt.Errorf("复盘开始日期无效")
	}
	end, err := parseShanghaiDate(draft.PeriodEnd)
	if err != nil || end.Before(start) {
		return store.WeeklyReviewRow{}, fmt.Errorf("复盘结束日期无效")
	}
	if strings.TrimSpace(draft.NextAllowedAction) == "" {
		return store.WeeklyReviewRow{}, fmt.Errorf("请写明下周唯一允许动作")
	}
	pending, err := s.store.PendingPostTradeReviews(ctx, start, end.AddDate(0, 0, 1).Add(-time.Nanosecond))
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	if len(pending) > 0 {
		return store.WeeklyReviewRow{}, CodedError{Code: "PENDING_POST_TRADE_REVIEW", Message: fmt.Sprintf("本周还有 %d 笔成交待纪律复盘", len(pending))}
	}
	dashboard, err := s.Dashboard(ctx)
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	score, err := s.periodDisciplineScore(ctx, start, end)
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return store.WeeklyReviewRow{}, err
	}
	row, err := s.store.SaveWeeklyReview(ctx, store.WeeklyReviewRow{
		PeriodStart: draft.PeriodStart, PeriodEnd: draft.PeriodEnd,
		Metrics:           store.ReviewMetrics{CashFen: dashboard.Portfolio.AvailableCashFen, ChinaTechExposureFen: dashboard.Portfolio.ChinaTechExposureFen, CumulativeLossFen: dashboard.Portfolio.CumulativeLossFen, ViolationCount: dashboard.ViolationCount},
		UserContent:       store.ReviewContent{ImpulseNotes: strings.TrimSpace(draft.ImpulseNotes), NextAllowedAction: strings.TrimSpace(draft.NextAllowedAction)},
		DisciplineScoreBP: score, RuleVersionID: ruleID, SubmittedAt: s.now(),
	})
	if errors.Is(err, store.ErrReviewPeriodExists) {
		return store.WeeklyReviewRow{}, CodedError{Code: "REVIEW_PERIOD_EXISTS", Message: err.Error()}
	}
	return row, err
}

func (s *Service) WeeklyReview(ctx context.Context, periodStart, periodEnd string) (*store.WeeklyReviewRow, error) {
	if periodStart != "" && periodEnd != "" {
		return s.store.WeeklyReviewByPeriod(ctx, periodStart, periodEnd)
	}
	return s.store.LatestWeeklyReview(ctx)
}

func (s *Service) LatestWeeklyReview(ctx context.Context) (*store.WeeklyReviewRow, error) {
	return s.store.LatestWeeklyReview(ctx)
}

var (
	planViolationCodes      = map[string]bool{"UNPLANNED_EXECUTION": true, "PLAN_NOT_QUALIFIED": true, "PLAN_INSTRUMENT_MISMATCH": true, "PLAN_EXPIRED": true}
	positionViolationCodes  = map[string]bool{"PLAN_QUANTITY_EXCEEDED": true, "INSUFFICIENT_CASH": true, "PORTFOLIO_LOSS_RED_LINE": true}
	averagingViolationCodes = map[string]bool{"NO_ADD_TO_LOSER": true}
)

func hasAnyViolation(codes []string, set map[string]bool) bool {
	for _, code := range codes {
		if set[code] {
			return true
		}
	}
	return false
}

func (s *Service) periodDisciplineScore(ctx context.Context, start, end time.Time) (int, error) {
	records, err := s.store.ListActiveExecutions(ctx, "")
	if err != nil {
		return 0, err
	}
	violations, err := s.store.ActiveViolationCodesByExecution(ctx)
	if err != nil {
		return 0, err
	}
	endBoundary := end.AddDate(0, 0, 1).Add(-time.Nanosecond)
	trades := make([]domain.ScorableTrade, 0)
	for _, record := range records {
		if record.ExecutedAt.Before(start) || record.ExecutedAt.After(endBoundary) {
			continue
		}
		codes := violations[record.ID]
		trade := domain.ScorableTrade{
			ID:                    record.ID,
			PlanQualified:         record.PlanID != "" && !hasAnyViolation(codes, planViolationCodes),
			PositionWithinLimit:   !hasAnyViolation(codes, positionViolationCodes),
			NoAveraging:           !hasAnyViolation(codes, averagingViolationCodes),
			TimelyImmutableRecord: true,
		}
		if record.Side == "sell" {
			hasEvidence := strings.TrimSpace(record.Evidence) != "" && !hasAnyViolation(codes, map[string]bool{"EXIT_CODE_REQUIRED": true})
			trade.ExitEvidence = &hasEvidence
		}
		trades = append(trades, trade)
	}
	if len(trades) == 0 {
		return 10_000, nil
	}
	return domain.ScoreTrades(trades).RateBP, nil
}
