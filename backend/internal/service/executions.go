package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type ExecutionDraft struct {
	PlanID              string    `json:"planId,omitempty"`
	InstrumentID        string    `json:"instrumentId"`
	Side                string    `json:"side"`
	ExecutedAt          time.Time `json:"executedAt"`
	Quantity            int       `json:"quantity"`
	LocalPriceMinor     int64     `json:"localPriceMinor"`
	LocalAmountMinor    int64     `json:"localAmountMinor"`
	SettlementFen       int64     `json:"settlementFen"`
	ExitCode            string    `json:"exitCode,omitempty"`
	Evidence            string    `json:"evidence,omitempty"`
	BrokerReference     string    `json:"brokerReference,omitempty"`
	ReferencePriceMinor int64     `json:"referencePriceMinor,omitempty"`
	ReferencePriceAt    time.Time `json:"referencePriceAt,omitempty"`
}

type CooldownReceipt struct {
	ID             string    `json:"id"`
	Reason         string    `json:"reason"`
	ExpectedEndsAt time.Time `json:"expectedEndsAt"`
}

type ExecutionReceipt struct {
	ID             string               `json:"id"`
	Classification string               `json:"classification"`
	ViolationCode  string               `json:"violationCode,omitempty"`
	Position       domain.PositionState `json:"position"`
	CashFen        int64                `json:"cashFen"`
	Cooldown       *CooldownReceipt     `json:"cooldown,omitempty"`
	PendingReview  bool                 `json:"pendingReview"`
}

func (s *Service) RecordExecution(ctx context.Context, draft ExecutionDraft) (ExecutionReceipt, error) {
	return s.recordExecution(ctx, draft, false)
}

func (s *Service) recordExecution(ctx context.Context, draft ExecutionDraft, quickRecord bool) (ExecutionReceipt, error) {
	code, lotSize, isChinaTech, err := s.store.Instrument(ctx, draft.InstrumentID)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	if draft.Quantity <= 0 || draft.Quantity%lotSize != 0 {
		return ExecutionReceipt{}, fmt.Errorf("数量必须是 %d 的正整数倍", lotSize)
	}
	if draft.Side != "buy" && draft.Side != "sell" {
		return ExecutionReceipt{}, fmt.Errorf("买卖方向无效")
	}
	if draft.Side == "buy" && draft.SettlementFen >= 0 {
		return ExecutionReceipt{}, fmt.Errorf("买入实际人民币扣款必须为负数")
	}
	if draft.Side == "sell" && draft.SettlementFen <= 0 {
		return ExecutionReceipt{}, fmt.Errorf("卖出实际人民币到账必须为正数")
	}
	if draft.ExecutedAt.IsZero() {
		draft.ExecutedAt = s.now()
	}
	portfolioBefore, err := s.Portfolio(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	if draft.Side == "sell" && portfolioBefore.Positions[draft.InstrumentID].Quantity < draft.Quantity {
		return ExecutionReceipt{}, fmt.Errorf("卖出数量超过本地持仓，请先补齐缺失成交")
	}
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}

	classification := "compliant"
	violationCode := ""
	markViolation := func(code string) {
		if violationCode == "" {
			classification = "serious_violation"
			violationCode = code
		}
	}
	if draft.PlanID == "" {
		markViolation("UNPLANNED_EXECUTION")
	} else {
		plan, err := s.store.Plan(ctx, draft.PlanID)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		switch {
		case plan.Status != "qualified":
			markViolation("PLAN_NOT_QUALIFIED")
		case plan.Draft.InstrumentID != draft.InstrumentID:
			markViolation("PLAN_INSTRUMENT_MISMATCH")
		case draft.ExecutedAt.After(plan.Draft.ValidUntil):
			markViolation("PLAN_EXPIRED")
		case draft.Side == "buy":
			used, err := s.store.ActivePlanExecutionQuantity(ctx, draft.PlanID, domain.ExecutionBuy)
			if err != nil {
				return ExecutionReceipt{}, err
			}
			if used+draft.Quantity > plan.Draft.Quantity {
				markViolation("PLAN_QUANTITY_EXCEEDED")
			}
		}
	}
	if draft.Side == "sell" && !validExitCode(draft.ExitCode) {
		markViolation("EXIT_CODE_REQUIRED")
	}
	if draft.Side == "buy" {
		position := portfolioBefore.Positions[draft.InstrumentID]
		maxShares := 0
		switch code {
		case "0700.HK":
			maxShares = rule.TencentMaxShares
		case "9988.HK":
			maxShares = rule.AlibabaMaxShares
		}
		if maxShares > 0 && position.Quantity+draft.Quantity > maxShares {
			markViolation("INSTRUMENT_SHARE_LIMIT")
		}
		if rule.NoAddToLosingInstrument && position.Quantity > 0 && position.UnrealizedPnLFen < 0 {
			markViolation("NO_ADD_TO_LOSER")
		}
		if rule.NoCrossInstrumentAveraging && isChinaTech && portfolioBefore.ChinaTechUnrealizedPnLFen < 0 {
			markViolation("NO_CROSS_INSTRUMENT_AVERAGING")
		}
		actualCost := -draft.SettlementFen
		if actualCost > portfolioBefore.AvailableCashFen {
			markViolation("INSUFFICIENT_CASH")
		}
		if isChinaTech && portfolioBefore.ChinaTechExposureFen+actualCost > rule.ChinaTechLimitFen {
			markViolation("CHINA_TECH_EXPOSURE_LIMIT")
		}
		if portfolioBefore.CumulativeLossFen >= rule.LossRedLineFen {
			markViolation("PORTFOLIO_LOSS_RED_LINE")
		}
	}

	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	event := domain.ExecutionEvent{
		EventType: domain.ExecutionType(draft.Side), PlanID: draft.PlanID, InstrumentID: draft.InstrumentID,
		Code: code, Quantity: draft.Quantity, LocalPriceMinor: draft.LocalPriceMinor, LocalAmountMinor: draft.LocalAmountMinor,
		SettlementFen: draft.SettlementFen, IsChinaTech: isChinaTech, ExitCode: draft.ExitCode, ExecutedAt: draft.ExecutedAt,
	}
	input := store.AppendExecutionInput{
		Event: event, RuleVersionID: ruleID, Classification: classification, ViolationCode: violationCode,
		ViolationFacts: map[string]any{"planId": draft.PlanID, "side": draft.Side, "quantity": draft.Quantity, "settlementFen": draft.SettlementFen},
		Evidence:       draft.Evidence, BrokerReference: draft.BrokerReference, ReferencePriceMinor: draft.ReferencePriceMinor, ReferencePriceAt: draft.ReferencePriceAt,
		QuickRecord: quickRecord,
	}
	if violationCode != "" {
		previousViolations, err := s.store.CountViolations(ctx)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		expectedEnd := addBusinessDays(draft.ExecutedAt, rule.Cooldown.FirstSeriousTradingDays)
		if previousViolations == 1 {
			expectedEnd = addBusinessDays(draft.ExecutedAt, rule.Cooldown.SecondSeriousTradingDays)
		} else if previousViolations >= 2 {
			expectedEnd = draft.ExecutedAt.AddDate(0, 0, rule.Cooldown.ThirdSeriousCalendarDays)
		}
		input.Cooldown = &store.CooldownInput{
			Reason: violationCode, Severity: "serious", StartsAt: draft.ExecutedAt,
			ExpectedEndsAt: expectedEnd,
			AllowedActions: []string{"record", "reduce", "risk_exit", "review", "simulate_plan"},
		}
	}
	result, err := s.store.AppendExecution(ctx, input)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	receipt := ExecutionReceipt{ID: result.ExecutionID, Classification: classification, ViolationCode: violationCode, Position: portfolio.Positions[draft.InstrumentID], CashFen: portfolio.AvailableCashFen, PendingReview: quickRecord}
	if input.Cooldown != nil {
		receipt.Cooldown = &CooldownReceipt{ID: result.CooldownID, Reason: input.Cooldown.Reason, ExpectedEndsAt: input.Cooldown.ExpectedEndsAt}
	}
	return receipt, nil
}

func (s *Service) ReverseExecution(ctx context.Context, id, reason string) (ExecutionReceipt, error) {
	if strings.TrimSpace(reason) == "" {
		return ExecutionReceipt{}, fmt.Errorf("冲正原因不能为空")
	}
	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	result, err := s.store.AppendExecutionReversal(ctx, id, ruleID, strings.TrimSpace(reason), s.now())
	if err != nil {
		return ExecutionReceipt{}, err
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	position := portfolio.Positions[result.InstrumentID]
	position.InstrumentID = result.InstrumentID
	return ExecutionReceipt{ID: result.ExecutionID, Classification: "reversed", Position: position, CashFen: portfolio.AvailableCashFen}, nil
}

func (s *Service) Portfolio(ctx context.Context) (domain.PortfolioState, error) {
	events, err := s.store.LoadExecutions(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	portfolio, err := domain.Replay(rule.InitialCapitalFen, events, nil)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	portfolio.AlibabaObservationTradingDays, portfolio.DisciplineScoreBP, err = s.store.DisciplineProgress(ctx, s.now())
	if err != nil {
		return domain.PortfolioState{}, err
	}
	quotes, err := s.store.LatestQuotes(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	portfolio.ChinaTechExposureFen = 0
	portfolio.ChinaTechUnrealizedPnLFen = 0
	for id, position := range portfolio.Positions {
		position.MarketValueFen = position.CostFen
		if quote, ok := quotes[id]; ok {
			position.ReferencePriceMinor = quote.PriceMinor
			position.ReferencePriceAt = quote.PriceAt
			position.ReferencePriceSource = quote.Source
			position.MarketValueFen = quote.PriceMinor * int64(position.Quantity)
			if quote.Currency == "HKD" {
				position.MarketValueFen = position.MarketValueFen * int64(rule.HKDCNYRateBP) / 10_000
			}
			position.UnrealizedPnLFen = position.MarketValueFen - position.CostFen
			if position.UnrealizedPnLFen < 0 {
				portfolio.CumulativeLossFen += -position.UnrealizedPnLFen
			}
		}
		if position.IsChinaTech {
			portfolio.ChinaTechExposureFen += position.MarketValueFen
			portfolio.ChinaTechUnrealizedPnLFen += position.UnrealizedPnLFen
		}
		portfolio.Positions[id] = position
	}
	return portfolio, nil
}

func validExitCode(code string) bool {
	return code == "T" || code == "B" || code == "R" || code == "C"
}

func addBusinessDays(start time.Time, days int) time.Time {
	result := start
	for added := 0; added < days; {
		result = result.AddDate(0, 0, 1)
		if result.Weekday() != time.Saturday && result.Weekday() != time.Sunday {
			added++
		}
	}
	return result
}
