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
