package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func seedLegacyTestData(t *testing.T, db *store.Store) {
	t.Helper()
	ctx := context.Background()
	stamp := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	raw, err := json.Marshal(rules.InitialSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE user_profiles SET mode='generic', onboarding_status='completed', investable_capital_fen=20000000, max_loss_fen=2000000, holding_horizon='6_to_12m', enabled_markets_json='["ashare_stock","ashare_etf","hk"]', completed_at=?, updated_at=? WHERE id='local-user'`, []any{stamp, stamp}},
		{`INSERT INTO accounts(id, name, base_currency, initial_capital_fen, enabled_at, status) VALUES('account-main','我的纪律账户','CNY',20000000,?,'active')`, []any{stamp}},
		{`INSERT INTO rule_versions(id, version, snapshot_json, change_reason, created_at, previous_id) VALUES('rule-1',1,?,'初始交易纪律规则',?,NULL)`, []any{string(raw), stamp}},
		{`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES('hk-0700','HK','0700.HK','腾讯控股','stock','HKD',100,'legacy_fixture',1,0,'active')`, nil},
		{`INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status) VALUES('hk-9988','HK','9988.HK','阿里巴巴-W','stock','HKD',100,'legacy_fixture',1,0,'active')`, nil},
	}
	for _, statement := range statements {
		if _, err := db.DB().ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.EnsureInitialAllocation(ctx, time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

func openGenericService(t *testing.T, capitalFen, maxLossFen int64) *Service {
	t.Helper()
	svc := openExecutionServiceWithoutSeed(t)
	_, err := svc.CompleteOnboarding(context.Background(), CompleteOnboardingInput{
		InvestableCapitalFen: capitalFen,
		MaxLossFen:           maxLossFen,
		HoldingHorizon:       domain.Horizon6To12M,
		EnabledMarkets:       []domain.MarketScope{domain.MarketAShareStock, domain.MarketAShareETF, domain.MarketHK},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}
