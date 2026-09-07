package service

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func TestResolveInstrumentKeyAcceptsHongKongAliases(t *testing.T) {
	for _, raw := range []string{"0700", "00700", "0700.HK", "hk0700"} {
		key, kind, err := resolveInstrumentKey("", raw)
		if err != nil || key.Market != "HK" || key.Code != "0700.HK" || kind != market.KindStock {
			t.Fatalf("raw=%q key=%#v kind=%s err=%v", raw, key, kind, err)
		}
	}
}

func TestRegisterLocalHongKongInstrument(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	item, err := svc.RegisterInstrument(context.Background(), RegisterInstrumentInput{
		Market: "HK", Code: "00700", Name: "腾讯控股", LocalOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Market != "HK" || item.Code != "0700.HK" || item.Currency != "HKD" || item.LotSize != 100 || item.Name != "腾讯控股" {
		t.Fatalf("instrument=%#v", item)
	}
	again, err := svc.RegisterInstrument(context.Background(), RegisterInstrumentInput{
		Market: "HK", Code: "0700", Name: "腾讯控股", LocalOnly: true,
	})
	if err != nil || again.ID != item.ID {
		t.Fatalf("expected idempotent register: %#v err=%v", again, err)
	}
}

func TestResolveHongKongInstrumentFromQuote(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{{
		TradeDate: "2026-08-25", Market: "HK", Code: "0700.HK", Name: "腾讯控股",
		AssetType: market.KindStock, CloseMinor: 48_020, Source: "fixture", SourceTime: time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC),
	}}})
	item, err := svc.ResolveInstrument(context.Background(), "00700", "HK")
	if err != nil {
		t.Fatal(err)
	}
	if item.Code != "0700.HK" || item.Currency != "HKD" || item.Name != "腾讯控股" {
		t.Fatalf("instrument=%#v", item)
	}
}
