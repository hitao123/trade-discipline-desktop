package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func (s *Service) ResolveInstrument(ctx context.Context, rawCode string) (store.InstrumentRow, error) {
	key, assetType, err := resolveAShareCode(rawCode)
	if err != nil {
		return store.InstrumentRow{}, err
	}
	if existing, found, err := s.store.InstrumentByMarketCode(ctx, key.Market, key.Code); err != nil {
		return store.InstrumentRow{}, err
	} else if found && validInstrumentName(existing.Name) {
		return existing, nil
	}

	quotes, primaryErr := s.marketProvider.FetchQuotes(ctx, []market.InstrumentKey{key})
	quote, found := matchingResolvedQuote(quotes, key)
	if !found && s.quoteFallback != nil {
		fallbackQuotes, fallbackErr := s.quoteFallback.FetchQuotes(ctx, []market.InstrumentKey{key})
		if fallbackErr == nil {
			quote, found = matchingResolvedQuote(fallbackQuotes, key)
		} else if primaryErr == nil {
			primaryErr = fallbackErr
		}
	}
	if !found {
		if primaryErr != nil {
			return store.InstrumentRow{}, fmt.Errorf("公开行情暂时无法验证证券 %s，请稍后重试: %w", key.Code, primaryErr)
		}
		return store.InstrumentRow{}, fmt.Errorf("未查询到有效证券 %s，请确认截图代码", key.Code)
	}
	quote.Market = key.Market
	quote.Code = key.Code
	quote.AssetType = assetType
	if assetType == market.KindETF && !market.IsEligibleETF(quote.Code, quote.Name) {
		return store.InstrumentRow{}, fmt.Errorf("不支持债券、货币或现金管理类 ETF")
	}
	if quote.SourceTime.IsZero() {
		quote.SourceTime = s.now()
	}
	if strings.TrimSpace(quote.Source) == "" {
		quote.Source = "public-quote"
	}
	return s.store.UpsertResolvedInstrument(ctx, quote, s.now())
}

func validInstrumentName(name string) bool {
	return utf8.ValidString(name) && !strings.ContainsRune(name, utf8.RuneError)
}

func resolveAShareCode(rawCode string) (market.InstrumentKey, market.RankingKind, error) {
	code := strings.TrimSpace(rawCode)
	if len(code) != 6 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return market.InstrumentKey{}, "", fmt.Errorf("证券代码必须是 6 位沪深代码")
	}
	switch {
	case strings.HasPrefix(code, "15"):
		return market.InstrumentKey{Market: "SZ", Code: code}, market.KindETF, nil
	case strings.HasPrefix(code, "51"), strings.HasPrefix(code, "56"), strings.HasPrefix(code, "58"):
		return market.InstrumentKey{Market: "SH", Code: code}, market.KindETF, nil
	case strings.HasPrefix(code, "00"), strings.HasPrefix(code, "30"):
		return market.InstrumentKey{Market: "SZ", Code: code}, market.KindStock, nil
	case strings.HasPrefix(code, "60"), strings.HasPrefix(code, "68"):
		return market.InstrumentKey{Market: "SH", Code: code}, market.KindStock, nil
	default:
		return market.InstrumentKey{}, "", fmt.Errorf("暂不支持自动录入代码 %s，请手动确认证券", code)
	}
}

func matchingResolvedQuote(quotes []market.Quote, key market.InstrumentKey) (market.Quote, bool) {
	for _, quote := range quotes {
		if quote.Market == key.Market && quote.Code == key.Code && strings.TrimSpace(quote.Name) != "" {
			return quote, true
		}
	}
	return market.Quote{}, false
}
