package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

type RankingDate struct {
	TradeDate string                `json:"tradeDate"`
	Quality   market.RankingQuality `json:"quality"`
}

// ListRankingDates returns one display version per close date. A verified
// correction wins; otherwise the newest saved version remains reviewable.
func (s *Store) ListRankingDates(ctx context.Context, kind market.RankingKind) ([]RankingDate, error) {
	rows, err := s.db.QueryContext(ctx, `WITH choices AS (
		SELECT s.trade_date, COALESCE(q.comparison_quality,'legacy_unverified') quality,
		ROW_NUMBER() OVER (PARTITION BY s.trade_date ORDER BY CASE COALESCE(q.comparison_quality,'legacy_unverified') WHEN 'verified_close' THEN 0 ELSE 1 END, s.version DESC) position
		FROM market_snapshots s LEFT JOIN market_snapshot_quality q ON q.snapshot_id=s.id
		WHERE s.ranking_kind=? AND s.snapshot_mode='close' AND s.status='success'
	) SELECT trade_date, quality FROM choices WHERE position=1 ORDER BY trade_date DESC`, kind)
	if err != nil {
		return nil, fmt.Errorf("list ranking dates: %w", err)
	}
	defer rows.Close()
	result := []RankingDate{}
	for rows.Next() {
		var row RankingDate
		if err := rows.Scan(&row.TradeDate, &row.Quality); err != nil {
			return nil, fmt.Errorf("scan ranking date: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) RankingSnapshotAt(ctx context.Context, kind market.RankingKind, date string) (MarketSnapshotRow, error) {
	var row MarketSnapshotRow
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT s.id,s.trade_date,s.source,s.fetched_at,s.version,COALESCE(q.comparison_quality,'legacy_unverified'),COALESCE(q.universe_version,''),COALESCE(q.quality_reason,'历史快照缺少完整性证明')
		FROM market_snapshots s LEFT JOIN market_snapshot_quality q ON q.snapshot_id=s.id
		WHERE s.ranking_kind=? AND s.snapshot_mode='close' AND s.status='success' AND s.trade_date=?
		ORDER BY CASE COALESCE(q.comparison_quality,'legacy_unverified') WHEN 'verified_close' THEN 0 ELSE 1 END,s.version DESC LIMIT 1`, kind, date).
		Scan(&row.ID, &row.TradeDate, &row.Source, &fetched, &row.Version, &row.Quality, &row.UniverseVersion, &row.QualityReason)
	if err == sql.ErrNoRows {
		return MarketSnapshotRow{}, sql.ErrNoRows
	}
	if err != nil {
		return row, fmt.Errorf("load ranking snapshot: %w", err)
	}
	row.Kind, row.Mode = kind, market.SnapshotModeClose
	row.FetchedAt, _ = time.Parse(time.RFC3339Nano, fetched)
	entries, err := s.rankingEntries(ctx, row.ID, row.Source)
	if err != nil {
		return row, err
	}
	row.Entries = entries
	return row, nil
}

func (s *Store) rankingEntries(ctx context.Context, snapshotID, source string) ([]market.Quote, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT trade_date,market,code,name,asset_type,close_minor,change_bp,turnover_fen,COALESCE(source_time,'') FROM market_rank_entries WHERE snapshot_id=? ORDER BY rank`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("load ranking entries: %w", err)
	}
	defer rows.Close()
	entries := []market.Quote{}
	for rows.Next() {
		var quote market.Quote
		var sourceTime string
		if err := rows.Scan(&quote.TradeDate, &quote.Market, &quote.Code, &quote.Name, &quote.AssetType, &quote.CloseMinor, &quote.ChangeBP, &quote.TurnoverFen, &sourceTime); err != nil {
			return nil, fmt.Errorf("scan ranking entry: %w", err)
		}
		quote.Source = source
		quote.SourceTime, _ = time.Parse(time.RFC3339Nano, sourceTime)
		entries = append(entries, quote)
	}
	return entries, rows.Err()
}
