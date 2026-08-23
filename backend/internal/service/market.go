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
	Stock    store.MarketSnapshotRow           `json:"stock"`
	ETF      store.MarketSnapshotRow           `json:"etf"`
	Overview MarketOverviewResult              `json:"overview"`
	Errors   map[string]string                 `json:"errors,omitempty"`
	Health   map[string]market.ComponentHealth `json:"health,omitempty"`
	Status   store.MarketRefreshStatusRow      `json:"status"`
}

type LiveMarketResult struct {
	MarketResult
	IsLive bool   `json:"isLive"`
	State  string `json:"state"`
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

func (s *Service) SetMarketRankingFallback(provider market.RankingProvider) {
	s.rankingFallback = provider
}

func (s *Service) RefreshMarket(ctx context.Context) (result MarketResult, returnErr error) {
	result = MarketResult{Errors: make(map[string]string)}
	attemptedAt := s.now()
	rankingsRefreshed := 0
	defer func() {
		status, err := s.store.SaveMarketRefreshStatus(ctx, market.SnapshotModeClose, attemptedAt, rankingsRefreshed == 2, result.Errors)
		if err != nil && returnErr == nil {
			returnErr = err
		}
		result.Status = status
	}()
	for _, kind := range []market.RankingKind{market.KindStock, market.KindETF} {
		quotes, err := s.marketProvider.FetchRankings(ctx, kind)
		if err != nil {
			primaryErr := err
			if s.rankingFallback == nil {
				result.Errors[string(kind)] = err.Error()
				continue
			}
			quotes, err = s.rankingFallback.FetchRankings(ctx, kind)
			if err != nil {
				result.Errors[string(kind)] = fmt.Sprintf("主来源失败：%s；备用来源失败：%s", primaryErr, err)
				continue
			}
		}
		row, err := s.store.SaveMarketSnapshot(ctx, kind, quotes, rankingSource(quotes), attemptedAt)
		if err != nil {
			result.Errors[string(kind)] = err.Error()
			continue
		}
		if kind == market.KindStock {
			result.Stock = row
		} else {
			result.ETF = row
		}
		rankingsRefreshed++
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
			result.Errors["quotes"] = "持仓参考报价暂未更新（东方财富公开接口临时不可用；已保留最近一次成功报价）"
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
	status, err := s.store.MarketRefreshStatus(ctx, market.SnapshotModeClose)
	if err != nil {
		return MarketResult{}, err
	}
	return MarketResult{Stock: stock, ETF: etf, Overview: overview, Status: status}, nil
}

func (s *Service) MarketRefreshStatuses(ctx context.Context) (map[string]store.MarketRefreshStatusRow, error) {
	closeStatus, err := s.store.MarketRefreshStatus(ctx, market.SnapshotModeClose)
	if err != nil {
		return nil, err
	}
	liveStatus, err := s.store.MarketRefreshStatus(ctx, market.SnapshotModeLive)
	if err != nil {
		return nil, err
	}
	return map[string]store.MarketRefreshStatusRow{
		string(market.SnapshotModeClose): closeStatus,
		string(market.SnapshotModeLive):  liveStatus,
	}, nil
}

func (s *Service) RefreshLiveMarket(ctx context.Context) (result LiveMarketResult, returnErr error) {
	attemptedAt := s.now()
	if !market.IsAShareTradingSession(attemptedAt) {
		result.State = "market_closed"
		cached, err := s.LatestLiveMarket(ctx)
		result.MarketResult = cached.MarketResult
		returnErr = err
		result.IsLive = false
		result.State = "market_closed"
		return result, returnErr
	}
	result.State = "live"
	result.Errors = make(map[string]string)
	result.Health = make(map[string]market.ComponentHealth)
	defer func() {
		status, err := s.store.SaveMarketRefreshStatus(ctx, market.SnapshotModeLive, attemptedAt, result.Stock.ID != "" && result.ETF.ID != "", result.Errors)
		if err != nil && returnErr == nil {
			returnErr = err
		}
		result.Status = status
	}()
	if s.rankingFallback == nil {
		return result, fmt.Errorf("实时行情来源尚未配置")
	}
	for _, kind := range []market.RankingKind{market.KindStock, market.KindETF} {
		healthKey := "live_" + string(kind)
		quotes, err := s.rankingFallback.FetchRankings(ctx, kind)
		if err != nil {
			result.Errors[string(kind)] = err.Error()
			result.Health[healthKey] = market.ComponentHealth{State: market.HealthUnavailable, Message: "暂不可用", DetailCode: "SOURCE_FETCH_FAILED"}
			continue
		}
		if len(quotes) == 0 {
			result.Errors[string(kind)] = "公开行情没有返回有效数据"
			result.Health[healthKey] = market.ComponentHealth{State: market.HealthUnavailable, Message: "暂不可用", DetailCode: "SOURCE_EMPTY"}
			continue
		}
		health := market.AssessFreshness(attemptedAt, quotes[0].TradeDate, quotes[0].SourceTime, 5*time.Minute)
		health.Source = rankingSource(quotes)
		result.Health[healthKey] = health
		if health.State != market.HealthLive {
			result.Errors[string(kind)] = "公开行情返回的数据时间已过期"
			continue
		}
		row, err := s.store.SaveMarketSnapshotForMode(ctx, market.SnapshotModeLive, kind, quotes, rankingSource(quotes), attemptedAt)
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
		result.Stock, _ = s.store.LatestMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindStock)
		result.Health["live_stock"] = cachedSnapshotHealth(result.Stock, result.Health["live_stock"])
	}
	if result.ETF.ID == "" {
		result.ETF, _ = s.store.LatestMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindETF)
		result.Health["live_etf"] = cachedSnapshotHealth(result.ETF, result.Health["live_etf"])
	}
	if result.Stock.ID == "" && result.ETF.ID == "" {
		result.IsLive = false
		result.State = "unavailable"
		return result, nil
	}
	result.IsLive = result.Health["live_stock"].State == market.HealthLive && result.Health["live_etf"].State == market.HealthLive
	if !result.IsLive {
		result.State = "degraded"
	}
	return result, nil
}

func cachedSnapshotHealth(snapshot store.MarketSnapshotRow, health market.ComponentHealth) market.ComponentHealth {
	if snapshot.ID == "" {
		health.State = market.HealthUnavailable
		health.Message = "暂不可用"
		if health.DetailCode == "" {
			health.DetailCode = "NO_LOCAL_CACHE"
		}
		return health
	}
	lastSuccessfulAt := snapshot.FetchedAt.UTC()
	health.State = market.HealthCached
	health.Source = snapshot.Source
	health.LastSuccessfulAt = &lastSuccessfulAt
	health.Message = "本地缓存"
	return health
}

func (s *Service) LatestLiveMarket(ctx context.Context) (LiveMarketResult, error) {
	stock, err := s.store.LatestMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindStock)
	if err != nil {
		return LiveMarketResult{}, err
	}
	etf, err := s.store.LatestMarketSnapshotForMode(ctx, market.SnapshotModeLive, market.KindETF)
	if err != nil {
		return LiveMarketResult{}, err
	}
	status, err := s.store.MarketRefreshStatus(ctx, market.SnapshotModeLive)
	if err != nil {
		return LiveMarketResult{}, err
	}
	now := s.now()
	health := map[string]market.ComponentHealth{
		"live_stock": snapshotHealth(now, stock),
		"live_etf":   snapshotHealth(now, etf),
	}
	isLive := health["live_stock"].State == market.HealthLive && health["live_etf"].State == market.HealthLive
	state := liveMarketState(now)
	if market.IsAShareTradingSession(now) && !isLive {
		if stock.ID == "" && etf.ID == "" {
			state = "unavailable"
		} else {
			state = "degraded"
		}
	}
	return LiveMarketResult{MarketResult: MarketResult{Stock: stock, ETF: etf, Health: health, Status: status}, IsLive: isLive, State: state}, nil
}

func snapshotHealth(now time.Time, snapshot store.MarketSnapshotRow) market.ComponentHealth {
	if snapshot.ID == "" || len(snapshot.Entries) == 0 {
		return market.ComponentHealth{State: market.HealthUnavailable, Message: "暂不可用", DetailCode: "NO_LOCAL_CACHE"}
	}
	if !market.IsAShareTradingSession(now) {
		return cachedSnapshotHealth(snapshot, market.ComponentHealth{})
	}
	health := market.AssessFreshness(now, snapshot.TradeDate, snapshot.Entries[0].SourceTime, 5*time.Minute)
	health.Source = snapshot.Source
	if health.State != market.HealthLive {
		return cachedSnapshotHealth(snapshot, health)
	}
	lastSuccessfulAt := snapshot.FetchedAt.UTC()
	health.LastSuccessfulAt = &lastSuccessfulAt
	return health
}

func liveMarketState(now time.Time) string {
	if market.IsAShareTradingSession(now) {
		return "live"
	}
	return "market_closed"
}

func rankingSource(quotes []market.Quote) string {
	if len(quotes) == 0 || quotes[0].Source == "" {
		return "public-ranking"
	}
	return quotes[0].Source
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
