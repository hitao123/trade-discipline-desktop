package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

type CashEventRow struct {
	ID              string    `json:"id"`
	EventType       string    `json:"eventType"`
	AmountFen       int64     `json:"amountFen"`
	Reason          string    `json:"reason"`
	OccurredAt      time.Time `json:"occurredAt"`
	OriginalEventID string    `json:"originalEventId,omitempty"`
}

type AppendCashEventInput struct {
	EventType       string
	AmountFen       int64
	Reason          string
	OccurredAt      time.Time
	OriginalEventID string
}

func (s *Store) AppendCashEvent(ctx context.Context, input AppendCashEventInput) (CashEventRow, error) {
	if input.OccurredAt.IsZero() {
		return CashEventRow{}, fmt.Errorf("资金变动时间无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CashEventRow{}, fmt.Errorf("begin cash event: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id := NewID("cash")
	stamp := input.OccurredAt.UTC().Format(time.RFC3339Nano)
	created := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO cash_events(id, event_type, amount_fen, reason, occurred_at, original_event_id) VALUES(?,?,?,?,?,?)`,
		id, input.EventType, input.AmountFen, input.Reason, stamp, nullable(input.OriginalEventID)); err != nil {
		return CashEventRow{}, fmt.Errorf("insert cash event: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"eventType": input.EventType, "amountFen": input.AmountFen, "reason": input.Reason})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'cash_event', ?, 'recorded', NULL, ?, ?)`, NewID("audit"), id, string(after), created); err != nil {
		return CashEventRow{}, fmt.Errorf("audit cash event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CashEventRow{}, fmt.Errorf("commit cash event: %w", err)
	}
	return CashEventRow{ID: id, EventType: input.EventType, AmountFen: input.AmountFen, Reason: input.Reason, OccurredAt: input.OccurredAt.UTC(), OriginalEventID: input.OriginalEventID}, nil
}

func (s *Store) ListCashEvents(ctx context.Context) ([]CashEventRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_type, amount_fen, reason, occurred_at, COALESCE(original_event_id,'') FROM cash_events ORDER BY occurred_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list cash events: %w", err)
	}
	defer rows.Close()
	items := make([]CashEventRow, 0)
	for rows.Next() {
		var item CashEventRow
		var occurred string
		if err := rows.Scan(&item.ID, &item.EventType, &item.AmountFen, &item.Reason, &occurred, &item.OriginalEventID); err != nil {
			return nil, fmt.Errorf("scan cash event: %w", err)
		}
		item.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) LoadCashEvents(ctx context.Context) ([]domain.CashEvent, error) {
	rows, err := s.ListCashEvents(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]domain.CashEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, domain.CashEvent{
			ID: row.ID, OriginalEventID: row.OriginalEventID, AmountFen: row.AmountFen, OccurredAt: row.OccurredAt,
		})
	}
	return events, nil
}

func (s *Store) CashEvent(ctx context.Context, id string) (CashEventRow, error) {
	var item CashEventRow
	var occurred string
	err := s.db.QueryRowContext(ctx, `SELECT id, event_type, amount_fen, reason, occurred_at, COALESCE(original_event_id,'') FROM cash_events WHERE id=?`, id).
		Scan(&item.ID, &item.EventType, &item.AmountFen, &item.Reason, &occurred, &item.OriginalEventID)
	if err != nil {
		return CashEventRow{}, fmt.Errorf("load cash event: %w", err)
	}
	item.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
	return item, nil
}

func ExternalCapitalFen(baseline int64, events []CashEventRow) int64 {
	reversed := make(map[string]bool, len(events))
	for _, event := range events {
		if event.OriginalEventID != "" {
			reversed[event.OriginalEventID] = true
		}
	}
	total := baseline
	for _, event := range events {
		if event.OriginalEventID != "" || reversed[event.ID] {
			continue
		}
		if event.EventType == "deposit" || event.EventType == "withdrawal" {
			total += event.AmountFen
		}
	}
	return total
}
