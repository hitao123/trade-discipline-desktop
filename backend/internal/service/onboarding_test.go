package service

import (
	"context"
	"testing"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
)

func TestUpdateUserProfileCreatesRuleVersionWithoutChangingAccountCash(t *testing.T) {
	svc := openGenericService(t, 20_000_000, 2_000_000)
	profile, err := svc.UpdateUserProfile(context.Background(), UpdateUserProfileInput{
		CompleteOnboardingInput: CompleteOnboardingInput{
			InvestableCapitalFen: 30_000_000,
			MaxLossFen:           2_500_000,
			HoldingHorizon:       domain.Horizon1To3Y,
			EnabledMarkets:       []domain.MarketScope{domain.MarketAShareETF, domain.MarketHK},
		},
		Reason: "调整明年的可投资资金",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 资金基准只通过资金变动维护，资料更新不改变本金，只更新损失上限等偏好。
	if profile.InvestableCapitalFen != 20_000_000 || profile.MaxLossFen != 2_500_000 {
		t.Fatalf("profile=%#v", profile)
	}
	versions, err := svc.ListRuleVersions(context.Background())
	if err != nil || len(versions) != 2 || versions[0].Snapshot.LossRedLineFen != 2_500_000 {
		t.Fatalf("versions=%#v err=%v", versions, err)
	}
	portfolio, err := svc.Portfolio(context.Background())
	if err != nil || portfolio.AvailableCashFen != 20_000_000 {
		t.Fatalf("portfolio=%#v err=%v", portfolio, err)
	}
}
