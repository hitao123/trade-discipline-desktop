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

const CurrentSchemaVersion = 2

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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
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
