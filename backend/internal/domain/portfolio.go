package domain

import "time"

type PositionState struct {
	InstrumentID         string    `json:"instrumentId"`
	Code                 string    `json:"code"`
	Quantity             int       `json:"quantity"`
	CostFen              int64     `json:"costFen"`
	MarketValueFen       int64     `json:"marketValueFen"`
	ReferencePriceMinor  int64     `json:"referencePriceMinor"`
	ReferencePriceAt     time.Time `json:"referencePriceAt,omitempty"`
	ReferencePriceSource string    `json:"referencePriceSource,omitempty"`
	UnrealizedPnLFen     int64     `json:"unrealizedPnLFen"`
	RealizedPnLFen       int64     `json:"realizedPnLFen"`
	IsChinaTech          bool      `json:"isChinaTech"`
}

type PortfolioState struct {
	AvailableCashFen              int64                    `json:"availableCashFen"`
	Positions                     map[string]PositionState `json:"positions"`
	ChinaTechExposureFen          int64                    `json:"chinaTechExposureFen"`
	ChinaTechUnrealizedPnLFen     int64                    `json:"chinaTechUnrealizedPnLFen"`
	CurrentPressureLossFen        int64                    `json:"currentPressureLossFen"`
	CumulativeLossFen             int64                    `json:"cumulativeLossFen"`
	ActiveCooldownUntil           time.Time                `json:"activeCooldownUntil,omitempty"`
	AlibabaObservationTradingDays int                      `json:"alibabaObservationTradingDays"`
	DisciplineScoreBP             int                      `json:"disciplineScoreBP"`
}
