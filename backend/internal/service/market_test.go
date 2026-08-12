package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

type fakeMarketProvider struct {
	stock    []market.Quote
	etf      []market.Quote
	quotes   []market.Quote
	stockErr error
	etfErr   error
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
	return p.quotes, nil
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
