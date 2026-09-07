package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type CashEventDraft struct {
	EventType  string    `json:"eventType"`
	AmountFen  int64     `json:"amountFen"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurredAt"`
}

func (s *Service) ListCashEvents(ctx context.Context) ([]store.CashEventRow, error) {
	return s.store.ListCashEvents(ctx)
}

func (s *Service) RecordCashEvent(ctx context.Context, draft CashEventDraft) (store.CashEventRow, error) {
	eventType := strings.ToLower(strings.TrimSpace(draft.EventType))
	if eventType != "deposit" && eventType != "withdrawal" && eventType != "dividend" {
		return store.CashEventRow{}, CodedError{Code: "INVALID_CASH_EVENT", Message: "资金变动类型只能是入金、出金或分红"}
	}
	if draft.AmountFen == 0 {
		return store.CashEventRow{}, CodedError{Code: "INVALID_CASH_EVENT", Message: "资金变动金额必须大于 0"}
	}
	if strings.TrimSpace(draft.Reason) == "" {
		return store.CashEventRow{}, CodedError{Code: "INVALID_CASH_EVENT", Message: "请填写资金变动原因"}
	}
	occurredAt := draft.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.now()
	}
	amount := absInt64(draft.AmountFen)
	if eventType == "withdrawal" {
		amount = -amount
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return store.CashEventRow{}, err
	}
	if portfolio.AvailableCashFen+amount < 0 {
		return store.CashEventRow{}, CodedError{Code: "INSUFFICIENT_CASH", Message: "出金超过当前可用现金"}
	}
	return s.store.AppendCashEvent(ctx, store.AppendCashEventInput{
		EventType: eventType, AmountFen: amount, Reason: strings.TrimSpace(draft.Reason), OccurredAt: occurredAt,
	})
}

func (s *Service) fundedCapitalFen(ctx context.Context) (int64, error) {
	baseline, err := s.store.AccountInitialCashFen(ctx)
	if err != nil {
		return 0, err
	}
	events, err := s.store.ListCashEvents(ctx)
	if err != nil {
		return 0, err
	}
	return store.ExternalCapitalFen(baseline, events), nil
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func (s *Service) ReverseCashEvent(ctx context.Context, id, reason string) (store.CashEventRow, error) {
	if strings.TrimSpace(reason) == "" {
		return store.CashEventRow{}, fmt.Errorf("请填写冲正原因")
	}
	original, err := s.store.CashEvent(ctx, id)
	if err != nil {
		return store.CashEventRow{}, err
	}
	if original.EventType == "reversal" || original.OriginalEventID != "" {
		return store.CashEventRow{}, fmt.Errorf("该资金记录已经是冲正，不能再次冲正")
	}
	return s.store.AppendCashEvent(ctx, store.AppendCashEventInput{
		EventType: "reversal", AmountFen: -original.AmountFen, Reason: strings.TrimSpace(reason), OccurredAt: s.now(), OriginalEventID: original.ID,
	})
}
