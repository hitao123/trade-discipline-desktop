package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
)

func (s *Store) InstrumentByMarketCode(ctx context.Context, marketName, code string) (InstrumentRow, bool, error) {
	var item InstrumentRow
	var tech int
	err := s.db.QueryRowContext(ctx, `SELECT id, market, code, name, asset_type, currency, lot_size, is_china_tech FROM instruments WHERE market=? AND code=? AND status='active'`, marketName, code).
		Scan(&item.ID, &item.Market, &item.Code, &item.Name, &item.AssetType, &item.Currency, &item.LotSize, &tech)
	if err == sql.ErrNoRows {
		return InstrumentRow{}, false, nil
	}
	if err != nil {
		return InstrumentRow{}, false, fmt.Errorf("load instrument by code: %w", err)
	}
	item.IsChinaTech = tech == 1
	if item.AssetType == string(market.KindETF) && !market.IsEligibleETF(item.Code, item.Name) {
		return InstrumentRow{}, false, nil
	}
	return item, true, nil
}

func (s *Store) UpsertResolvedInstrument(ctx context.Context, quote market.Quote, resolvedAt time.Time) (InstrumentRow, error) {
	if quote.AssetType == market.KindETF && !market.IsEligibleETF(quote.Code, quote.Name) {
		return InstrumentRow{}, fmt.Errorf("不支持债券、货币或现金管理类 ETF")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InstrumentRow{}, fmt.Errorf("begin resolved instrument: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID, existingName string
	err = tx.QueryRowContext(ctx, `SELECT id, name FROM instruments WHERE market=? AND code=?`, quote.Market, quote.Code).Scan(&existingID, &existingName)
	created := err == sql.ErrNoRows
	if err != nil && err != sql.ErrNoRows {
		return InstrumentRow{}, fmt.Errorf("check resolved instrument: %w", err)
	}
	if created {
		existingID = strings.ToLower(quote.Market) + "-" + quote.Code
	}
	currency := market.CurrencyForMarket(quote.Market)
	lotSize := market.DefaultLotSize(quote.Market)
	_, err = tx.ExecContext(ctx, `INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status, latest_price_minor, latest_price_at, latest_price_source)
		VALUES(?,?,?,?,?,?,?,'public_quote',0,0,'active',?,?,?)
		ON CONFLICT(market,code) DO UPDATE SET name=excluded.name, asset_type=excluded.asset_type, status='active', latest_price_minor=excluded.latest_price_minor, latest_price_at=excluded.latest_price_at, latest_price_source=excluded.latest_price_source`,
		existingID, quote.Market, quote.Code, quote.Name, quote.AssetType, currency, lotSize, quote.CloseMinor, quote.SourceTime.UTC().Format(time.RFC3339Nano), quote.Source)
	if err != nil {
		return InstrumentRow{}, fmt.Errorf("upsert resolved instrument: %w", err)
	}
	if created {
		after, _ := json.Marshal(map[string]any{"market": quote.Market, "code": quote.Code, "name": quote.Name, "assetType": quote.AssetType, "source": quote.Source})
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'instrument', ?, 'resolved_from_public_quote', NULL, ?, ?)`, NewID("audit"), existingID, string(after), resolvedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return InstrumentRow{}, fmt.Errorf("audit resolved instrument: %w", err)
		}
	} else if existingName != quote.Name {
		before, _ := json.Marshal(map[string]any{"name": existingName})
		after, _ := json.Marshal(map[string]any{"name": quote.Name, "source": quote.Source})
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'instrument', ?, 'resolved_instrument_name_repaired', ?, ?, ?)`, NewID("audit"), existingID, string(before), string(after), resolvedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return InstrumentRow{}, fmt.Errorf("audit repaired instrument name: %w", err)
		}
	}

	var item InstrumentRow
	var tech int
	if err := tx.QueryRowContext(ctx, `SELECT id, market, code, name, asset_type, currency, lot_size, is_china_tech FROM instruments WHERE market=? AND code=?`, quote.Market, quote.Code).
		Scan(&item.ID, &item.Market, &item.Code, &item.Name, &item.AssetType, &item.Currency, &item.LotSize, &tech); err != nil {
		return InstrumentRow{}, fmt.Errorf("load resolved instrument: %w", err)
	}
	item.IsChinaTech = tech == 1
	if err := tx.Commit(); err != nil {
		return InstrumentRow{}, fmt.Errorf("commit resolved instrument: %w", err)
	}
	return item, nil
}

type RegisterInstrumentInput struct {
	Market    string
	Code      string
	Name      string
	AssetType string
	Currency  string
	LotSize   int
	Source    string
}

func (s *Store) RegisterInstrument(ctx context.Context, input RegisterInstrumentInput, now time.Time) (InstrumentRow, error) {
	if input.LotSize <= 0 {
		input.LotSize = market.DefaultLotSize(input.Market)
	}
	if input.Currency == "" {
		input.Currency = market.CurrencyForMarket(input.Market)
	}
	if input.Source == "" {
		input.Source = "manual"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InstrumentRow{}, fmt.Errorf("begin register instrument: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id := strings.ToLower(input.Market) + "-" + strings.ReplaceAll(input.Code, ".", "")
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO instruments(id, market, code, name, asset_type, currency, lot_size, lot_source, is_china_tech, is_st, status)
		VALUES(?,?,?,?,?,?,?,?,0,0,'active')
		ON CONFLICT(market,code) DO UPDATE SET name=excluded.name, asset_type=excluded.asset_type, currency=excluded.currency, lot_size=excluded.lot_size, status='active'`,
		id, input.Market, input.Code, input.Name, input.AssetType, input.Currency, input.LotSize, input.Source); err != nil {
		return InstrumentRow{}, fmt.Errorf("register instrument: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"market": input.Market, "code": input.Code, "name": input.Name, "currency": input.Currency, "lotSize": input.LotSize, "source": input.Source})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?, 'instrument', ?, 'registered', NULL, ?, ?)`, NewID("audit"), id, string(after), stamp); err != nil {
		return InstrumentRow{}, fmt.Errorf("audit registered instrument: %w", err)
	}
	var item InstrumentRow
	var tech int
	if err := tx.QueryRowContext(ctx, `SELECT id, market, code, name, asset_type, currency, lot_size, is_china_tech FROM instruments WHERE market=? AND code=?`, input.Market, input.Code).
		Scan(&item.ID, &item.Market, &item.Code, &item.Name, &item.AssetType, &item.Currency, &item.LotSize, &tech); err != nil {
		return InstrumentRow{}, fmt.Errorf("load registered instrument: %w", err)
	}
	item.IsChinaTech = tech == 1
	if err := tx.Commit(); err != nil {
		return InstrumentRow{}, fmt.Errorf("commit registered instrument: %w", err)
	}
	return item, nil
}
