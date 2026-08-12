package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchQuotesNormalizesHongKongAndASharePriceScales(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("secids"); !strings.Contains(got, "116.00700") || !strings.Contains(got, "1.600000") {
			t.Fatalf("unexpected secids %q", got)
		}
		_, _ = w.Write([]byte(`{"data":{"diff":[{"f2":461600,"f3":-195,"f6":12679743488,"f12":"00700","f13":116,"f14":"腾讯控股","f124":1786522086},{"f2":917,"f3":-43,"f6":429212224,"f12":"600000","f13":1,"f14":"浦发银行","f124":1786522301}]}}`))
	}))
	defer server.Close()
	provider := EastmoneyProvider{QuoteURL: server.URL, Client: server.Client()}
	quotes, err := provider.FetchQuotes(context.Background(), []InstrumentKey{{Market: "HK", Code: "0700.HK"}, {Market: "SH", Code: "600000"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 2 || quotes[0].Code != "0700.HK" || quotes[0].CloseMinor != 46_160 || quotes[1].CloseMinor != 917 {
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

func TestNormalizeETFTruncatesToTen(t *testing.T) {
	rows := make([]eastmoneyRow, 12)
	for index := range rows {
		rows[index] = eastmoneyRow{Price: 1, Turnover: float64(index + 1), Code: "510000", Market: 1, Name: "ETF", QuoteUnix: 1786492800}
	}
	got := normalizeRanking(rows, KindETF, 10)
	if len(got) != 10 || got[0].TurnoverFen != 1_200 {
		t.Fatalf("unexpected ETF ranking: %#v", got)
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
