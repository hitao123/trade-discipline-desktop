package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchDailyBarsNormalizesPriceAndTurnover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "1.600000" || r.URL.Query().Get("klt") != "101" || r.URL.Query().Get("lmt") != "120" {
			t.Fatalf("unexpected history query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Query().Get("fields2"), "f57") {
			t.Fatalf("history query must request turnover amount f57: %s", r.URL.RawQuery)
		}
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Referer") == "" {
			t.Fatalf("missing source headers: %#v", r.Header)
		}
		_, _ = w.Write([]byte(`{"data":{"code":"600000","market":1,"name":"浦发银行","klines":["2026-08-11,10.00,10.25,10.40,9.95,12345,1250000000"]}}`))
	}))
	defer server.Close()
	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		HistoryURL: server.URL,
		Client:     server.Client(),
		Now:        func() time.Time { return time.Date(2026, 8, 12, 16, 30, 0, 0, location) },
	}

	bars, err := provider.FetchDailyBars(context.Background(), InstrumentKey{Market: "SH", Code: "600000"}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].TradeDate != "2026-08-11" || bars[0].CloseMinor != 1_025 || bars[0].TurnoverFen != 125_000_000_000 {
		t.Fatalf("unexpected bars: %#v", bars)
	}
	if bars[0].Source != "eastmoney-public-history" || bars[0].SourceTime.IsZero() {
		t.Fatalf("missing source metadata: %#v", bars[0])
	}
}

func TestFetchDailyBarsRetriesWhenSourceClosesConnectionWithEOF(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support connection hijacking")
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack test connection: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = w.Write([]byte(`{"data":{"klines":["2026-08-11,10.00,10.25,10.40,9.95,12345,1250000000"]}}`))
	}))
	defer server.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		HistoryURL: server.URL,
		Client:     server.Client(),
		Now:        func() time.Time { return time.Date(2026, 8, 12, 16, 30, 0, 0, location) },
	}

	bars, err := provider.FetchDailyBars(context.Background(), InstrumentKey{Market: "SZ", Code: "300502"}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || len(bars) != 1 || bars[0].Code != "300502" {
		t.Fatalf("attempts=%d bars=%#v", attempts.Load(), bars)
	}
}

func TestFetchDailyBarsStopsAfterThreeTransientEOFs(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support connection hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack test connection: %v", err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()

	provider := EastmoneyProvider{HistoryURL: server.URL, Client: server.Client()}
	_, err := provider.FetchDailyBars(context.Background(), InstrumentKey{Market: "SZ", Code: "300502"}, 120)
	if err == nil {
		t.Fatal("expected persistent EOF failure")
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts=%d want 3", attempts.Load())
	}
	if !strings.Contains(err.Error(), "连续 3 次") {
		t.Fatalf("persistent EOF error is not actionable: %v", err)
	}
}

func TestFetchDailyBarsFallsBackToTencentAfterEastmoneyEOFs(t *testing.T) {
	var primaryAttempts atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryAttempts.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack primary connection: %v", err)
			return
		}
		_ = connection.Close()
	}))
	defer primary.Close()

	var fallbackAttempts atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackAttempts.Add(1)
		if r.URL.Query().Get("param") != "sz300502,day,,,120" {
			t.Errorf("unexpected fallback query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"sz300502":{"day":[{"meta":true},["2026-08-11",10.00,10.25,10.40,9.95,12345]]}}}`))
	}))
	defer fallback.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		HistoryURL:         primary.URL,
		FallbackHistoryURL: fallback.URL,
		Client:             primary.Client(),
		Now:                func() time.Time { return time.Date(2026, 8, 12, 16, 30, 0, 0, location) },
	}

	bars, err := provider.FetchDailyBars(context.Background(), InstrumentKey{Market: "SZ", Code: "300502"}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if primaryAttempts.Load() != 3 || fallbackAttempts.Load() != 1 {
		t.Fatalf("primary attempts=%d fallback attempts=%d", primaryAttempts.Load(), fallbackAttempts.Load())
	}
	if len(bars) != 1 || bars[0].CloseMinor != 1_025 || bars[0].TurnoverFen != 0 || bars[0].Source != "tencent-public-history" {
		t.Fatalf("unexpected fallback bars: %#v", bars)
	}
}

func TestFetchMarketMetricsCombinesShanghaiShenzhenAndSouthboundChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("secid") == "1.000001":
			_, _ = w.Write([]byte(`{"data":{"klines":["2026-08-11,0,0,0,0,0,700000000000"]}}`))
		case query.Get("secid") == "0.399106":
			_, _ = w.Write([]byte(`{"data":{"klines":["2026-08-11,0,0,0,0,0,800000000000"]}}`))
		case strings.Contains(query.Get("filter"), `MUTUAL_TYPE="002"`):
			_, _ = w.Write([]byte(`{"result":{"data":[{"TRADE_DATE":"2026-08-11 00:00:00","NET_DEAL_AMT":-100}]}}`))
		case strings.Contains(query.Get("filter"), `MUTUAL_TYPE="004"`):
			_, _ = w.Write([]byte(`{"result":{"data":[{"TRADE_DATE":"2026-08-11 00:00:00","NET_DEAL_AMT":30}]}}`))
		default:
			t.Fatalf("unexpected metrics query: %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()
	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		HistoryURL:    server.URL,
		DataCenterURL: server.URL,
		Client:        server.Client(),
		Now:           func() time.Time { return time.Date(2026, 8, 12, 16, 30, 0, 0, location) },
	}

	batch := provider.FetchMarketMetrics(context.Background(), 120)
	if len(batch.Errors) != 0 {
		t.Fatalf("unexpected metric errors: %#v", batch.Errors)
	}
	values := make(map[MetricKind]int64)
	for _, point := range batch.Points {
		if point.TradeDate != "2026-08-11" {
			t.Fatalf("unexpected metric date: %#v", point)
		}
		values[point.Metric] = point.ValueFen
	}
	want := map[MetricKind]int64{
		MetricSHTurnover:         70_000_000_000_000,
		MetricSZTurnover:         80_000_000_000_000,
		MetricAShareTurnover:     150_000_000_000_000,
		MetricSouthboundSHNetBuy: -10_000_000_000,
		MetricSouthboundSZNetBuy: 3_000_000_000,
		MetricSouthboundNetBuy:   -7_000_000_000,
	}
	if len(values) != len(want) {
		t.Fatalf("unexpected metrics: %#v", batch.Points)
	}
	for metric, expected := range want {
		if values[metric] != expected {
			t.Fatalf("metric %s=%d want %d", metric, values[metric], expected)
		}
	}
}

func TestFetchMarketMetricsFallsBackToSinaIndexTurnover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sina" {
			_, _ = w.Write([]byte("var hq_str_sh000001=\"上证指数,0,0,0,0,0,0,0,0,700000000000,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:00:00,00,\";\nvar hq_str_sz399106=\"深证成指,0,0,0,0,0,0,0,0,800000000000,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2026-08-21,15:00:00,00\";"))
			return
		}
		if strings.Contains(r.URL.Query().Get("filter"), `MUTUAL_TYPE="002"`) || strings.Contains(r.URL.Query().Get("filter"), `MUTUAL_TYPE="004"`) {
			_, _ = w.Write([]byte(`{"result":{"data":[{"TRADE_DATE":"2026-08-21 00:00:00","NET_DEAL_AMT":0}]}}`))
			return
		}
		w.Header().Set("Connection", "close")
	}))
	defer server.Close()
	provider := EastmoneyProvider{HistoryURL: server.URL, DataCenterURL: server.URL, SinaMetricsURL: server.URL + "/sina", Client: server.Client()}

	batch := provider.FetchMarketMetrics(context.Background(), 120)
	if batch.Errors[MetricSHTurnover] != "" || batch.Errors[MetricSZTurnover] != "" || batch.Errors[MetricAShareTurnover] != "" {
		t.Fatalf("turnover fallback errors: %#v", batch.Errors)
	}
	values := make(map[MetricKind]int64)
	for _, point := range batch.Points {
		values[point.Metric] = point.ValueFen
	}
	if values[MetricAShareTurnover] != 150_000_000_000_000 {
		t.Fatalf("A-share turnover=%d points=%#v", values[MetricAShareTurnover], batch.Points)
	}
}

func TestCombineMetricPointsPreservesDerivedSourceProvenance(t *testing.T) {
	tradeDate := "2026-08-21"
	left := []MetricPoint{{TradeDate: tradeDate, Source: "sina-public-metrics", ValueFen: 1}}
	right := []MetricPoint{{TradeDate: tradeDate, Source: "sina-public-metrics", ValueFen: 2}}

	combined := combineMetricPoints(left, right, MetricAShareTurnover)
	if len(combined) != 1 || combined[0].Source != "sina-public-derived" {
		t.Fatalf("same-source derived provenance=%#v", combined)
	}

	right[0].Source = "eastmoney-public-history"
	combined = combineMetricPoints(left, right, MetricAShareTurnover)
	if len(combined) != 1 || combined[0].Source != "mixed-public-derived" {
		t.Fatalf("mixed-source derived provenance=%#v", combined)
	}
}

func TestFetchDailyBarsDropsCurrentAShareSessionBeforeClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"klines":["2026-08-11,0,10.00,0,0,0,1000000","2026-08-12,0,10.20,0,0,0,2000000"]}}`))
	}))
	defer server.Close()
	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		HistoryURL: server.URL,
		Client:     server.Client(),
		Now:        func() time.Time { return time.Date(2026, 8, 12, 14, 59, 0, 0, location) },
	}

	bars, err := provider.FetchDailyBars(context.Background(), InstrumentKey{Market: "SH", Code: "600000"}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].TradeDate != "2026-08-11" {
		t.Fatalf("incomplete session leaked: %#v", bars)
	}
}

func TestFetchSouthboundDropsCurrentSessionBeforeFinalClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"data":[{"TRADE_DATE":"2026-08-12 00:00:00","NET_DEAL_AMT":20000000},{"TRADE_DATE":"2026-08-11 00:00:00","NET_DEAL_AMT":10000000}]}}`))
	}))
	defer server.Close()
	location, _ := time.LoadLocation("Asia/Shanghai")
	provider := EastmoneyProvider{
		DataCenterURL: server.URL,
		Client:        server.Client(),
		Now:           func() time.Time { return time.Date(2026, 8, 12, 16, 10, 0, 0, location) },
	}

	points, err := provider.fetchSouthboundChannel(context.Background(), "002", MetricSouthboundSHNetBuy, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].TradeDate != "2026-08-11" {
		t.Fatalf("incomplete southbound session leaked: %#v", points)
	}
}
