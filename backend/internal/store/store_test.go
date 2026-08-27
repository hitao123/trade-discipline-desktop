package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
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

func openLegacyTestStore(t *testing.T) *Store {
	t.Helper()
	db := openTestStore(t)
	stamp := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	raw, err := json.Marshal(rules.InitialSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE user_profiles SET mode='legacy', onboarding_status='completed', investable_capital_fen=20000000, max_loss_fen=2000000, holding_horizon='legacy_unspecified', enabled_markets_json='["ashare_stock","ashare_etf","hk"]', completed_at=?, updated_at=? WHERE id='local-user'`, []any{stamp, stamp}},
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
	return db
}

func TestFreshMigrationCreatesPendingGenericProfileWithoutPersonalSeeds(t *testing.T) {
	db := openTestStore(t)
	profile, err := db.UserProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.Mode != domain.UserModeGeneric || profile.OnboardingStatus != domain.OnboardingPending {
		t.Fatalf("profile=%#v", profile)
	}
	for _, table := range []string{"accounts", "rule_versions", "instruments", "allocation_profiles"} {
		var count int
		if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s count=%d", table, count)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTestStore(t)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for table, expected := range map[string]int{"user_profiles": 1, "accounts": 0, "rule_versions": 0, "instruments": 0} {
		var count int
		if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Fatalf("%s count = %d", table, count)
		}
	}
}

func TestMigrationMarksExistingDatabaseLegacyWithoutChangingFacts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE accounts(id TEXT PRIMARY KEY, name TEXT NOT NULL, base_currency TEXT NOT NULL, initial_capital_fen INTEGER NOT NULL, enabled_at TEXT NOT NULL, status TEXT NOT NULL)`,
		`CREATE TABLE rule_versions(id TEXT PRIMARY KEY, version INTEGER NOT NULL UNIQUE, snapshot_json TEXT NOT NULL, change_reason TEXT NOT NULL, created_at TEXT NOT NULL, previous_id TEXT)`,
		`INSERT INTO schema_migrations VALUES(10,'2026-08-25T00:00:00Z')`,
		`INSERT INTO accounts VALUES('account-main','我的纪律账户','CNY',20000000,'2026-08-12T00:00:00Z','active')`,
	} {
		if _, err := db.DB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(rules.InitialSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO rule_versions VALUES('rule-1',1,?,'初始交易纪律规则','2026-08-12T00:00:00Z',NULL)`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := db.UserProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Mode != domain.UserModeLegacy || profile.OnboardingStatus != domain.OnboardingCompleted {
		t.Fatalf("profile=%#v", profile)
	}
	var accounts, versions int
	var after string
	if err := db.DB().QueryRow(`SELECT count(*) FROM accounts`).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM rule_versions`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT snapshot_json FROM rule_versions WHERE id='rule-1'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 || versions != 1 || after != string(raw) {
		t.Fatalf("accounts=%d versions=%d snapshotChanged=%v", accounts, versions, after != string(raw))
	}
}

func TestCompleteOnboardingCreatesAccountRuleAndAuditOnce(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	pending, err := db.UserProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pending.InvestableCapitalFen = 20_000_000
	pending.MaxLossFen = 2_000_000
	pending.HoldingHorizon = domain.Horizon6To12M
	pending.EnabledMarkets = []domain.MarketScope{domain.MarketAShareETF, domain.MarketHK}
	completed, err := db.CompleteGenericOnboarding(ctx, pending, rules.GenericSnapshot(20_000_000, 2_000_000), now)
	if err != nil {
		t.Fatal(err)
	}
	if completed.OnboardingStatus != domain.OnboardingCompleted || completed.CompletedAt == nil {
		t.Fatalf("profile=%#v", completed)
	}
	for table, want := range map[string]int{"accounts": 1, "rule_versions": 1, "audit_events": 1} {
		var got int
		if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s=%d want=%d", table, got, want)
		}
	}
	if _, err := db.CompleteGenericOnboarding(ctx, pending, rules.GenericSnapshot(20_000_000, 2_000_000), now); !errors.Is(err, ErrOnboardingAlreadyCompleted) {
		t.Fatalf("second completion err=%v", err)
	}
}

func TestMigrationRemovesUnusedForbiddenETFs(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO market_snapshots(id, trade_date, ranking_kind, snapshot_mode, source, fetched_at, status, version)
		VALUES ('market-bond', '2026-08-24', 'etf', 'close', 'fixture', '2026-08-24T08:00:00Z', 'success', 1)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, code, name string }{
		{"SH-511360", "511360", "短融ETF海富通"},
		{"SH-511880", "511880", "银华日利ETF"},
		{"SH-511990", "511990", "华宝添益ETF"},
		{"SH-515880", "515880", "通信ETF国泰"},
	} {
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status)
			VALUES (?, 'SH', ?, ?, 'etf', 'CNY', 100, 'fixture', 0, 0, 'active')`, row.id, row.code, row.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO market_rank_entries(id, snapshot_id, instrument_id, rank, market, code, name, asset_type, close_minor, change_bp, turnover_fen, trade_date)
		VALUES ('rank-bond', 'market-bond', 'SH-511360', 1, 'SH', '511360', '短融ETF海富通', 'etf', 10000, 0, 100000, '2026-08-24')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=10`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var forbidden, allowed, rank int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM instruments WHERE code IN ('511360','511880','511990')`).Scan(&forbidden); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM instruments WHERE code='515880'`).Scan(&allowed); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM market_rank_entries WHERE id='rank-bond'`).Scan(&rank); err != nil {
		t.Fatal(err)
	}
	if forbidden != 0 || rank != 0 || allowed != 1 {
		t.Fatalf("forbidden=%d rank=%d allowed=%d", forbidden, rank, allowed)
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
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
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
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsAssetAllocationTables(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"allocation_profiles", "allocation_versions", "allocation_value_events", "allocation_adjustment_events"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestMigrationAddsMarketModeAndRefreshStatus(t *testing.T) {
	db := openTestStore(t)
	var version int
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
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
	if err := db.DB().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != CurrentSchemaVersion {
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
	if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}
