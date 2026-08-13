package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

type ObservationStats struct {
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	FromDate   string `json:"fromDate,omitempty"`
	ToDate     string `json:"toDate,omitempty"`
}

func (s *Store) SaveDailyBars(ctx context.Context, bars []market.DailyBar, fetchedAt time.Time) (ObservationStats, error) {
	stats := observationRangeDaily(bars)
	if len(bars) == 0 {
		return stats, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin daily bar observations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stamp := fetchedAt.UTC().Format(time.RFC3339Nano)
	for _, bar := range bars {
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO market_daily_bar_observations(id, market, code, trade_date, close_minor, turnover_fen, source, source_time, fetched_at, last_seen_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			NewID("bar"), bar.Market, bar.Code, bar.TradeDate, bar.CloseMinor, bar.TurnoverFen, bar.Source, bar.SourceTime.UTC().Format(time.RFC3339Nano), stamp, stamp)
		if err != nil {
			return stats, fmt.Errorf("insert daily bar %s %s: %w", bar.Code, bar.TradeDate, err)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			stats.Duplicates++
			if _, err := tx.ExecContext(ctx, `UPDATE market_daily_bar_observations SET last_seen_at=? WHERE market=? AND code=? AND trade_date=? AND close_minor=? AND turnover_fen=? AND source=?`, stamp, bar.Market, bar.Code, bar.TradeDate, bar.CloseMinor, bar.TurnoverFen, bar.Source); err != nil {
				return stats, fmt.Errorf("touch daily bar %s %s: %w", bar.Code, bar.TradeDate, err)
			}
		} else {
			stats.Inserted++
		}
	}
	if err := appendMarketSyncAudit(ctx, tx, "market_daily_bars", stats, stamp); err != nil {
		return stats, err
	}
	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit daily bar observations: %w", err)
	}
	return stats, nil
}

func (s *Store) LatestDailyBars(ctx context.Context, key market.InstrumentKey, since string) ([]market.DailyBar, time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT trade_date, market, code, close_minor, turnover_fen, source, source_time,
			ROW_NUMBER() OVER (PARTITION BY trade_date ORDER BY fetched_at DESC, rowid DESC) AS position
		FROM market_daily_bar_observations WHERE market=? AND code=? AND trade_date>=?
	) SELECT trade_date, market, code, close_minor, turnover_fen, source, source_time FROM ranked WHERE position=1 ORDER BY trade_date`, key.Market, key.Code, since)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query daily bars: %w", err)
	}
	defer rows.Close()
	bars := make([]market.DailyBar, 0)
	for rows.Next() {
		var bar market.DailyBar
		var sourceTime string
		if err := rows.Scan(&bar.TradeDate, &bar.Market, &bar.Code, &bar.CloseMinor, &bar.TurnoverFen, &bar.Source, &sourceTime); err != nil {
			return nil, time.Time{}, fmt.Errorf("scan daily bar: %w", err)
		}
		bar.SourceTime, _ = time.Parse(time.RFC3339Nano, sourceTime)
		bars = append(bars, bar)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("read daily bars: %w", err)
	}
	var lastSeen sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(last_seen_at) FROM market_daily_bar_observations WHERE market=? AND code=?`, key.Market, key.Code).Scan(&lastSeen); err != nil {
		return nil, time.Time{}, fmt.Errorf("load daily bar freshness: %w", err)
	}
	fetchedAt, _ := time.Parse(time.RFC3339Nano, lastSeen.String)
	return bars, fetchedAt, nil
}

func (s *Store) SaveMetricPoints(ctx context.Context, points []market.MetricPoint, fetchedAt time.Time) (ObservationStats, error) {
	stats := observationRangeMetrics(points)
	if len(points) == 0 {
		return stats, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin metric observations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stamp := fetchedAt.UTC().Format(time.RFC3339Nano)
	for _, point := range points {
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO market_metric_observations(id, metric, trade_date, value_fen, source, source_time, fetched_at, last_seen_at) VALUES(?,?,?,?,?,?,?,?)`,
			NewID("metric"), point.Metric, point.TradeDate, point.ValueFen, point.Source, point.SourceTime.UTC().Format(time.RFC3339Nano), stamp, stamp)
		if err != nil {
			return stats, fmt.Errorf("insert metric %s %s: %w", point.Metric, point.TradeDate, err)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			stats.Duplicates++
			if _, err := tx.ExecContext(ctx, `UPDATE market_metric_observations SET last_seen_at=? WHERE metric=? AND trade_date=? AND value_fen=? AND source=?`, stamp, point.Metric, point.TradeDate, point.ValueFen, point.Source); err != nil {
				return stats, fmt.Errorf("touch metric %s %s: %w", point.Metric, point.TradeDate, err)
			}
		} else {
			stats.Inserted++
		}
	}
	if err := appendMarketSyncAudit(ctx, tx, "market_metrics", stats, stamp); err != nil {
		return stats, err
	}
	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit metric observations: %w", err)
	}
	return stats, nil
}

func (s *Store) LatestMetricPoints(ctx context.Context, metricKind market.MetricKind, since string) ([]market.MetricPoint, time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT trade_date, metric, value_fen, source, source_time,
			ROW_NUMBER() OVER (PARTITION BY trade_date ORDER BY fetched_at DESC, rowid DESC) AS position
		FROM market_metric_observations WHERE metric=? AND trade_date>=?
	) SELECT trade_date, metric, value_fen, source, source_time FROM ranked WHERE position=1 ORDER BY trade_date`, metricKind, since)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query metric points: %w", err)
	}
	defer rows.Close()
	points := make([]market.MetricPoint, 0)
	for rows.Next() {
		var point market.MetricPoint
		var sourceTime string
		if err := rows.Scan(&point.TradeDate, &point.Metric, &point.ValueFen, &point.Source, &sourceTime); err != nil {
			return nil, time.Time{}, fmt.Errorf("scan metric point: %w", err)
		}
		point.SourceTime, _ = time.Parse(time.RFC3339Nano, sourceTime)
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("read metric points: %w", err)
	}
	var lastSeen sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(last_seen_at) FROM market_metric_observations WHERE metric=?`, metricKind).Scan(&lastSeen); err != nil {
		return nil, time.Time{}, fmt.Errorf("load metric freshness: %w", err)
	}
	fetchedAt, _ := time.Parse(time.RFC3339Nano, lastSeen.String)
	return points, fetchedAt, nil
}

func appendMarketSyncAudit(ctx context.Context, tx *sql.Tx, entityID string, stats ObservationStats, stamp string) error {
	after, _ := json.Marshal(stats)
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'market_data', ?, 'synced', NULL, ?, ?)`, NewID("audit"), entityID, string(after), stamp); err != nil {
		return fmt.Errorf("audit market data sync: %w", err)
	}
	return nil
}

func observationRangeDaily(bars []market.DailyBar) ObservationStats {
	stats := ObservationStats{}
	for _, bar := range bars {
		if stats.FromDate == "" || bar.TradeDate < stats.FromDate {
			stats.FromDate = bar.TradeDate
		}
		if bar.TradeDate > stats.ToDate {
			stats.ToDate = bar.TradeDate
		}
	}
	return stats
}

func observationRangeMetrics(points []market.MetricPoint) ObservationStats {
	stats := ObservationStats{}
	for _, point := range points {
		if stats.FromDate == "" || point.TradeDate < stats.FromDate {
			stats.FromDate = point.TradeDate
		}
		if point.TradeDate > stats.ToDate {
			stats.ToDate = point.TradeDate
		}
	}
	return stats
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
