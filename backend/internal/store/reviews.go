package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type ReviewMetrics struct {
	CashFen              int64 `json:"cashFen"`
	ChinaTechExposureFen int64 `json:"chinaTechExposureFen"`
	CumulativeLossFen    int64 `json:"cumulativeLossFen"`
	ViolationCount       int   `json:"violationCount"`
}

type ReviewContent struct {
	ImpulseNotes      string `json:"impulseNotes"`
	NextAllowedAction string `json:"nextAllowedAction"`
}

type WeeklyReviewRow struct {
	ID                string        `json:"id"`
	PeriodStart       string        `json:"periodStart"`
	PeriodEnd         string        `json:"periodEnd"`
	Metrics           ReviewMetrics `json:"metrics"`
	UserContent       ReviewContent `json:"userContent"`
	DisciplineScoreBP int           `json:"disciplineScoreBP"`
	RuleVersionID     string        `json:"ruleVersionId"`
	SubmittedAt       time.Time     `json:"submittedAt"`
}

func (s *Store) SaveWeeklyReview(ctx context.Context, row WeeklyReviewRow) (WeeklyReviewRow, error) {
	metrics, _ := json.Marshal(row.Metrics)
	content, _ := json.Marshal(row.UserContent)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WeeklyReviewRow{}, fmt.Errorf("begin review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row.ID = NewID("review")
	stamp := row.SubmittedAt.UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO weekly_reviews(id, period_start, period_end, auto_metrics_json, user_content_json, discipline_score_bp, submitted_at, rule_version_id) VALUES(?,?,?,?,?,?,?,?)`, row.ID, row.PeriodStart, row.PeriodEnd, string(metrics), string(content), row.DisciplineScoreBP, stamp, row.RuleVersionID)
	if err != nil {
		return WeeklyReviewRow{}, fmt.Errorf("insert review: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'weekly_review', ?, 'submitted', NULL, ?, ?)`, NewID("audit"), row.ID, string(content), stamp); err != nil {
		return WeeklyReviewRow{}, fmt.Errorf("audit review: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WeeklyReviewRow{}, fmt.Errorf("commit review: %w", err)
	}
	return row, nil
}

func (s *Store) LatestWeeklyReview(ctx context.Context) (*WeeklyReviewRow, error) {
	var row WeeklyReviewRow
	var metrics, content, submitted string
	err := s.db.QueryRowContext(ctx, `SELECT id, period_start, period_end, auto_metrics_json, user_content_json, discipline_score_bp, rule_version_id, submitted_at FROM weekly_reviews ORDER BY period_end DESC LIMIT 1`).Scan(&row.ID, &row.PeriodStart, &row.PeriodEnd, &metrics, &content, &row.DisciplineScoreBP, &row.RuleVersionID, &submitted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load review: %w", err)
	}
	if err := json.Unmarshal([]byte(metrics), &row.Metrics); err != nil {
		return nil, fmt.Errorf("decode review metrics: %w", err)
	}
	if err := json.Unmarshal([]byte(content), &row.UserContent); err != nil {
		return nil, fmt.Errorf("decode review content: %w", err)
	}
	row.SubmittedAt, _ = time.Parse(time.RFC3339Nano, submitted)
	return &row, nil
}
