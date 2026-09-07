package service

import (
	"context"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type Dashboard struct {
	ProfileMode       domain.UserMode       `json:"profileMode"`
	InitialCapitalFen int64                 `json:"initialCapitalFen"`
	Portfolio         domain.PortfolioState `json:"portfolio"`
	LossCautionFen    int64                 `json:"lossCautionFen"`
	LossRedLineFen    int64                 `json:"lossRedLineFen"`
	LossUsedFen       int64                 `json:"lossUsedFen"`
	ChinaTechLimitFen int64                 `json:"chinaTechLimitFen"`
	ViolationCount    int                   `json:"violationCount"`
	AllowedAction     string                `json:"allowedAction"`
	Cooldown          *store.CooldownInput  `json:"cooldown,omitempty"`
	LastMarketFetch   *time.Time            `json:"lastMarketFetch,omitempty"`
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	violations, err := s.store.CountViolations(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	cooldown, err := s.store.LatestCooldown(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	allowedAction := "先写完整计划，再决定是否行动"
	if cooldown != nil && s.now().Before(cooldown.ExpectedEndsAt) {
		portfolio.ActiveCooldownUntil = cooldown.ExpectedEndsAt
		allowedAction = "冷静期内只允许记录、减仓、风险退出和复盘"
	}
	initialCapitalFen, err := s.fundedCapitalFen(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	return Dashboard{
		ProfileMode: profile.Mode, InitialCapitalFen: initialCapitalFen, Portfolio: portfolio, LossCautionFen: rule.LossCautionFen,
		LossRedLineFen: rule.LossRedLineFen, LossUsedFen: portfolio.CumulativeLossFen,
		ChinaTechLimitFen: rule.ChinaTechLimitFen, ViolationCount: violations, AllowedAction: allowedAction,
		Cooldown: cooldown, LastMarketFetch: s.store.LatestSuccessfulMarketFetch(ctx),
	}, nil
}

func (s *Service) Instruments(ctx context.Context) ([]store.InstrumentRow, error) {
	return s.store.ListInstruments(ctx)
}

func (s *Service) Audit(ctx context.Context) ([]store.AuditRow, error) {
	return s.store.ListAudit(ctx, 100)
}
