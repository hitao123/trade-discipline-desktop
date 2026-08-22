package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

type AllocationOverview = domain.AllocationOverview
type AllocationItemProgress = domain.AllocationItemProgress

var allocationItemKeys = map[string]struct{}{
	"tencent":                            {},
	"csi300-etf":                         {},
	"semiconductor-equipment-etf-159558": {},
	"gold-etf":                           {},
	"communication-etf":                  {},
	"china-internet-etf":                 {},
	"cash-fund":                          {},
}

func (s *Service) Allocation(ctx context.Context) (domain.AllocationOverview, error) {
	snapshot, err := s.store.EnsureInitialAllocation(ctx, s.now())
	if err != nil {
		return domain.AllocationOverview{}, err
	}

	overview := domain.AllocationOverview{
		ProfileID: snapshot.ProfileID,
		Version:   snapshot.Version,
		Items:     make([]domain.AllocationItemProgress, 0, len(snapshot.Version.Draft.Items)),
	}
	for _, value := range snapshot.Values {
		overview.CurrentTotalFen += value.ValueFen
	}
	draft := snapshot.Version.Draft
	overview.TargetTotalFen = draft.InitialCapitalFen * int64(10_000+draft.TargetReturnBP) / 10_000
	if draft.InitialCapitalFen > 0 {
		overview.ReturnBP = int((overview.CurrentTotalFen - draft.InitialCapitalFen) * 10_000 / draft.InitialCapitalFen)
	}
	overview.GoalGapFen = overview.TargetTotalFen - overview.CurrentTotalFen

	for _, item := range draft.Items {
		progress := domain.AllocationItemProgress{
			Item:               item,
			BuildTargetFen:     draft.InitialCapitalFen * int64(item.TargetWeightBP) / 10_000,
			RebalanceTargetFen: overview.CurrentTotalFen * int64(item.TargetWeightBP) / 10_000,
		}
		if value, ok := snapshot.Values[item.Key]; ok {
			valueCopy := value
			progress.CurrentValueFen = value.ValueFen
			progress.LatestValueRecorded = &valueCopy
		}
		if overview.CurrentTotalFen > 0 {
			progress.CurrentWeightBP = int(progress.CurrentValueFen * 10_000 / overview.CurrentTotalFen)
		}
		progress.BuildGapFen = progress.BuildTargetFen - progress.CurrentValueFen
		progress.RebalanceGapFen = progress.RebalanceTargetFen - progress.CurrentValueFen
		overview.Items = append(overview.Items, progress)
	}
	return overview, nil
}

func (s *Service) ReviseAllocation(ctx context.Context, draft domain.AllocationDraft, reason string) (domain.AllocationVersion, error) {
	if _, err := s.store.EnsureInitialAllocation(ctx, s.now()); err != nil {
		return domain.AllocationVersion{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.AllocationVersion{}, fmt.Errorf("请填写修改原因")
	}
	if err := validateAllocationDraft(&draft); err != nil {
		return domain.AllocationVersion{}, err
	}
	return s.store.CreateAllocationVersion(ctx, draft, reason, s.now())
}

func (s *Service) RecordAllocationValue(ctx context.Context, itemKey string, valueFen int64, observedAt time.Time) (domain.AllocationValueEvent, error) {
	snapshot, err := s.store.EnsureInitialAllocation(ctx, s.now())
	if err != nil {
		return domain.AllocationValueEvent{}, err
	}
	if _, ok := allocationItemKeys[itemKey]; !ok {
		return domain.AllocationValueEvent{}, fmt.Errorf("未知的资产项目：%s", itemKey)
	}
	if valueFen < 0 {
		return domain.AllocationValueEvent{}, fmt.Errorf("市值不能为负数")
	}
	if observedAt.IsZero() {
		return domain.AllocationValueEvent{}, fmt.Errorf("市值观察时间不能为空")
	}
	return s.store.AppendAllocationValueEvent(ctx, snapshot.ProfileID, domain.AllocationValueEvent{
		ItemKey:    itemKey,
		ValueFen:   valueFen,
		Source:     "manual",
		ObservedAt: observedAt,
	}, s.now())
}

func (s *Service) AllocationVersions(ctx context.Context) ([]domain.AllocationVersion, error) {
	if _, err := s.store.EnsureInitialAllocation(ctx, s.now()); err != nil {
		return nil, err
	}
	return s.store.ListAllocationVersions(ctx)
}

func (s *Service) AllocationValueEvents(ctx context.Context, itemKey string) ([]domain.AllocationValueEvent, error) {
	snapshot, err := s.store.EnsureInitialAllocation(ctx, s.now())
	if err != nil {
		return nil, err
	}
	if _, ok := allocationItemKeys[itemKey]; !ok {
		return nil, fmt.Errorf("未知的资产项目：%s", itemKey)
	}
	return s.store.ListAllocationValueEvents(ctx, snapshot.ProfileID, itemKey)
}

func validateAllocationDraft(draft *domain.AllocationDraft) error {
	if draft.InitialCapitalFen <= 0 {
		return fmt.Errorf("初始资金必须大于 0")
	}
	if draft.TargetReturnBP < 0 {
		return fmt.Errorf("目标收益不能为负数")
	}
	if _, err := time.Parse("2006-01-02", draft.TargetDeadline); err != nil {
		return fmt.Errorf("目标截止日格式应为 YYYY-MM-DD")
	}
	if len(draft.Items) != len(allocationItemKeys) {
		return fmt.Errorf("资产项目必须保留当前的 7 项配置")
	}
	seen := make(map[string]struct{}, len(draft.Items))
	weightTotal := 0
	for index := range draft.Items {
		item := &draft.Items[index]
		item.Key = strings.TrimSpace(item.Key)
		item.Name = strings.TrimSpace(item.Name)
		item.BuyRule = strings.TrimSpace(item.BuyRule)
		item.SellRule = strings.TrimSpace(item.SellRule)
		item.Note = strings.TrimSpace(item.Note)
		if _, ok := allocationItemKeys[item.Key]; !ok {
			return fmt.Errorf("不支持的资产项目：%s", item.Key)
		}
		if _, duplicated := seen[item.Key]; duplicated {
			return fmt.Errorf("资产项目不能重复：%s", item.Key)
		}
		seen[item.Key] = struct{}{}
		if item.Name == "" || item.BuyRule == "" || item.SellRule == "" {
			return fmt.Errorf("请完整填写 %s 的名称和规则", item.Key)
		}
		if item.TargetWeightBP < 0 || item.TargetWeightBP > 10_000 {
			return fmt.Errorf("%s 的目标权重无效", item.Name)
		}
		if item.TargetShares < 0 {
			return fmt.Errorf("%s 的目标股数不能为负数", item.Name)
		}
		weightTotal += item.TargetWeightBP
	}
	if weightTotal != 10_000 {
		return fmt.Errorf("目标权重合计必须为 100%%")
	}
	return nil
}
