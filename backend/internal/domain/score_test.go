package domain

import "testing"

func TestScoreDoesNotAwardPendingSellEvidence(t *testing.T) {
	got := ScoreTrades([]ScorableTrade{{ID: "open", PlanQualified: true, PositionWithinLimit: true, NoAveraging: true, ExitEvidence: nil, TimelyImmutableRecord: true}})
	if got.Earned != 80 || got.Available != 80 || got.RateBP != 10_000 {
		t.Fatalf("unexpected score: %#v", got)
	}
}

func TestCompliantLossOutscoresViolatingProfit(t *testing.T) {
	yes := true
	got := ScoreTrades([]ScorableTrade{
		{ID: "loss", PlanQualified: true, PositionWithinLimit: true, NoAveraging: true, ExitEvidence: &yes, TimelyImmutableRecord: true, RealizedPnLFen: -100_000},
		{ID: "profit", PlanQualified: false, PositionWithinLimit: true, NoAveraging: false, ExitEvidence: &yes, TimelyImmutableRecord: true, RealizedPnLFen: 200_000},
	})
	if got.ByTrade[0].Points <= got.ByTrade[1].Points {
		t.Fatalf("unexpected score: %#v", got)
	}
}
