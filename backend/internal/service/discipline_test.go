package service

import (
	"context"
	"testing"
	"time"
)

func TestConfirmPreTradeRequiresThirtySecondsAndAllAttestations(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	now := svc.now()
	valid := PreTradeConfirmationInput{StartedAt: now.Add(-30 * time.Second), NoFOMO: true, NoLossRecovery: true, NoAveragingDown: true}
	if _, err := svc.ConfirmPreTrade(context.Background(), plan.ID, PreTradeConfirmationInput{StartedAt: now.Add(-29 * time.Second), NoFOMO: true, NoLossRecovery: true, NoAveragingDown: true}); err == nil {
		t.Fatal("confirmation should wait for 30 seconds")
	}
	if _, err := svc.ConfirmPreTrade(context.Background(), plan.ID, PreTradeConfirmationInput{StartedAt: now.Add(-30 * time.Second), NoFOMO: false, NoLossRecovery: true, NoAveragingDown: true}); err == nil {
		t.Fatal("confirmation should require every attestation")
	}
	confirmed, err := svc.ConfirmPreTrade(context.Background(), plan.ID, valid)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.PlanID != plan.ID || confirmed.PlanSnapshotJSON == "" {
		t.Fatalf("confirmation=%#v", confirmed)
	}
}

func TestConfirmPreTradeRejectsRejectedAndExpiredPlans(t *testing.T) {
	svc := openExecutionService(t)
	rejectedDraft := validPlanDraft()
	rejectedDraft.Quantity = 150
	rejected, err := svc.CreatePlan(context.Background(), rejectedDraft)
	if err != nil {
		t.Fatal(err)
	}
	expiredDraft := validPlanDraft()
	expiredDraft.ValidUntil = svc.now().Add(-time.Minute)
	expired, err := svc.CreatePlan(context.Background(), expiredDraft)
	if err != nil {
		t.Fatal(err)
	}
	input := PreTradeConfirmationInput{StartedAt: svc.now().Add(-30 * time.Second), NoFOMO: true, NoLossRecovery: true, NoAveragingDown: true}
	if _, err := svc.ConfirmPreTrade(context.Background(), rejected.ID, input); err == nil {
		t.Fatal("rejected plan should not be confirmed")
	}
	if _, err := svc.ConfirmPreTrade(context.Background(), expired.ID, input); err == nil {
		t.Fatal("expired plan should not be confirmed")
	}
}
