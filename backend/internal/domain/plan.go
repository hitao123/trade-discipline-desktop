package domain

import "time"

type TradePlanDraft struct {
	InstrumentID        string    `json:"instrumentId"`
	Code                string    `json:"code"`
	IsChinaTech         bool      `json:"isChinaTech"`
	LotSize             int       `json:"lotSize"`
	Thesis              string    `json:"thesis"`
	Falsification       string    `json:"falsification"`
	PricedExpectation   string    `json:"pricedExpectation"`
	Evidence            []string  `json:"evidence"`
	BreakCondition      string    `json:"breakCondition"`
	ExitCondition       string    `json:"exitCondition"`
	EntryLowMinor       int64     `json:"entryLowMinor"`
	EntryHighMinor      int64     `json:"entryHighMinor"`
	RiskExitMinor       int64     `json:"riskExitMinor"`
	TargetExitLowMinor  int64     `json:"targetExitLowMinor"`
	TargetExitHighMinor int64     `json:"targetExitHighMinor"`
	Quantity            int       `json:"quantity"`
	EstimatedCostFen    int64     `json:"estimatedCostFen"`
	MaxPlanLossFen      int64     `json:"maxPlanLossFen"`
	StressDropBP        int       `json:"stressDropBP"`
	ReferencePriceAt    time.Time `json:"referencePriceAt"`
	ValidUntil          time.Time `json:"validUntil"`
	FearScore           int       `json:"fearScore"`
	GreedScore          int       `json:"greedScore"`
	RevengeScore        int       `json:"revengeScore"`
}
