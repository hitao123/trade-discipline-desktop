package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchQuotesUsesDecimalPricesForHongKongAndAShare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("secids"); !strings.Contains(got, "116.00700") || !strings.Contains(got, "1.515880") {
			t.Fatalf("unexpected secids %q", got)
		}
		if query.Get("fltt") != "2" || query.Get("invt") != "2" {
			t.Fatalf("decimal quote parameters missing: %s", query.Encode())
		}
		_, _ = w.Write([]byte(`{"data":{"diff":[{"f2":461.6,"f3":-1.95,"f6":12679743488,"f12":"00700","f13":116,"f14":"腾讯控股","f124":1786522086},{"f2":0.643,"f3":1.2,"f6":429212224,"f12":"515880","f13":1,"f14":"通信ETF国泰","f124":1786522301}]}}`))
	}))
	defer server.Close()
	provider := EastmoneyProvider{QuoteURL: server.URL, Client: server.Client()}
	quotes, err := provider.FetchQuotes(context.Background(), []InstrumentKey{{Market: "HK", Code: "0700.HK"}, {Market: "SH", Code: "515880"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 2 || quotes[0].Code != "0700.HK" || quotes[0].CloseMinor != 46_160 || quotes[1].CloseMinor != 64 {
		t.Fatalf("unexpected quotes: %#v", quotes)
	}
}

func TestNormalizeStocksSortsByTurnoverAndExcludesST(t *testing.T) {
	rows := []eastmoneyRow{
		{Price: 10.2, ChangePct: 1.2, Turnover: 1_000_000, Code: "600001", Market: 1, Name: "普通一", QuoteUnix: 1786492800},
		{Price: 8.8, ChangePct: -0.3, Turnover: 9_000_000, Code: "000001", Market: 0, Name: "普通二", QuoteUnix: 1786492800},
		{Price: 3.2, ChangePct: 4.0, Turnover: 99_000_000, Code: "600002", Market: 1, Name: "*ST示例", QuoteUnix: 1786492800},
	}
	got := normalizeRanking(rows, KindStock, 20)
	if len(got) != 2 || got[0].Code != "000001" || got[0].TurnoverFen < got[1].TurnoverFen {
		t.Fatalf("unexpected ranking: %#v", got)
	}
	for _, quote := range got {
		if strings.Contains(quote.Name, "ST") {
			t.Fatalf("ST leaked: %#v", quote)
		}
	}
}

func TestFetchETFTopTwentyFiltersCashAndBondProductsBeforeTruncating(t *testing.T) {
	if IsEligibleETF("511880", "银华日利ETF") || IsEligibleETF("511990", "华宝添益ETF") {
		t.Fatal("cash-management ETF codes must be rejected even when the public name omits 货币")
	}
	if IsEligibleETFName("公司债ETF南方") {
		t.Fatal("bond ETF must be rejected")
	}
	if !IsEligibleETFName("通信ETF国泰") {
		t.Fatal("equity ETF must remain eligible")
	}
	rows := []eastmoneyWireRow{
		{Price: 100, Turnover: 130, Code: "511880", Market: 1, Name: "银华日利ETF", QuoteUnix: 1786492800},
		{Price: 100, Turnover: 125, Code: "511990", Market: 1, Name: "华宝添益ETF", QuoteUnix: 1786492800},
		{Price: 100, Turnover: 120, Code: "511360", Market: 1, Name: "短融ETF海富通", QuoteUnix: 1786492800},
		{Price: 100, Turnover: 110, Code: "159115", Market: 0, Name: "科创债", QuoteUnix: 1786492800},
	}
	for index := 0; index < 22; index++ {
		rows = append(rows, eastmoneyWireRow{
			Price: 1, Turnover: flexibleFloat(100 - index), Code: fmt.Sprintf("15%04d", index), Market: 0,
			Name: fmt.Sprintf("权益ETF%02d", index), QuoteUnix: 1786492800,
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Data struct {
				Diff []eastmoneyWireRow `json:"diff"`
			} `json:"data"`
		}{Data: struct {
			Diff []eastmoneyWireRow `json:"diff"`
		}{Diff: rows}})
	}))
	defer server.Close()

	got, err := (&EastmoneyProvider{BaseURL: server.URL, Client: server.Client()}).FetchRankings(context.Background(), KindETF)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 20 || got[0].Name != "权益ETF00" || got[19].Name != "权益ETF19" {
		t.Fatalf("unexpected ETF ranking: %#v", got)
	}
	for _, quote := range got {
		if strings.Contains(quote.Name, "债") || strings.Contains(quote.Name, "货币") || strings.Contains(quote.Name, "短融") {
			t.Fatalf("fixed-income ETF leaked: %#v", quote)
		}
	}
}

func TestPreviewCSVHandlesBOMDuplicateAndBadRowWithoutWriting(t *testing.T) {
	content := "\ufefftrade_date,market,code,name,asset_type,close,change_pct,turnover\n" +
		"2026-08-11,SH,510300,沪深300ETF,etf,4.20,0.15,1000000\n" +
		"2026-08-11,SH,510300,沪深300ETF,etf,4.20,0.15,1000000\n" +
		"2026-08-11,HK,0700,腾讯,stock,480,1.0,2000000\n"
	got := PreviewCSV(strings.NewReader(content))
	if len(got.Valid) != 1 || len(got.Duplicates) != 1 || len(got.Errors) != 1 {
		t.Fatalf("unexpected preview: %#v", got)
	}
	if got.Valid[0].CloseMinor != 420 || got.Valid[0].TurnoverFen != 100_000_000 {
		t.Fatalf("unexpected money conversion: %#v", got.Valid[0])
	}
}
