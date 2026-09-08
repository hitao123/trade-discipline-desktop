package market

import (
	"fmt"
	"testing"
	"time"
)

func verifiedRankingFixture(kind RankingKind, day string) []Quote {
	items := make([]Quote, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, Quote{TradeDate: day, Market: "SH", Code: fmt.Sprintf("%06d", 600000+i), Name: fmt.Sprintf("样本%d", i), AssetType: kind, CloseMinor: 100 + int64(i), TurnoverFen: int64(20-i) * 100, Source: "fixture", SourceTime: time.Date(2026, 8, 12, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))})
	}
	return items
}

func TestAssessCloseRankingRequiresCompleteAfterCloseSortedSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 12, 15, 10, 0, 0, time.FixedZone("CST", 8*3600))
	quotes := verifiedRankingFixture(KindStock, "2026-08-12")
	if got := AssessCloseRanking(now, KindStock, quotes); got.Quality != RankingQualityVerifiedClose {
		t.Fatalf("quality=%#v", got)
	}
	if got := AssessCloseRanking(now.Add(-time.Minute), KindStock, quotes); got.Quality != RankingQualityIncomplete {
		t.Fatalf("early quality=%#v", got)
	}
	if got := AssessCloseRanking(now, KindStock, quotes[:19]); got.Quality != RankingQualityIncomplete {
		t.Fatalf("short quality=%#v", got)
	}
	quotes[1].TurnoverFen = quotes[0].TurnoverFen + 1
	if got := AssessCloseRanking(now, KindStock, quotes); got.Quality != RankingQualityIncomplete {
		t.Fatalf("unsorted quality=%#v", got)
	}
}
