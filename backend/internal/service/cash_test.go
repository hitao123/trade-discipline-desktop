package service

import (
	"context"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

func TestCashDepositIncreasesPortfolioAndDashboardCapital(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	if _, err := svc.RecordCashEvent(context.Background(), CashEventDraft{
		EventType: "deposit", AmountFen: 5_000_000, Reason: "券商转入", OccurredAt: time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil || portfolio.AvailableCashFen != 25_000_000 {
		t.Fatalf("portfolio=%#v err=%v", portfolio, err)
	}
	dashboard, err := svc.Dashboard(context.Background())
	if err != nil || dashboard.InitialCapitalFen != 25_000_000 {
		t.Fatalf("dashboard=%#v err=%v", dashboard, err)
	}
}

func TestCashWithdrawalReducesAvailableCashWithoutChangingProfile(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	if _, err := svc.RecordCashEvent(context.Background(), CashEventDraft{
		EventType: "withdrawal", AmountFen: 3_000_000, Reason: "转出生活费",
	}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil || portfolio.AvailableCashFen != 17_000_000 {
		t.Fatalf("portfolio=%#v err=%v", portfolio, err)
	}
	profile, err := svc.Onboarding(context.Background())
	if err != nil || profile.InvestableCapitalFen != 20_000_000 {
		t.Fatalf("profile should stay at onboarding baseline: %#v err=%v", profile, err)
	}
	dashboard, err := svc.Dashboard(context.Background())
	if err != nil || dashboard.InitialCapitalFen != 17_000_000 {
		t.Fatalf("dashboard=%#v err=%v", dashboard, err)
	}
}

func TestDividendDoesNotChangeFundedCapital(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	if _, err := svc.RecordCashEvent(context.Background(), CashEventDraft{
		EventType: "dividend", AmountFen: 12_000, Reason: "ETF 分红",
	}); err != nil {
		t.Fatal(err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil || portfolio.AvailableCashFen != 20_012_000 {
		t.Fatalf("portfolio=%#v err=%v", portfolio, err)
	}
	dashboard, err := svc.Dashboard(context.Background())
	if err != nil || dashboard.InitialCapitalFen != 20_000_000 {
		t.Fatalf("dividend should not change funded capital: %#v", dashboard)
	}
}

func TestProfileCapitalEditDoesNotChangeLedger(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	if _, err := svc.UpdateUserProfile(context.Background(), UpdateUserProfileInput{
		CompleteOnboardingInput: CompleteOnboardingInput{
			InvestableCapitalFen: 40_000_000,
			MaxLossFen:           3_000_000,
			HoldingHorizon:       domain.Horizon1To3Y,
			EnabledMarkets:       []domain.MarketScope{domain.MarketHK},
		},
		Reason: "只改资料展示",
	}); err != nil {
		t.Fatal(err)
	}
	dashboard, err := svc.Dashboard(context.Background())
	if err != nil || dashboard.InitialCapitalFen != 20_000_000 {
		t.Fatalf("dashboard still uses account baseline: %#v err=%v", dashboard, err)
	}
}
