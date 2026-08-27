package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
)

type PlanRow struct {
	ID            string
	RuleVersionID string
	Status        string
	Draft         domain.TradePlanDraft
	Validation    rules.Decision
	CreatedAt     time.Time
}

func (s *Store) Plan(ctx context.Context, id string) (PlanRow, error) {
	var plan PlanRow
	var draftJSON, validationJSON, created string
	err := s.db.QueryRowContext(ctx, `SELECT id, rule_version_id, status, draft_json, validation_json, created_at FROM trade_plans WHERE id=?`, id).Scan(&plan.ID, &plan.RuleVersionID, &plan.Status, &draftJSON, &validationJSON, &created)
	if err != nil {
		return PlanRow{}, fmt.Errorf("load plan: %w", err)
	}
	if err := json.Unmarshal([]byte(draftJSON), &plan.Draft); err != nil {
		return PlanRow{}, fmt.Errorf("decode plan: %w", err)
	}
	if err := json.Unmarshal([]byte(validationJSON), &plan.Validation); err != nil {
		return PlanRow{}, fmt.Errorf("decode plan decision: %w", err)
	}
	plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return plan, nil
}

func (s *Store) SavePlan(ctx context.Context, ruleVersionID, status string, draft domain.TradePlanDraft, decision rules.Decision, now time.Time) (PlanRow, error) {
	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return PlanRow{}, fmt.Errorf("encode plan: %w", err)
	}
	validationJSON, err := json.Marshal(decision)
	if err != nil {
		return PlanRow{}, fmt.Errorf("encode decision: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlanRow{}, fmt.Errorf("begin plan: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id := NewID("plan")
	stamp := now.UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO trade_plans(id, instrument_id, rule_version_id, status, draft_json, validation_json, created_at, updated_at, valid_until) VALUES(?,?,?,?,?,?,?,?,?)`, id, draft.InstrumentID, ruleVersionID, status, string(draftJSON), string(validationJSON), stamp, stamp, draft.ValidUntil.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return PlanRow{}, fmt.Errorf("insert plan: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'plan', ?, 'created', NULL, ?, ?)`, NewID("audit"), id, string(draftJSON), stamp); err != nil {
		return PlanRow{}, fmt.Errorf("audit plan: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PlanRow{}, fmt.Errorf("commit plan: %w", err)
	}
	return PlanRow{ID: id, RuleVersionID: ruleVersionID, Status: status, Draft: draft, Validation: decision, CreatedAt: now.UTC()}, nil
}

func (s *Store) RevisePlan(ctx context.Context, id, ruleVersionID, status, reason string, draft domain.TradePlanDraft, decision rules.Decision, now time.Time) (PlanRow, error) {
	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return PlanRow{}, fmt.Errorf("encode plan: %w", err)
	}
	validationJSON, err := json.Marshal(decision)
	if err != nil {
		return PlanRow{}, fmt.Errorf("encode decision: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlanRow{}, fmt.Errorf("begin plan revision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var beforeJSON string
	var revision int
	if err := tx.QueryRowContext(ctx, `SELECT draft_json FROM trade_plans WHERE id=?`, id).Scan(&beforeJSON); err != nil {
		return PlanRow{}, fmt.Errorf("load plan for revision: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision_number), 0) + 1 FROM trade_plan_revisions WHERE plan_id=?`, id).Scan(&revision); err != nil {
		return PlanRow{}, fmt.Errorf("next plan revision: %w", err)
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO trade_plan_revisions(id, plan_id, revision_number, before_json, after_json, change_reason, created_at) VALUES(?,?,?,?,?,?,?)`, NewID("revision"), id, revision, beforeJSON, string(draftJSON), reason, stamp); err != nil {
		return PlanRow{}, fmt.Errorf("insert plan revision: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE trade_plans SET instrument_id=?, rule_version_id=?, status=?, draft_json=?, validation_json=?, updated_at=?, valid_until=? WHERE id=?`, draft.InstrumentID, ruleVersionID, status, string(draftJSON), string(validationJSON), stamp, draft.ValidUntil.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return PlanRow{}, fmt.Errorf("update plan: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return PlanRow{}, fmt.Errorf("计划不存在")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'plan', ?, 'revised', ?, ?, ?)`, NewID("audit"), id, beforeJSON, string(draftJSON), stamp); err != nil {
		return PlanRow{}, fmt.Errorf("audit plan revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PlanRow{}, fmt.Errorf("commit plan revision: %w", err)
	}
	return PlanRow{ID: id, RuleVersionID: ruleVersionID, Status: status, Draft: draft, Validation: decision, CreatedAt: now.UTC()}, nil
}

func (s *Store) ListPlans(ctx context.Context) ([]PlanRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, rule_version_id, status, draft_json, validation_json, created_at FROM trade_plans ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	plans := make([]PlanRow, 0)
	for rows.Next() {
		var plan PlanRow
		var draftJSON, validationJSON, created string
		if err := rows.Scan(&plan.ID, &plan.RuleVersionID, &plan.Status, &draftJSON, &validationJSON, &created); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		if err := json.Unmarshal([]byte(draftJSON), &plan.Draft); err != nil {
			return nil, fmt.Errorf("decode plan: %w", err)
		}
		if err := json.Unmarshal([]byte(validationJSON), &plan.Validation); err != nil {
			return nil, fmt.Errorf("decode plan decision: %w", err)
		}
		plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

type RuleVersionRow struct {
	ID        string         `json:"id"`
	Version   int            `json:"version"`
	Snapshot  rules.Snapshot `json:"snapshot"`
	Reason    string         `json:"reason"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (s *Store) CreateRuleVersion(ctx context.Context, reason string, snapshot rules.Snapshot, now time.Time) (RuleVersionRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RuleVersionRow{}, fmt.Errorf("begin rule version: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var previousID, previousJSON string
	var previousVersion int
	if err := tx.QueryRowContext(ctx, `SELECT id, version, snapshot_json FROM rule_versions ORDER BY version DESC LIMIT 1`).Scan(&previousID, &previousVersion, &previousJSON); err != nil {
		return RuleVersionRow{}, fmt.Errorf("load previous rule: %w", err)
	}
	snapshot.Version = previousVersion + 1
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return RuleVersionRow{}, fmt.Errorf("encode rule: %w", err)
	}
	id := NewID("rule")
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id) VALUES(?,?,?,?,?,?)`, id, snapshot.Version, string(raw), reason, stamp, previousID); err != nil {
		return RuleVersionRow{}, fmt.Errorf("insert rule version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'rule', ?, 'version_created', ?, ?, ?)`, NewID("audit"), id, previousJSON, string(raw), stamp); err != nil {
		return RuleVersionRow{}, fmt.Errorf("audit rule version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RuleVersionRow{}, fmt.Errorf("commit rule version: %w", err)
	}
	return RuleVersionRow{ID: id, Version: snapshot.Version, Snapshot: snapshot, Reason: reason, CreatedAt: now.UTC()}, nil
}

func (s *Store) ListRuleVersions(ctx context.Context) ([]RuleVersionRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, version, snapshot_json, change_reason, created_at FROM rule_versions ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	versions := make([]RuleVersionRow, 0)
	for rows.Next() {
		var item RuleVersionRow
		var raw, created string
		if err := rows.Scan(&item.ID, &item.Version, &raw, &item.Reason, &created); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &item.Snapshot); err != nil {
			return nil, fmt.Errorf("decode rule: %w", err)
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		versions = append(versions, item)
	}
	return versions, rows.Err()
}
