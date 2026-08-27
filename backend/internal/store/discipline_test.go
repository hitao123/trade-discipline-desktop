package store

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
)

var disciplineNow = time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)

func disciplinePlan(t *testing.T, db *Store) PlanRow {
	t.Helper()
	plan, err := db.SavePlan(context.Background(), "rule-1", "qualified", domain.TradePlanDraft{
		InstrumentID: "hk-9988", RiskExitMinor: 9_000, ValidUntil: disciplineNow.AddDate(0, 0, 7),
	}, rules.Decision{Qualified: true}, disciplineNow)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestCreatePreTradeConfirmationKeepsPlanSnapshot(t *testing.T) {
	db := openLegacyTestStore(t)
	plan := disciplinePlan(t, db)
	confirmed, err := db.CreatePreTradeConfirmation(context.Background(), plan.ID, `{"code":"9988.HK"}`, disciplineNow.Add(-30*time.Second), disciplineNow)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := db.LatestPreTradeConfirmation(context.Background(), plan.ID)
	if err != nil || latest == nil {
		t.Fatalf("latest=%#v err=%v", latest, err)
	}
	if latest.ID != confirmed.ID || latest.PlanSnapshotJSON != `{"code":"9988.HK"}` {
		t.Fatalf("confirmation=%#v latest=%#v", confirmed, latest)
	}
}

func TestCreatePriceAlertIfCrossedSuppressesContinuousTrigger(t *testing.T) {
	db := openLegacyTestStore(t)
	plan := disciplinePlan(t, db)
	alert := domain.PriceAlertEvent{
		PlanID: plan.ID, InstrumentID: "hk-9988", Kind: domain.AlertRiskExit,
		TriggerPriceMinor: 8_900, ThresholdMinor: 9_000, PlanSnapshotJSON: `{"riskExitMinor":9000}`,
		Source: "fixture", SourceTime: disciplineNow, TriggeredAt: disciplineNow,
	}
	created, first, err := db.CreatePriceAlertIfCrossed(context.Background(), alert)
	if err != nil || !created || first.ID == "" {
		t.Fatalf("created=%v first=%#v err=%v", created, first, err)
	}
	created, second, err := db.CreatePriceAlertIfCrossed(context.Background(), alert)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("created=%v second=%#v err=%v", created, second, err)
	}
	if err := db.RecordAlertState(context.Background(), plan.ID, domain.AlertRiskExit, false, disciplineNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	created, third, err := db.CreatePriceAlertIfCrossed(context.Background(), alert)
	if err != nil || !created || third.ID == first.ID {
		t.Fatalf("created=%v third=%#v err=%v", created, third, err)
	}
}

func TestCreatePositionReviewClosesOnlyThatAlert(t *testing.T) {
	db := openLegacyTestStore(t)
	plan := disciplinePlan(t, db)
	first := createRiskAlert(t, db, plan.ID, disciplineNow)
	if err := db.RecordAlertState(context.Background(), plan.ID, domain.AlertRiskExit, false, disciplineNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	second := createRiskAlert(t, db, plan.ID, disciplineNow.Add(2*time.Minute))
	review, err := db.CreatePositionReview(context.Background(), first.ID, domain.ReviewHold, "继续观察财报证据", disciplineNow.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if review.AlertID != first.ID {
		t.Fatalf("review=%#v", review)
	}
	pending, err := db.ListPendingAlerts(context.Background())
	if err != nil || len(pending) != 1 || pending[0].ID != second.ID {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
}

func TestListUnnotifiedAlertsSkipsReviewedAlert(t *testing.T) {
	db := openLegacyTestStore(t)
	plan := disciplinePlan(t, db)
	alert := createRiskAlert(t, db, plan.ID, disciplineNow)
	if _, err := db.CreatePositionReview(context.Background(), alert.ID, domain.ReviewHold, "已经完成复核", disciplineNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	alerts, err := db.ListUnnotifiedAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 0 {
		t.Fatalf("unnotified alerts=%#v", alerts)
	}
}

func TestMonitorSettingsDefaultsToTenMinutesAndRejectsUnknownInterval(t *testing.T) {
	db := openTestStore(t)
	settings, err := db.MonitorSettings(context.Background())
	if err != nil || settings.Interval != "10m" {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
	if err := db.SaveMonitorSettings(context.Background(), domain.MonitorSettings{Interval: "1m"}, disciplineNow); err == nil {
		t.Fatal("unknown interval should be rejected")
	}
}

func createRiskAlert(t *testing.T, db *Store, planID string, now time.Time) domain.PriceAlertEvent {
	t.Helper()
	created, alert, err := db.CreatePriceAlertIfCrossed(context.Background(), domain.PriceAlertEvent{
		PlanID: planID, InstrumentID: "hk-9988", Kind: domain.AlertRiskExit,
		TriggerPriceMinor: 8_900, ThresholdMinor: 9_000, PlanSnapshotJSON: `{"riskExitMinor":9000}`,
		Source: "fixture", SourceTime: now, TriggeredAt: now,
	})
	if err != nil || !created {
		t.Fatalf("created=%v alert=%#v err=%v", created, alert, err)
	}
	return alert
}
