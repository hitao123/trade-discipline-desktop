package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTencentQuoteProviderNormalizesAShareAndHongKongQuotes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "sh600000,hk00700" {
			t.Fatalf("symbols=%q", got)
		}
		rows := []string{
			`v_sh600000="1~浦发银行~600000~10.25~~~~~~~~~~~~~~~~~~~~~~~~~~20260821145930";`,
			`v_hk00700="100~腾讯控股~00700~480.20~~~~~~~~~~~~~~~~~~~~~~~~~~20260821145931";`,
		}
		_, _ = w.Write([]byte(strings.Join(rows, "\n")))
	}))
	defer server.Close()

	quotes, err := (TencentQuoteProvider{BaseURL: server.URL, Client: server.Client()}).FetchQuotes(context.Background(), []InstrumentKey{{Market: "SH", Code: "600000"}, {Market: "HK", Code: "0700.HK"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 2 {
		t.Fatalf("quotes=%#v", quotes)
	}
	if quotes[0].Market != "SH" || quotes[0].Code != "600000" || quotes[0].CloseMinor != 1_025 || quotes[0].Source != "tencent-public-quote" {
		t.Fatalf("A-share quote=%#v", quotes[0])
	}
	if quotes[1].Market != "HK" || quotes[1].Code != "0700.HK" || quotes[1].CloseMinor != 48_020 {
		t.Fatalf("HK quote=%#v", quotes[1])
	}
	wantTime := time.Date(2026, 8, 21, 14, 59, 30, 0, time.FixedZone("CST", 8*60*60)).UTC()
	if quotes[0].TradeDate != "2026-08-21" || !quotes[0].SourceTime.Equal(wantTime) {
		t.Fatalf("source metadata=%#v want=%s", quotes[0], wantTime)
	}
}

func TestTencentQuoteProviderDecodesGB18030SecurityNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=GBK")
		body := append([]byte(`v_sz159361="1~A500ETF`), []byte{0xd2, 0xd7, 0xb7, 0xbd, 0xb4, 0xef}...)
		body = append(body, []byte(`~159361~1.22~~~~~~~~~~~~~~~~~~~~~~~~~~20260824134633";`)...)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	quotes, err := (TencentQuoteProvider{BaseURL: server.URL, Client: server.Client()}).FetchQuotes(context.Background(), []InstrumentKey{{Market: "SZ", Code: "159361"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].Name != "A500ETF易方达" {
		t.Fatalf("quote name=%q", quotes[0].Name)
	}
}
