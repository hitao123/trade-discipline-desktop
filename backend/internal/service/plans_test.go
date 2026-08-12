package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

func validPlanDraft() domain.TradePlanDraft {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	return domain.TradePlanDraft{
		InstrumentID: "hk-9988", Thesis: "云业务利润率改善", Falsification: "云收入降速",
		PricedExpectation: "市场只计入温和复苏", Evidence: []string{"下季云收入增速"}, BreakCondition: "云收入同比转负",
		ExitCondition: "逻辑破坏或估值过高", EntryLowMinor: 11_000, EntryHighMinor: 12_000, RiskExitMinor: 9_000,
		Quantity: 100, EstimatedCostFen: 1_200_000,
		MaxPlanLossFen: 360_000, StressDropBP: 3_000, ReferencePriceAt: now.Add(-time.Hour), ValidUntil: now.AddDate(0, 0, 7),
	}
}

func TestCreatePlanPersistsRejectedDecision(t *testing.T) {
	svc := openExecutionService(t)
	draft := validPlanDraft()
	draft.Quantity = 150
	plan, err := svc.CreatePlan(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "rejected" || plan.Validation.Qualified {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Validation.Findings[0].Code != "BOARD_LOT_REQUIRED" {
		t.Fatalf("unexpected findings: %#v", plan.Validation.Findings)
	}
	var count int
	if err := svc.store.DB().QueryRow("SELECT count(*) FROM trade_plans WHERE id=?", plan.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestRuleVersionRequiresReasonAndPreservesPlanReference(t *testing.T) {
	svc := openExecutionService(t)
	plan, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.store.CurrentRule(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.ChinaTechLimitFen = 7_000_000
	if _, err := svc.CreateRuleVersion(context.Background(), "", current); err == nil {
		t.Fatal("expected empty reason rejection")
	}
	version, err := svc.CreateRuleVersion(context.Background(), "降低未来中国科技敞口", current)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RuleVersionID == version.ID || version.Version != 2 {
		t.Fatalf("plan=%#v version=%#v", plan, version)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(svc.store.Path()), "backups", "before-rule-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("rule change backup missing: %v, %v", backups, err)
	}
}

func TestRevisePlanKeepsOriginalInRevisionAndAudit(t *testing.T) {
	svc := openExecutionService(t)
	created, err := svc.CreatePlan(context.Background(), validPlanDraft())
	if err != nil {
		t.Fatal(err)
	}
	draft := created.Draft
	draft.Thesis = "云业务利润率改善，且回购持续"
	revised, err := svc.RevisePlan(context.Background(), created.ID, "补充回购证据", draft)
	if err != nil {
		t.Fatal(err)
	}
	if revised.Draft.Thesis == created.Draft.Thesis {
		t.Fatalf("plan was not revised: %#v", revised)
	}
	var revisions, audits int
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM trade_plan_revisions WHERE plan_id=?`, created.ID).Scan(&revisions); err != nil || revisions != 1 {
		t.Fatalf("revisions=%d err=%v", revisions, err)
	}
	if err := svc.store.DB().QueryRow(`SELECT count(*) FROM audit_events WHERE entity_id=? AND action='revised'`, created.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
}
