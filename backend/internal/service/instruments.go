package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type RegisterInstrumentInput struct {
	Market    string `json:"market"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Currency  string `json:"currency,omitempty"`
	LotSize   int    `json:"lotSize,omitempty"`
	AssetType string `json:"assetType,omitempty"`
	LocalOnly bool   `json:"localOnly,omitempty"`
}

func (s *Service) ResolveInstrument(ctx context.Context, rawCode, marketHint string) (store.InstrumentRow, error) {
	key, assetType, err := resolveInstrumentKey(marketHint, rawCode)
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

func (s *Service) RegisterInstrument(ctx context.Context, input RegisterInstrumentInput) (store.InstrumentRow, error) {
	key, assetType, err := resolveInstrumentKey(input.Market, input.Code)
	if err != nil {
		return store.InstrumentRow{}, CodedError{Code: "INVALID_INSTRUMENT", Message: err.Error()}
	}
	if input.AssetType == "etf" {
		assetType = market.KindETF
	} else if input.AssetType == "stock" {
		assetType = market.KindStock
	}
	if existing, found, err := s.store.InstrumentByMarketCode(ctx, key.Market, key.Code); err != nil {
		return store.InstrumentRow{}, err
	} else if found && validInstrumentName(existing.Name) {
		return existing, nil
	}
	if !input.LocalOnly {
		resolved, err := s.ResolveInstrument(ctx, key.Code, key.Market)
		if err == nil {
			return resolved, nil
		}
		if strings.TrimSpace(input.Name) == "" {
			return store.InstrumentRow{}, err
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return store.InstrumentRow{}, CodedError{Code: "INVALID_INSTRUMENT", Message: "本地登记需要填写证券名称"}
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		currency = market.CurrencyForMarket(key.Market)
	}
	if currency != "CNY" && currency != "HKD" {
		return store.InstrumentRow{}, CodedError{Code: "INVALID_INSTRUMENT", Message: "币种只能是 CNY 或 HKD"}
	}
	lotSize := input.LotSize
	if lotSize <= 0 {
		lotSize = market.DefaultLotSize(key.Market)
	}
	return s.store.RegisterInstrument(ctx, store.RegisterInstrumentInput{
		Market: key.Market, Code: key.Code, Name: name, AssetType: string(assetType), Currency: currency, LotSize: lotSize, Source: "manual",
	}, s.now())
}

func validInstrumentName(name string) bool {
	return utf8.ValidString(name) && !strings.ContainsRune(name, utf8.RuneError)
}

func resolveInstrumentKey(marketHint, rawCode string) (market.InstrumentKey, market.RankingKind, error) {
	hint := strings.ToUpper(strings.TrimSpace(marketHint))
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return market.InstrumentKey{}, "", fmt.Errorf("证券代码不能为空")
	}
	if hint == "HK" || strings.HasSuffix(code, ".HK") || strings.HasPrefix(code, "HK") || looksLikeHKCode(code) {
		normalized, err := normalizeHKCode(code)
		if err != nil {
			return market.InstrumentKey{}, "", err
		}
		return market.InstrumentKey{Market: "HK", Code: normalized}, market.KindStock, nil
	}
	return resolveAShareCode(code)
}

func looksLikeHKCode(code string) bool {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(code, "HK"), ".HK")
	if len(trimmed) < 4 || len(trimmed) > 5 {
		return false
	}
	return digitsOnly(trimmed)
}

func normalizeHKCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	code = strings.TrimPrefix(code, "HK")
	code = strings.TrimSuffix(code, ".HK")
	if !digitsOnly(code) || len(code) == 0 || len(code) > 5 {
		return "", fmt.Errorf("港股代码无效：%s", raw)
	}
	trimmed := strings.TrimLeft(code, "0")
	if trimmed == "" {
		return "", fmt.Errorf("港股代码无效：%s", raw)
	}
	if len(trimmed) < 4 {
		trimmed = strings.Repeat("0", 4-len(trimmed)) + trimmed
	}
	return trimmed + ".HK", nil
}

func digitsOnly(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func resolveAShareCode(rawCode string) (market.InstrumentKey, market.RankingKind, error) {
	code := strings.TrimSpace(rawCode)
	if len(code) != 6 || !digitsOnly(code) {
		return market.InstrumentKey{}, "", fmt.Errorf("证券代码必须是 6 位沪深代码或港股代码")
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
