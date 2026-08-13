package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

type fakeMarketProvider struct {
	stock     []market.Quote
	etf       []market.Quote
	quotes    []market.Quote
	dailyBars []market.DailyBar
	metrics   market.MetricBatch
	stockErr  error
	etfErr    error
	dailyErr  error
	quotesErr error
}

func (p fakeMarketProvider) FetchRankings(_ context.Context, kind market.RankingKind) ([]market.Quote, error) {
	if kind == market.KindStock {
		if p.stockErr != nil {
			return nil, p.stockErr
		}
		return p.stock, nil
	}
	if p.etfErr != nil {
		return nil, p.etfErr
	}
	return p.etf, nil
}

func TestRefreshMarketKeepsLatestSuccessfulKindOnPartialFailure(t *testing.T) {
	svc := openExecutionService(t)
	oldETF := market.Quote{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "沪深300ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture"}
	svc.SetMarketProvider(fakeMarketProvider{
		stock: []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "旧股票", AssetType: market.KindStock, CloseMinor: 1000, TurnoverFen: 9_000_000, Source: "fixture"}},
		etf:   []market.Quote{oldETF},
	})
	first, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetMarketProvider(fakeMarketProvider{
		stock:  []market.Quote{{TradeDate: "2026-08-12", Market: "SH", Code: "600002", Name: "新股票", AssetType: market.KindStock, CloseMinor: 1100, TurnoverFen: 10_000_000, Source: "fixture"}},
		etfErr: errors.New("ETF source unavailable"),
	})
	second, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Stock.TradeDate != "2026-08-12" || second.ETF.ID != first.ETF.ID || second.Errors["etf"] == "" {
		t.Fatalf("unexpected partial refresh: %#v", second)
	}
}

func (p fakeMarketProvider) FetchQuotes(_ context.Context, _ []market.InstrumentKey) ([]market.Quote, error) {
	return p.quotes, p.quotesErr
}

func (p fakeMarketProvider) FetchDailyBars(_ context.Context, _ market.InstrumentKey, _ int) ([]market.DailyBar, error) {
	return p.dailyBars, p.dailyErr
}

func (p fakeMarketProvider) FetchMarketMetrics(_ context.Context, _ int) market.MetricBatch {
	return p.metrics
}

func TestRefreshMarketStillRefreshesOverviewWhenQuoteUpdateFails(t *testing.T) {
	svc := openExecutionService(t)
	svc.SetMarketProvider(fakeMarketProvider{
		stock:     []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "示例股票", AssetType: market.KindStock, CloseMinor: 1_000, TurnoverFen: 9_000_000, Source: "fixture"}},
		etf:       []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "沪深300ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture"}},
		quotesErr: errors.New("quotes unavailable"),
		metrics: market.MetricBatch{Points: []market.MetricPoint{{
			TradeDate: "2026-08-11", Metric: market.MetricAShareTurnover, ValueFen: 150_000_000_000_000, Source: "fixture",
		}}},
	})

	result, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors["quotes"] == "" || len(result.Overview.AShareTurnover) != 1 {
		t.Fatalf("overview was skipped after quote error: %#v", result)
	}
}

func TestRefreshMarketStillRefreshesOverviewWhenBothRankingsFail(t *testing.T) {
	svc := openExecutionService(t)
	svc.SetMarketProvider(fakeMarketProvider{
		stockErr: errors.New("stock ranking unavailable"),
		etfErr:   errors.New("ETF ranking unavailable"),
		metrics: market.MetricBatch{Points: []market.MetricPoint{
			{TradeDate: "2026-08-11", Metric: market.MetricAShareTurnover, ValueFen: 150_000_000_000_000, Source: "fixture"},
			{TradeDate: "2026-08-11", Metric: market.MetricSouthboundNetBuy, ValueFen: -7_000_000_000, Source: "fixture"},
		}},
	})

	result, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Overview.AShareTurnover) != 1 || len(result.Overview.SouthboundNetBuy) != 1 {
		t.Fatalf("overview was skipped after ranking errors: %#v", result)
	}
	if result.Errors["stock"] == "" || result.Errors["etf"] == "" {
		t.Fatalf("ranking errors were not preserved: %#v", result.Errors)
	}
}

func TestRefreshMarketKeepsCachedOverviewOnPartialFailure(t *testing.T) {
	svc := openExecutionService(t)
	base := fakeMarketProvider{
		stock: []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "示例股票", AssetType: market.KindStock, CloseMinor: 1_000, TurnoverFen: 9_000_000, Source: "fixture"}},
		etf:   []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "沪深300ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture"}},
		metrics: market.MetricBatch{
			Points: []market.MetricPoint{{TradeDate: "2026-08-11", Metric: market.MetricAShareTurnover, ValueFen: 150_000_000_000_000, Source: "fixture"}},
			Errors: map[market.MetricKind]string{market.MetricSouthboundNetBuy: "southbound unavailable"},
		},
	}
	svc.SetMarketProvider(base)
	first, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Overview.AShareTurnover) != 1 || first.Overview.AShareTurnover[0].ValueFen != 150_000_000_000_000 || first.Overview.Errors[string(market.MetricSouthboundNetBuy)] == "" {
		t.Fatalf("unexpected first overview: %#v", first.Overview)
	}

	base.metrics = market.MetricBatch{Errors: map[market.MetricKind]string{
		market.MetricAShareTurnover:   "turnover unavailable",
		market.MetricSouthboundNetBuy: "southbound unavailable",
	}}
	svc.SetMarketProvider(base)
	second, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Overview.AShareTurnover) != 1 || !second.Overview.Cached || second.Overview.Errors[string(market.MetricAShareTurnover)] == "" {
		t.Fatalf("cached overview missing: %#v", second.Overview)
	}
}

func TestRefreshMarketHistoryFallsBackToCache(t *testing.T) {
	svc := openExecutionService(t)
	key := market.InstrumentKey{Market: "SH", Code: "600001"}
	svc.SetMarketProvider(fakeMarketProvider{dailyBars: []market.DailyBar{{
		TradeDate: "2026-08-11", Market: "SH", Code: "600001", CloseMinor: 1_020,
		TurnoverFen: 8_000_000_000, Source: "fixture", SourceTime: time.Date(2026, 8, 11, 7, 0, 0, 0, time.UTC),
	}}})
	first, err := svc.RefreshMarketHistory(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Points) != 1 || first.Points[0].CloseMinor != 1_020 || first.Cached || first.Error != "" {
		t.Fatalf("unexpected first history: %#v", first)
	}

	svc.SetMarketProvider(fakeMarketProvider{dailyErr: errors.New("history unavailable")})
	second, err := svc.RefreshMarketHistory(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Points) != 1 || second.Points[0].CloseMinor != 1_020 || !second.Cached || !strings.Contains(second.Error, "history unavailable") {
		t.Fatalf("cache fallback missing: %#v", second)
	}
}

func TestRefreshMarketPersistsIndependentStockAndETFSnapshots(t *testing.T) {
	svc := openExecutionService(t)
	svc.SetMarketProvider(fakeMarketProvider{
		stock:  []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "示例股票", AssetType: market.KindStock, CloseMinor: 1000, TurnoverFen: 9_000_000, Source: "fixture"}},
		etf:    []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "沪深300ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture"}},
		quotes: []market.Quote{{TradeDate: "2026-08-11", Market: "HK", Code: "9988.HK", Name: "阿里巴巴-W", AssetType: market.KindStock, CloseMinor: 12_200, Source: "fixture"}},
	})
	result, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stock.Entries) != 1 || len(result.ETF.Entries) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	latest, err := svc.LatestMarket(context.Background())
	if err != nil || latest.Stock.ID == "" || latest.ETF.ID == "" {
		t.Fatalf("latest=%#v err=%v", latest, err)
	}
	var latestPrice int64
	if err := svc.store.DB().QueryRow(`SELECT latest_price_minor FROM instruments WHERE code='9988.HK'`).Scan(&latestPrice); err != nil || latestPrice != 12_200 {
		t.Fatalf("latest price=%d err=%v", latestPrice, err)
	}
}

func TestCSVPreviewConfirmUsesSameDigest(t *testing.T) {
	svc := openExecutionService(t)
	content := "trade_date,market,code,name,asset_type,close,change_pct,turnover\n2026-08-11,SH,510300,沪深300ETF,etf,4.20,0.15,1000000\n"
	preview, digest := svc.PreviewMarketCSV(strings.NewReader(content))
	if len(preview.Valid) != 1 || digest == "" {
		t.Fatalf("preview=%#v digest=%q", preview, digest)
	}
	if _, err := svc.ConfirmMarketCSV(context.Background(), preview, "changed"); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if _, err := svc.ConfirmMarketCSV(context.Background(), preview, digest); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(svc.store.Path()), "backups", "before-csv-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("CSV import backup missing: %v, %v", backups, err)
	}
}
