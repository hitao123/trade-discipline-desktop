package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
)

const localUserProfileID = "local-user"

var ErrOnboardingAlreadyCompleted = errors.New("首次启动向导已经完成")
var ErrGenericProfileRequired = errors.New("只有通用模式可以修改基础资料")

func hasExistingApplicationState(ctx context.Context, tx *sql.Tx) (bool, error) {
	for _, table := range []string{
		"schema_migrations", "accounts", "rule_versions", "instruments", "trade_plans",
		"execution_events", "weekly_reviews", "audit_events", "allocation_profiles", "market_snapshots",
	} {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
			return false, fmt.Errorf("inspect existing database table %s: %w", table, err)
		}
		if exists == 0 {
			continue
		}
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return false, fmt.Errorf("inspect existing database state %s: %w", table, err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func ensureUserProfileMigration(ctx context.Context, tx *sql.Tx, legacyDatabase bool, now time.Time) error {
	stamp := now.UTC().Format(time.RFC3339Nano)
	if legacyDatabase {
		capitalFen, maxLossFen := legacyProfileAmounts(ctx, tx)
		markets, err := json.Marshal([]domain.MarketScope{domain.MarketAShareStock, domain.MarketAShareETF, domain.MarketHK})
		if err != nil {
			return fmt.Errorf("encode legacy market scopes: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_profiles(
			id, mode, onboarding_status, investable_capital_fen, max_loss_fen, holding_horizon,
			enabled_markets_json, completed_at, updated_at
		) VALUES(?, 'legacy', 'completed', ?, ?, ?, ?, ?, ?)`,
			localUserProfileID, nullableMigrationInt(capitalFen), nullableMigrationInt(maxLossFen), domain.HorizonLegacy, string(markets), stamp, stamp); err != nil {
			return fmt.Errorf("create legacy user profile: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_profiles(
		id, mode, onboarding_status, investable_capital_fen, max_loss_fen, holding_horizon,
		enabled_markets_json, completed_at, updated_at
	) VALUES(?, 'generic', 'pending', NULL, NULL, NULL, '[]', NULL, ?)`, localUserProfileID, stamp); err != nil {
		return fmt.Errorf("create pending user profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (11, ?)`, stamp); err != nil {
		return fmt.Errorf("record onboarding migration: %w", err)
	}
	return nil
}

func legacyProfileAmounts(ctx context.Context, tx *sql.Tx) (int64, int64) {
	var capital sql.NullInt64
	_ = tx.QueryRowContext(ctx, `SELECT initial_capital_fen FROM accounts WHERE status='active' LIMIT 1`).Scan(&capital)
	var raw string
	var maxLoss int64
	if err := tx.QueryRowContext(ctx, `SELECT snapshot_json FROM rule_versions ORDER BY version DESC LIMIT 1`).Scan(&raw); err == nil {
		var snapshot rules.Snapshot
		if json.Unmarshal([]byte(raw), &snapshot) == nil {
			maxLoss = snapshot.LossRedLineFen
			if !capital.Valid {
				capital.Int64 = snapshot.InitialCapitalFen
				capital.Valid = snapshot.InitialCapitalFen > 0
			}
		}
	}
	return capital.Int64, maxLoss
}

func nullableMigrationInt(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func (s *Store) UserProfile(ctx context.Context) (domain.UserProfile, error) {
	var profile domain.UserProfile
	var capital, maxLoss sql.NullInt64
	var horizon, completed sql.NullString
	var marketsJSON, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, mode, onboarding_status, investable_capital_fen,
		max_loss_fen, holding_horizon, enabled_markets_json, completed_at, updated_at
		FROM user_profiles WHERE id=?`, localUserProfileID).Scan(
		&profile.ID, &profile.Mode, &profile.OnboardingStatus, &capital, &maxLoss, &horizon,
		&marketsJSON, &completed, &updated,
	)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("load user profile: %w", err)
	}
	profile.InvestableCapitalFen = capital.Int64
	profile.MaxLossFen = maxLoss.Int64
	profile.HoldingHorizon = domain.HoldingHorizon(horizon.String)
	if err := json.Unmarshal([]byte(marketsJSON), &profile.EnabledMarkets); err != nil {
		return domain.UserProfile{}, fmt.Errorf("decode enabled markets: %w", err)
	}
	if profile.EnabledMarkets == nil {
		profile.EnabledMarkets = []domain.MarketScope{}
	}
	if completed.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, completed.String)
		if err != nil {
			return domain.UserProfile{}, fmt.Errorf("decode onboarding completion time: %w", err)
		}
		profile.CompletedAt = &parsed
	}
	profile.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("decode user profile update time: %w", err)
	}
	return profile, nil
}

func (s *Store) CompleteGenericOnboarding(ctx context.Context, profile domain.UserProfile, snapshot rules.Snapshot, now time.Time) (domain.UserProfile, error) {
	marketsJSON, err := json.Marshal(profile.EnabledMarkets)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode enabled markets: %w", err)
	}
	ruleJSON, err := json.Marshal(snapshot)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode generic rule: %w", err)
	}
	afterJSON, err := json.Marshal(map[string]any{
		"mode":                 domain.UserModeGeneric,
		"onboardingStatus":     domain.OnboardingCompleted,
		"investableCapitalFen": profile.InvestableCapitalFen,
		"maxLossFen":           profile.MaxLossFen,
		"holdingHorizon":       profile.HoldingHorizon,
		"enabledMarkets":       profile.EnabledMarkets,
	})
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode onboarding audit: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("begin onboarding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var mode domain.UserMode
	var status domain.OnboardingStatus
	if err := tx.QueryRowContext(ctx, `SELECT mode, onboarding_status FROM user_profiles WHERE id=?`, localUserProfileID).Scan(&mode, &status); err != nil {
		return domain.UserProfile{}, fmt.Errorf("load onboarding state: %w", err)
	}
	if mode != domain.UserModeGeneric || status != domain.OnboardingPending {
		return domain.UserProfile{}, ErrOnboardingAlreadyCompleted
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, name, base_currency, initial_capital_fen, enabled_at, status)
		VALUES('account-main', '本地纪律账户', 'CNY', ?, ?, 'active')`, profile.InvestableCapitalFen, stamp); err != nil {
		return domain.UserProfile{}, fmt.Errorf("create onboarding account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id)
		VALUES(?, 1, ?, '完成首次启动向导', ?, NULL)`, NewID("rule"), string(ruleJSON), stamp); err != nil {
		return domain.UserProfile{}, fmt.Errorf("create onboarding rule: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at)
		VALUES(?, 'user_profile', ?, 'onboarding_completed', NULL, ?, ?)`, NewID("audit"), localUserProfileID, string(afterJSON), stamp); err != nil {
		return domain.UserProfile{}, fmt.Errorf("audit onboarding: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE user_profiles SET onboarding_status='completed', investable_capital_fen=?,
		max_loss_fen=?, holding_horizon=?, enabled_markets_json=?, completed_at=?, updated_at=?
		WHERE id=? AND mode='generic' AND onboarding_status='pending'`, profile.InvestableCapitalFen, profile.MaxLossFen,
		profile.HoldingHorizon, string(marketsJSON), stamp, stamp, localUserProfileID)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("complete onboarding profile: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return domain.UserProfile{}, ErrOnboardingAlreadyCompleted
	}
	if err := tx.Commit(); err != nil {
		return domain.UserProfile{}, fmt.Errorf("commit onboarding: %w", err)
	}
	return s.UserProfile(ctx)
}

func (s *Store) UpdateGenericProfile(ctx context.Context, profile domain.UserProfile, snapshot rules.Snapshot, reason string, now time.Time) (domain.UserProfile, error) {
	marketsJSON, err := json.Marshal(profile.EnabledMarkets)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode profile markets: %w", err)
	}
	ruleJSON, err := json.Marshal(snapshot)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode profile rule: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("begin profile update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var mode domain.UserMode
	var status domain.OnboardingStatus
	var beforeProfileJSON string
	if err := tx.QueryRowContext(ctx, `SELECT mode, onboarding_status, json_object(
		'investableCapitalFen', investable_capital_fen, 'maxLossFen', max_loss_fen,
		'holdingHorizon', holding_horizon, 'enabledMarkets', json(enabled_markets_json)
	) FROM user_profiles WHERE id=?`, localUserProfileID).Scan(&mode, &status, &beforeProfileJSON); err != nil {
		return domain.UserProfile{}, fmt.Errorf("load profile before update: %w", err)
	}
	if mode != domain.UserModeGeneric || status != domain.OnboardingCompleted {
		return domain.UserProfile{}, ErrGenericProfileRequired
	}
	var previousID, previousJSON string
	var previousVersion int
	if err := tx.QueryRowContext(ctx, `SELECT id, version, snapshot_json FROM rule_versions ORDER BY version DESC LIMIT 1`).Scan(&previousID, &previousVersion, &previousJSON); err != nil {
		return domain.UserProfile{}, fmt.Errorf("load current profile rule: %w", err)
	}
	snapshot.Version = previousVersion + 1
	ruleJSON, err = json.Marshal(snapshot)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("encode versioned profile rule: %w", err)
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id) VALUES(?,?,?,?,?,?)`, NewID("rule"), snapshot.Version, string(ruleJSON), reason, stamp, previousID); err != nil {
		return domain.UserProfile{}, fmt.Errorf("insert profile rule: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_profiles SET investable_capital_fen=?, max_loss_fen=?, holding_horizon=?, enabled_markets_json=?, updated_at=? WHERE id=?`, profile.InvestableCapitalFen, profile.MaxLossFen, profile.HoldingHorizon, string(marketsJSON), stamp, localUserProfileID); err != nil {
		return domain.UserProfile{}, fmt.Errorf("update profile: %w", err)
	}
	afterProfileJSON, _ := json.Marshal(profile)
	for _, audit := range []struct{ entityType, action, before, after string }{
		{"user_profile", "profile_updated", beforeProfileJSON, string(afterProfileJSON)},
		{"rule", "version_created", previousJSON, string(ruleJSON)},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?,?,?,?,?,?,?)`, NewID("audit"), audit.entityType, localUserProfileID, audit.action, audit.before, audit.after, stamp); err != nil {
			return domain.UserProfile{}, fmt.Errorf("audit profile update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.UserProfile{}, fmt.Errorf("commit profile update: %w", err)
	}
	return s.UserProfile(ctx)
}
