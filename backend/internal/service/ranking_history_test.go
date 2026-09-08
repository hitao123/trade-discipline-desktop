package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func rankingFixture(day string, promoted string) []market.Quote {
	quotes := make([]market.Quote, 0, 20)
	stamp := time.Date(2026, 8, 12, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for i := 0; i < 20; i++ {
		code := fmt.Sprintf("%06d", 600000+i)
		turnover := int64(20-i) * 100
		if code == promoted {
			turnover = 2_500
		}
		quotes = append(quotes, market.Quote{TradeDate: day, Market: "SH", Code: code, Name: code, AssetType: market.KindStock, CloseMinor: 100, TurnoverFen: turnover, Source: "fixture", SourceTime: stamp})
	}
	// Store expects provider-normalized amount order.
	for i := range quotes {
		for j := i + 1; j < len(quotes); j++ {
			if quotes[j].TurnoverFen > quotes[i].TurnoverFen {
				quotes[i], quotes[j] = quotes[j], quotes[i]
			}
		}
	}
	return quotes
}

func TestRankingComparisonUsesCalendarBaselineAndNeverInventsTwentyFirstRank(t *testing.T) {
	svc := openExecutionService(t)
	ctx := context.Background()
	quality := market.RankingQualityInfo{Quality: market.RankingQualityVerifiedClose, UniverseVersion: market.RankingUniverseV1}
	if _, err := svc.store.SaveMarketSnapshotWithQuality(ctx, market.KindStock, rankingFixture("2026-08-11", ""), "fixture", time.Now(), quality); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.store.SaveMarketSnapshotWithQuality(ctx, market.KindStock, rankingFixture("2026-08-12", "600010"), "fixture", time.Now(), quality); err != nil {
		t.Fatal(err)
	}
	result, err := svc.RankingComparison(ctx, market.KindStock, "2026-08-12")
	if err != nil {
		t.Fatal(err)
	}
	if result.BaselineDate == nil || *result.BaselineDate != "2026-08-11" {
		t.Fatalf("baseline=%v", result.BaselineDate)
	}
	for _, entry := range result.Entries {
		if entry.Quote.Code == "600010" {
			if entry.RankDelta == nil || *entry.RankDelta <= 0 || entry.ChangeState != "up" {
				t.Fatalf("promoted=%#v", entry)
			}
			return
		}
	}
	t.Fatal("promoted entry missing")
}

func TestRankingComparisonKeepsFirstVerifiedSnapshotWhenBaselineIsMissing(t *testing.T) {
	svc := openExecutionService(t)
	ctx := context.Background()
	quality := market.RankingQualityInfo{Quality: market.RankingQualityVerifiedClose, UniverseVersion: market.RankingUniverseV1}
	if _, err := svc.store.SaveMarketSnapshotWithQuality(ctx, market.KindStock, rankingFixture("2026-08-12", ""), "fixture", time.Now(), quality); err != nil {
		t.Fatal(err)
	}

	result, err := svc.RankingComparison(ctx, market.KindStock, "2026-08-12")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "missing_baseline" || len(result.Entries) != 20 {
		t.Fatalf("reason=%q entries=%d", result.Reason, len(result.Entries))
	}
	for _, entry := range result.Entries {
		if entry.ChangeState != "unknown" || entry.RankDelta != nil || entry.StreakDays == nil || *entry.StreakDays != 1 || entry.StreakExact {
			t.Fatalf("first snapshot entry=%#v", entry)
		}
	}
}

func TestRankingComparisonKeepsCurrentDayStreakWhenUniverseChanges(t *testing.T) {
	svc := openExecutionService(t)
	ctx := context.Background()
	if _, err := svc.store.SaveMarketSnapshotWithQuality(ctx, market.KindStock, rankingFixture("2026-08-11", ""), "fixture", time.Now(), market.RankingQualityInfo{Quality: market.RankingQualityVerifiedClose, UniverseVersion: "prior-universe"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.store.SaveMarketSnapshotWithQuality(ctx, market.KindStock, rankingFixture("2026-08-12", ""), "fixture", time.Now(), market.RankingQualityInfo{Quality: market.RankingQualityVerifiedClose, UniverseVersion: market.RankingUniverseV1}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.RankingComparison(ctx, market.KindStock, "2026-08-12")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != "universe_changed" || len(result.Entries) != 20 {
		t.Fatalf("reason=%q entries=%d", result.Reason, len(result.Entries))
	}
	for _, entry := range result.Entries {
		if entry.ChangeState != "unknown" || entry.StreakDays == nil || *entry.StreakDays != 1 || entry.StreakExact {
			t.Fatalf("universe boundary entry=%#v", entry)
		}
	}
}
