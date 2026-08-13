package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type MarketResult struct {
	Stock    store.MarketSnapshotRow `json:"stock"`
	ETF      store.MarketSnapshotRow `json:"etf"`
	Overview MarketOverviewResult    `json:"overview"`
	Errors   map[string]string       `json:"errors,omitempty"`
}

type MarketOverviewResult struct {
	Range            string               `json:"range"`
	AShareTurnover   []market.MetricPoint `json:"aShareTurnover"`
	SouthboundNetBuy []market.MetricPoint `json:"southboundNetBuy"`
	LastSuccessfulAt *time.Time           `json:"lastSuccessfulAt,omitempty"`
	Cached           bool                 `json:"cached"`
	Errors           map[string]string    `json:"errors,omitempty"`
}

type MarketHistoryResult struct {
	Market           string            `json:"market"`
	Code             string            `json:"code"`
	Range            string            `json:"range"`
	Points           []market.DailyBar `json:"points"`
	LastSuccessfulAt *time.Time        `json:"lastSuccessfulAt,omitempty"`
	Cached           bool              `json:"cached"`
	Error            string            `json:"error,omitempty"`
}

func (s *Service) SetMarketProvider(provider market.Provider) {
	s.marketProvider = provider
}

func (s *Service) RefreshMarket(ctx context.Context) (MarketResult, error) {
	result := MarketResult{Errors: make(map[string]string)}
	for _, kind := range []market.RankingKind{market.KindStock, market.KindETF} {
		quotes, err := s.marketProvider.FetchRankings(ctx, kind)
		if err != nil {
			result.Errors[string(kind)] = err.Error()
			continue
		}
		row, err := s.store.SaveMarketSnapshot(ctx, kind, quotes, "eastmoney-public-close", s.now())
		if err != nil {
			result.Errors[string(kind)] = err.Error()
			continue
		}
		if kind == market.KindStock {
			result.Stock = row
		} else {
			result.ETF = row
		}
	}
	if result.Stock.ID == "" {
		latest, err := s.store.LatestMarketSnapshot(ctx, market.KindStock)
		if err != nil {
			result.Errors["stock_cache"] = err.Error()
		} else {
			result.Stock = latest
		}
	}
	if result.ETF.ID == "" {
		latest, err := s.store.LatestMarketSnapshot(ctx, market.KindETF)
		if err != nil {
			result.Errors["etf_cache"] = err.Error()
		} else {
			result.ETF = latest
		}
	}
	keys, err := s.store.ActiveQuoteKeys(ctx)
	if err != nil {
		result.Errors["quotes"] = err.Error()
	} else {
		quotes, err := s.marketProvider.FetchQuotes(ctx, keys)
		if err != nil {
			result.Errors["quotes"] = err.Error()
		} else if err := s.store.UpdateQuotes(ctx, quotes); err != nil {
			result.Errors["quotes"] = err.Error()
		}
	}
	batch := s.marketProvider.FetchMarketMetrics(ctx, 120)
	for kind, message := range batch.Errors {
		result.Errors[string(kind)] = message
	}
	if len(batch.Points) > 0 {
		if _, err := s.store.SaveMetricPoints(ctx, batch.Points, s.now()); err != nil {
			result.Errors["market_metrics"] = err.Error()
		}
	}
	overview, err := s.MarketOverview(ctx, "3m")
	if err != nil {
		result.Errors["overview_cache"] = err.Error()
	} else {
		overview.Errors = copyErrors(result.Errors, marketMetricErrorKeys...)
		overview.Cached = len(overview.Errors) > 0 && (len(overview.AShareTurnover) > 0 || len(overview.SouthboundNetBuy) > 0)
		result.Overview = overview
	}
	if result.Stock.ID == "" && result.ETF.ID == "" &&
		len(result.Overview.AShareTurnover) == 0 && len(result.Overview.SouthboundNetBuy) == 0 {
		return result, fmt.Errorf("股票、ETF 和市场概览均无可用数据")
	}
	return result, nil
}

func (s *Service) LatestMarket(ctx context.Context) (MarketResult, error) {
	stock, err := s.store.LatestMarketSnapshot(ctx, market.KindStock)
	if err != nil {
		return MarketResult{}, err
	}
	etf, err := s.store.LatestMarketSnapshot(ctx, market.KindETF)
	if err != nil {
		return MarketResult{}, err
	}
	overview, err := s.MarketOverview(ctx, "3m")
	if err != nil {
		return MarketResult{}, err
	}
	return MarketResult{Stock: stock, ETF: etf, Overview: overview}, nil
}

var marketMetricErrorKeys = []string{
	string(market.MetricAShareTurnover), string(market.MetricSouthboundNetBuy),
	string(market.MetricSHTurnover), string(market.MetricSZTurnover),
	string(market.MetricSouthboundSHNetBuy), string(market.MetricSouthboundSZNetBuy), "market_metrics",
}

func (s *Service) MarketOverview(ctx context.Context, rangeName string) (MarketOverviewResult, error) {
	since, err := marketRangeStart(s.now(), rangeName)
	if err != nil {
		return MarketOverviewResult{}, err
	}
	turnover, turnoverFetchedAt, err := s.store.LatestMetricPoints(ctx, market.MetricAShareTurnover, since)
	if err != nil {
		return MarketOverviewResult{}, err
	}
	southbound, southboundFetchedAt, err := s.store.LatestMetricPoints(ctx, market.MetricSouthboundNetBuy, since)
	if err != nil {
		return MarketOverviewResult{}, err
	}
	lastSuccessfulAt := turnoverFetchedAt
	if southboundFetchedAt.After(lastSuccessfulAt) {
		lastSuccessfulAt = southboundFetchedAt
	}
	result := MarketOverviewResult{
		Range: rangeName, AShareTurnover: turnover, SouthboundNetBuy: southbound,
	}
	if !lastSuccessfulAt.IsZero() {
		stamp := lastSuccessfulAt.UTC()
		result.LastSuccessfulAt = &stamp
	}
	return result, nil
}

func (s *Service) MarketHistory(ctx context.Context, key market.InstrumentKey, rangeName string) (MarketHistoryResult, error) {
	if err := validateHistoryKey(key); err != nil {
		return MarketHistoryResult{}, err
	}
	since, err := marketRangeStart(s.now(), rangeName)
	if err != nil {
		return MarketHistoryResult{}, err
	}
	points, fetchedAt, err := s.store.LatestDailyBars(ctx, key, since)
	if err != nil {
		return MarketHistoryResult{}, err
	}
	result := MarketHistoryResult{Market: key.Market, Code: key.Code, Range: rangeName, Points: points}
	if !fetchedAt.IsZero() {
		stamp := fetchedAt.UTC()
		result.LastSuccessfulAt = &stamp
	}
	return result, nil
}

func (s *Service) RefreshMarketHistory(ctx context.Context, key market.InstrumentKey) (MarketHistoryResult, error) {
	if err := validateHistoryKey(key); err != nil {
		return MarketHistoryResult{}, err
	}
	bars, fetchErr := s.marketProvider.FetchDailyBars(ctx, key, 120)
	if fetchErr == nil {
		if len(bars) == 0 {
			fetchErr = fmt.Errorf("历史行情来源没有返回已收盘数据")
		} else if _, err := s.store.SaveDailyBars(ctx, bars, s.now()); err != nil {
			fetchErr = err
		}
	}
	result, cacheErr := s.MarketHistory(ctx, key, "3m")
	if cacheErr != nil {
		return MarketHistoryResult{}, cacheErr
	}
	if fetchErr != nil {
		if len(result.Points) == 0 {
			return MarketHistoryResult{}, fmt.Errorf("历史行情刷新失败且没有本地缓存：%w", fetchErr)
		}
		result.Cached = true
		result.Error = fetchErr.Error()
	}
	return result, nil
}

func validateHistoryKey(key market.InstrumentKey) error {
	if key.Market != "SH" && key.Market != "SZ" && key.Market != "HK" {
		return fmt.Errorf("历史行情市场必须是 SH、SZ 或 HK")
	}
	if key.Code == "" {
		return fmt.Errorf("历史行情代码不能为空")
	}
	return nil
}

func marketRangeStart(now time.Time, rangeName string) (string, error) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(location)
	switch rangeName {
	case "1m":
		return localNow.AddDate(0, -1, 0).Format("2006-01-02"), nil
	case "3m":
		return localNow.AddDate(0, -3, 0).Format("2006-01-02"), nil
	default:
		return "", fmt.Errorf("行情范围必须是 1m 或 3m")
	}
}

func copyErrors(source map[string]string, keys ...string) map[string]string {
	result := make(map[string]string)
	for _, key := range keys {
		if message := source[key]; message != "" {
			result[key] = message
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (s *Service) PreviewMarketCSV(reader io.Reader) (market.CSVPreview, string) {
	preview := market.PreviewCSV(reader)
	return preview, previewDigest(preview)
}

func (s *Service) ConfirmMarketCSV(ctx context.Context, preview market.CSVPreview, digest string) (MarketResult, error) {
	if previewDigest(preview) != digest {
		return MarketResult{}, fmt.Errorf("CSV 预览内容已变化，请重新预览")
	}
	if err := s.createAutomaticBackup(ctx, "csv"); err != nil {
		return MarketResult{}, err
	}
	groups := map[market.RankingKind][]market.Quote{
		market.KindStock: preview.StockTop20,
		market.KindETF:   preview.ETFTop10,
	}
	rows, err := s.store.SaveMarketBatch(ctx, groups, "csv", s.now())
	if err != nil {
		return MarketResult{}, err
	}
	return MarketResult{Stock: rows[market.KindStock], ETF: rows[market.KindETF]}, nil
}

func previewDigest(preview market.CSVPreview) string {
	raw, _ := json.Marshal(preview)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
