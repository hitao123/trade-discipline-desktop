package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

type PreTradeConfirmationInput struct {
	StartedAt       time.Time `json:"startedAt"`
	NoFOMO          bool      `json:"noFomo"`
	NoLossRecovery  bool      `json:"noLossRecovery"`
	NoAveragingDown bool      `json:"noAveragingDown"`
}

func (s *Service) ConfirmPreTrade(ctx context.Context, planID string, input PreTradeConfirmationInput) (domain.PreTradeConfirmation, error) {
	plan, err := s.store.Plan(ctx, planID)
	if err != nil {
		return domain.PreTradeConfirmation{}, err
	}
	now := s.now()
	if plan.Status != "qualified" {
		return domain.PreTradeConfirmation{}, fmt.Errorf("只有合格计划可以开仓前确认")
	}
	if !plan.Draft.ValidUntil.After(now) {
		return domain.PreTradeConfirmation{}, fmt.Errorf("计划已过期，不能开仓前确认")
	}
	if input.StartedAt.IsZero() || now.Sub(input.StartedAt) < 30*time.Second {
		return domain.PreTradeConfirmation{}, fmt.Errorf("请完成 30 秒冷静确认")
	}
	if !input.NoFOMO || !input.NoLossRecovery || !input.NoAveragingDown {
		return domain.PreTradeConfirmation{}, fmt.Errorf("请确认并非 FOMO、补亏或摊平")
	}
	snapshot, err := json.Marshal(map[string]any{
		"draft": plan.Draft, "validation": plan.Validation, "ruleVersionId": plan.RuleVersionID,
	})
	if err != nil {
		return domain.PreTradeConfirmation{}, fmt.Errorf("编码开仓前确认快照: %w", err)
	}
	return s.store.CreatePreTradeConfirmation(ctx, planID, string(snapshot), input.StartedAt, now)
}

func (s *Service) LatestPreTradeConfirmation(ctx context.Context, planID string) (*domain.PreTradeConfirmation, error) {
	return s.store.LatestPreTradeConfirmation(ctx, planID)
}
