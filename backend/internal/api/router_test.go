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
