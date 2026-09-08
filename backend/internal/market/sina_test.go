package market

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSinaProviderFetchesRankingsWithExactTradingTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			_, _ = w.Write([]byte(`var hq_str_sh000001="上证指数,3891.1751,3903.7210,3905.2026,3912.1314,3883.7870,0,0,446895868,883423480099,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:43:32,00";`))
		case "/ranking":
			if r.URL.Query().Get("node") != "hs_a" || r.URL.Query().Get("sort") != "amount" {
				t.Fatalf("unexpected ranking query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"symbol":"sz300308","code":"300308","name":"中际旭创","trade":"943.000","changepercent":4.291,"amount":26686400633,"ticktime":"15:43:20"},{"symbol":"sh688825","code":"688825","name":"长鑫科技","trade":"58.000","changepercent":0.747,"amount":16107953634,"ticktime":"15:34:58"}]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := SinaProvider{RankingURL: server.URL + "/ranking", IndexURL: server.URL + "/index", Client: server.Client()}
	quotes, err := provider.FetchRankings(context.Background(), KindStock)
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 2 || quotes[0].Code != "300308" || quotes[0].Market != "SZ" || quotes[0].CloseMinor != 94_300 || quotes[0].TurnoverFen != 2_668_640_063_300 {
		t.Fatalf("unexpected quotes: %#v", quotes)
	}
	if quotes[0].TradeDate != "2026-08-21" || quotes[0].SourceTime.Format(time.RFC3339) != "2026-08-21T07:43:20Z" || quotes[0].Source != "sina-public-ranking" {
		t.Fatalf("unexpected source metadata: %#v", quotes[0])
	}
}

func TestSinaProviderUsesETFNodeAndReturnsVisibleETF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			_, _ = w.Write([]byte(`var hq_str_sh000001="上证指数,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:43:32,00";`))
		case "/ranking":
			if r.URL.Query().Get("node") != "etf_hq_fund" {
				t.Fatalf("expected ETF node, got %s", r.URL.Query().Get("node"))
			}
			_, _ = w.Write([]byte(`[{"symbol":"sh515880","code":"515880","name":"通信ETF国泰","trade":"0.643","changepercent":1.2,"amount":27635286184,"ticktime":"15:34:58"}]`))
		}
	}))
	defer server.Close()

	quotes, err := (SinaProvider{RankingURL: server.URL + "/ranking", IndexURL: server.URL + "/index", Client: server.Client()}).FetchRankings(context.Background(), KindETF)
	if err != nil || len(quotes) != 1 || quotes[0].AssetType != KindETF || quotes[0].Market != "SH" {
		t.Fatalf("quotes=%#v err=%v", quotes, err)
	}
}

func TestSinaProviderAcceptsIndexPayloadWithTrailingComma(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			_, _ = w.Write([]byte(`var hq_str_sh000001="上证指数,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:43:32,00,";`))
		case "/ranking":
			_, _ = w.Write([]byte(`[{"symbol":"sh600001","code":"600001","name":"示例股票","trade":"10.00","changepercent":1.0,"amount":1000000,"ticktime":"15:00:00"}]`))
		}
	}))
	defer server.Close()

	quotes, err := (SinaProvider{RankingURL: server.URL + "/ranking", IndexURL: server.URL + "/index", Client: server.Client()}).FetchRankings(context.Background(), KindStock)
	if err != nil || len(quotes) != 1 || quotes[0].TradeDate != "2026-08-21" || quotes[0].SourceTime.Format(time.RFC3339) != "2026-08-21T07:00:00Z" {
		t.Fatalf("quotes=%#v err=%v", quotes, err)
	}
}

func TestSinaProviderDoesNotSubstituteIndexTimeForMissingOrInvalidTicktime(t *testing.T) {
	for _, ticktime := range []string{"", "not-a-time"} {
		t.Run(fmt.Sprintf("ticktime=%q", ticktime), func(t *testing.T) {
			rows := make([]string, 0, StockRankingLimit)
			for index := 0; index < StockRankingLimit; index++ {
				rowTicktime := "15:20:00"
				if index == 10 {
					rowTicktime = ticktime
				}
				rows = append(rows, fmt.Sprintf(`{"symbol":"sh%06d","code":"%06d","name":"样本%d","trade":"10.00","changepercent":1.0,"amount":%d,"ticktime":"%s"}`, 600000+index, 600000+index, index, 20-index, rowTicktime))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index":
					_, _ = w.Write([]byte(`var hq_str_sh000001="上证指数,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:43:32,00";`))
				case "/ranking":
					_, _ = w.Write([]byte("[" + strings.Join(rows, ",") + "]"))
				}
			}))
			defer server.Close()

			quotes, err := (SinaProvider{RankingURL: server.URL + "/ranking", IndexURL: server.URL + "/index", Client: server.Client()}).FetchRankings(context.Background(), KindStock)
			if err != nil {
				t.Fatal(err)
			}
			for _, quote := range quotes {
				if quote.Code == "600010" && !quote.SourceTime.IsZero() {
					t.Fatalf("missing ticktime inherited source time %s", quote.SourceTime)
				}
			}
			quality := AssessCloseRanking(time.Date(2026, 8, 21, 15, 30, 0, 0, time.FixedZone("CST", 8*3600)), KindStock, quotes)
			if quality.Quality != RankingQualityIncomplete || quality.Reason != "来源时间不能证明收盘" {
				t.Fatalf("quality=%#v", quality)
			}
		})
	}
}

func TestSinaProviderFetchesAShareTurnoverMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/indices" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("var hq_str_sh000001=\"上证指数,0,0,0,0,0,0,0,0,883423480099,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:43:32,00,\";\nvar hq_str_sz399106=\"深证成指,0,0,0,0,0,0,0,0,995840925246.299,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:00:03,00\";"))
	}))
	defer server.Close()

	batch := (SinaProvider{IndexMetricsURL: server.URL + "/indices", Client: server.Client()}).FetchAShareTurnoverMetrics(context.Background())
	if len(batch.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", batch.Errors)
	}
	values := make(map[MetricKind]int64)
	for _, point := range batch.Points {
		values[point.Metric] = point.ValueFen
	}
	if values[MetricSHTurnover] != 88_342_348_009_900 || values[MetricSZTurnover] != 99_584_092_524_630 || values[MetricAShareTurnover] != 187_926_440_534_530 {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestIsAShareTradingSession(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	if !IsAShareTradingSession(time.Date(2026, 8, 21, 10, 0, 0, 0, location)) {
		t.Fatal("expected Friday morning session to be open")
	}
	if IsAShareTradingSession(time.Date(2026, 8, 21, 12, 0, 0, 0, location)) || IsAShareTradingSession(time.Date(2026, 8, 22, 10, 0, 0, 0, location)) {
		t.Fatal("expected noon and Saturday to be closed")
	}
}
