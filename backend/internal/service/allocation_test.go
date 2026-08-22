package service

import (
	"context"
	"testing"
)

func TestAllocationTracksTenPercentGoalAndTwoKindsOfDeviation(t *testing.T) {
	svc := openExecutionService(t)
	snapshot, err := svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTotalFen != 20_000_000 || snapshot.TargetTotalFen != 22_000_000 || snapshot.ReturnBP != 0 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if _, err := svc.RecordAllocationValue(context.Background(), "semiconductor-equipment-etf-159558", 1_250_000, svc.now()); err != nil {
		t.Fatal(err)
	}
	snapshot, err = svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTotalFen != 20_250_000 || snapshot.ReturnBP != 125 || snapshot.GoalGapFen != 1_750_000 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	item := allocationItemByKey(t, snapshot, "semiconductor-equipment-etf-159558")
	if item.BuildGapFen != 1_250_000 || item.RebalanceGapFen != 1_281_250 {
		t.Fatalf("item=%#v", item)
	}
}

func TestAllocationRevisionRequiresReasonAndExactWeights(t *testing.T) {
	svc := openExecutionService(t)
	initial, err := svc.Allocation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
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

func TestAllocationRejectsValueForUnknownAsset(t *testing.T) {
	svc := openExecutionService(t)
	if _, err := svc.RecordAllocationValue(context.Background(), "unknown", 1_000, svc.now()); err == nil {
		t.Fatal("expected unknown asset rejection")
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
