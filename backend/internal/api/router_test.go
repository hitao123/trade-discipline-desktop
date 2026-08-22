package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/service"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func newAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC) }
	server := httptest.NewServer(NewRouter(service.New(db, now), "secret"))
	t.Cleanup(server.Close)
	return server
}

type apiMarketProvider struct {
	daily []market.DailyBar
}

func (p apiMarketProvider) FetchRankings(_ context.Context, _ market.RankingKind) ([]market.Quote, error) {
	return nil, nil
}

func (p apiMarketProvider) FetchQuotes(_ context.Context, _ []market.InstrumentKey) ([]market.Quote, error) {
	return nil, nil
}

func (p apiMarketProvider) FetchDailyBars(_ context.Context, _ market.InstrumentKey, _ int) ([]market.DailyBar, error) {
	return p.daily, nil
}

func (p apiMarketProvider) FetchMarketMetrics(_ context.Context, _ int) market.MetricBatch {
	return market.MetricBatch{}
}

func newMarketAPIServer(t *testing.T, provider market.Provider) (*httptest.Server, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC) }
	svc := service.New(db, now)
	svc.SetMarketProvider(provider)
	server := httptest.NewServer(NewRouter(svc, "secret"))
	t.Cleanup(server.Close)
	return server, db
}

func authorizedRequest(t *testing.T, method, endpoint string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestAllocationAPITracksGoalAndAppendsManualValues(t *testing.T) {
	server := newAPIServer(t)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/allocation", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("allocation status=%d", response.StatusCode)
	}
	var initial struct {
		OK   bool                       `json:"ok"`
		Data service.AllocationOverview `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	if !initial.OK || initial.Data.CurrentTotalFen != 20_000_000 || initial.Data.TargetTotalFen != 22_000_000 || initial.Data.ReturnBP != 0 {
		t.Fatalf("unexpected allocation overview: %#v", initial.Data)
	}

	recorded, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/allocation/items/semiconductor-equipment-etf-159558/value-events", []byte(`{"valueFen":1250000,"observedAt":"2026-08-12T08:00:00Z"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer recorded.Body.Close()
	if recorded.StatusCode != http.StatusCreated {
		t.Fatalf("record value status=%d", recorded.StatusCode)
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/allocation", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var updated struct {
		Data service.AllocationOverview `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.CurrentTotalFen != 20_250_000 || updated.Data.ReturnBP != 125 || updated.Data.GoalGapFen != 1_750_000 {
		t.Fatalf("unexpected updated allocation overview: %#v", updated.Data)
	}
}

func TestAllocationAPIRequiresRevisionReason(t *testing.T) {
	server := newAPIServer(t)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/allocation", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var current struct {
		Data service.AllocationOverview `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(struct {
		Reason string                 `json:"reason"`
		Draft  domain.AllocationDraft `json:"draft"`
	}{Draft: current.Data.Version.Draft})
	if err != nil {
		t.Fatal(err)
	}
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPut, server.URL+"/api/allocation", body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing reason status=%d", response.StatusCode)
	}
}

func TestMarketHistoryAPIRefreshesAndReadsCache(t *testing.T) {
	provider := apiMarketProvider{daily: []market.DailyBar{{
		TradeDate: "2026-08-11", Market: "SH", Code: "600001", CloseMinor: 1_020,
		TurnoverFen: 8_000_000_000, Source: "fixture", SourceTime: time.Date(2026, 8, 11, 7, 0, 0, 0, time.UTC),
	}}}
	server, _ := newMarketAPIServer(t, provider)
	client := server.Client()

	response, err := client.Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/market/history/refresh", []byte(`{"market":"SH","code":"600001"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("refresh status=%d", response.StatusCode)
	}
	var refreshed struct {
		OK   bool                        `json:"ok"`
		Data service.MarketHistoryResult `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&refreshed); err != nil {
		t.Fatal(err)
	}
	if !refreshed.OK || len(refreshed.Data.Points) != 1 || refreshed.Data.Points[0].CloseMinor != 1_020 {
		t.Fatalf("unexpected refresh: %#v", refreshed)
	}

	response, err = client.Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/market/history?market=SH&code=600001&range=1m", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("history status=%d", response.StatusCode)
	}
	var cached struct {
		OK   bool                        `json:"ok"`
		Data service.MarketHistoryResult `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&cached); err != nil || len(cached.Data.Points) != 1 || cached.Data.Range != "1m" {
		t.Fatalf("unexpected cache: %#v err=%v", cached, err)
	}
}

func TestMarketHistoryAPIRejectsUnknownRefreshFields(t *testing.T) {
	server, _ := newMarketAPIServer(t, apiMarketProvider{})
	response, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/market/history/refresh", []byte(`{"market":"SH","code":"600001","extra":true}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestMarketOverviewAPIReadsCacheAndRejectsInvalidRange(t *testing.T) {
	server, db := newMarketAPIServer(t, apiMarketProvider{})
	_, err := db.SaveMetricPoints(context.Background(), []market.MetricPoint{
		{TradeDate: "2026-08-11", Metric: market.MetricAShareTurnover, ValueFen: 150_000_000_000_000, Source: "fixture"},
		{TradeDate: "2026-08-11", Metric: market.MetricSouthboundNetBuy, ValueFen: -7_000_000_000, Source: "fixture"},
	}, time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/market/overview?range=3m", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("overview status=%d", response.StatusCode)
	}
	var envelope struct {
		OK   bool                         `json:"ok"`
		Data service.MarketOverviewResult `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || len(envelope.Data.AShareTurnover) != 1 || len(envelope.Data.SouthboundNetBuy) != 1 {
		t.Fatalf("unexpected overview: %#v err=%v", envelope, err)
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/market/overview?range=6m", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid range status=%d", response.StatusCode)
	}
}

func TestMarketRefreshStatusAPIReadsBothModes(t *testing.T) {
	server, db := newMarketAPIServer(t, apiMarketProvider{})
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	if _, err := db.SaveMarketRefreshStatus(context.Background(), market.SnapshotModeClose, now, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveMarketRefreshStatus(context.Background(), market.SnapshotModeLive, now, false, map[string]string{"stock": "market closed"}); err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/market/refresh-status", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	var envelope struct {
		OK   bool                                    `json:"ok"`
		Data map[string]store.MarketRefreshStatusRow `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || !envelope.OK || envelope.Data["close"].LastSuccessfulAt == nil || envelope.Data["live"].Errors["stock"] != "market closed" {
		t.Fatalf("refresh status=%#v err=%v", envelope, err)
	}
}

func TestCreateRejectedPlanThroughAPI(t *testing.T) {
	server := newAPIServer(t)
	draft := domain.TradePlanDraft{
		InstrumentID: "hk-9988", Thesis: "云业务利润率改善", Falsification: "云收入降速", PricedExpectation: "温和复苏",
		Evidence: []string{"下季云收入"}, BreakCondition: "同比转负", ExitCondition: "逻辑破坏",
		EntryLowMinor: 11_000, EntryHighMinor: 12_000, RiskExitMinor: 9_000, Quantity: 150,
		EstimatedCostFen: 1_800_000, MaxPlanLossFen: 540_000, StressDropBP: 3_000,
		ReferencePriceAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC), ValidUntil: time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC),
	}
	body, _ := json.Marshal(draft)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/plans", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", res.StatusCode)
	}
	var envelope struct {
		OK   bool         `json:"ok"`
		Data service.Plan `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Status != "rejected" || envelope.Data.Validation.Findings[0].Code != "BOARD_LOT_REQUIRED" {
		t.Fatalf("unexpected response: %#v", envelope)
	}
}

func TestPreTradeConfirmationAPIRequiresAttestations(t *testing.T) {
	server := newAPIServer(t)
	planBody := []byte(`{"instrumentId":"hk-9988","thesis":"云业务利润率改善","falsification":"云收入降速","pricedExpectation":"温和复苏","evidence":["下季云收入"],"breakCondition":"云收入同比转负","exitCondition":"逻辑破坏","entryLowMinor":11000,"entryHighMinor":12000,"riskExitMinor":9000,"quantity":100,"estimatedCostFen":1200000,"maxPlanLossFen":360000,"stressDropBP":3000,"referencePriceAt":"2026-08-12T07:00:00Z","validUntil":"2026-08-19T08:00:00Z"}`)
	created, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/plans", planBody))
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	var plan struct {
		Data service.Plan `json:"data"`
	}
	if err := json.NewDecoder(created.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	missing := []byte(`{"startedAt":"2026-08-12T07:59:30Z","noFomo":false,"noLossRecovery":true,"noAveragingDown":true}`)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/plans/"+plan.Data.ID+"/pre-trade-confirmations", missing))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("attestation response=%d", response.StatusCode)
	}
	valid := []byte(`{"startedAt":"2026-08-12T07:59:30Z","noFomo":true,"noLossRecovery":true,"noAveragingDown":true}`)
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/plans/"+plan.Data.ID+"/pre-trade-confirmations", valid))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("confirmation response=%d", response.StatusCode)
	}
}

func TestMonitorSettingsAPIReadsDefaultAndSavesInterval(t *testing.T) {
	server := newAPIServer(t)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/monitor/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("default settings status=%d", response.StatusCode)
	}
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPut, server.URL+"/api/monitor/settings", []byte(`{"interval":"15m"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save settings status=%d", response.StatusCode)
	}
}
