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
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
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
	seedLegacyAPITestData(t, db)
	now := func() time.Time { return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC) }
	server := httptest.NewServer(NewRouter(service.New(db, now), "secret"))
	t.Cleanup(server.Close)
	return server
}

func seedLegacyAPITestData(t *testing.T, db *store.Store) {
	t.Helper()
	stamp := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	raw, err := json.Marshal(rules.InitialSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE user_profiles SET mode='generic', onboarding_status='completed', investable_capital_fen=20000000, max_loss_fen=2000000, holding_horizon='6_to_12m', enabled_markets_json='["ashare_stock","ashare_etf","hk"]', completed_at=?, updated_at=? WHERE id='local-user'`, []any{stamp, stamp}},
		{`INSERT INTO accounts(id, name, base_currency, initial_capital_fen, enabled_at, status) VALUES('account-main','我的纪律账户','CNY',20000000,?,'active')`, []any{stamp}},
		{`INSERT INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id) VALUES('rule-1',1,?,'初始交易纪律规则',?,NULL)`, []any{string(raw), stamp}},
		{`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES('hk-0700','HK','0700.HK','腾讯控股','stock','HKD',100,'legacy_fixture',1,0,'active')`, nil},
		{`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES('hk-9988','HK','9988.HK','阿里巴巴-W','stock','HKD',100,'legacy_fixture',1,0,'active')`, nil},
	}
	for _, statement := range statements {
		if _, err := db.DB().Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.EnsureInitialAllocation(context.Background(), time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

func newFreshAPIServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC) }
	server := httptest.NewServer(NewRouter(service.New(db, now), "secret"))
	t.Cleanup(server.Close)
	return server, db
}

type apiMarketProvider struct {
	daily  []market.DailyBar
	quotes []market.Quote
}

func (p apiMarketProvider) FetchRankings(_ context.Context, _ market.RankingKind) ([]market.Quote, error) {
	return nil, nil
}

func (p apiMarketProvider) FetchQuotes(_ context.Context, _ []market.InstrumentKey) ([]market.Quote, error) {
	return p.quotes, nil
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
	seedLegacyAPITestData(t, db)
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

func TestOnboardingAPICompletesFreshAccountOnce(t *testing.T) {
	server, db := newFreshAPIServer(t)

	response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/onboarding", nil))
	if err != nil {
		t.Fatal(err)
	}
	var pending struct {
		Data domain.UserProfile `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || pending.Data.Mode != domain.UserModeGeneric || pending.Data.OnboardingStatus != domain.OnboardingPending {
		t.Fatalf("status=%d profile=%#v", response.StatusCode, pending.Data)
	}

	invalidBody := []byte(`{"investableCapitalFen":20000000,"maxLossFen":20000000,"holdingHorizon":"6_to_12m","enabledMarkets":["ashare_etf"]}`)
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/onboarding/complete", invalidBody))
	if err != nil {
		t.Fatal(err)
	}
	var invalid Envelope[any]
	if err := json.NewDecoder(response.Body).Decode(&invalid); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity || invalid.Error == nil || invalid.Error.Code != "INVALID_ONBOARDING" {
		t.Fatalf("status=%d body=%#v", response.StatusCode, invalid)
	}

	validBody := []byte(`{"investableCapitalFen":20000000,"maxLossFen":2000000,"holdingHorizon":"6_to_12m","enabledMarkets":["ashare_etf","hk"]}`)
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/onboarding/complete", validBody))
	if err != nil {
		t.Fatal(err)
	}
	var completed struct {
		Data domain.UserProfile `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&completed); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || completed.Data.OnboardingStatus != domain.OnboardingCompleted || completed.Data.MaxLossFen != 2_000_000 {
		t.Fatalf("status=%d profile=%#v", response.StatusCode, completed.Data)
	}
	for table, want := range map[string]int{"accounts": 1, "rule_versions": 1, "instruments": 0, "allocation_profiles": 0} {
		var got int
		if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s=%d want=%d", table, got, want)
		}
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/onboarding/complete", validBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var conflict Envelope[any]
	if err := json.NewDecoder(response.Body).Decode(&conflict); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || conflict.Error == nil || conflict.Error.Code != "ONBOARDING_ALREADY_COMPLETED" {
		t.Fatalf("status=%d body=%#v", response.StatusCode, conflict)
	}
}

func TestFreshWorkspaceListAPIsReturnEmptyArrays(t *testing.T) {
	server, _ := newFreshAPIServer(t)
	completed, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/onboarding/complete", []byte(`{"investableCapitalFen":20000000,"maxLossFen":2000000,"holdingHorizon":"6_to_12m","enabledMarkets":["ashare_etf"]}`)))
	if err != nil {
		t.Fatal(err)
	}
	completed.Body.Close()
	if completed.StatusCode != http.StatusCreated {
		t.Fatalf("complete onboarding status=%d", completed.StatusCode)
	}

	for _, endpoint := range []string{"/api/instruments", "/api/watchlist", "/api/plans", "/api/executions"} {
		response, err := server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+endpoint, nil))
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || string(envelope.Data) != "[]" {
			t.Fatalf("%s status=%d data=%s, want []", endpoint, response.StatusCode, envelope.Data)
		}
	}
}

func TestResolveInstrumentAPIAddsValidAShareETFOnce(t *testing.T) {
	provider := apiMarketProvider{quotes: []market.Quote{{
		TradeDate: "2026-08-25", Market: "SZ", Code: "159361", Name: "A500ETF易方达",
		AssetType: market.KindStock, CloseMinor: 122, Source: "fixture-quote", SourceTime: time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC),
	}}}
	server, db := newMarketAPIServer(t, provider)

	resolve := func() store.InstrumentRow {
		t.Helper()
		response, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/instruments/resolve", []byte(`{"code":"159361"}`)))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("resolve status=%d", response.StatusCode)
		}
		var result struct {
			Data store.InstrumentRow `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || result.Data.Market != "SZ" || result.Data.Code != "159361" || result.Data.AssetType != "etf" || result.Data.LotSize != 100 {
			t.Fatalf("status=%d instrument=%#v", response.StatusCode, result.Data)
		}
		return result.Data
	}

	if got := resolve(); got.Name != "A500ETF易方达" {
		t.Fatalf("created name=%q", got.Name)
	}
	if _, err := db.DB().Exec(`UPDATE instruments SET name='A500ETF� ���' WHERE market='SZ' AND code='159361'`); err != nil {
		t.Fatal(err)
	}
	if got := resolve(); got.Name != "A500ETF易方达" {
		t.Fatalf("repaired name=%q", got.Name)
	}

	var instrumentCount, createAuditCount, repairAuditCount int
	if err := db.DB().QueryRow(`SELECT count(*) FROM instruments WHERE market='SZ' AND code='159361'`).Scan(&instrumentCount); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM audit_events WHERE entity_type='instrument' AND action='resolved_from_public_quote'`).Scan(&createAuditCount); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM audit_events WHERE entity_type='instrument' AND action='resolved_instrument_name_repaired'`).Scan(&repairAuditCount); err != nil {
		t.Fatal(err)
	}
	if instrumentCount != 1 || createAuditCount != 1 || repairAuditCount != 1 {
		t.Fatalf("instrumentCount=%d createAuditCount=%d repairAuditCount=%d", instrumentCount, createAuditCount, repairAuditCount)
	}
}

func TestDashboardCooldownUsesFrontendJSONContract(t *testing.T) {
	server := newAPIServer(t)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/executions/quick", []byte(`{"instrumentId":"hk-9988","side":"buy","quantity":100,"localPriceMinor":12000,"settlementFen":-1205000}`)))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("quick status=%d", response.StatusCode)
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/dashboard", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Data struct {
			Cooldown map[string]any `json:"cooldown"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if got := result.Data.Cooldown["expectedEndsAt"]; got != "2026-08-19T08:00:00Z" {
		t.Fatalf("expectedEndsAt=%v cooldown=%#v", got, result.Data.Cooldown)
	}
}

func TestQuickExecutionAndPostTradeReviewAPI(t *testing.T) {
	server := newAPIServer(t)
	response, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/executions/quick", []byte(`{"instrumentId":"hk-9988","side":"buy","quantity":100,"localPriceMinor":12000,"settlementFen":-1205000}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("quick status=%d", response.StatusCode)
	}
	var recorded struct {
		OK   bool                     `json:"ok"`
		Data service.ExecutionReceipt `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.OK || !recorded.Data.PendingReview || recorded.Data.Position.Quantity != 100 {
		t.Fatalf("recorded=%#v", recorded)
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodGet, server.URL+"/api/post-trade-reviews?periodStart=2026-08-10&periodEnd=2026-08-16", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var pending struct {
		OK   bool                    `json:"ok"`
		Data []store.PostTradeReview `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(pending.Data) != 1 {
		t.Fatalf("pending status=%d data=%#v", response.StatusCode, pending)
	}
	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/reviews", []byte(`{"periodStart":"2026-08-10","periodEnd":"2026-08-16","nextAllowedAction":"只按计划行动"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var blocked Envelope[any]
	if err := json.NewDecoder(response.Body).Decode(&blocked); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnprocessableEntity || blocked.Error == nil || blocked.Error.Code != "PENDING_POST_TRADE_REVIEW" {
		t.Fatalf("blocked status=%d body=%#v", response.StatusCode, blocked)
	}

	response, err = server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/post-trade-reviews/"+recorded.Data.ID+"/complete", []byte(`{"note":"追涨后补录，今后只按计划行动"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("complete status=%d", response.StatusCode)
	}
}

func TestAllocationAPITracksGoalAndAppendsManualAdjustments(t *testing.T) {
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
		OK   bool                   `json:"ok"`
		Data domain.AllocationState `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	if !initial.OK || initial.Data.Overview == nil || initial.Data.Overview.CurrentTotalFen != 20_000_000 || initial.Data.Overview.TargetTotalFen != 22_000_000 || initial.Data.Overview.ReturnBP != 0 {
		t.Fatalf("unexpected allocation overview: %#v", initial.Data)
	}

	recorded, err := server.Client().Do(authorizedRequest(t, http.MethodPost, server.URL+"/api/allocation/items/semiconductor-equipment-etf-159558/adjustments", []byte(`{"adjustmentFen":250000,"observedAt":"2026-08-12T08:00:00Z"}`)))
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
		Data domain.AllocationState `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.Overview == nil || updated.Data.Overview.CurrentTotalFen != 20_250_000 || updated.Data.Overview.ReturnBP != 125 || updated.Data.Overview.GoalGapFen != 1_750_000 {
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
