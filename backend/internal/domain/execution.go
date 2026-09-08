package domain

import "time"

type ExecutionType string

const (
	ExecutionBuy      ExecutionType = "buy"
	ExecutionSell     ExecutionType = "sell"
	ExecutionReversal ExecutionType = "reversal"
)

type ExecutionEmotion struct {
	FearScore    int `json:"fearScore"`
	GreedScore   int `json:"greedScore"`
	RevengeScore int `json:"revengeScore"`
	// State keeps a missing answer distinct from an explicit zero. Empty is
	// reserved for records written before this field existed and is never
	// reinterpreted as a score.
	State string `json:"state,omitempty"`
}

type ExecutionEvent struct {
	ID                      string
	OriginalEventID         string
	EventType               ExecutionType
	PlanID                  string
	InstrumentID            string
	Code                    string
	Quantity                int
	LocalPriceMinor         int64
	LocalPriceTenThousandth int64
	LocalAmountMinor        int64
	SettlementFen           int64
	IsChinaTech             bool
	ExitCode                string
	ExecutedAt              time.Time
}

type CashEvent struct {
	ID              string
	OriginalEventID string
	AmountFen       int64
	OccurredAt      time.Time
}
