package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type PostTradeReview struct {
	ID          string     `json:"id"`
	ExecutionID string     `json:"executionId"`
	Status      string     `json:"status"`
	Note        string     `json:"note"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type ExecutionFXObservation struct {
	ID            string    `json:"id"`
	ExecutionID   string    `json:"executionId"`
	BaseCurrency  string    `json:"baseCurrency"`
	QuoteCurrency string    `json:"quoteCurrency"`
	RateMinor     int64     `json:"rateMinor"`
	Source        string    `json:"source"`
	SourceTime    time.Time `json:"sourceTime"`
	ObservedAt    time.Time `json:"observedAt"`
}

func (s *Store) PostTradeReview(ctx context.Context, executionID string) (PostTradeReview, error) {
	return scanPostTradeReview(s.db.QueryRowContext(ctx, `SELECT id, execution_id, status, note, created_at, completed_at FROM post_trade_reviews WHERE execution_id=?`, executionID))
}

func (s *Store) PendingPostTradeReviews(ctx context.Context, periodStart, periodEnd time.Time) ([]PostTradeReview, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.execution_id, r.status, r.note, r.created_at, r.completed_at
		FROM post_trade_reviews r JOIN execution_events e ON e.id=r.execution_id
		WHERE r.status='pending' AND e.executed_at>=? AND e.executed_at<=?
		ORDER BY e.executed_at, r.created_at`, periodStart.UTC().Format(time.RFC3339Nano), periodEnd.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("list pending post-trade reviews: %w", err)
	}
	defer rows.Close()
	items := make([]PostTradeReview, 0)
	for rows.Next() {
		item, err := scanPostTradeReview(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CompletePostTradeReview(ctx context.Context, executionID, note string, completedAt time.Time) (PostTradeReview, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return PostTradeReview{}, fmt.Errorf("事后说明不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PostTradeReview{}, fmt.Errorf("begin post-trade review completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var reviewID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM post_trade_reviews WHERE execution_id=? AND status='pending'`, executionID).Scan(&reviewID); err != nil {
		return PostTradeReview{}, fmt.Errorf("待复盘记录不存在或已经完成")
	}
	result, err := tx.ExecContext(ctx, `UPDATE post_trade_reviews SET status='completed', note=?, completed_at=? WHERE id=? AND status='pending'`, note, completedAt.UTC().Format(time.RFC3339Nano), reviewID)
	if err != nil {
		return PostTradeReview{}, fmt.Errorf("complete post-trade review: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return PostTradeReview{}, fmt.Errorf("待复盘记录不存在或已经完成")
	}
	afterJSON, _ := json.Marshal(map[string]any{"status": "completed", "note": note, "executionId": executionID})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'post_trade_review', ?, 'completed', NULL, ?, ?)`, NewID("audit"), reviewID, string(afterJSON), completedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return PostTradeReview{}, fmt.Errorf("audit post-trade review completion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PostTradeReview{}, fmt.Errorf("commit post-trade review completion: %w", err)
	}
	return s.PostTradeReview(ctx, executionID)
}

func (s *Store) SaveExecutionFXObservation(ctx context.Context, observation ExecutionFXObservation) (ExecutionFXObservation, error) {
	if observation.ExecutionID == "" || observation.BaseCurrency == "" || observation.QuoteCurrency == "" || observation.Source == "" || observation.RateMinor <= 0 || observation.SourceTime.IsZero() {
		return ExecutionFXObservation{}, fmt.Errorf("参考汇率记录不完整")
	}
	if observation.ID == "" {
		observation.ID = NewID("fx-observation")
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO execution_fx_observations(id, execution_id, base_currency, quote_currency, rate_minor, source, source_time, observed_at) VALUES(?,?,?,?,?,?,?,?)`, observation.ID, observation.ExecutionID, observation.BaseCurrency, observation.QuoteCurrency, observation.RateMinor, observation.Source, observation.SourceTime.UTC().Format(time.RFC3339Nano), observation.ObservedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return ExecutionFXObservation{}, fmt.Errorf("save execution FX observation: %w", err)
	}
	return observation, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPostTradeReview(row rowScanner) (PostTradeReview, error) {
	var item PostTradeReview
	var createdAt string
	var completedAt sql.NullString
	if err := row.Scan(&item.ID, &item.ExecutionID, &item.Status, &item.Note, &createdAt, &completedAt); err != nil {
		return PostTradeReview{}, fmt.Errorf("scan post-trade review: %w", err)
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if completedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		item.CompletedAt = &parsed
	}
	return item, nil
}
