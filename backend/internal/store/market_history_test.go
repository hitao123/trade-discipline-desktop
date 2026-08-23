package store

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func TestDailyBarObservationsKeepCorrectionsAndReturnLatest(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	first := market.DailyBar{
		TradeDate: "2026-08-11", Market: "SH", Code: "600001", CloseMinor: 1_000,
		TurnoverFen: 125_000_000_000, Source: "fixture", SourceTime: time.Date(2026, 8, 11, 7, 0, 0, 0, time.UTC),
	}
	corrected := first
	corrected.CloseMinor = 1_020

	one := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	two := one.Add(time.Hour)
	three := two.Add(time.Hour)
	stats, err := db.SaveDailyBars(ctx, []market.DailyBar{first}, one)
	if err != nil || stats.Inserted != 1 || stats.Duplicates != 0 {
		t.Fatalf("first save stats=%#v err=%v", stats, err)
	}
	stats, err = db.SaveDailyBars(ctx, []market.DailyBar{first}, two)
	if err != nil || stats.Inserted != 0 || stats.Duplicates != 1 {
		t.Fatalf("duplicate save stats=%#v err=%v", stats, err)
	}
	stats, err = db.SaveDailyBars(ctx, []market.DailyBar{corrected}, three)
	if err != nil || stats.Inserted != 1 {
		t.Fatalf("correction save stats=%#v err=%v", stats, err)
	}

	var observations int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM market_daily_bar_observations WHERE market='SH' AND code='600001'`).Scan(&observations); err != nil || observations != 2 {
		t.Fatalf("observation count=%d err=%v", observations, err)
	}
	latest, fetchedAt, err := db.LatestDailyBars(ctx, market.InstrumentKey{Market: "SH", Code: "600001"}, "2026-07-01")
	if err != nil || len(latest) != 1 || latest[0].CloseMinor != 1_020 || !fetchedAt.Equal(three) {
		t.Fatalf("latest=%#v fetchedAt=%s err=%v", latest, fetchedAt, err)
	}
}

func TestMetricObservationsKeepCorrectionsAndReturnLatest(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	first := market.MetricPoint{
		TradeDate: "2026-08-11", Metric: market.MetricSouthboundNetBuy, ValueFen: -8_500_000_000,
		Source: "fixture", SourceTime: time.Date(2026, 8, 11, 8, 20, 0, 0, time.UTC),
	}
	corrected := first
	corrected.ValueFen = -8_200_000_000
	one := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	two := one.Add(time.Hour)
	three := two.Add(time.Hour)

	if _, err := db.SaveMetricPoints(ctx, []market.MetricPoint{first}, one); err != nil {
		t.Fatal(err)
	}
	stats, err := db.SaveMetricPoints(ctx, []market.MetricPoint{first}, two)
	if err != nil || stats.Duplicates != 1 {
		t.Fatalf("duplicate stats=%#v err=%v", stats, err)
	}
	if _, err := db.SaveMetricPoints(ctx, []market.MetricPoint{corrected}, three); err != nil {
		t.Fatal(err)
	}

	var observations int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM market_metric_observations WHERE metric='southbound_net_buy'`).Scan(&observations); err != nil || observations != 2 {
		t.Fatalf("observation count=%d err=%v", observations, err)
	}
	latest, fetchedAt, err := db.LatestMetricPoints(ctx, market.MetricSouthboundNetBuy, "2026-07-01")
	if err != nil || len(latest) != 1 || latest[0].ValueFen != -8_200_000_000 || !fetchedAt.Equal(three) {
		t.Fatalf("latest=%#v fetchedAt=%s err=%v", latest, fetchedAt, err)
	}
}

func TestMarketSnapshotsKeepCloseAndLiveModesSeparate(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	quote := market.Quote{
		TradeDate: "2026-08-21", Market: "SZ", Code: "300308", Name: "中际旭创", AssetType: market.KindStock,
		CloseMinor: 94_300, TurnoverFen: 2_668_640_063_300, Source: "fixture", SourceTime: time.Date(2026, 8, 21, 7, 40, 0, 0, time.UTC),
	}
	if _, err := db.SaveMarketSnapshot(ctx, market.KindStock, []market.Quote{quote}, "fixture-close", time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	liveQuote := quote
	liveQuote.CloseMinor = 94_500
	if _, err := db.SaveMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindStock, []market.Quote{liveQuote}, "fixture-live", time.Date(2026, 8, 21, 8, 5, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	closeSnapshot, err := db.LatestMarketSnapshot(ctx, market.KindStock)
	if err != nil || closeSnapshot.Mode != market.SnapshotModeClose || closeSnapshot.Source != "fixture-close" || closeSnapshot.Entries[0].CloseMinor != 94_300 {
		t.Fatalf("close snapshot=%#v err=%v", closeSnapshot, err)
	}
	liveSnapshot, err := db.LatestMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindStock)
	if err != nil || liveSnapshot.Mode != market.SnapshotModeLive || liveSnapshot.Source != "fixture-live" || liveSnapshot.Entries[0].CloseMinor != 94_500 {
		t.Fatalf("live snapshot=%#v err=%v", liveSnapshot, err)
	}
}

func TestMarketRefreshStatusKeepsLastSuccessAfterFailure(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	successAt := time.Date(2026, 8, 21, 7, 0, 0, 0, time.UTC)
	if _, err := db.SaveMarketRefreshStatus(ctx, market.SnapshotModeClose, successAt, true, nil); err != nil {
		t.Fatal(err)
	}
	failureAt := successAt.Add(time.Hour)
	status, err := db.SaveMarketRefreshStatus(ctx, market.SnapshotModeClose, failureAt, false, map[string]string{"stock": "source unavailable"})
	if err != nil || !status.LastAttemptAt.Equal(failureAt) || status.LastSuccessfulAt == nil || !status.LastSuccessfulAt.Equal(successAt) || status.Errors["stock"] != "source unavailable" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestMarketRefreshStatusPersistsComponentHealth(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	attemptedAt := time.Date(2026, 8, 21, 8, 5, 0, 0, time.UTC)
	stockSourceTime := time.Date(2026, 8, 21, 8, 4, 30, 0, time.UTC)
	quoteSuccess := time.Date(2026, 8, 21, 7, 58, 0, 0, time.UTC)
	components := map[string]market.ComponentHealth{
		"stock":  {State: market.HealthLive, Source: "sina-public-ranking", SourceTime: &stockSourceTime, LastSuccessfulAt: &attemptedAt, Message: "实时"},
		"quotes": {State: market.HealthCached, Source: "eastmoney-public-quote", LastSuccessfulAt: &quoteSuccess, Message: "本地缓存", DetailCode: "PRIMARY_TEMPORARY_FAILURE"},
	}

	status, err := db.SaveMarketRefreshStatusWithHealth(ctx, market.SnapshotModeClose, attemptedAt, true, map[string]string{"quotes": "source unavailable"}, components)
	if err != nil {
		t.Fatal(err)
	}
	if status.Components["stock"].State != market.HealthLive || status.Components["stock"].SourceTime == nil || !status.Components["stock"].SourceTime.Equal(stockSourceTime) {
		t.Fatalf("stock health=%#v", status.Components["stock"])
	}
	if status.Components["quotes"].State != market.HealthCached || status.Components["quotes"].DetailCode != "PRIMARY_TEMPORARY_FAILURE" {
		t.Fatalf("quote health=%#v", status.Components["quotes"])
	}
}
