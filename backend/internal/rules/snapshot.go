package rules

type CooldownRule struct {
	FirstSeriousTradingDays  int `json:"firstSeriousTradingDays"`
	SecondSeriousTradingDays int `json:"secondSeriousTradingDays"`
	ThirdSeriousCalendarDays int `json:"thirdSeriousCalendarDays"`
	LowScoreTradingDays      int `json:"lowScoreTradingDays"`
}

type Snapshot struct {
	Version                    int          `json:"version"`
	ProfileMode                string       `json:"profileMode,omitempty"`
	InitialCapitalFen          int64        `json:"initialCapitalFen"`
	LossCautionFen             int64        `json:"lossCautionFen"`
	LossRedLineFen             int64        `json:"lossRedLineFen"`
	ChinaTechLimitFen          int64        `json:"chinaTechLimitFen"`
	TencentMaxShares           int          `json:"tencentMaxShares"`
	AlibabaMaxShares           int          `json:"alibabaMaxShares"`
	HKBoardLot                 int          `json:"hkBoardLot"`
	HKDCNYRateBP               int          `json:"hkdCnyRateBP"`
	MinimumDisciplineScoreBP   int          `json:"minimumDisciplineScoreBP"`
	TencentObservationDays     int          `json:"tencentObservationDays"`
	EnforceTencentSequenceGate bool         `json:"enforceTencentSequenceGate"`
	NoAddToLosingInstrument    bool         `json:"noAddToLosingInstrument"`
	NoCrossInstrumentAveraging bool         `json:"noCrossInstrumentAveraging"`
	ExitCodes                  []string     `json:"exitCodes"`
	Cooldown                   CooldownRule `json:"cooldown"`
}

func (s Snapshot) EffectiveProfileMode() string {
	if s.ProfileMode == "generic" {
		return "generic"
	}
	return "legacy"
}

func GenericSnapshot(capitalFen, maxLossFen int64) Snapshot {
	snapshot := InitialSnapshot()
	snapshot.ProfileMode = "generic"
	snapshot.InitialCapitalFen = capitalFen
	snapshot.LossCautionFen = maxLossFen * 75 / 100
	snapshot.LossRedLineFen = maxLossFen
	snapshot.ChinaTechLimitFen = 0
	snapshot.TencentMaxShares = 0
	snapshot.AlibabaMaxShares = 0
	snapshot.EnforceTencentSequenceGate = false
	snapshot.NoCrossInstrumentAveraging = false
	return snapshot
}

func InitialSnapshot() Snapshot {
	return Snapshot{
		Version:                    1,
		InitialCapitalFen:          20_000_000,
		LossCautionFen:             1_500_000,
		LossRedLineFen:             2_000_000,
		ChinaTechLimitFen:          8_000_000,
		TencentMaxShares:           100,
		AlibabaMaxShares:           100,
		HKBoardLot:                 100,
		HKDCNYRateBP:               9_500,
		MinimumDisciplineScoreBP:   9_000,
		TencentObservationDays:     20,
		EnforceTencentSequenceGate: false,
		NoAddToLosingInstrument:    true,
		NoCrossInstrumentAveraging: true,
		ExitCodes:                  []string{"T", "B", "R", "C"},
		Cooldown: CooldownRule{
			FirstSeriousTradingDays:  5,
			SecondSeriousTradingDays: 20,
			ThirdSeriousCalendarDays: 30,
			LowScoreTradingDays:      10,
		},
	}
}
