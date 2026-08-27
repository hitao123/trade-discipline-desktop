package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

const CurrentSchemaVersion = 11

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Path() string { return s.path }

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	legacyDatabase, err := hasExistingApplicationState(ctx, tx)
	if err != nil {
		return err
	}

	for _, statement := range migrationStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migration statement: %w", err)
		}
	}
	if err := ensureMarketSnapshotMode(ctx, tx); err != nil {
		return err
	}
	if err := ensureMarketRefreshHealth(ctx, tx); err != nil {
		return err
	}
	if err := ensureExecutionPricePrecision(ctx, tx); err != nil {
		return err
	}
	if err := ensureCooldownExecutionLink(ctx, tx); err != nil {
		return err
	}
	if err := migrateReferenceQuoteScaleV8(ctx, tx); err != nil {
		return err
	}
	if err := purgeForbiddenETFs(ctx, tx, 9); err != nil {
		return err
	}
	if err := purgeForbiddenETFs(ctx, tx, 10); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record market history migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record discipline loop migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record allocation migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record market mode migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record market health migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (7, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record post-trade review migration: %w", err)
	}
	if err := ensureUserProfileMigration(ctx, tx, legacyDatabase, time.Now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func ensureCooldownExecutionLink(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(cooldown_periods)`)
	if err != nil {
		return fmt.Errorf("inspect cooldown schema: %w", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan cooldown schema: %w", err)
		}
		if name == "execution_id" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close cooldown schema: %w", err)
	}
	if !found {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE cooldown_periods ADD COLUMN execution_id TEXT REFERENCES execution_events(id)`); err != nil {
			return fmt.Errorf("link cooldown to execution: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cooldown_periods
		SET execution_id=(SELECT v.execution_id FROM violation_events v WHERE v.rule_code=cooldown_periods.reason AND v.occurred_at=cooldown_periods.starts_at ORDER BY v.rowid DESC LIMIT 1)
		WHERE execution_id IS NULL`); err != nil {
		return fmt.Errorf("backfill cooldown execution: %w", err)
	}
	return nil
}

func purgeForbiddenETFs(ctx context.Context, tx *sql.Tx, version int) error {
	var applied int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=?`, version).Scan(&applied); err != nil {
		return fmt.Errorf("inspect forbidden ETF migration: %w", err)
	}
	if applied > 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, code, name FROM instruments WHERE asset_type='etf'`)
	if err != nil {
		return fmt.Errorf("list ETFs for cleanup: %w", err)
	}
	type candidate struct{ id, code, name string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.code, &item.name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan ETF for cleanup: %w", err)
		}
		if !market.IsEligibleETF(item.code, item.name) {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close ETF cleanup rows: %w", err)
	}
	for _, item := range candidates {
		for _, ref := range []struct{ table, column string }{
			{"execution_events", "instrument_id"},
			{"trade_plans", "instrument_id"},
			{"watchlist_items", "instrument_id"},
			{"price_alert_events", "instrument_id"},
		} {
			var count int
			query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s=?", ref.table, ref.column)
			if err := tx.QueryRowContext(ctx, query, item.id).Scan(&count); err != nil {
				return fmt.Errorf("inspect %s reference for %s: %w", ref.table, item.code, err)
			}
			if count > 0 {
				return fmt.Errorf("债基 %s 已被业务记录引用，未执行自动删除", item.code)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM market_rank_entries WHERE instrument_id=?`, item.id); err != nil {
			return fmt.Errorf("delete forbidden ETF ranking %s: %w", item.code, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM instruments WHERE id=?`, item.id); err != nil {
			return fmt.Errorf("delete forbidden ETF %s: %w", item.code, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, version); err != nil {
		return fmt.Errorf("record forbidden ETF migration: %w", err)
	}
	return nil
}

func ensureExecutionPricePrecision(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(execution_events)`)
	if err != nil {
		return fmt.Errorf("inspect execution price schema: %w", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan execution price schema: %w", err)
		}
		if name == "local_price_ten_thousandth" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close execution price schema: %w", err)
	}
	if found {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE execution_events ADD COLUMN local_price_ten_thousandth INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add precise execution price: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE execution_events SET local_price_ten_thousandth=local_price_minor*100`); err != nil {
		return fmt.Errorf("backfill precise execution price: %w", err)
	}
	return nil
}

func migrateReferenceQuoteScaleV8(ctx context.Context, tx *sql.Tx) error {
	var applied int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=8`).Scan(&applied); err != nil {
		return fmt.Errorf("inspect reference quote scale migration: %w", err)
	}
	if applied > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE instruments
		SET latest_price_minor=NULL, latest_price_at=NULL, latest_price_source=NULL
		WHERE market IN ('SH','SZ') AND latest_price_source='eastmoney-public-close'`); err != nil {
		return fmt.Errorf("clear legacy A-share reference prices: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); err != nil {
		return fmt.Errorf("record reference quote scale migration: %w", err)
	}
	return nil
}

func ensureMarketSnapshotMode(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(market_snapshots)`)
	if err != nil {
		return fmt.Errorf("inspect market snapshot schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("scan market snapshot schema: %w", err)
		}
		if name == "snapshot_mode" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read market snapshot schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE market_snapshots ADD COLUMN snapshot_mode TEXT NOT NULL DEFAULT 'close' CHECK(snapshot_mode IN ('close','live'))`); err != nil {
		return fmt.Errorf("add market snapshot mode: %w", err)
	}
	return nil
}

func ensureMarketRefreshHealth(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(market_refresh_status)`)
	if err != nil {
		return fmt.Errorf("inspect market refresh status schema: %w", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan market refresh status schema: %w", err)
		}
		if name == "health_json" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read market refresh status schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close market refresh status schema: %w", err)
	}
	if found {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE market_refresh_status ADD COLUMN health_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(health_json))`); err != nil {
		return fmt.Errorf("add market refresh health: %w", err)
	}
	return nil
}

func (s *Store) CurrentRule(ctx context.Context) (rules.Snapshot, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT snapshot_json FROM rule_versions ORDER BY version DESC LIMIT 1`).Scan(&raw)
	if err != nil {
		return rules.Snapshot{}, fmt.Errorf("load current rule: %w", err)
	}
	var snapshot rules.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return rules.Snapshot{}, fmt.Errorf("decode current rule: %w", err)
	}
	return snapshot, nil
}
