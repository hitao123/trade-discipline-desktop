package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

type MarketSnapshotRow struct {
	ID        string             `json:"id"`
	TradeDate string             `json:"tradeDate"`
	Kind      market.RankingKind `json:"kind"`
	Source    string             `json:"source"`
	FetchedAt time.Time          `json:"fetchedAt"`
	Version   int                `json:"version"`
	Entries   []market.Quote     `json:"entries"`
}

func (s *Store) ActiveQuoteKeys(ctx context.Context) ([]market.InstrumentKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT market, code FROM instruments WHERE status='active' ORDER BY market, code`)
	if err != nil {
		return nil, fmt.Errorf("list quote keys: %w", err)
	}
	defer rows.Close()
	var keys []market.InstrumentKey
	for rows.Next() {
		var key market.InstrumentKey
		if err := rows.Scan(&key.Market, &key.Code); err != nil {
			return nil, fmt.Errorf("scan quote key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) UpdateQuotes(ctx context.Context, quotes []market.Quote) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin quote update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, quote := range quotes {
		result, err := tx.ExecContext(ctx, `UPDATE instruments SET latest_price_minor=?, latest_price_at=?, latest_price_source=? WHERE market=? AND code=? AND status='active'`, quote.CloseMinor, quote.SourceTime.UTC().Format(time.RFC3339Nano), quote.Source, quote.Market, quote.Code)
		if err != nil {
			return fmt.Errorf("update quote %s: %w", quote.Code, err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			continue
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit quote update: %w", err)
	}
	return nil
}

func (s *Store) SaveMarketSnapshot(ctx context.Context, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time) (MarketSnapshotRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("begin market snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row, err := saveMarketSnapshotTx(ctx, tx, kind, quotes, source, fetchedAt)
	if err != nil {
		return MarketSnapshotRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("commit market snapshot: %w", err)
	}
	return row, nil
}

func (s *Store) SaveMarketBatch(ctx context.Context, groups map[market.RankingKind][]market.Quote, source string, fetchedAt time.Time) (map[market.RankingKind]MarketSnapshotRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin market batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result := make(map[market.RankingKind]MarketSnapshotRow)
	for _, kind := range []market.RankingKind{market.KindStock, market.KindETF} {
		quotes := groups[kind]
		if len(quotes) == 0 {
			continue
		}
		row, err := saveMarketSnapshotTx(ctx, tx, kind, quotes, source, fetchedAt)
		if err != nil {
			return nil, err
		}
		result[kind] = row
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可导入的有效榜单行")
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit market batch: %w", err)
	}
	return result, nil
}

func saveMarketSnapshotTx(ctx context.Context, tx *sql.Tx, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time) (MarketSnapshotRow, error) {
	if len(quotes) == 0 {
		return MarketSnapshotRow{}, fmt.Errorf("%s 榜单没有有效数据", kind)
	}
	tradeDate := quotes[0].TradeDate
	for _, quote := range quotes {
		if quote.TradeDate != tradeDate || quote.AssetType != kind {
			return MarketSnapshotRow{}, fmt.Errorf("%s 榜单交易日或资产类型不一致", kind)
		}
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM market_snapshots WHERE trade_date=? AND ranking_kind=?`, tradeDate, kind).Scan(&version); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("next market version: %w", err)
	}
	id := NewID("market")
	stamp := fetchedAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO market_snapshots(id, trade_date, ranking_kind, source, fetched_at, status, version) VALUES(?,?,?,?,?,'success',?)`, id, tradeDate, kind, source, stamp, version); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("insert market snapshot: %w", err)
	}
	for index, quote := range quotes {
		currency := "CNY"
		lotSize := 100
		instrumentID := quote.Market + "-" + quote.Code
		_, err := tx.ExecContext(ctx, `INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status, latest_price_minor, latest_price_at, latest_price_source)
			VALUES(?,?,?,?,?,?,?,'market',0,0,'active',?,?,?)
			ON CONFLICT(market,code) DO UPDATE SET name=excluded.name, asset_type=excluded.asset_type, latest_price_minor=excluded.latest_price_minor, latest_price_at=excluded.latest_price_at, latest_price_source=excluded.latest_price_source`,
			instrumentID, quote.Market, quote.Code, quote.Name, quote.AssetType, currency, lotSize, quote.CloseMinor, quote.SourceTime.UTC().Format(time.RFC3339Nano), source)
		if err != nil {
			return MarketSnapshotRow{}, fmt.Errorf("upsert market instrument %s: %w", quote.Code, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO market_rank_entries(id, snapshot_id, instrument_id, rank, market, code, name, asset_type, close_minor, change_bp, turnover_fen, trade_date, source_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, NewID("rank"), id, instrumentID, index+1, quote.Market, quote.Code, quote.Name, quote.AssetType, quote.CloseMinor, quote.ChangeBP, quote.TurnoverFen, quote.TradeDate, quote.SourceTime.UTC().Format(time.RFC3339Nano)); err != nil {
			return MarketSnapshotRow{}, fmt.Errorf("insert market rank %s: %w", quote.Code, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'market_snapshot', ?, 'imported', NULL, ?, ?)`, NewID("audit"), id, fmt.Sprintf(`{"kind":%q,"rows":%d}`, kind, len(quotes)), stamp); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("audit market snapshot: %w", err)
	}
	return MarketSnapshotRow{ID: id, TradeDate: tradeDate, Kind: kind, Source: source, FetchedAt: fetchedAt.UTC(), Version: version, Entries: quotes}, nil
}

func (s *Store) LatestMarketSnapshot(ctx context.Context, kind market.RankingKind) (MarketSnapshotRow, error) {
	var row MarketSnapshotRow
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT id, trade_date, source, fetched_at, version FROM market_snapshots WHERE ranking_kind=? AND status='success' ORDER BY trade_date DESC, version DESC LIMIT 1`, kind).Scan(&row.ID, &row.TradeDate, &row.Source, &fetched, &row.Version)
	if err == sql.ErrNoRows {
		row.Kind = kind
		row.Entries = []market.Quote{}
		return row, nil
	}
	if err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("load latest market snapshot: %w", err)
	}
	row.Kind = kind
	row.FetchedAt, _ = time.Parse(time.RFC3339Nano, fetched)
	rows, err := s.db.QueryContext(ctx, `SELECT trade_date, market, code, name, asset_type, close_minor, change_bp, turnover_fen, COALESCE(source_time,'') FROM market_rank_entries WHERE snapshot_id=? ORDER BY rank`, row.ID)
	if err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("load market entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var quote market.Quote
		var sourceTime string
		if err := rows.Scan(&quote.TradeDate, &quote.Market, &quote.Code, &quote.Name, &quote.AssetType, &quote.CloseMinor, &quote.ChangeBP, &quote.TurnoverFen, &sourceTime); err != nil {
			return MarketSnapshotRow{}, fmt.Errorf("scan market entry: %w", err)
		}
		quote.Source = row.Source
		quote.SourceTime, _ = time.Parse(time.RFC3339Nano, sourceTime)
		row.Entries = append(row.Entries, quote)
	}
	return row, rows.Err()
}
