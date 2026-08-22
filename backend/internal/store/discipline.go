package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

const (
	monitorSettingsKey = "monitor-settings"
	monitorStatusKey   = "monitor-status"
)

func (s *Store) CreatePreTradeConfirmation(ctx context.Context, planID, snapshotJSON string, startedAt, confirmedAt time.Time) (domain.PreTradeConfirmation, error) {
	if !json.Valid([]byte(snapshotJSON)) {
		return domain.PreTradeConfirmation{}, fmt.Errorf("计划确认快照格式无效")
	}
	confirmation := domain.PreTradeConfirmation{
		ID: NewID("pretrade"), PlanID: planID, PlanSnapshotJSON: snapshotJSON,
		StartedAt: startedAt.UTC(), ConfirmedAt: confirmedAt.UTC(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.PreTradeConfirmation{}, fmt.Errorf("begin pre-trade confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO pre_trade_confirmations(id, plan_id, plan_snapshot_json, started_at, confirmed_at) VALUES(?,?,?,?,?)`, confirmation.ID, confirmation.PlanID, confirmation.PlanSnapshotJSON, confirmation.StartedAt.Format(time.RFC3339Nano), confirmation.ConfirmedAt.Format(time.RFC3339Nano)); err != nil {
		return domain.PreTradeConfirmation{}, fmt.Errorf("insert pre-trade confirmation: %w", err)
	}
	if err := appendAudit(ctx, tx, "plan", planID, "pre_trade_confirmed", nil, snapshotJSON, confirmedAt); err != nil {
		return domain.PreTradeConfirmation{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PreTradeConfirmation{}, fmt.Errorf("commit pre-trade confirmation: %w", err)
	}
	return confirmation, nil
}

func (s *Store) LatestPreTradeConfirmation(ctx context.Context, planID string) (*domain.PreTradeConfirmation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, plan_id, plan_snapshot_json, started_at, confirmed_at FROM pre_trade_confirmations WHERE plan_id=? ORDER BY confirmed_at DESC LIMIT 1`, planID)
	confirmation, err := scanPreTradeConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest pre-trade confirmation: %w", err)
	}
	return &confirmation, nil
}

func (s *Store) CreatePriceAlertIfCrossed(ctx context.Context, alert domain.PriceAlertEvent) (bool, domain.PriceAlertEvent, error) {
	if !json.Valid([]byte(alert.PlanSnapshotJSON)) {
		return false, domain.PriceAlertEvent{}, fmt.Errorf("价格提醒快照格式无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, domain.PriceAlertEvent{}, fmt.Errorf("begin price alert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	active, err := alertState(ctx, tx, alert.PlanID, alert.Kind)
	if err != nil {
		return false, domain.PriceAlertEvent{}, err
	}
	if active {
		existing, err := scanPriceAlert(tx.QueryRowContext(ctx, `SELECT id, plan_id, instrument_id, kind, trigger_price_minor, threshold_minor, plan_snapshot_json, source, source_time, triggered_at, notified_at FROM price_alert_events WHERE plan_id=? AND kind=? ORDER BY triggered_at DESC LIMIT 1`, alert.PlanID, alert.Kind))
		if err != nil {
			return false, domain.PriceAlertEvent{}, fmt.Errorf("load active price alert: %w", err)
		}
		return false, existing, nil
	}
	alert.ID = NewID("alert")
	alert.SourceTime = alert.SourceTime.UTC()
	alert.TriggeredAt = alert.TriggeredAt.UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO price_alert_events(id, plan_id, instrument_id, kind, trigger_price_minor, threshold_minor, plan_snapshot_json, source, source_time, triggered_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, alert.ID, alert.PlanID, alert.InstrumentID, alert.Kind, alert.TriggerPriceMinor, alert.ThresholdMinor, alert.PlanSnapshotJSON, alert.Source, alert.SourceTime.Format(time.RFC3339Nano), alert.TriggeredAt.Format(time.RFC3339Nano)); err != nil {
		return false, domain.PriceAlertEvent{}, fmt.Errorf("insert price alert: %w", err)
	}
	if err := saveAlertState(ctx, tx, alert.PlanID, alert.Kind, true, alert.TriggeredAt); err != nil {
		return false, domain.PriceAlertEvent{}, err
	}
	after, _ := json.Marshal(map[string]any{"kind": alert.Kind, "triggerPriceMinor": alert.TriggerPriceMinor, "thresholdMinor": alert.ThresholdMinor, "source": alert.Source, "sourceTime": alert.SourceTime})
	if err := appendAudit(ctx, tx, "price_alert", alert.ID, "price_alert_triggered", nil, string(after), alert.TriggeredAt); err != nil {
		return false, domain.PriceAlertEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return false, domain.PriceAlertEvent{}, fmt.Errorf("commit price alert: %w", err)
	}
	return true, alert, nil
}

func (s *Store) RecordAlertState(ctx context.Context, planID string, kind domain.AlertKind, triggered bool, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveAlertState(ctx, tx, planID, kind, triggered, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert state: %w", err)
	}
	return nil
}

func (s *Store) ListPendingAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.plan_id, a.instrument_id, a.kind, a.trigger_price_minor, a.threshold_minor, a.plan_snapshot_json, a.source, a.source_time, a.triggered_at, a.notified_at FROM price_alert_events a LEFT JOIN position_review_events r ON r.alert_id=a.id WHERE r.id IS NULL ORDER BY a.triggered_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list pending price alerts: %w", err)
	}
	defer rows.Close()
	alerts := make([]domain.PriceAlertEvent, 0)
	for rows.Next() {
		alert, err := scanPriceAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending price alert: %w", err)
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (s *Store) ListUnnotifiedAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.plan_id, a.instrument_id, a.kind, a.trigger_price_minor, a.threshold_minor, a.plan_snapshot_json, a.source, a.source_time, a.triggered_at, a.notified_at FROM price_alert_events a LEFT JOIN position_review_events r ON r.alert_id=a.id WHERE a.notified_at IS NULL AND r.id IS NULL ORDER BY a.triggered_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list unnotified price alerts: %w", err)
	}
	defer rows.Close()
	alerts := make([]domain.PriceAlertEvent, 0)
	for rows.Next() {
		alert, err := scanPriceAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unnotified price alert: %w", err)
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (s *Store) MarkAlertNotified(ctx context.Context, id string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark alert notified: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE price_alert_events SET notified_at=? WHERE id=? AND notified_at IS NULL`, now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("mark price alert notified: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		if err := appendAudit(ctx, tx, "price_alert", id, "price_alert_notified", nil, `{"notified":true}`, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mark alert notified: %w", err)
	}
	return nil
}

func (s *Store) CreatePositionReview(ctx context.Context, alertID string, decision domain.ReviewDecision, reason string, now time.Time) (domain.PositionReviewEvent, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.PositionReviewEvent{}, fmt.Errorf("复核原因不能为空")
	}
	if !validReviewDecision(decision) {
		return domain.PositionReviewEvent{}, fmt.Errorf("复核决定无效")
	}
	review := domain.PositionReviewEvent{ID: NewID("review"), AlertID: alertID, Decision: decision, Reason: reason, CreatedAt: now.UTC()}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.PositionReviewEvent{}, fmt.Errorf("begin position review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO position_review_events(id, alert_id, decision, reason, created_at) VALUES(?,?,?,?,?)`, review.ID, review.AlertID, review.Decision, review.Reason, review.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return domain.PositionReviewEvent{}, fmt.Errorf("insert position review: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"decision": review.Decision, "reason": review.Reason})
	if err := appendAudit(ctx, tx, "price_alert", alertID, "position_reviewed", nil, string(after), review.CreatedAt); err != nil {
		return domain.PositionReviewEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PositionReviewEvent{}, fmt.Errorf("commit position review: %w", err)
	}
	return review, nil
}

func (s *Store) MonitorSettings(ctx context.Context) (domain.MonitorSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key=?`, monitorSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return domain.MonitorSettings{Interval: "10m"}, nil
	}
	if err != nil {
		return domain.MonitorSettings{}, fmt.Errorf("load monitor settings: %w", err)
	}
	var settings domain.MonitorSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil || !validMonitorInterval(settings.Interval) {
		return domain.MonitorSettings{}, fmt.Errorf("decode monitor settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SaveMonitorSettings(ctx context.Context, settings domain.MonitorSettings, now time.Time) error {
	if !validMonitorInterval(settings.Interval) {
		return fmt.Errorf("监控间隔仅支持 off、10m、15m 或 30m")
	}
	raw, _ := json.Marshal(settings)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin monitor settings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var before sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key=?`, monitorSettingsKey).Scan(&before); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load previous monitor settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key, value_json, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`, monitorSettingsKey, string(raw), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("save monitor settings: %w", err)
	}
	var beforeJSON *string
	if before.Valid {
		beforeJSON = &before.String
	}
	if err := appendAudit(ctx, tx, "monitor_settings", monitorSettingsKey, "monitor_settings_changed", beforeJSON, string(raw), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitor settings: %w", err)
	}
	return nil
}

func (s *Store) MonitorStatus(ctx context.Context) (domain.MonitorStatus, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key=?`, monitorStatusKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return domain.MonitorStatus{}, nil
	}
	if err != nil {
		return domain.MonitorStatus{}, fmt.Errorf("load monitor status: %w", err)
	}
	var status domain.MonitorStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return domain.MonitorStatus{}, fmt.Errorf("decode monitor status: %w", err)
	}
	return status, nil
}

func (s *Store) SaveMonitorStatus(ctx context.Context, status domain.MonitorStatus, now time.Time) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode monitor status: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO app_settings(key, value_json, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`, monitorStatusKey, string(raw), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("save monitor status: %w", err)
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanPreTradeConfirmation(row scanner) (domain.PreTradeConfirmation, error) {
	var item domain.PreTradeConfirmation
	var startedAt, confirmedAt string
	err := row.Scan(&item.ID, &item.PlanID, &item.PlanSnapshotJSON, &startedAt, &confirmedAt)
	if err != nil {
		return domain.PreTradeConfirmation{}, err
	}
	item.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
	item.ConfirmedAt, _ = time.Parse(time.RFC3339Nano, confirmedAt)
	return item, nil
}

func scanPriceAlert(row scanner) (domain.PriceAlertEvent, error) {
	var item domain.PriceAlertEvent
	var sourceTime, triggeredAt string
	var notifiedAt sql.NullString
	err := row.Scan(&item.ID, &item.PlanID, &item.InstrumentID, &item.Kind, &item.TriggerPriceMinor, &item.ThresholdMinor, &item.PlanSnapshotJSON, &item.Source, &sourceTime, &triggeredAt, &notifiedAt)
	if err != nil {
		return domain.PriceAlertEvent{}, err
	}
	item.SourceTime, _ = time.Parse(time.RFC3339Nano, sourceTime)
	item.TriggeredAt, _ = time.Parse(time.RFC3339Nano, triggeredAt)
	if notifiedAt.Valid {
		stamp, err := time.Parse(time.RFC3339Nano, notifiedAt.String)
		if err == nil {
			item.NotifiedAt = &stamp
		}
	}
	return item, nil
}

func alertState(ctx context.Context, tx *sql.Tx, planID string, kind domain.AlertKind) (bool, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key=?`, alertStateKey(planID, kind)).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load alert state: %w", err)
	}
	var state struct {
		Triggered bool `json:"triggered"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return false, fmt.Errorf("decode alert state: %w", err)
	}
	return state.Triggered, nil
}

func saveAlertState(ctx context.Context, tx *sql.Tx, planID string, kind domain.AlertKind, triggered bool, now time.Time) error {
	raw, _ := json.Marshal(struct {
		Triggered bool `json:"triggered"`
	}{Triggered: triggered})
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key, value_json, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`, alertStateKey(planID, kind), string(raw), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("save alert state: %w", err)
	}
	return nil
}

func alertStateKey(planID string, kind domain.AlertKind) string {
	return "monitor-alert-state:" + planID + ":" + string(kind)
}

func validReviewDecision(decision domain.ReviewDecision) bool {
	return decision == domain.ReviewHold || decision == domain.ReviewTrim || decision == domain.ReviewSell || decision == domain.ReviewWait
}

func validMonitorInterval(interval string) bool {
	return interval == "off" || interval == "10m" || interval == "15m" || interval == "30m"
}

func appendAudit(ctx context.Context, tx *sql.Tx, entityType, entityID, action string, beforeJSON *string, afterJSON string, now time.Time) error {
	var before any
	if beforeJSON != nil {
		before = *beforeJSON
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?,?,?,?,?,?,?)`, NewID("audit"), entityType, entityID, action, before, afterJSON, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}
