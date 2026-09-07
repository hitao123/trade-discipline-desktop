package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type Plan struct {
	ID            string                `json:"id"`
	RuleVersionID string                `json:"ruleVersionId"`
	Status        string                `json:"status"`
	Draft         domain.TradePlanDraft `json:"draft"`
	Validation    rules.Decision        `json:"validation"`
}

func (s *Service) CreatePlan(ctx context.Context, draft domain.TradePlanDraft) (Plan, error) {
	draft, decision, status, ruleID, err := s.evaluatePlan(ctx, draft)
	if err != nil {
		return Plan{}, err
	}
	row, err := s.store.SavePlan(ctx, ruleID, status, draft, decision, s.now())
	if err != nil {
		return Plan{}, err
	}
	return planFromRow(row), nil
}

func (s *Service) RevisePlan(ctx context.Context, id, reason string, draft domain.TradePlanDraft) (Plan, error) {
	if strings.TrimSpace(reason) == "" {
		return Plan{}, fmt.Errorf("修改原因不能为空")
	}
	if _, err := s.store.Plan(ctx, id); err != nil {
		return Plan{}, err
	}
	draft, decision, status, ruleID, err := s.evaluatePlan(ctx, draft)
	if err != nil {
		return Plan{}, err
	}
	row, err := s.store.RevisePlan(ctx, id, ruleID, status, strings.TrimSpace(reason), draft, decision, s.now())
	if err != nil {
		return Plan{}, err
	}
	return planFromRow(row), nil
}

func (s *Service) evaluatePlan(ctx context.Context, draft domain.TradePlanDraft) (domain.TradePlanDraft, rules.Decision, string, string, error) {
	code, lotSize, isChinaTech, err := s.store.Instrument(ctx, draft.InstrumentID)
	if err != nil {
		return domain.TradePlanDraft{}, rules.Decision{}, "", "", err
	}
	draft.Code = code
	draft.LotSize = lotSize
	draft.IsChinaTech = isChinaTech
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return domain.TradePlanDraft{}, rules.Decision{}, "", "", err
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return domain.TradePlanDraft{}, rules.Decision{}, "", "", err
	}
	if cooldown, err := s.store.LatestCooldown(ctx); err != nil {
		return domain.TradePlanDraft{}, rules.Decision{}, "", "", err
	} else if cooldown != nil && s.now().Before(cooldown.ExpectedEndsAt) {
		portfolio.ActiveCooldownUntil = cooldown.ExpectedEndsAt
	}
	decision := rules.EvaluatePlan(s.now(), rule, portfolio, draft)
	status := "rejected"
	if decision.Qualified {
		status = "qualified"
	}
	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return domain.TradePlanDraft{}, rules.Decision{}, "", "", err
	}
	return draft, decision, status, ruleID, nil
}

func (s *Service) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	plans := make([]Plan, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, planFromRow(row))
	}
	return plans, nil
}

func planFromRow(row store.PlanRow) Plan {
	return Plan{ID: row.ID, RuleVersionID: row.RuleVersionID, Status: row.Status, Draft: row.Draft, Validation: row.Validation}
}

func (s *Service) CreateRuleVersion(ctx context.Context, reason string, snapshot rules.Snapshot) (store.RuleVersionRow, error) {
	if strings.TrimSpace(reason) == "" {
		return store.RuleVersionRow{}, fmt.Errorf("修改原因不能为空")
	}
	current, err := s.store.CurrentRule(ctx)
	if err != nil {
		return store.RuleVersionRow{}, err
	}
	if snapshot.InitialCapitalFen != current.InitialCapitalFen {
		return store.RuleVersionRow{}, fmt.Errorf("账户初始资金不能通过规则版本修改")
	}
	if snapshot.LossRedLineFen <= 0 || snapshot.LossCautionFen <= 0 || snapshot.LossCautionFen >= snapshot.LossRedLineFen {
		return store.RuleVersionRow{}, fmt.Errorf("损失警戒线必须小于红线且均大于零")
	}
	if err := s.createAutomaticBackup(ctx, "rule"); err != nil {
		return store.RuleVersionRow{}, err
	}
	return s.store.CreateRuleVersion(ctx, strings.TrimSpace(reason), snapshot, s.now())
}

func (s *Service) ListRuleVersions(ctx context.Context) ([]store.RuleVersionRow, error) {
	return s.store.ListRuleVersions(ctx)
}
