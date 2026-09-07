package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type ExecutionDraft struct {
	PlanID                  string           `json:"planId,omitempty"`
	InstrumentID            string           `json:"instrumentId"`
	Side                    string           `json:"side"`
	ExecutedAt              time.Time        `json:"executedAt"`
	Quantity                int              `json:"quantity"`
	LocalPriceMinor         int64            `json:"localPriceMinor"`
	LocalPriceTenThousandth int64            `json:"localPriceTenThousandth,omitempty"`
	LocalAmountMinor        int64            `json:"localAmountMinor"`
	SettlementFen           int64            `json:"settlementFen"`
	ExitCode                string           `json:"exitCode,omitempty"`
	Evidence                string           `json:"evidence,omitempty"`
	BrokerReference         string           `json:"brokerReference,omitempty"`
	ReferencePriceMinor     int64            `json:"referencePriceMinor,omitempty"`
	ReferencePriceAt        time.Time        `json:"referencePriceAt,omitempty"`
	Emotion                 ExecutionEmotion `json:"emotion"`
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
	portfolioBefore, err := s.Portfolio(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	input, classification, violationCode, err := s.prepareExecution(ctx, draft, quickRecord, portfolioBefore, "")
	if err != nil {
		return ExecutionReceipt{}, err
	}
	result, err := s.store.AppendExecution(ctx, input)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	return s.executionReceipt(ctx, result, input, classification, violationCode)
}

func (s *Service) prepareExecution(ctx context.Context, draft ExecutionDraft, quickRecord bool, portfolioBefore domain.PortfolioState, excludedExecutionID string) (store.AppendExecutionInput, string, string, error) {
	if draft.LocalPriceTenThousandth <= 0 && draft.LocalPriceMinor > 0 {
		draft.LocalPriceTenThousandth = draft.LocalPriceMinor * 100
	}
	if err := validateExecutionEmotion(draft.Emotion); err != nil {
		return store.AppendExecutionInput{}, "", "", err
	}
	code, lotSize, isChinaTech, err := s.store.Instrument(ctx, draft.InstrumentID)
	if err != nil {
		return store.AppendExecutionInput{}, "", "", err
	}
	if draft.Quantity <= 0 {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("成交数量必须大于 0")
	}
	if lotSize > 0 && draft.Quantity%lotSize != 0 {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("成交数量必须是当前交易单位 %d 的整数倍", lotSize)
	}
	if draft.Side != "buy" && draft.Side != "sell" {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("买卖方向无效")
	}
	if draft.Side == "buy" && draft.SettlementFen >= 0 {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("买入实际人民币扣款必须为负数")
	}
	if draft.Side == "sell" && draft.SettlementFen <= 0 {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("卖出实际人民币到账必须为正数")
	}
	if draft.ExecutedAt.IsZero() {
		draft.ExecutedAt = s.now()
	}
	if draft.Side == "sell" && portfolioBefore.Positions[draft.InstrumentID].Quantity < draft.Quantity {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("卖出数量超过本地持仓，请先补齐缺失成交")
	}
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return store.AppendExecutionInput{}, "", "", err
	}
	if err := validateCNYSettlement(code, draft); err != nil {
		return store.AppendExecutionInput{}, "", "", err
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
			return store.AppendExecutionInput{}, "", "", err
		}
		switch {
		case plan.Status != "qualified":
			markViolation("PLAN_NOT_QUALIFIED")
		case plan.Draft.InstrumentID != draft.InstrumentID:
			markViolation("PLAN_INSTRUMENT_MISMATCH")
		case draft.ExecutedAt.After(plan.Draft.ValidUntil):
			markViolation("PLAN_EXPIRED")
		case draft.Side == "buy":
			used, err := s.store.ActivePlanExecutionQuantityExcluding(ctx, draft.PlanID, domain.ExecutionBuy, excludedExecutionID)
			if err != nil {
				return store.AppendExecutionInput{}, "", "", err
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
		if rule.NoAddToLosingInstrument && position.Quantity > 0 && position.UnrealizedPnLFen < 0 {
			markViolation("NO_ADD_TO_LOSER")
		}
		actualCost := -draft.SettlementFen
		if actualCost > portfolioBefore.AvailableCashFen {
			markViolation("INSUFFICIENT_CASH")
		}
		if portfolioBefore.CumulativeLossFen >= rule.LossRedLineFen {
			markViolation("PORTFOLIO_LOSS_RED_LINE")
		}
	}

	ruleID, err := s.store.CurrentRuleVersionID(ctx)
	if err != nil {
		return store.AppendExecutionInput{}, "", "", err
	}
	event := domain.ExecutionEvent{
		EventType: domain.ExecutionType(draft.Side), PlanID: draft.PlanID, InstrumentID: draft.InstrumentID,
		Code: code, Quantity: draft.Quantity, LocalPriceMinor: draft.LocalPriceMinor, LocalPriceTenThousandth: draft.LocalPriceTenThousandth, LocalAmountMinor: draft.LocalAmountMinor,
		SettlementFen: draft.SettlementFen, IsChinaTech: isChinaTech, ExitCode: draft.ExitCode, ExecutedAt: draft.ExecutedAt,
	}
	emotionJSON, err := json.Marshal(draft.Emotion)
	if err != nil {
		return store.AppendExecutionInput{}, "", "", fmt.Errorf("保存当时情绪失败: %w", err)
	}
	input := store.AppendExecutionInput{
		Event: event, RuleVersionID: ruleID, Classification: classification, ViolationCode: violationCode,
		ViolationFacts: map[string]any{"planId": draft.PlanID, "side": draft.Side, "quantity": draft.Quantity, "settlementFen": draft.SettlementFen},
		Evidence:       draft.Evidence, BrokerReference: draft.BrokerReference, ReferencePriceMinor: draft.ReferencePriceMinor, ReferencePriceAt: draft.ReferencePriceAt,
		EmotionJSON: string(emotionJSON), QuickRecord: quickRecord,
	}
	if violationCode != "" {
		previousViolations, err := s.store.CountViolations(ctx)
		if err != nil {
			return store.AppendExecutionInput{}, "", "", err
		}
		if excludedExecutionID != "" {
			active, err := s.store.ExecutionHasActiveViolation(ctx, excludedExecutionID)
			if err != nil {
				return store.AppendExecutionInput{}, "", "", err
			}
			if active && previousViolations > 0 {
				previousViolations--
			}
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
	return input, classification, violationCode, nil
}

func (s *Service) executionReceipt(ctx context.Context, result store.AppendExecutionResult, input store.AppendExecutionInput, classification, violationCode string) (ExecutionReceipt, error) {
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	receipt := ExecutionReceipt{ID: result.ExecutionID, Classification: classification, ViolationCode: violationCode, Position: portfolio.Positions[input.Event.InstrumentID], CashFen: portfolio.AvailableCashFen, PendingReview: input.QuickRecord || input.ViolationCode != "" || input.Event.EventType == domain.ExecutionSell}
	if input.Cooldown != nil {
		receipt.Cooldown = &CooldownReceipt{ID: result.CooldownID, Reason: input.Cooldown.Reason, ExpectedEndsAt: input.Cooldown.ExpectedEndsAt}
	}
	return receipt, nil
}

func (s *Service) ListActiveExecutions(ctx context.Context, instrumentID string) ([]store.ExecutionRecord, error) {
	return s.store.ListActiveExecutions(ctx, strings.TrimSpace(instrumentID))
}

func (s *Service) CorrectExecution(ctx context.Context, originalID, reason string, draft ExecutionDraft) (ExecutionReceipt, error) {
	originalID = strings.TrimSpace(originalID)
	reason = strings.TrimSpace(reason)
	if originalID == "" || reason == "" {
		return ExecutionReceipt{}, fmt.Errorf("请选择成交并填写修正原因")
	}
	if draft.LocalPriceTenThousandth <= 0 && draft.LocalPriceMinor <= 0 {
		return ExecutionReceipt{}, fmt.Errorf("修正后的成交均价必须大于 0")
	}
	quickRecord, err := s.store.ExecutionHasPostTradeReview(ctx, originalID)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	events, err := s.store.LoadExecutions(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	found := false
	for _, event := range events {
		if event.ID == originalID && event.EventType != domain.ExecutionReversal {
			found = true
			break
		}
	}
	if !found {
		return ExecutionReceipt{}, fmt.Errorf("未找到可修正的原成交")
	}
	events = append(events, domain.ExecutionEvent{ID: "correction-preview", OriginalEventID: originalID, EventType: domain.ExecutionReversal})
	portfolioBefore, err := s.portfolioFromEvents(ctx, events)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	input, classification, violationCode, err := s.prepareExecution(ctx, draft, quickRecord, portfolioBefore, originalID)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	result, err := s.store.AppendExecutionCorrection(ctx, originalID, input.RuleVersionID, reason, input, s.now())
	if err != nil {
		return ExecutionReceipt{}, err
	}
	return s.executionReceipt(ctx, result.Replacement, input, classification, violationCode)
}

func validateExecutionEmotion(emotion ExecutionEmotion) error {
	for label, value := range map[string]int{"恐惧": emotion.FearScore, "贪婪": emotion.GreedScore, "回本/报复性冲动": emotion.RevengeScore} {
		if value < 0 || value > 10 {
			return fmt.Errorf("%s评分必须在 0 到 10 之间", label)
		}
	}
	return nil
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
	return s.portfolioFromEvents(ctx, events)
}

func (s *Service) portfolioFromEvents(ctx context.Context, events []domain.ExecutionEvent) (domain.PortfolioState, error) {
	rule, err := s.store.CurrentRule(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	initialCashFen, err := s.store.AccountInitialCashFen(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	cashEvents, err := s.store.LoadCashEvents(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	portfolio, err := domain.Replay(initialCashFen, events, cashEvents)
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
	instruments, err := s.store.InstrumentsByID(ctx)
	if err != nil {
		return domain.PortfolioState{}, err
	}
	portfolio.ChinaTechExposureFen = 0
	portfolio.ChinaTechUnrealizedPnLFen = 0
	for id, position := range portfolio.Positions {
		if instrument, ok := instruments[id]; ok {
			position.Name = instrument.Name
			position.Market = instrument.Market
			position.Currency = instrument.Currency
			position.LotSize = instrument.LotSize
		}
		position.MarketValueFen = position.CostFen
		if quote, ok := quotes[id]; ok {
			position.ReferencePriceMinor = quote.PriceMinor
			position.ReferencePriceAt = quote.PriceAt
			position.ReferencePriceSource = quote.Source
			position.MarketValueFen = quote.PriceMinor * int64(position.Quantity)
			currency := position.Currency
			if currency == "" {
				currency = quote.Currency
			}
			position.MarketValueFen = position.MarketValueFen * int64(rule.RateBP(currency)) / 10_000
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
	portfolio.CurrentPressureLossFen = portfolio.CumulativeLossFen
	return portfolio, nil
}

func validateCNYSettlement(code string, draft ExecutionDraft) error {
	if draft.LocalAmountMinor <= 0 || strings.HasSuffix(strings.ToUpper(code), ".HK") {
		return nil
	}
	expected := draft.LocalAmountMinor
	actual := absInt64(draft.SettlementFen)
	tolerance := expected / 100
	if tolerance < 100 {
		tolerance = 100
	}
	if actual > expected+tolerance || actual+tolerance < expected {
		return fmt.Errorf("实际人民币金额与本币成交金额偏差过大，请核对券商费用")
	}
	return nil
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
