package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

type MonitorStatus = domain.MonitorStatus

type MonitorResult struct {
	Checked   int           `json:"checked"`
	Triggered int           `json:"triggered"`
	Status    MonitorStatus `json:"status"`
}

func (s *Service) RunMonitorOnce(ctx context.Context) (MonitorResult, error) {
	now := s.now().UTC()
	settings, err := s.store.MonitorSettings(ctx)
	if err != nil {
		return MonitorResult{}, err
	}
	status, err := s.store.MonitorStatus(ctx)
	if err != nil {
		return MonitorResult{}, err
	}
	status.Enabled = settings.Interval != "off"
	status.Interval = settings.Interval
	status.LastAttemptAt = &now
	status.LastError = ""
	if settings.Interval == "off" {
		return s.finishMonitorRun(ctx, MonitorResult{Status: status})
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		status.LastError = err.Error()
		return s.finishMonitorRun(ctx, MonitorResult{Status: status})
	}
	type candidate struct {
		instrumentID, planID string
		plan                 Plan
		key                  market.InstrumentKey
	}
	candidates := make([]candidate, 0)
	for instrumentID, position := range portfolio.Positions {
		if position.Quantity <= 0 {
			continue
		}
		planID, err := s.store.LatestQualifiedBuyPlanID(ctx, instrumentID)
		if err != nil {
			return MonitorResult{}, err
		}
		if planID == "" {
			continue
		}
		row, err := s.store.Plan(ctx, planID)
		if err != nil {
			return MonitorResult{}, err
		}
		key, err := s.store.InstrumentKey(ctx, instrumentID)
		if err != nil {
			return MonitorResult{}, err
		}
		candidates = append(candidates, candidate{instrumentID: instrumentID, planID: planID, plan: planFromRow(row), key: key})
	}
	if len(candidates) == 0 {
		status.LastSuccessfulAt = &now
		return s.finishMonitorRun(ctx, MonitorResult{Status: status})
	}
	keys := make([]market.InstrumentKey, 0, len(candidates))
	for _, item := range candidates {
		keys = append(keys, item.key)
	}
	quotes, err := s.marketProvider.FetchQuotes(ctx, keys)
	if err != nil && s.quoteFallback != nil {
		fallbackQuotes, fallbackErr := s.quoteFallback.FetchQuotes(ctx, keys)
		if fallbackErr == nil && validQuoteFallback(now, keys, fallbackQuotes) {
			quotes = fallbackQuotes
			err = nil
		}
	}
	if err != nil {
		status.LastError = "报价来源暂不可用，未评估提醒"
		return s.finishMonitorRun(ctx, MonitorResult{Status: status})
	}
	byCode := make(map[string]market.Quote, len(quotes))
	for _, quote := range quotes {
		byCode[quote.Code] = quote
	}
	result := MonitorResult{Status: status}
	for _, item := range candidates {
		quote, ok := byCode[item.key.Code]
		if !ok {
			continue
		}
		if !freshAlertQuote(now, quote) {
			result.Status.LastError = "报价不是当日有效数据，未触发提醒"
			continue
		}
		result.Checked++
		if err := s.store.UpdateQuotes(ctx, []market.Quote{quote}); err != nil {
			return MonitorResult{}, err
		}
		snapshot, _ := json.Marshal(map[string]any{"draft": item.plan.Draft, "validation": item.plan.Validation})
		for _, condition := range []struct {
			kind      domain.AlertKind
			threshold int64
			active    bool
		}{
			{domain.AlertRiskExit, item.plan.Draft.RiskExitMinor, quote.CloseMinor <= item.plan.Draft.RiskExitMinor},
			{domain.AlertTargetZone, item.plan.Draft.TargetExitLowMinor, item.plan.Draft.TargetExitLowMinor > 0 && quote.CloseMinor >= item.plan.Draft.TargetExitLowMinor && quote.CloseMinor <= item.plan.Draft.TargetExitHighMinor},
		} {
			if !condition.active {
				if err := s.store.RecordAlertState(ctx, item.planID, condition.kind, false, now); err != nil {
					return MonitorResult{}, err
				}
				continue
			}
			created, _, err := s.store.CreatePriceAlertIfCrossed(ctx, domain.PriceAlertEvent{PlanID: item.planID, InstrumentID: item.instrumentID, Kind: condition.kind, TriggerPriceMinor: quote.CloseMinor, ThresholdMinor: condition.threshold, PlanSnapshotJSON: string(snapshot), Source: quote.Source, SourceTime: quote.SourceTime, TriggeredAt: now})
			if err != nil {
				return MonitorResult{}, err
			}
			if created {
				result.Triggered++
			}
		}
	}
	result.Status.LastSuccessfulAt = &now
	return s.finishMonitorRun(ctx, result)
}

func freshAlertQuote(now time.Time, quote market.Quote) bool {
	if quote.SourceTime.IsZero() || quote.SourceTime.After(now) || now.Sub(quote.SourceTime) > 20*time.Minute {
		return false
	}
	if quote.TradeDate == "" {
		return true
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return false
	}
	return quote.TradeDate == now.In(location).Format("2006-01-02")
}

func (s *Service) finishMonitorRun(ctx context.Context, result MonitorResult) (MonitorResult, error) {
	if err := s.store.SaveMonitorStatus(ctx, result.Status, s.now()); err != nil {
		return MonitorResult{}, err
	}
	return result, nil
}

func (s *Service) StartMonitor(ctx context.Context) {
	go func() {
		for {
			settings, err := s.store.MonitorSettings(ctx)
			interval := 10 * time.Minute
			if err == nil {
				interval = monitorInterval(settings.Interval)
			}
			if interval > 0 {
				_, _ = s.RunMonitorOnce(ctx)
			}
			if interval == 0 {
				interval = time.Minute
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

func monitorInterval(interval string) time.Duration {
	switch interval {
	case "10m":
		return 10 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	default:
		return 0
	}
}

func (s *Service) MonitorStatus(ctx context.Context) (MonitorStatus, error) {
	settings, err := s.store.MonitorSettings(ctx)
	if err != nil {
		return MonitorStatus{}, err
	}
	status, err := s.store.MonitorStatus(ctx)
	if err != nil {
		return MonitorStatus{}, err
	}
	status.Enabled = settings.Interval != "off"
	status.Interval = settings.Interval
	return status, nil
}

func (s *Service) MonitorSettings(ctx context.Context) (domain.MonitorSettings, error) {
	return s.store.MonitorSettings(ctx)
}

func (s *Service) SaveMonitorSettings(ctx context.Context, settings domain.MonitorSettings) error {
	return s.store.SaveMonitorSettings(ctx, settings, s.now())
}

func (s *Service) PendingPriceAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error) {
	return s.store.ListPendingAlerts(ctx)
}

func (s *Service) UnnotifiedPriceAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error) {
	return s.store.ListUnnotifiedAlerts(ctx)
}

func (s *Service) MarkPriceAlertNotified(ctx context.Context, alertID string) error {
	return s.store.MarkAlertNotified(ctx, alertID, s.now())
}

func (s *Service) CompletePositionReview(ctx context.Context, alertID string, decision domain.ReviewDecision, reason string) (domain.PositionReviewEvent, error) {
	if reason == "" {
		return domain.PositionReviewEvent{}, fmt.Errorf("复核原因不能为空")
	}
	return s.store.CreatePositionReview(ctx, alertID, decision, reason, s.now())
}
