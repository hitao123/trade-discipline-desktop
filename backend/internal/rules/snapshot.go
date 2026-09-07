package rules

type CooldownRule struct {
	FirstSeriousTradingDays  int `json:"firstSeriousTradingDays"`
	SecondSeriousTradingDays int `json:"secondSeriousTradingDays"`
	ThirdSeriousCalendarDays int `json:"thirdSeriousCalendarDays"`
	LowScoreTradingDays      int `json:"lowScoreTradingDays"`
}

type Snapshot struct {
	Version                    int            `json:"version"`
	ProfileMode                string         `json:"profileMode,omitempty"`
	InitialCapitalFen          int64          `json:"initialCapitalFen"`
	LossCautionFen             int64          `json:"lossCautionFen"`
	LossRedLineFen             int64          `json:"lossRedLineFen"`
	ChinaTechLimitFen          int64          `json:"chinaTechLimitFen,omitempty"`
	TencentMaxShares           int            `json:"tencentMaxShares,omitempty"`
	AlibabaMaxShares           int            `json:"alibabaMaxShares,omitempty"`
	HKBoardLot                 int            `json:"hkBoardLot,omitempty"`
	HKDCNYRateBP               int            `json:"hkdCnyRateBP,omitempty"`
	CurrencyRatesBP            map[string]int `json:"currencyRatesBP,omitempty"`
	MinimumDisciplineScoreBP   int            `json:"minimumDisciplineScoreBP,omitempty"`
	TencentObservationDays     int            `json:"tencentObservationDays,omitempty"`
	EnforceTencentSequenceGate bool           `json:"enforceTencentSequenceGate,omitempty"`
	NoAddToLosingInstrument    bool           `json:"noAddToLosingInstrument"`
	NoCrossInstrumentAveraging bool           `json:"noCrossInstrumentAveraging,omitempty"`
	ExitCodes                  []string       `json:"exitCodes"`
	Cooldown                   CooldownRule   `json:"cooldown"`
}

func (s Snapshot) RateBP(currency string) int {
	if currency == "" || currency == "CNY" {
		if s.CurrencyRatesBP != nil {
			if rate := s.CurrencyRatesBP["CNY"]; rate > 0 {
				return rate
			}
		}
		return 10_000
	}
	if s.CurrencyRatesBP != nil {
		if rate := s.CurrencyRatesBP[currency]; rate > 0 {
			return rate
		}
	}
	if currency == "HKD" && s.HKDCNYRateBP > 0 {
		return s.HKDCNYRateBP
	}
	return 10_000
}

func GenericSnapshot(capitalFen, maxLossFen int64) Snapshot {
	snapshot := InitialSnapshot()
	snapshot.ProfileMode = "generic"
	snapshot.InitialCapitalFen = capitalFen
	snapshot.LossCautionFen = maxLossFen * 75 / 100
	snapshot.LossRedLineFen = maxLossFen
	return snapshot
}

func InitialSnapshot() Snapshot {
	return Snapshot{
		Version:                  1,
		ProfileMode:              "generic",
		InitialCapitalFen:        20_000_000,
		LossCautionFen:           1_500_000,
		LossRedLineFen:           2_000_000,
		HKBoardLot:               100,
		HKDCNYRateBP:             9_500,
		CurrencyRatesBP:          map[string]int{"CNY": 10_000, "HKD": 9_500},
		MinimumDisciplineScoreBP: 9_000,
		NoAddToLosingInstrument:  false,
		ExitCodes:                []string{"T", "B", "R", "C"},
		Cooldown: CooldownRule{
			FirstSeriousTradingDays:  5,
			SecondSeriousTradingDays: 20,
			ThirdSeriousCalendarDays: 30,
			LowScoreTradingDays:      10,
		},
	}
}
