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
	seenKeys  *[]market.InstrumentKey
}

func TestRefreshMarketUsesFallbackAndPersistsSuccessStatus(t *testing.T) {
	svc := openExecutionService(t)
	svc.SetMarketProvider(fakeMarketProvider{stockErr: errors.New("eastmoney EOF"), etfErr: errors.New("eastmoney EOF")})
	svc.SetMarketRankingFallback(fakeMarketProvider{
		stock: []market.Quote{{TradeDate: "2026-08-12", Market: "SZ", Code: "300308", Name: "中际旭创", AssetType: market.KindStock, CloseMinor: 94_300, TurnoverFen: 2_600_000_000, Source: "sina-public-ranking"}},
		etf:   []market.Quote{{TradeDate: "2026-08-12", Market: "SH", Code: "510300", Name: "沪深300ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 800_000_000, Source: "sina-public-ranking"}},
	})
	result, err := svc.RefreshMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Stock.Source != "sina-public-ranking" || result.ETF.Source != "sina-public-ranking" || result.Errors["stock"] != "" || result.Status.LastSuccessfulAt == nil {
		t.Fatalf("fallback result=%#v", result)
	}
}

func TestRefreshLiveMarketDoesNotFetchOutsideTradingSession(t *testing.T) {
	svc := openExecutionService(t)
	called := false
	svc.SetMarketRankingFallback(rankingProviderFunc(func(_ context.Context, _ market.RankingKind) ([]market.Quote, error) {
		called = true
		return nil, nil
	}))
	result, err := svc.RefreshLiveMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if called || result.IsLive || result.State != "market_closed" {
		t.Fatalf("unexpected closed-market result=%#v called=%v", result, called)
	}
}

func TestRefreshLiveMarketDoesNotSavePreviousTradeDateAsLive(t *testing.T) {
	svc := openExecutionService(t)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, shanghai)
	svc.now = func() time.Time { return now }
	staleTime := time.Date(2026, 8, 11, 15, 0, 0, 0, shanghai)
	svc.SetMarketRankingFallback(fakeMarketProvider{
		stock: []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "旧股票", AssetType: market.KindStock, CloseMinor: 1_000, TurnoverFen: 9_000_000, Source: "fixture", SourceTime: staleTime}},
		etf:   []market.Quote{{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "旧ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture", SourceTime: staleTime}},
	})

	result, err := svc.RefreshLiveMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.IsLive || result.State != "unavailable" {
		t.Fatalf("result marked live: %#v", result)
	}
	if result.Health["live_stock"].State != market.HealthUnavailable || result.Health["live_etf"].State != market.HealthUnavailable {
		t.Fatalf("unexpected health: %#v", result.Health)
	}
	var snapshots int
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM market_snapshots WHERE snapshot_mode='live'`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 {
		t.Fatalf("stale source created %d live snapshots", snapshots)
	}
}

func TestLatestLiveMarketMarksPreviousTradeDateSnapshotAsCached(t *testing.T) {
	svc := openExecutionService(t)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	oldSourceTime := time.Date(2026, 8, 11, 15, 0, 0, 0, shanghai)
	for _, item := range []struct {
		kind  market.RankingKind
		quote market.Quote
	}{
		{kind: market.KindStock, quote: market.Quote{TradeDate: "2026-08-11", Market: "SH", Code: "600001", Name: "旧股票", AssetType: market.KindStock, CloseMinor: 1_000, TurnoverFen: 9_000_000, Source: "fixture", SourceTime: oldSourceTime}},
		{kind: market.KindETF, quote: market.Quote{TradeDate: "2026-08-11", Market: "SH", Code: "510300", Name: "旧ETF", AssetType: market.KindETF, CloseMinor: 420, TurnoverFen: 8_000_000, Source: "fixture", SourceTime: oldSourceTime}},
	} {
		if _, err := svc.store.SaveMarketSnapshotForMode(context.Background(), market.SnapshotModeLive, item.kind, []market.Quote{item.quote}, "fixture", oldSourceTime); err != nil {
			t.Fatal(err)
		}
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, shanghai) }

	result, err := svc.LatestLiveMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.IsLive || result.State != "degraded" {
		t.Fatalf("cached snapshots marked live: %#v", result)
	}
	if result.Health["live_stock"].State != market.HealthCached || result.Health["live_etf"].State != market.HealthCached {
		t.Fatalf("unexpected health: %#v", result.Health)
	}
}

func TestRefreshLiveMarketTreatsEmptySourceRowsAsUnavailable(t *testing.T) {
	svc := openExecutionService(t)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, shanghai) }
	svc.SetMarketRankingFallback(fakeMarketProvider{})

	result, err := svc.RefreshLiveMarket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.IsLive || result.State != "unavailable" {
		t.Fatalf("empty source result=%#v", result)
	}
}

type rankingProviderFunc func(context.Context, market.RankingKind) ([]market.Quote, error)

func (f rankingProviderFunc) FetchRankings(ctx context.Context, kind market.RankingKind) ([]market.Quote, error) {
	return f(ctx, kind)
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

func (p fakeMarketProvider) FetchQuotes(_ context.Context, keys []market.InstrumentKey) ([]market.Quote, error) {
	if p.seenKeys != nil {
		*p.seenKeys = append(*p.seenKeys, keys...)
	}
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
	if result.Errors["quotes"] != "持仓参考报价暂未更新（东方财富公开接口临时不可用；已保留最近一次成功报价）" || len(result.Overview.AShareTurnover) != 1 {
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
