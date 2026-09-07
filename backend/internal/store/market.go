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
	ID              string                `json:"id"`
	TradeDate       string                `json:"tradeDate"`
	Kind            market.RankingKind    `json:"kind"`
	Mode            market.SnapshotMode   `json:"mode"`
	Source          string                `json:"source"`
	FetchedAt       time.Time             `json:"fetchedAt"`
	Version         int                   `json:"version"`
	Entries         []market.Quote        `json:"entries"`
	Quality         market.RankingQuality `json:"quality"`
	UniverseVersion string                `json:"universeVersion,omitempty"`
	QualityReason   string                `json:"qualityReason,omitempty"`
}

type MarketRefreshStatusRow struct {
	Mode             market.SnapshotMode               `json:"mode"`
	LastAttemptAt    time.Time                         `json:"lastAttemptAt"`
	LastSuccessfulAt *time.Time                        `json:"lastSuccessfulAt,omitempty"`
	Errors           map[string]string                 `json:"errors,omitempty"`
	Components       map[string]market.ComponentHealth `json:"components,omitempty"`
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
	return s.SaveMarketSnapshotWithQuality(ctx, kind, quotes, source, fetchedAt, market.RankingQualityInfo{Quality: market.RankingQualityLegacyUnverified, Reason: "未提供收盘完整性证明"})
}

func (s *Store) SaveMarketSnapshotWithQuality(ctx context.Context, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time, quality market.RankingQualityInfo) (MarketSnapshotRow, error) {
	return s.saveMarketSnapshotWithQuality(ctx, market.SnapshotModeClose, kind, quotes, source, fetchedAt, quality)
}

func (s *Store) SaveMarketSnapshotForMode(ctx context.Context, mode market.SnapshotMode, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time) (MarketSnapshotRow, error) {
	return s.saveMarketSnapshotWithQuality(ctx, mode, kind, quotes, source, fetchedAt, market.RankingQualityInfo{Quality: market.RankingQualityLegacyUnverified, Reason: "非收盘比较快照"})
}

func (s *Store) saveMarketSnapshotWithQuality(ctx context.Context, mode market.SnapshotMode, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time, quality market.RankingQualityInfo) (MarketSnapshotRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("begin market snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row, err := saveMarketSnapshotTx(ctx, tx, mode, kind, quotes, source, fetchedAt, quality)
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
		row, err := saveMarketSnapshotTx(ctx, tx, market.SnapshotModeClose, kind, quotes, source, fetchedAt, market.RankingQualityInfo{Quality: market.RankingQualityManualUnverified, Reason: "CSV 导入未验证完整市场范围"})
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

func saveMarketSnapshotTx(ctx context.Context, tx *sql.Tx, mode market.SnapshotMode, kind market.RankingKind, quotes []market.Quote, source string, fetchedAt time.Time, quality market.RankingQualityInfo) (MarketSnapshotRow, error) {
	if mode != market.SnapshotModeClose && mode != market.SnapshotModeLive {
		return MarketSnapshotRow{}, fmt.Errorf("unsupported market snapshot mode %q", mode)
	}
	eligibleQuotes := make([]market.Quote, 0, len(quotes))
	for _, quote := range quotes {
		if kind == market.KindETF && !market.IsEligibleETF(quote.Code, quote.Name) {
			continue
		}
		eligibleQuotes = append(eligibleQuotes, quote)
	}
	if len(eligibleQuotes) == 0 {
		return MarketSnapshotRow{}, fmt.Errorf("%s 榜单没有有效数据", kind)
	}
	tradeDate := eligibleQuotes[0].TradeDate
	for _, quote := range eligibleQuotes {
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO market_snapshots(id, trade_date, ranking_kind, snapshot_mode, source, fetched_at, status, version) VALUES(?,?,?,?,?,?,'success',?)`, id, tradeDate, kind, mode, source, stamp, version); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("insert market snapshot: %w", err)
	}
	if quality.Quality == "" {
		quality.Quality = market.RankingQualityLegacyUnverified
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO market_snapshot_quality(snapshot_id, comparison_quality, universe_version, quality_reason) VALUES(?,?,?,?)`, id, quality.Quality, quality.UniverseVersion, quality.Reason); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("save market snapshot quality: %w", err)
	}
	for index, quote := range eligibleQuotes {
		currency := market.CurrencyForMarket(quote.Market)
		lotSize := market.DefaultLotSize(quote.Market)
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'market_snapshot', ?, 'imported', NULL, ?, ?)`, NewID("audit"), id, fmt.Sprintf(`{"kind":%q,"rows":%d}`, kind, len(eligibleQuotes)), stamp); err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("audit market snapshot: %w", err)
	}
	return MarketSnapshotRow{ID: id, TradeDate: tradeDate, Kind: kind, Mode: mode, Source: source, FetchedAt: fetchedAt.UTC(), Version: version, Entries: eligibleQuotes, Quality: quality.Quality, UniverseVersion: quality.UniverseVersion, QualityReason: quality.Reason}, nil
}

func (s *Store) LatestMarketSnapshot(ctx context.Context, kind market.RankingKind) (MarketSnapshotRow, error) {
	return s.LatestMarketSnapshotForMode(ctx, market.SnapshotModeClose, kind)
}

func (s *Store) LatestMarketSnapshotForMode(ctx context.Context, mode market.SnapshotMode, kind market.RankingKind) (MarketSnapshotRow, error) {
	var row MarketSnapshotRow
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT s.id, s.trade_date, s.source, s.fetched_at, s.version, COALESCE(q.comparison_quality,'legacy_unverified'), COALESCE(q.universe_version,''), COALESCE(q.quality_reason,'历史快照缺少完整性证明') FROM market_snapshots s LEFT JOIN market_snapshot_quality q ON q.snapshot_id=s.id WHERE s.ranking_kind=? AND s.snapshot_mode=? AND s.status='success' ORDER BY s.trade_date DESC, s.version DESC LIMIT 1`, kind, mode).Scan(&row.ID, &row.TradeDate, &row.Source, &fetched, &row.Version, &row.Quality, &row.UniverseVersion, &row.QualityReason)
	if err == sql.ErrNoRows {
		row.Kind = kind
		row.Mode = mode
		row.Entries = []market.Quote{}
		return row, nil
	}
	if err != nil {
		return MarketSnapshotRow{}, fmt.Errorf("load latest market snapshot: %w", err)
	}
	row.Kind = kind
	row.Mode = mode
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

func (s *Store) SaveMarketRefreshStatus(ctx context.Context, mode market.SnapshotMode, attemptedAt time.Time, succeeded bool, errors map[string]string) (MarketRefreshStatusRow, error) {
	return s.SaveMarketRefreshStatusWithHealth(ctx, mode, attemptedAt, succeeded, errors, nil)
}

func (s *Store) SaveMarketRefreshStatusWithHealth(ctx context.Context, mode market.SnapshotMode, attemptedAt time.Time, succeeded bool, errors map[string]string, components map[string]market.ComponentHealth) (MarketRefreshStatusRow, error) {
	if mode != market.SnapshotModeClose && mode != market.SnapshotModeLive {
		return MarketRefreshStatusRow{}, fmt.Errorf("unsupported market refresh mode %q", mode)
	}
	if errors == nil {
		errors = map[string]string{}
	}
	if components == nil {
		components = map[string]market.ComponentHealth{}
	}
	rawErrors, err := json.Marshal(errors)
	if err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("encode market refresh errors: %w", err)
	}
	rawHealth, err := json.Marshal(components)
	if err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("encode market refresh health: %w", err)
	}
	attemptedAt = attemptedAt.UTC()
	var successfulAt any
	if succeeded {
		successfulAt = attemptedAt.Format(time.RFC3339Nano)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO market_refresh_status(snapshot_mode, last_attempt_at, last_success_at, errors_json, health_json)
		VALUES(?,?,?,?,?)
		ON CONFLICT(snapshot_mode) DO UPDATE SET
			last_attempt_at=excluded.last_attempt_at,
			last_success_at=CASE WHEN excluded.last_success_at IS NULL THEN market_refresh_status.last_success_at ELSE excluded.last_success_at END,
			errors_json=excluded.errors_json,
			health_json=excluded.health_json`, mode, attemptedAt.Format(time.RFC3339Nano), successfulAt, string(rawErrors), string(rawHealth)); err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("save market refresh status: %w", err)
	}
	return s.MarketRefreshStatus(ctx, mode)
}

func (s *Store) MarketRefreshStatus(ctx context.Context, mode market.SnapshotMode) (MarketRefreshStatusRow, error) {
	row := MarketRefreshStatusRow{Mode: mode, Errors: map[string]string{}, Components: map[string]market.ComponentHealth{}}
	var attempted, successful sql.NullString
	var rawErrors, rawHealth string
	err := s.db.QueryRowContext(ctx, `SELECT last_attempt_at, last_success_at, errors_json, health_json FROM market_refresh_status WHERE snapshot_mode=?`, mode).Scan(&attempted, &successful, &rawErrors, &rawHealth)
	if err == sql.ErrNoRows {
		return row, nil
	}
	if err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("load market refresh status: %w", err)
	}
	row.LastAttemptAt, _ = time.Parse(time.RFC3339Nano, attempted.String)
	if successful.Valid {
		stamp, _ := time.Parse(time.RFC3339Nano, successful.String)
		row.LastSuccessfulAt = &stamp
	}
	if err := json.Unmarshal([]byte(rawErrors), &row.Errors); err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("decode market refresh status: %w", err)
	}
	if err := json.Unmarshal([]byte(rawHealth), &row.Components); err != nil {
		return MarketRefreshStatusRow{}, fmt.Errorf("decode market refresh health: %w", err)
	}
	return row, nil
}
