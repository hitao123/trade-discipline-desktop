package domain

import "time"

type ExecutionType string

const (
	ExecutionBuy      ExecutionType = "buy"
	ExecutionSell     ExecutionType = "sell"
	ExecutionReversal ExecutionType = "reversal"
)

type ExecutionEvent struct {
	ID               string
	OriginalEventID  string
	EventType        ExecutionType
	PlanID           string
	InstrumentID     string
	Code             string
	Quantity         int
	LocalPriceMinor  int64
	LocalAmountMinor int64
	SettlementFen    int64
	IsChinaTech      bool
	ExitCode         string
	ExecutedAt       time.Time
}

type CashEvent struct {
	ID              string
	OriginalEventID string
	AmountFen       int64
	OccurredAt      time.Time
}
