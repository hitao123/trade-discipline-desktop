package service

import (
	"context"
	"testing"
)

func TestWatchlistAcceptsMarketRankingButDoesNotCreateExecution(t *testing.T) {
	svc := openExecutionService(t)
	item, err := svc.AddWatchlist(context.Background(), WatchlistDraft{Market: "HK", Code: "9988.HK", Reason: "等待云业务证据", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Instrument.Code != "9988.HK" || item.Reason == "" {
		t.Fatalf("unexpected item: %#v", item)
	}
	var executions int
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM execution_events`).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("execution count=%d err=%v", executions, err)
	}
}
