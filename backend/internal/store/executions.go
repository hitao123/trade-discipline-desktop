package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

type CooldownInput struct {
	Reason         string    `json:"reason"`
	Severity       string    `json:"severity"`
	StartsAt       time.Time `json:"startsAt"`
	ExpectedEndsAt time.Time `json:"expectedEndsAt"`
	AllowedActions []string  `json:"allowedActions"`
}

type AppendExecutionInput struct {
	Event               domain.ExecutionEvent
	RuleVersionID       string
	Classification      string
	ViolationCode       string
	ViolationFacts      any
	Cooldown            *CooldownInput
	Evidence            string
	BrokerReference     string
	ReferencePriceMinor int64
	ReferencePriceAt    time.Time
	EmotionJSON         string
	QuickRecord         bool
}

type AppendExecutionResult struct {
	ExecutionID string
	ViolationID string
	CooldownID  string
}

type AppendReversalResult struct {
	ExecutionID  string
	InstrumentID string
}

type AppendCorrectionResult struct {
	ReversalID  string
	Replacement AppendExecutionResult
}

type ExecutionRecord struct {
	ID                      string                  `json:"id"`
	PlanID                  string                  `json:"planId,omitempty"`
	InstrumentID            string                  `json:"instrumentId"`
	Code                    string                  `json:"code"`
	Name                    string                  `json:"name"`
	Side                    string                  `json:"side"`
	Quantity                int                     `json:"quantity"`
	LocalPriceMinor         int64                   `json:"localPriceMinor"`
	LocalPriceTenThousandth int64                   `json:"localPriceTenThousandth"`
	LocalAmountMinor        int64                   `json:"localAmountMinor"`
	SettlementFen           int64                   `json:"settlementFen"`
	ExitCode                string                  `json:"exitCode,omitempty"`
	Evidence                string                  `json:"evidence,omitempty"`
	BrokerReference         string                  `json:"brokerReference,omitempty"`
	Emotion                 domain.ExecutionEmotion `json:"emotion"`
	ExecutedAt              time.Time               `json:"executedAt"`
	QuickRecord             bool                    `json:"quickRecord"`
}

func NewID(prefix string) string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("secure random unavailable: %v", err))
	}
	return prefix + "-" + hex.EncodeToString(bytes[:])
}

func (s *Store) CurrentRuleVersionID(ctx context.Context) (string, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM rule_versions ORDER BY version DESC LIMIT 1`).Scan(&id); err != nil {
		return "", fmt.Errorf("load current rule id: %w", err)
	}
	return id, nil
}

func (s *Store) Instrument(ctx context.Context, id string) (code string, lotSize int, isChinaTech bool, err error) {
	var tech int
	err = s.db.QueryRowContext(ctx, `SELECT code, lot_size, is_china_tech FROM instruments WHERE id=? AND status='active'`, id).Scan(&code, &lotSize, &tech)
	if err != nil {
		return "", 0, false, fmt.Errorf("load instrument: %w", err)
	}
	return code, lotSize, tech == 1, nil
}

func (s *Store) AppendExecution(ctx context.Context, input AppendExecutionInput) (AppendExecutionResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendExecutionResult{}, fmt.Errorf("begin execution: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := appendExecutionTx(ctx, tx, input, time.Now().UTC())
	if err != nil {
		return AppendExecutionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppendExecutionResult{}, fmt.Errorf("commit execution: %w", err)
	}
	return result, nil
}

func appendExecutionTx(ctx context.Context, tx *sql.Tx, input AppendExecutionInput, now time.Time) (AppendExecutionResult, error) {
	eventID := input.Event.ID
	if eventID == "" {
		eventID = NewID("execution")
	}
	createdAt := now.UTC().Format(time.RFC3339Nano)
	executedAt := input.Event.ExecutedAt.UTC().Format(time.RFC3339Nano)
	if input.EmotionJSON == "" {
		input.EmotionJSON = "{}"
	}
	var original any
	if input.Event.OriginalEventID != "" {
		original = input.Event.OriginalEventID
	}
	var plan any
	if input.Event.PlanID != "" {
		plan = input.Event.PlanID
	}
	var referenceAt any
	if !input.ReferencePriceAt.IsZero() {
		referenceAt = input.ReferencePriceAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO execution_events(
		id, original_event_id, event_type, plan_id, instrument_id, rule_version_id, quantity,
		local_price_minor, local_price_ten_thousandth, local_amount_minor, settlement_fen, exit_code, evidence,
		broker_reference, reference_price_minor, reference_price_at, emotion_json, executed_at, created_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		eventID, original, input.Event.EventType, plan, input.Event.InstrumentID, input.RuleVersionID, input.Event.Quantity,
		input.Event.LocalPriceMinor, input.Event.LocalPriceTenThousandth, input.Event.LocalAmountMinor, input.Event.SettlementFen, nullable(input.Event.ExitCode), nullable(input.Evidence),
		nullable(input.BrokerReference), nullableInt(input.ReferencePriceMinor), referenceAt, input.EmotionJSON, executedAt, createdAt,
	)
	if err != nil {
		return AppendExecutionResult{}, fmt.Errorf("insert execution: %w", err)
	}

	result := AppendExecutionResult{ExecutionID: eventID}
	var emotion any
	_ = json.Unmarshal([]byte(input.EmotionJSON), &emotion)
	afterJSON, _ := json.Marshal(map[string]any{"classification": input.Classification, "settlementFen": input.Event.SettlementFen, "quantity": input.Event.Quantity, "emotion": emotion})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'execution', ?, 'recorded', NULL, ?, ?)`, NewID("audit"), eventID, string(afterJSON), createdAt); err != nil {
		return AppendExecutionResult{}, fmt.Errorf("audit execution: %w", err)
	}
	if input.QuickRecord || input.ViolationCode != "" || input.Event.EventType == domain.ExecutionSell {
		if _, err := tx.ExecContext(ctx, `INSERT INTO post_trade_reviews(id, execution_id, status, note, created_at) VALUES(?, ?, 'pending', '', ?)`, NewID("post-review"), eventID, createdAt); err != nil {
			return AppendExecutionResult{}, fmt.Errorf("create pending post-trade review: %w", err)
		}
	}
	if input.ViolationCode != "" {
		facts, err := json.Marshal(input.ViolationFacts)
		if err != nil {
			return AppendExecutionResult{}, fmt.Errorf("encode violation facts: %w", err)
		}
		result.ViolationID = NewID("violation")
		if _, err := tx.ExecContext(ctx, `INSERT INTO violation_events(id, plan_id, execution_id, rule_code, severity, fact_snapshot_json, action, occurred_at) VALUES(?,?,?,?,?,?,?,?)`, result.ViolationID, plan, eventID, input.ViolationCode, "serious", string(facts), "启动冷静期并要求复盘", executedAt); err != nil {
			return AppendExecutionResult{}, fmt.Errorf("insert violation: %w", err)
		}
	}
	if input.Cooldown != nil {
		allowed, _ := json.Marshal(input.Cooldown.AllowedActions)
		result.CooldownID = NewID("cooldown")
		if _, err := tx.ExecContext(ctx, `INSERT INTO cooldown_periods(id, execution_id, reason, severity, starts_at, expected_ends_at, allowed_actions_json) VALUES(?,?,?,?,?,?,?)`, result.CooldownID, eventID, input.Cooldown.Reason, input.Cooldown.Severity, input.Cooldown.StartsAt.UTC().Format(time.RFC3339Nano), input.Cooldown.ExpectedEndsAt.UTC().Format(time.RFC3339Nano), string(allowed)); err != nil {
			return AppendExecutionResult{}, fmt.Errorf("insert cooldown: %w", err)
		}
	}
	return result, nil
}

func (s *Store) AppendExecutionReversal(ctx context.Context, originalID, ruleVersionID, reason string, now time.Time) (AppendReversalResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendReversalResult{}, fmt.Errorf("begin reversal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := appendReversalTx(ctx, tx, originalID, ruleVersionID, reason, now)
	if err != nil {
		return AppendReversalResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppendReversalResult{}, fmt.Errorf("commit reversal: %w", err)
	}
	return result, nil
}

func appendReversalTx(ctx context.Context, tx *sql.Tx, originalID, ruleVersionID, reason string, now time.Time) (AppendReversalResult, error) {
	var original domain.ExecutionEvent
	var eventType string
	if err := tx.QueryRowContext(ctx, `SELECT event_type, instrument_id, quantity, local_price_minor, local_price_ten_thousandth, local_amount_minor, settlement_fen FROM execution_events WHERE id=?`, originalID).Scan(&eventType, &original.InstrumentID, &original.Quantity, &original.LocalPriceMinor, &original.LocalPriceTenThousandth, &original.LocalAmountMinor, &original.SettlementFen); err != nil {
		return AppendReversalResult{}, fmt.Errorf("load execution for reversal: %w", err)
	}
	if eventType == string(domain.ExecutionReversal) {
		return AppendReversalResult{}, fmt.Errorf("冲正记录不能再次冲正")
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM execution_events WHERE original_event_id=? AND event_type='reversal'`, originalID).Scan(&existing); err != nil {
		return AppendReversalResult{}, fmt.Errorf("check reversal: %w", err)
	}
	if existing > 0 {
		return AppendReversalResult{}, fmt.Errorf("该成交已经冲正")
	}
	id := NewID("execution")
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_events(id, original_event_id, event_type, plan_id, instrument_id, rule_version_id, quantity, local_price_minor, local_price_ten_thousandth, local_amount_minor, settlement_fen, exit_code, evidence, broker_reference, reference_price_minor, reference_price_at, emotion_json, executed_at, created_at) VALUES(?,?,'reversal',NULL,?,?,?,?,?,?,?,NULL,?,NULL,NULL,NULL,'{}',?,?)`, id, originalID, original.InstrumentID, ruleVersionID, original.Quantity, original.LocalPriceMinor, original.LocalPriceTenThousandth, original.LocalAmountMinor, original.SettlementFen, reason, stamp, stamp); err != nil {
		return AppendReversalResult{}, fmt.Errorf("insert reversal: %w", err)
	}
	afterJSON, _ := json.Marshal(map[string]any{"reversalId": id, "reason": reason})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'execution', ?, 'reversed', NULL, ?, ?)`, NewID("audit"), originalID, string(afterJSON), stamp); err != nil {
		return AppendReversalResult{}, fmt.Errorf("audit reversal: %w", err)
	}
	return AppendReversalResult{ExecutionID: id, InstrumentID: original.InstrumentID}, nil
}

func (s *Store) AppendExecutionCorrection(ctx context.Context, originalID, ruleVersionID, reason string, replacement AppendExecutionInput, now time.Time) (AppendCorrectionResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("begin execution correction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	reversal, err := appendReversalTx(ctx, tx, originalID, ruleVersionID, reason, now)
	if err != nil {
		return AppendCorrectionResult{}, err
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE violation_events SET acknowledged_at=? WHERE execution_id=? AND acknowledged_at IS NULL`, stamp, originalID); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("supersede corrected violation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cooldown_periods SET actual_ends_at=? WHERE execution_id=? AND actual_ends_at IS NULL`, stamp, originalID); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("close corrected cooldown: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE post_trade_reviews SET status='completed', note='原成交已修正', completed_at=? WHERE execution_id=? AND status='pending'`, stamp, originalID); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("supersede corrected review: %w", err)
	}
	replacement.Event.OriginalEventID = ""
	replacementResult, err := appendExecutionTx(ctx, tx, replacement, now)
	if err != nil {
		return AppendCorrectionResult{}, err
	}
	correctionID := NewID("execution-correction")
	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_corrections(id, original_execution_id, reversal_execution_id, replacement_execution_id, reason, created_at) VALUES(?,?,?,?,?,?)`, correctionID, originalID, reversal.ExecutionID, replacementResult.ExecutionID, reason, stamp); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("insert execution correction: %w", err)
	}
	afterJSON, _ := json.Marshal(map[string]any{"reversalId": reversal.ExecutionID, "replacementId": replacementResult.ExecutionID, "reason": reason})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'execution', ?, 'corrected', NULL, ?, ?)`, NewID("audit"), originalID, string(afterJSON), stamp); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("audit execution correction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AppendCorrectionResult{}, fmt.Errorf("commit execution correction: %w", err)
	}
	return AppendCorrectionResult{ReversalID: reversal.ExecutionID, Replacement: replacementResult}, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (s *Store) LoadExecutions(ctx context.Context) ([]domain.ExecutionEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, COALESCE(e.original_event_id,''), e.event_type, COALESCE(e.plan_id,''), e.instrument_id, i.code, e.quantity, e.local_price_minor, e.local_price_ten_thousandth, e.local_amount_minor, e.settlement_fen, i.is_china_tech, COALESCE(e.exit_code,''), e.executed_at FROM execution_events e JOIN instruments i ON i.id=e.instrument_id ORDER BY e.executed_at, e.created_at, e.id`)
	if err != nil {
		return nil, fmt.Errorf("load executions: %w", err)
	}
	defer rows.Close()
	var events []domain.ExecutionEvent
	for rows.Next() {
		var event domain.ExecutionEvent
		var eventType string
		var tech int
		var executed string
		if err := rows.Scan(&event.ID, &event.OriginalEventID, &eventType, &event.PlanID, &event.InstrumentID, &event.Code, &event.Quantity, &event.LocalPriceMinor, &event.LocalPriceTenThousandth, &event.LocalAmountMinor, &event.SettlementFen, &tech, &event.ExitCode, &executed); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		event.EventType = domain.ExecutionType(eventType)
		event.IsChinaTech = tech == 1
		event.ExecutedAt, _ = time.Parse(time.RFC3339Nano, executed)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ActivePlanExecutionQuantity(ctx context.Context, planID string, eventType domain.ExecutionType) (int, error) {
	return s.ActivePlanExecutionQuantityExcluding(ctx, planID, eventType, "")
}

func (s *Store) ActivePlanExecutionQuantityExcluding(ctx context.Context, planID string, eventType domain.ExecutionType, excludedExecutionID string) (int, error) {
	var quantity int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(e.quantity), 0)
		FROM execution_events e
		WHERE e.plan_id=? AND e.event_type=? AND (?='' OR e.id<>?)
		AND NOT EXISTS (SELECT 1 FROM execution_events reversal WHERE reversal.original_event_id=e.id AND reversal.event_type='reversal')`, planID, eventType, excludedExecutionID, excludedExecutionID).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("sum plan executions: %w", err)
	}
	return quantity, nil
}

func (s *Store) ListActiveExecutions(ctx context.Context, instrumentID string) ([]ExecutionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, COALESCE(e.plan_id,''), e.instrument_id, i.code, i.name, e.event_type,
		e.quantity, e.local_price_minor, e.local_price_ten_thousandth, e.local_amount_minor, e.settlement_fen,
		COALESCE(e.exit_code,''), COALESCE(e.evidence,''), COALESCE(e.broker_reference,''), e.emotion_json, e.executed_at,
		EXISTS(SELECT 1 FROM post_trade_reviews r WHERE r.execution_id=e.id)
		FROM execution_events e JOIN instruments i ON i.id=e.instrument_id
		WHERE e.event_type IN ('buy','sell') AND (?='' OR e.instrument_id=?)
		AND NOT EXISTS (SELECT 1 FROM execution_events reversal WHERE reversal.original_event_id=e.id AND reversal.event_type='reversal')
		ORDER BY e.executed_at DESC, e.created_at DESC LIMIT 100`, instrumentID, instrumentID)
	if err != nil {
		return nil, fmt.Errorf("list active executions: %w", err)
	}
	defer rows.Close()
	items := make([]ExecutionRecord, 0)
	for rows.Next() {
		var item ExecutionRecord
		var emotionJSON, executedAt string
		var quick int
		if err := rows.Scan(&item.ID, &item.PlanID, &item.InstrumentID, &item.Code, &item.Name, &item.Side,
			&item.Quantity, &item.LocalPriceMinor, &item.LocalPriceTenThousandth, &item.LocalAmountMinor, &item.SettlementFen,
			&item.ExitCode, &item.Evidence, &item.BrokerReference, &emotionJSON, &executedAt, &quick); err != nil {
			return nil, fmt.Errorf("scan active execution: %w", err)
		}
		_ = json.Unmarshal([]byte(emotionJSON), &item.Emotion)
		item.ExecutedAt, _ = time.Parse(time.RFC3339Nano, executedAt)
		item.QuickRecord = quick == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ExecutionHasPostTradeReview(ctx context.Context, executionID string) (bool, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM post_trade_reviews WHERE execution_id=?)`, executionID).Scan(&exists); err != nil {
		return false, fmt.Errorf("inspect execution review: %w", err)
	}
	return exists == 1, nil
}

func (s *Store) LatestCooldown(ctx context.Context) (*CooldownInput, error) {
	var item CooldownInput
	var starts, ends string
	err := s.db.QueryRowContext(ctx, `SELECT reason, severity, starts_at, expected_ends_at FROM cooldown_periods WHERE actual_ends_at IS NULL ORDER BY starts_at DESC LIMIT 1`).Scan(&item.Reason, &item.Severity, &starts, &ends)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load cooldown: %w", err)
	}
	item.StartsAt, _ = time.Parse(time.RFC3339Nano, starts)
	item.ExpectedEndsAt, _ = time.Parse(time.RFC3339Nano, ends)
	return &item, nil
}
