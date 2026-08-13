package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "discipline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateSeedsOneAccountAndInitialRule(t *testing.T) {
	db := openTestStore(t)
	rule, err := db.CurrentRule(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rule.InitialCapitalFen != 20_000_000 || rule.LossRedLineFen != 2_000_000 || rule.ChinaTechLimitFen != 8_000_000 {
		t.Fatalf("unexpected initial rule: %#v", rule)
	}
	if rule.TencentMaxShares != 100 || rule.AlibabaMaxShares != 100 || rule.HKBoardLot != 100 {
		t.Fatalf("unexpected HK limits: %#v", rule)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTestStore(t)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "rule_versions"} {
		var count int
		if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s count = %d", table, count)
		}
	}
}

func TestMigrationCreatesCompleteLedgerTables(t *testing.T) {
	db := openTestStore(t)
	required := []string{
		"accounts", "rule_versions", "instruments", "watchlist_items", "trade_plans",
		"trade_plan_revisions", "execution_events", "cash_events", "violation_events",
		"cooldown_periods", "weekly_reviews", "market_snapshots", "market_rank_entries",
		"audit_events", "app_settings",
	}
	for _, table := range required {
		var name string
		err := db.DB().QueryRowContext(context.Background(), "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil || name != table {
			t.Fatalf("missing table %s: %v", table, err)
		}
	}
}

func TestMigrationAddsAppendOnlyMarketHistorySchema(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"market_daily_bar_observations", "market_metric_observations"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}
