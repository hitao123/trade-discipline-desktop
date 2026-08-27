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

func (s *Service) Allocation(ctx context.Context) (domain.AllocationState, error) {
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return domain.AllocationState{}, err
	}
	hasAllocation, err := s.store.HasAllocation(ctx)
	if err != nil {
		return domain.AllocationState{}, err
	}
	if !hasAllocation && profile.Mode == domain.UserModeGeneric {
		return domain.AllocationState{Configured: false, SuggestedCapitalFen: profile.InvestableCapitalFen}, nil
	}
	if !hasAllocation {
		if _, err := s.store.EnsureInitialAllocation(ctx, s.now()); err != nil {
			return domain.AllocationState{}, err
		}
	}
	snapshot, err := s.store.CurrentAllocation(ctx)
	if err != nil {
		return domain.AllocationState{}, err
	}
	overview, err := s.allocationOverview(ctx, snapshot, profile.Mode == domain.UserModeLegacy)
	if err != nil {
		return domain.AllocationState{}, err
	}
	return domain.AllocationState{Configured: true, SuggestedCapitalFen: profile.InvestableCapitalFen, Overview: &overview}, nil
}

func (s *Service) allocationOverview(ctx context.Context, snapshot domain.AllocationSnapshot, legacy bool) (domain.AllocationOverview, error) {
	if legacy {
		snapshot.Version.Draft = allocationWithDefaultBindings(snapshot.Version.Draft)
	}
	portfolio, err := s.Portfolio(ctx)
	if err != nil {
		return domain.AllocationOverview{}, err
	}
	adjustments, err := s.store.LatestAllocationAdjustments(ctx, snapshot.ProfileID)
	if err != nil {
		return domain.AllocationOverview{}, err
	}
	overview := domain.AllocationOverview{
		ProfileID: snapshot.ProfileID,
		Version:   snapshot.Version,
		Items:     make([]domain.AllocationItemProgress, 0, len(snapshot.Version.Draft.Items)),
	}
	draft := snapshot.Version.Draft
	positionByCode := make(map[string]domain.PositionState, len(portfolio.Positions))
	for _, position := range portfolio.Positions {
		positionByCode[strings.ToUpper(position.Code)] = position
		overview.CurrentTotalFen += position.MarketValueFen
	}
	overview.CurrentTotalFen += portfolio.AvailableCashFen
	for _, adjustment := range adjustments {
		overview.CurrentTotalFen += adjustment.AdjustmentFen
	}
	overview.TargetTotalFen = draft.InitialCapitalFen * int64(10_000+draft.TargetReturnBP) / 10_000
	if draft.InitialCapitalFen > 0 {
		overview.ReturnBP = int((overview.CurrentTotalFen - draft.InitialCapitalFen) * 10_000 / draft.InitialCapitalFen)
	}
	overview.GoalGapFen = overview.TargetTotalFen - overview.CurrentTotalFen

	assigned := make(map[string]struct{})
	for _, item := range draft.Items {
		progress := domain.AllocationItemProgress{
			Item:               item,
			BuildTargetFen:     draft.InitialCapitalFen * int64(item.TargetWeightBP) / 10_000,
			RebalanceTargetFen: overview.CurrentTotalFen * int64(item.TargetWeightBP) / 10_000,
			LinkedPositions:    make([]domain.PositionState, 0),
		}
		if item.EffectiveRole() == domain.AllocationRoleCash {
			progress.LinkedValueFen = portfolio.AvailableCashFen
		}
		for _, code := range item.InstrumentCodes {
			code = strings.ToUpper(strings.TrimSpace(code))
			position, ok := positionByCode[code]
			if !ok {
				continue
			}
			progress.LinkedPositions = append(progress.LinkedPositions, position)
			progress.LinkedValueFen += position.MarketValueFen
			assigned[code] = struct{}{}
		}
		if adjustment, ok := adjustments[item.Key]; ok {
			adjustmentCopy := adjustment
			progress.ManualAdjustmentFen = adjustment.AdjustmentFen
			progress.LatestAdjustment = &adjustmentCopy
		}
		progress.CurrentValueFen = progress.LinkedValueFen + progress.ManualAdjustmentFen
		if overview.CurrentTotalFen > 0 {
			progress.CurrentWeightBP = int(progress.CurrentValueFen * 10_000 / overview.CurrentTotalFen)
		}
		progress.BuildGapFen = progress.BuildTargetFen - progress.CurrentValueFen
		progress.RebalanceGapFen = progress.RebalanceTargetFen - progress.CurrentValueFen
		overview.Items = append(overview.Items, progress)
	}
	for code, position := range positionByCode {
		if _, ok := assigned[code]; !ok {
			overview.UnassignedPositions = append(overview.UnassignedPositions, position)
		}
	}
	return overview, nil
}

func (s *Service) ReviseAllocation(ctx context.Context, draft domain.AllocationDraft, reason string) (domain.AllocationVersion, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.AllocationVersion{}, fmt.Errorf("请填写修改原因")
	}
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return domain.AllocationVersion{}, err
	}
	legacy := profile.Mode == domain.UserModeLegacy
	if legacy {
		draft = allocationWithDefaultBindings(draft)
	}
	if err := validateAllocationDraft(&draft, legacy); err != nil {
		return domain.AllocationVersion{}, err
	}
	has, err := s.store.HasAllocation(ctx)
	if err != nil {
		return domain.AllocationVersion{}, err
	}
	if !has {
		return s.store.CreateAllocationProfile(ctx, draft, reason, s.now())
	}
	return s.store.CreateAllocationVersion(ctx, draft, reason, s.now())
}

func (s *Service) RecordAllocationAdjustment(ctx context.Context, itemKey string, adjustmentFen int64, observedAt time.Time) (domain.AllocationAdjustmentEvent, error) {
	state, err := s.Allocation(ctx)
	if err != nil {
		return domain.AllocationAdjustmentEvent{}, err
	}
	if state.Overview == nil {
		return domain.AllocationAdjustmentEvent{}, fmt.Errorf("请先建立资产配置")
	}
	overview := *state.Overview
	var linkedValue int64
	found := false
	for _, item := range overview.Items {
		if item.Item.Key == itemKey {
			linkedValue = item.LinkedValueFen
			found = true
			break
		}
	}
	if !found {
		return domain.AllocationAdjustmentEvent{}, fmt.Errorf("未知的资产项目：%s", itemKey)
	}
	if linkedValue+adjustmentFen < 0 {
		return domain.AllocationAdjustmentEvent{}, fmt.Errorf("调整后市值不能为负数")
	}
	if observedAt.IsZero() {
		return domain.AllocationAdjustmentEvent{}, fmt.Errorf("配置调整时间不能为空")
	}
	return s.store.AppendAllocationAdjustment(ctx, overview.ProfileID, domain.AllocationAdjustmentEvent{
		ItemKey: itemKey, AdjustmentFen: adjustmentFen, Source: "manual_adjustment", ObservedAt: observedAt,
	}, s.now())
}

func (s *Service) RecordAllocationValue(ctx context.Context, itemKey string, valueFen int64, observedAt time.Time) (domain.AllocationValueEvent, error) {
	snapshot, configured, err := s.currentAllocationSnapshot(ctx)
	if err != nil {
		return domain.AllocationValueEvent{}, err
	}
	if !configured {
		return domain.AllocationValueEvent{}, CodedError{Code: "ALLOCATION_NOT_CONFIGURED", Message: "请先建立资产配置"}
	}
	if !allocationContainsItem(snapshot.Version.Draft, itemKey) {
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
	_, configured, err := s.currentAllocationSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if !configured {
		return []domain.AllocationVersion{}, nil
	}
	return s.store.ListAllocationVersions(ctx)
}

func (s *Service) AllocationValueEvents(ctx context.Context, itemKey string) ([]domain.AllocationValueEvent, error) {
	snapshot, configured, err := s.currentAllocationSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if !configured {
		return []domain.AllocationValueEvent{}, nil
	}
	if !allocationContainsItem(snapshot.Version.Draft, itemKey) {
		return nil, fmt.Errorf("未知的资产项目：%s", itemKey)
	}
	return s.store.ListAllocationValueEvents(ctx, snapshot.ProfileID, itemKey)
}

func (s *Service) currentAllocationSnapshot(ctx context.Context) (domain.AllocationSnapshot, bool, error) {
	hasAllocation, err := s.store.HasAllocation(ctx)
	if err != nil {
		return domain.AllocationSnapshot{}, false, err
	}
	if hasAllocation {
		snapshot, err := s.store.CurrentAllocation(ctx)
		return snapshot, true, err
	}
	profile, err := s.store.UserProfile(ctx)
	if err != nil {
		return domain.AllocationSnapshot{}, false, err
	}
	if profile.Mode == domain.UserModeGeneric {
		return domain.AllocationSnapshot{}, false, nil
	}
	snapshot, err := s.store.EnsureInitialAllocation(ctx, s.now())
	return snapshot, err == nil, err
}

func allocationContainsItem(draft domain.AllocationDraft, itemKey string) bool {
	for _, item := range draft.Items {
		if item.Key == itemKey {
			return true
		}
	}
	return false
}

func validateAllocationDraft(draft *domain.AllocationDraft, legacy bool) error {
	if draft.InitialCapitalFen <= 0 {
		return fmt.Errorf("初始资金必须大于 0")
	}
	if draft.TargetReturnBP < 0 {
		return fmt.Errorf("目标收益不能为负数")
	}
	if draft.TargetDeadline != "" {
		if _, err := time.Parse("2006-01-02", draft.TargetDeadline); err != nil {
			return fmt.Errorf("目标截止日格式应为 YYYY-MM-DD")
		}
	}
	if legacy && len(draft.Items) != len(allocationItemKeys) {
		return fmt.Errorf("资产项目必须保留当前的 7 项配置")
	}
	if len(draft.Items) == 0 {
		return fmt.Errorf("请至少添加一个资产项目")
	}
	seen := make(map[string]struct{}, len(draft.Items))
	boundCodes := make(map[string]string)
	weightTotal := 0
	cashCount := 0
	for index := range draft.Items {
		item := &draft.Items[index]
		if item.Role != "" && item.Role != domain.AllocationRoleHolding && item.Role != domain.AllocationRoleCash {
			return fmt.Errorf("%s 的项目类型无效", item.Name)
		}
		item.Key = strings.TrimSpace(item.Key)
		item.Name = strings.TrimSpace(item.Name)
		item.BuyRule = strings.TrimSpace(item.BuyRule)
		item.SellRule = strings.TrimSpace(item.SellRule)
		item.Note = strings.TrimSpace(item.Note)
		if item.Key == "" {
			item.Key = fmt.Sprintf("allocation-item-%d", index+1)
		}
		if legacy {
			if _, ok := allocationItemKeys[item.Key]; !ok {
				return fmt.Errorf("不支持的资产项目：%s", item.Key)
			}
		}
		if _, duplicated := seen[item.Key]; duplicated {
			return fmt.Errorf("资产项目不能重复：%s", item.Key)
		}
		seen[item.Key] = struct{}{}
		if item.Name == "" || (legacy && (item.BuyRule == "" || item.SellRule == "")) {
			return fmt.Errorf("请完整填写 %s 的名称和规则", item.Key)
		}
		role := item.EffectiveRole()
		item.Role = role
		if role != domain.AllocationRoleHolding && role != domain.AllocationRoleCash {
			return fmt.Errorf("%s 的项目类型无效", item.Name)
		}
		if role == domain.AllocationRoleCash {
			cashCount++
			if len(item.InstrumentCodes) > 0 {
				return fmt.Errorf("现金项目不能关联证券")
			}
		}
		if item.TargetWeightBP < 0 || item.TargetWeightBP > 10_000 {
			return fmt.Errorf("%s 的目标权重无效", item.Name)
		}
		if item.TargetShares < 0 {
			return fmt.Errorf("%s 的目标股数不能为负数", item.Name)
		}
		for codeIndex, code := range item.InstrumentCodes {
			code = strings.ToUpper(strings.TrimSpace(code))
			if code == "" {
				return fmt.Errorf("%s 存在空的证券代码", item.Name)
			}
			if previous, duplicated := boundCodes[code]; duplicated {
				return fmt.Errorf("证券 %s 不能同时关联到 %s 和 %s", code, previous, item.Name)
			}
			boundCodes[code] = item.Name
			item.InstrumentCodes[codeIndex] = code
		}
		weightTotal += item.TargetWeightBP
	}
	if cashCount > 1 {
		return fmt.Errorf("最多只能有一个现金项目")
	}
	if weightTotal != 10_000 {
		return fmt.Errorf("目标权重合计必须为 100%%")
	}
	return nil
}

func allocationWithDefaultBindings(draft domain.AllocationDraft) domain.AllocationDraft {
	defaults := map[string][]string{
		"tencent":                            {"0700.HK"},
		"semiconductor-equipment-etf-159558": {"159558"},
		"communication-etf":                  {"515880"},
		"china-internet-etf":                 {"513050"},
	}
	for index := range draft.Items {
		if draft.Items[index].InstrumentCodes != nil {
			continue
		}
		if codes, ok := defaults[draft.Items[index].Key]; ok {
			draft.Items[index].InstrumentCodes = append([]string(nil), codes...)
		} else {
			draft.Items[index].InstrumentCodes = []string{}
		}
	}
	return draft
}
