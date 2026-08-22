package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

const allocationProfileID = "allocation-main"

func (s *Store) EnsureInitialAllocation(ctx context.Context, now time.Time) (domain.AllocationSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AllocationSnapshot{}, fmt.Errorf("begin initial allocation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM allocation_profiles LIMIT 1`).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return domain.AllocationSnapshot{}, fmt.Errorf("load allocation profile: %w", err)
	}
	if err == sql.ErrNoRows {
		draft := initialAllocationDraft()
		raw, err := json.Marshal(draft)
		if err != nil {
			return domain.AllocationSnapshot{}, fmt.Errorf("encode initial allocation: %w", err)
		}
		stamp := now.UTC().Format(time.RFC3339Nano)
		versionID := NewID("allocation-version")
		if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_profiles(id, created_at) VALUES(?,?)`, allocationProfileID, stamp); err != nil {
			return domain.AllocationSnapshot{}, fmt.Errorf("insert allocation profile: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_versions(id, profile_id, version, draft_json, change_reason, created_at, previous_id) VALUES(?,?,?,?,?,?,NULL)`, versionID, allocationProfileID, 1, string(raw), "根据 20 万资产配置动态跟踪表建立初始基准", stamp); err != nil {
			return domain.AllocationSnapshot{}, fmt.Errorf("insert initial allocation version: %w", err)
		}
		for key, value := range initialAllocationValues() {
			if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_value_events(id, profile_id, item_key, value_fen, source, observed_at, created_at) VALUES(?,?,?,?,?,?,?)`, NewID("allocation-value"), allocationProfileID, key, value, "initial_import", stamp, stamp); err != nil {
				return domain.AllocationSnapshot{}, fmt.Errorf("insert initial allocation value %s: %w", key, err)
			}
		}
		if err := appendAudit(ctx, tx, "allocation_profile", allocationProfileID, "initial_import", nil, string(raw), now); err != nil {
			return domain.AllocationSnapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.AllocationSnapshot{}, fmt.Errorf("commit initial allocation: %w", err)
	}
	return s.CurrentAllocation(ctx)
}

func (s *Store) CurrentAllocation(ctx context.Context) (domain.AllocationSnapshot, error) {
	var snapshot domain.AllocationSnapshot
	var draftJSON, created string
	err := s.db.QueryRowContext(ctx, `SELECT profile_id, id, version, draft_json, change_reason, created_at FROM allocation_versions ORDER BY version DESC LIMIT 1`).Scan(&snapshot.ProfileID, &snapshot.Version.ID, &snapshot.Version.Version, &draftJSON, &snapshot.Version.Reason, &created)
	if err != nil {
		return domain.AllocationSnapshot{}, fmt.Errorf("load current allocation: %w", err)
	}
	if err := json.Unmarshal([]byte(draftJSON), &snapshot.Version.Draft); err != nil {
		return domain.AllocationSnapshot{}, fmt.Errorf("decode current allocation: %w", err)
	}
	snapshot.Version.ProfileID = snapshot.ProfileID
	snapshot.Version.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	values, err := s.latestAllocationValues(ctx, snapshot.ProfileID)
	if err != nil {
		return domain.AllocationSnapshot{}, err
	}
	snapshot.Values = values
	return snapshot, nil
}

func (s *Store) CreateAllocationVersion(ctx context.Context, draft domain.AllocationDraft, reason string, now time.Time) (domain.AllocationVersion, error) {
	raw, err := json.Marshal(draft)
	if err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("encode allocation version: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("begin allocation revision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var profileID, previousID, previousJSON string
	var previousVersion int
	err = tx.QueryRowContext(ctx, `SELECT profile_id, id, version, draft_json FROM allocation_versions ORDER BY version DESC LIMIT 1`).Scan(&profileID, &previousID, &previousVersion, &previousJSON)
	if err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("load allocation for revision: %w", err)
	}
	version := domain.AllocationVersion{ID: NewID("allocation-version"), ProfileID: profileID, Version: previousVersion + 1, Draft: draft, Reason: reason, CreatedAt: now.UTC()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_versions(id, profile_id, version, draft_json, change_reason, created_at, previous_id) VALUES(?,?,?,?,?,?,?)`, version.ID, version.ProfileID, version.Version, string(raw), version.Reason, version.CreatedAt.Format(time.RFC3339Nano), previousID); err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("insert allocation revision: %w", err)
	}
	if err := appendAudit(ctx, tx, "allocation_profile", profileID, "revised", &previousJSON, string(raw), now); err != nil {
		return domain.AllocationVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("commit allocation revision: %w", err)
	}
	return version, nil
}

func (s *Store) AppendAllocationValueEvent(ctx context.Context, profileID string, event domain.AllocationValueEvent, now time.Time) (domain.AllocationValueEvent, error) {
	if event.ValueFen < 0 {
		return domain.AllocationValueEvent{}, fmt.Errorf("市值不能为负数")
	}
	if event.Source != "manual" && event.Source != "initial_import" {
		return domain.AllocationValueEvent{}, fmt.Errorf("市值来源无效")
	}
	if event.ObservedAt.IsZero() {
		return domain.AllocationValueEvent{}, fmt.Errorf("市值观察时间不能为空")
	}
	event.ID = NewID("allocation-value")
	event.ProfileID = profileID
	event.ObservedAt = event.ObservedAt.UTC()
	event.CreatedAt = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AllocationValueEvent{}, fmt.Errorf("begin allocation value: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_value_events(id, profile_id, item_key, value_fen, source, observed_at, created_at) VALUES(?,?,?,?,?,?,?)`, event.ID, event.ProfileID, event.ItemKey, event.ValueFen, event.Source, event.ObservedAt.Format(time.RFC3339Nano), event.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return domain.AllocationValueEvent{}, fmt.Errorf("insert allocation value: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"itemKey": event.ItemKey, "valueFen": event.ValueFen, "source": event.Source, "observedAt": event.ObservedAt})
	if err := appendAudit(ctx, tx, "allocation_value", event.ID, "recorded", nil, string(after), now); err != nil {
		return domain.AllocationValueEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AllocationValueEvent{}, fmt.Errorf("commit allocation value: %w", err)
	}
	return event, nil
}

func (s *Store) ListAllocationVersions(ctx context.Context) ([]domain.AllocationVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, profile_id, version, draft_json, change_reason, created_at FROM allocation_versions ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list allocation versions: %w", err)
	}
	defer rows.Close()
	versions := make([]domain.AllocationVersion, 0)
	for rows.Next() {
		version, err := scanAllocationVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) ListAllocationValueEvents(ctx context.Context, profileID, itemKey string) ([]domain.AllocationValueEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, profile_id, item_key, value_fen, source, observed_at, created_at FROM allocation_value_events WHERE profile_id=? AND item_key=? ORDER BY observed_at DESC, created_at DESC, rowid DESC`, profileID, itemKey)
	if err != nil {
		return nil, fmt.Errorf("list allocation values: %w", err)
	}
	defer rows.Close()
	events := make([]domain.AllocationValueEvent, 0)
	for rows.Next() {
		event, err := scanAllocationValueEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) latestAllocationValues(ctx context.Context, profileID string) (map[string]domain.AllocationValueEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, profile_id, item_key, value_fen, source, observed_at, created_at FROM allocation_value_events WHERE profile_id=? ORDER BY item_key, observed_at DESC, created_at DESC, rowid DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list latest allocation values: %w", err)
	}
	defer rows.Close()
	values := make(map[string]domain.AllocationValueEvent)
	for rows.Next() {
		event, err := scanAllocationValueEvent(rows)
		if err != nil {
			return nil, err
		}
		if _, exists := values[event.ItemKey]; !exists {
			values[event.ItemKey] = event
		}
	}
	return values, rows.Err()
}

type allocationScanner interface{ Scan(...any) error }

func scanAllocationVersion(row allocationScanner) (domain.AllocationVersion, error) {
	var version domain.AllocationVersion
	var raw, created string
	err := row.Scan(&version.ID, &version.ProfileID, &version.Version, &raw, &version.Reason, &created)
	if err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("scan allocation version: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &version.Draft); err != nil {
		return domain.AllocationVersion{}, fmt.Errorf("decode allocation version: %w", err)
	}
	version.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return version, nil
}

func scanAllocationValueEvent(row allocationScanner) (domain.AllocationValueEvent, error) {
	var event domain.AllocationValueEvent
	var observed, created string
	err := row.Scan(&event.ID, &event.ProfileID, &event.ItemKey, &event.ValueFen, &event.Source, &observed, &created)
	if err != nil {
		return domain.AllocationValueEvent{}, fmt.Errorf("scan allocation value: %w", err)
	}
	event.ObservedAt, _ = time.Parse(time.RFC3339Nano, observed)
	event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return event, nil
}

func initialAllocationDraft() domain.AllocationDraft {
	return domain.AllocationDraft{
		InitialCapitalFen: 20_000_000,
		TargetReturnBP:    1_000,
		TargetDeadline:    "2026-12-31",
		Items: []domain.AllocationItem{
			{Key: "tencent", Name: "腾讯", TargetWeightBP: 3_900, TargetShares: 200, BuyRule: "分批到 200 股；若一路上涨，不为凑股数追高。", SellRule: "主要按仓位再平衡和基本面变化卖；游戏、广告、微信生态与自由现金流持续恶化，或 AI 投入长期无法转化时重新评估。", Note: "核心成长；计划最终约 200 股。"},
			{Key: "csi300-etf", Name: "沪深 300 ETF", TargetWeightBP: 1_750, BuyRule: "分批建核心底仓；大盘调整约 5%–8% 后补充，出现约 10% 以上明显调整后再补充。", SellRule: "原则上只做再平衡；长期中国权益配置逻辑发生重大改变时重新评估。", Note: "核心底仓。"},
			{Key: "semiconductor-equipment-etf-159558", Name: "半导体设备 ETF 159558", TargetWeightBP: 1_250, BuyRule: "明显回撤且基本面未坏时分档加仓；更深回撤或极端杀估值时再加。", SellRule: "估值过热或产业资本开支、龙头订单增速和盈利预期系统性转弱时分批减仓。", Note: "已有 1 万元；达到目标仓后不机械加仓。"},
			{Key: "gold-etf", Name: "黄金 ETF", TargetWeightBP: 750, BuyRule: "从阶段高点回撤 8% 加第二笔；回撤约 12%–15% 再加。", SellRule: "主要按组合占比再平衡；实际利率长期明显上行、美元强周期、避险与央行需求同步转弱时重新评估。", Note: "对冲仓；若不想追涨，可等第一档回撤再开始。"},
			{Key: "communication-etf", Name: "通信 ETF", TargetWeightBP: 600, BuyRule: "先观察，完成 ETF 筛选或出现合理回撤后分批建立。", SellRule: "估值过热或 AI 资本开支预期、光通信订单逻辑转弱时分批减仓。", Note: "AI 基础设施卫星仓。"},
			{Key: "china-internet-etf", Name: "中概互联网 ETF", TargetWeightBP: 0, BuyRule: "当前不配置；若未来增加，从腾讯目标额度中调整。", SellRule: "当前无配置，不以价格波动建立仓位。", Note: "暂定 0%。"},
			{Key: "cash-fund", Name: "现金/货基", TargetWeightBP: 1_750, BuyRule: "保留加仓弹药，按实际资金变化记录。", SellRule: "按组合配置与流动性需要再平衡。", Note: "保留流动性。"},
		},
	}
}

func initialAllocationValues() map[string]int64 {
	return map[string]int64{
		"tencent":                            0,
		"csi300-etf":                         0,
		"semiconductor-equipment-etf-159558": 1_000_000,
		"gold-etf":                           0,
		"communication-etf":                  0,
		"china-internet-etf":                 0,
		"cash-fund":                          19_000_000,
	}
}
