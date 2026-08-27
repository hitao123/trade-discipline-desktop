package service

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

func TestAllocationUsesLinkedPortfolioCashAndSignedAdjustment(t *testing.T) {
	svc := openExecutionService(t)
	ctx := context.Background()
	for _, instrument := range []struct {
		id, code, name string
	}{
		{"sz-159558", "159558", "半导体设备ETF"},
		{"sh-510050", "510050", "上证50ETF"},
	} {
		if _, err := svc.store.DB().ExecContext(ctx, `INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES(?, 'SZ', ?, ?, 'etf', 'CNY', 100, 'fixture', 0, 0, 'active')`, instrument.id, instrument.code, instrument.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.RecordExecution(ctx, ExecutionDraft{InstrumentID: "sz-159558", Side: "buy", Quantity: 1000, LocalPriceTenThousandth: 11_400, SettlementFen: -114_000, ExecutedAt: svc.now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordExecution(ctx, ExecutionDraft{InstrumentID: "sh-510050", Side: "buy", Quantity: 200, LocalPriceTenThousandth: 10_000, SettlementFen: -20_000, ExecutedAt: svc.now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordAllocationAdjustment(ctx, "semiconductor-equipment-etf-159558", 1_000, svc.now()); err != nil {
		t.Fatal(err)
	}

	state, err := svc.Allocation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	overview := allocationOverviewFromState(t, state)
	semi := allocationItemByKey(t, overview, "semiconductor-equipment-etf-159558")
	cash := allocationItemByKey(t, overview, "cash-fund")
	if semi.LinkedValueFen != 114_000 || semi.ManualAdjustmentFen != 1_000 || semi.CurrentValueFen != 115_000 {
		t.Fatalf("semi=%#v", semi)
	}
	if cash.CurrentValueFen != 19_866_000 || overview.CurrentTotalFen != 20_001_000 {
		t.Fatalf("cash=%#v overview=%#v", cash, overview)
	}
	if len(overview.UnassignedPositions) != 1 || overview.UnassignedPositions[0].Code != "510050" {
		t.Fatalf("unassigned=%#v", overview.UnassignedPositions)
	}
}

func TestAllocationTracksTenPercentGoalAndTwoKindsOfDeviation(t *testing.T) {
	svc := openExecutionService(t)
	state, err := svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := allocationOverviewFromState(t, state)
	if snapshot.CurrentTotalFen != 20_000_000 || snapshot.TargetTotalFen != 22_000_000 || snapshot.ReturnBP != 0 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if _, err := svc.RecordAllocationAdjustment(context.Background(), "semiconductor-equipment-etf-159558", 250_000, svc.now()); err != nil {
		t.Fatal(err)
	}
	state, err = svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot = allocationOverviewFromState(t, state)
	if snapshot.CurrentTotalFen != 20_250_000 || snapshot.ReturnBP != 125 || snapshot.GoalGapFen != 1_750_000 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	item := allocationItemByKey(t, snapshot, "semiconductor-equipment-etf-159558")
	if item.BuildGapFen != 2_250_000 || item.RebalanceGapFen != 2_281_250 {
		t.Fatalf("item=%#v", item)
	}
}

func TestAllocationRevisionRequiresReasonAndExactWeights(t *testing.T) {
	svc := openExecutionService(t)
	state, err := svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	initial := allocationOverviewFromState(t, state)
	if _, err := svc.ReviseAllocation(context.Background(), initial.Version.Draft, ""); err == nil {
		t.Fatal("expected missing reason rejection")
	}
	bad := initial.Version.Draft
	bad.Items[0].TargetWeightBP--
	if _, err := svc.ReviseAllocation(context.Background(), bad, "权重未加总到 100%"); err == nil {
		t.Fatal("expected invalid weight rejection")
	}
	good := initial.Version.Draft
	good.Items[0].TargetWeightBP = 3_800
	good.Items[1].TargetWeightBP = 1_850
	revised, err := svc.ReviseAllocation(context.Background(), good, "降低腾讯目标权重")
	if err != nil || revised.Version != 2 {
		t.Fatalf("revised=%#v err=%v", revised, err)
	}
}

func allocationOverviewFromState(t *testing.T, state domain.AllocationState) AllocationOverview {
	t.Helper()
	if !state.Configured || state.Overview == nil {
		t.Fatalf("allocation is not configured: %#v", state)
	}
	return *state.Overview
}

func TestAllocationRejectsValueForUnknownAsset(t *testing.T) {
	svc := openExecutionService(t)
	if _, err := svc.RecordAllocationValue(context.Background(), "unknown", 1_000, svc.now()); err == nil {
		t.Fatal("expected unknown asset rejection")
	}
}

func TestGenericAllocationStartsBlankAndAcceptsArbitraryItems(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	versions, err := svc.AllocationVersions(context.Background())
	if err != nil || len(versions) != 0 {
		t.Fatalf("blank generic allocation versions=%#v err=%v", versions, err)
	}
	state, err := svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Configured || state.SuggestedCapitalFen != 20_000_000 || state.Overview != nil {
		t.Fatalf("state=%#v", state)
	}
	_, err = svc.ReviseAllocation(context.Background(), domain.AllocationDraft{
		InitialCapitalFen: 20_000_000,
		Items: []domain.AllocationItem{
			{Key: "global-equity", Name: "全球股票", Role: domain.AllocationRoleHolding, TargetWeightBP: 5_000},
			{Key: "cash", Name: "现金", Role: domain.AllocationRoleCash, TargetWeightBP: 5_000},
		},
	}, "建立第一版通用配置")
	if err != nil {
		t.Fatal(err)
	}
	state, err = svc.Allocation(context.Background())
	if err != nil || !state.Configured || state.Overview == nil || len(state.Overview.Items) != 2 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if cash := allocationItemByKey(t, *state.Overview, "cash"); cash.LinkedValueFen != 20_000_000 {
		t.Fatalf("cash=%#v", cash)
	}
	if _, err := svc.RecordAllocationValue(context.Background(), "global-equity", 100_000, svc.now()); err != nil {
		t.Fatal(err)
	}
}

func allocationItemByKey(t *testing.T, snapshot AllocationOverview, key string) AllocationItemProgress {
	t.Helper()
	for _, item := range snapshot.Items {
		if item.Item.Key == key {
			return item
		}
	}
	t.Fatalf("missing item %s", key)
	return AllocationItemProgress{}
}
