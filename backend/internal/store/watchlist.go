package store

import (
	"context"
	"fmt"
	"time"
)

type WatchlistRow struct {
	ID         string        `json:"id"`
	Instrument InstrumentRow `json:"instrument"`
	SourceType string        `json:"sourceType"`
	Reason     string        `json:"reason"`
	AddedAt    time.Time     `json:"addedAt"`
	Status     string        `json:"status"`
}

func (s *Store) AddWatchlist(ctx context.Context, market, code, sourceType, reason string, now time.Time) (WatchlistRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WatchlistRow{}, fmt.Errorf("begin watchlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var instrument InstrumentRow
	var tech int
	err = tx.QueryRowContext(ctx, `SELECT id, market, code, name, asset_type, currency, lot_size, is_china_tech FROM instruments WHERE market=? AND code=? AND status='active'`, market, code).Scan(&instrument.ID, &instrument.Market, &instrument.Code, &instrument.Name, &instrument.AssetType, &instrument.Currency, &instrument.LotSize, &tech)
	if err != nil {
		return WatchlistRow{}, fmt.Errorf("未找到证券 %s %s，请先刷新榜单或使用已有证券", market, code)
	}
	instrument.IsChinaTech = tech == 1
	var existing string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM watchlist_items WHERE instrument_id=? AND status='active' LIMIT 1`, instrument.ID).Scan(&existing); err == nil {
		return WatchlistRow{}, fmt.Errorf("该证券已在观察名单中")
	}
	id := NewID("watch")
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_items(id, instrument_id, source_type, reason, added_at, status) VALUES(?,?,?,?,?,'active')`, id, instrument.ID, sourceType, reason, stamp); err != nil {
		return WatchlistRow{}, fmt.Errorf("insert watchlist: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'watchlist', ?, 'added', NULL, ?, ?)`, NewID("audit"), id, fmt.Sprintf(`{"instrument":%q,"reason":%q}`, instrument.Code, reason), stamp); err != nil {
		return WatchlistRow{}, fmt.Errorf("audit watchlist: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WatchlistRow{}, fmt.Errorf("commit watchlist: %w", err)
	}
	return WatchlistRow{ID: id, Instrument: instrument, SourceType: sourceType, Reason: reason, AddedAt: now.UTC(), Status: "active"}, nil
}

func (s *Store) ListWatchlist(ctx context.Context) ([]WatchlistRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT w.id, i.id, i.market, i.code, i.name, i.asset_type, i.currency, i.lot_size, i.is_china_tech, w.source_type, w.reason, w.added_at, w.status FROM watchlist_items w JOIN instruments i ON i.id=w.instrument_id WHERE w.status='active' ORDER BY w.added_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list watchlist: %w", err)
	}
	defer rows.Close()
	items := make([]WatchlistRow, 0)
	for rows.Next() {
		var item WatchlistRow
		var tech int
		var added string
		if err := rows.Scan(&item.ID, &item.Instrument.ID, &item.Instrument.Market, &item.Instrument.Code, &item.Instrument.Name, &item.Instrument.AssetType, &item.Instrument.Currency, &item.Instrument.LotSize, &tech, &item.SourceType, &item.Reason, &added, &item.Status); err != nil {
			return nil, fmt.Errorf("scan watchlist: %w", err)
		}
		item.Instrument.IsChinaTech = tech == 1
		item.AddedAt, _ = time.Parse(time.RFC3339Nano, added)
		items = append(items, item)
	}
	return items, rows.Err()
}
