package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func (s *Store) DisciplineProgress(ctx context.Context, now time.Time) (int, int, error) {
	var firstAlibabaBuy sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT MIN(e.executed_at)
		FROM execution_events e
		JOIN instruments i ON i.id=e.instrument_id
		WHERE i.code='9988.HK' AND e.event_type='buy'
		AND NOT EXISTS (SELECT 1 FROM execution_events reversal WHERE reversal.original_event_id=e.id AND reversal.event_type='reversal')`).Scan(&firstAlibabaBuy)
	if err != nil {
		return 0, 0, fmt.Errorf("load Alibaba observation start: %w", err)
	}
	days := 0
	if firstAlibabaBuy.Valid && len(firstAlibabaBuy.String) >= 10 {
		if err := s.db.QueryRowContext(ctx, `SELECT count(DISTINCT trade_date) FROM market_snapshots WHERE ranking_kind='stock' AND status='success' AND trade_date>=? AND trade_date<=?`, firstAlibabaBuy.String[:10], now.UTC().Format("2006-01-02")).Scan(&days); err != nil {
			return 0, 0, fmt.Errorf("count Alibaba observation days: %w", err)
		}
	}
	var score sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT discipline_score_bp FROM weekly_reviews WHERE submitted_at<=? ORDER BY submitted_at DESC LIMIT 1`, now.UTC().Format(time.RFC3339Nano)).Scan(&score)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, fmt.Errorf("load latest discipline score: %w", err)
	}
	return days, int(score.Int64), nil
}

type InstrumentRow struct {
	ID          string `json:"id"`
	Market      string `json:"market"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	AssetType   string `json:"assetType"`
	Currency    string `json:"currency"`
	LotSize     int    `json:"lotSize"`
	IsChinaTech bool   `json:"isChinaTech"`
}

func (s *Store) ListInstruments(ctx context.Context) ([]InstrumentRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, market, code, name, asset_type, currency, lot_size, is_china_tech FROM instruments WHERE status='active' ORDER BY market, code`)
	if err != nil {
		return nil, fmt.Errorf("list instruments: %w", err)
	}
	defer rows.Close()
	var instruments []InstrumentRow
	for rows.Next() {
		var item InstrumentRow
		var tech int
		if err := rows.Scan(&item.ID, &item.Market, &item.Code, &item.Name, &item.AssetType, &item.Currency, &item.LotSize, &tech); err != nil {
			return nil, fmt.Errorf("scan instrument: %w", err)
		}
		item.IsChinaTech = tech == 1
		instruments = append(instruments, item)
	}
	return instruments, rows.Err()
}

type AuditRow struct {
	ID         string `json:"id"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Action     string `json:"action"`
	BeforeJSON string `json:"beforeJson,omitempty"`
	AfterJSON  string `json:"afterJson,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, entity_type, entity_id, action, COALESCE(before_json,''), COALESCE(after_json,''), created_at FROM audit_events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	var entries []AuditRow
	for rows.Next() {
		var item AuditRow
		if err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.Action, &item.BeforeJSON, &item.AfterJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		entries = append(entries, item)
	}
	return entries, rows.Err()
}

func (s *Store) CountViolations(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM violation_events`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count violations: %w", err)
	}
	return count, nil
}

func (s *Store) LatestSuccessfulMarketFetch(ctx context.Context) *time.Time {
	var stamp string
	if err := s.db.QueryRowContext(ctx, `SELECT fetched_at FROM market_snapshots WHERE status='success' ORDER BY fetched_at DESC LIMIT 1`).Scan(&stamp); err != nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return nil
	}
	return &parsed
}

type LatestQuoteRow struct {
	InstrumentID string
	Currency     string
	PriceMinor   int64
	PriceAt      time.Time
	Source       string
}

func (s *Store) LatestQuotes(ctx context.Context) (map[string]LatestQuoteRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, currency, latest_price_minor, latest_price_at, latest_price_source FROM instruments WHERE latest_price_minor IS NOT NULL AND latest_price_at IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list latest quotes: %w", err)
	}
	defer rows.Close()
	quotes := make(map[string]LatestQuoteRow)
	for rows.Next() {
		var quote LatestQuoteRow
		var stamp string
		if err := rows.Scan(&quote.InstrumentID, &quote.Currency, &quote.PriceMinor, &stamp, &quote.Source); err != nil {
			return nil, fmt.Errorf("scan latest quote: %w", err)
		}
		quote.PriceAt, _ = time.Parse(time.RFC3339Nano, stamp)
		quotes[quote.InstrumentID] = quote
	}
	return quotes, rows.Err()
}

func (s *Store) LatestQualifiedBuyPlanID(ctx context.Context, instrumentID string) (string, error) {
	var planID string
	err := s.db.QueryRowContext(ctx, `SELECT e.plan_id FROM execution_events e
		JOIN trade_plans p ON p.id=e.plan_id
		WHERE e.instrument_id=? AND e.event_type='buy' AND e.plan_id IS NOT NULL AND p.status='qualified'
		AND NOT EXISTS (SELECT 1 FROM execution_events reversal WHERE reversal.original_event_id=e.id AND reversal.event_type='reversal')
		ORDER BY e.executed_at DESC, e.created_at DESC LIMIT 1`, instrumentID).Scan(&planID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load latest qualified buy plan: %w", err)
	}
	return planID, nil
}

func (s *Store) InstrumentKey(ctx context.Context, instrumentID string) (market.InstrumentKey, error) {
	var key market.InstrumentKey
	err := s.db.QueryRowContext(ctx, `SELECT market, code FROM instruments WHERE id=? AND status='active'`, instrumentID).Scan(&key.Market, &key.Code)
	if err != nil {
		return market.InstrumentKey{}, fmt.Errorf("load instrument key: %w", err)
	}
	return key, nil
}
