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
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsDisciplineLoopTables(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"pre_trade_confirmations", "price_alert_events", "position_review_events"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsAssetAllocationTables(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"allocation_profiles", "allocation_versions", "allocation_value_events"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsMarketModeAndRefreshStatus(t *testing.T) {
	db := openTestStore(t)
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	var table string
	if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='market_refresh_status'").Scan(&table); err != nil || table != "market_refresh_status" {
		t.Fatalf("market_refresh_status missing: %v", err)
	}
	var column string
	if err := db.DB().QueryRow("SELECT name FROM pragma_table_info('market_snapshots') WHERE name='snapshot_mode'").Scan(&column); err != nil || column != "snapshot_mode" {
		t.Fatalf("snapshot mode column missing: %v", err)
	}
}

func TestMigrationUpgradesExistingMarketSnapshotsToCloseMode(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.DB().Exec(`CREATE TABLE market_snapshots (
		id TEXT PRIMARY KEY,
		trade_date TEXT NOT NULL,
		ranking_kind TEXT NOT NULL,
		source TEXT NOT NULL,
		fetched_at TEXT NOT NULL,
		status TEXT NOT NULL,
		version INTEGER NOT NULL,
		UNIQUE(trade_date, ranking_kind, version)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO market_snapshots(id, trade_date, ranking_kind, source, fetched_at, status, version) VALUES ('legacy-stock', '2026-08-21', 'stock', 'fixture', '2026-08-21T08:00:00Z', 'success', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.DB().QueryRow(`SELECT snapshot_mode FROM market_snapshots WHERE id='legacy-stock'`).Scan(&mode); err != nil || mode != "close" {
		t.Fatalf("legacy mode=%q err=%v", mode, err)
	}
}

func TestMigrationUpgradesExistingMarketRefreshStatusWithHealth(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy-health.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.DB().Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (5, '2026-08-22T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`CREATE TABLE market_refresh_status (
		snapshot_mode TEXT PRIMARY KEY CHECK(snapshot_mode IN ('close','live')),
		last_attempt_at TEXT NOT NULL,
		last_success_at TEXT,
		errors_json TEXT NOT NULL CHECK(json_valid(errors_json))
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO market_refresh_status(snapshot_mode, last_attempt_at, last_success_at, errors_json) VALUES ('close', '2026-08-22T08:00:00Z', '2026-08-22T08:00:00Z', '{"quotes":"source unavailable"}')`); err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var healthJSON, errorsJSON string
	if err := db.DB().QueryRow(`SELECT health_json, errors_json FROM market_refresh_status WHERE snapshot_mode='close'`).Scan(&healthJSON, &errorsJSON); err != nil {
		t.Fatal(err)
	}
	if healthJSON != "{}" || errorsJSON != `{"quotes":"source unavailable"}` {
		t.Fatalf("health=%q errors=%q", healthJSON, errorsJSON)
	}
	var version int
	if err := db.DB().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsPostTradeReviewAndFXObservationTables(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"post_trade_reviews", "execution_fx_observations"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}
