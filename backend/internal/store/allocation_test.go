package store

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

var allocationFixedNow = time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)

func TestInitialAllocationSeedsTheCheckedTwentyWanBaseline(t *testing.T) {
	db := openTestStore(t)
	snapshot, err := db.EnsureInitialAllocation(context.Background(), allocationFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProfileID == "" || snapshot.Version.Version != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if snapshot.Version.Draft.InitialCapitalFen != 20_000_000 || snapshot.Version.Draft.TargetReturnBP != 1_000 || snapshot.Version.Draft.TargetDeadline != "2026-12-31" {
		t.Fatalf("draft=%#v", snapshot.Version.Draft)
	}
	if len(snapshot.Version.Draft.Items) != 7 {
		t.Fatalf("items=%#v", snapshot.Version.Draft.Items)
	}
	if got := snapshot.Values["semiconductor-equipment-etf-159558"].ValueFen; got != 1_000_000 {
		t.Fatalf("semiconductor value=%d", got)
	}
	if got := snapshot.Values["cash-fund"].ValueFen; got != 19_000_000 {
		t.Fatalf("cash value=%d", got)
	}
}

func TestAllocationValueEventsAppendWithoutChangingTheConfigurationVersion(t *testing.T) {
	db := openTestStore(t)
	initial, err := db.EnsureInitialAllocation(context.Background(), allocationFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	second := domain.AllocationValueEvent{
		ItemKey:    "semiconductor-equipment-etf-159558",
		ValueFen:   1_250_000,
		Source:     "manual",
		ObservedAt: allocationFixedNow.Add(time.Hour),
	}
	if _, err := db.AppendAllocationValueEvent(context.Background(), initial.ProfileID, second, allocationFixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	current, err := db.CurrentAllocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Version.Version != 1 || current.Values[second.ItemKey].ValueFen != 1_250_000 {
		t.Fatalf("current=%#v", current)
	}
	events, err := db.ListAllocationValueEvents(context.Background(), initial.ProfileID, second.ItemKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ValueFen != 1_250_000 || events[1].ValueFen != 1_000_000 {
		t.Fatalf("events=%#v", events)
	}
}
