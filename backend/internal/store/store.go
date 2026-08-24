package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

const CurrentSchemaVersion = 8

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
	if err := migrateReferenceQuoteScaleV8(ctx, tx); err != nil {
		return err
	}

	ruleJSON, err := json.Marshal(rules.InitialSnapshot())
	if err != nil {
		return fmt.Errorf("encode initial rule: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO accounts(id, name, base_currency, initial_capital_fen, enabled_at, status)
		VALUES ('account-main', '我的纪律账户', 'CNY', 20000000, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'active')`); err != nil {
		return fmt.Errorf("seed account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id)
		VALUES ('rule-1', 1, ?, '初始交易纪律规则', strftime('%Y-%m-%dT%H:%M:%fZ','now'), NULL)`, string(ruleJSON)); err != nil {
		return fmt.Errorf("seed rules: %w", err)
	}
	for _, instrument := range []struct {
		id, market, code, name string
	}{
		{"hk-0700", "HK", "0700.HK", "腾讯控股"},
		{"hk-9988", "HK", "9988.HK", "阿里巴巴-W"},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status)
			VALUES (?, ?, ?, ?, 'stock', 'HKD', 100, 'initial_rule', 1, 0, 'active')`, instrument.id, instrument.market, instrument.code, instrument.name); err != nil {
			return fmt.Errorf("seed instrument %s: %w", instrument.code, err)
		}
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
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
