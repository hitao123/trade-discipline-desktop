package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type MarketResult struct {
	Stock  store.MarketSnapshotRow `json:"stock"`
	ETF    store.MarketSnapshotRow `json:"etf"`
	Errors map[string]string       `json:"errors,omitempty"`
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
	if result.Stock.ID == "" && result.ETF.ID == "" {
		return result, fmt.Errorf("股票和 ETF 收盘榜单均刷新失败")
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
		return result, nil
	}
	quotes, err := s.marketProvider.FetchQuotes(ctx, keys)
	if err != nil {
		result.Errors["quotes"] = err.Error()
		return result, nil
	}
	if err := s.store.UpdateQuotes(ctx, quotes); err != nil {
		result.Errors["quotes"] = err.Error()
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
	return MarketResult{Stock: stock, ETF: etf}, nil
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
