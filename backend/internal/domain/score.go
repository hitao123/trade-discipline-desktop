package domain

type ScorableTrade struct {
	ID                    string
	PlanQualified         bool
	PositionWithinLimit   bool
	NoAveraging           bool
	ExitEvidence          *bool
	TimelyImmutableRecord bool
	RealizedPnLFen        int64
}

type TradeScore struct {
	ID        string `json:"id"`
	Points    int    `json:"points"`
	Available int    `json:"available"`
	RateBP    int    `json:"rateBP"`
}

type ScoreSummary struct {
	Earned    int          `json:"earned"`
	Available int          `json:"available"`
	RateBP    int          `json:"rateBP"`
	ByTrade   []TradeScore `json:"byTrade"`
}

func ScoreTrades(trades []ScorableTrade) ScoreSummary {
	summary := ScoreSummary{ByTrade: make([]TradeScore, 0, len(trades))}
	for _, trade := range trades {
		item := TradeScore{ID: trade.ID, Available: 80}
		if trade.PlanQualified {
			item.Points += 20
		}
		if trade.PositionWithinLimit {
			item.Points += 20
		}
		if trade.NoAveraging {
			item.Points += 20
		}
		if trade.TimelyImmutableRecord {
			item.Points += 20
		}
		if trade.ExitEvidence != nil {
			item.Available += 20
			if *trade.ExitEvidence {
				item.Points += 20
			}
		}
		if item.Available > 0 {
			item.RateBP = item.Points * 10_000 / item.Available
		}
		summary.Earned += item.Points
		summary.Available += item.Available
		summary.ByTrade = append(summary.ByTrade, item)
	}
	if summary.Available > 0 {
		summary.RateBP = summary.Earned * 10_000 / summary.Available
	}
	return summary
}
