package market

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultEastmoneyURL = "https://push2.eastmoney.com/api/qt/clist/get"
const defaultEastmoneyQuoteURL = "https://push2.eastmoney.com/api/qt/ulist.np/get"

type EastmoneyProvider struct {
	BaseURL            string
	QuoteURL           string
	HistoryURL         string
	FallbackHistoryURL string
	DataCenterURL      string
	SinaMetricsURL     string
	Client             *http.Client
	Now                func() time.Time
}

type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || string(data) == `"-"` {
		*f = 0
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		value, err := strconv.ParseFloat(number.String(), 64)
		if err == nil {
			*f = flexibleFloat(value)
			return nil
		}
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*f = flexibleFloat(value)
	return nil
}

type eastmoneyRow struct {
	Price     float64
	ChangePct float64
	Turnover  float64
	Code      string
	Market    int
	Name      string
	QuoteUnix int64
}

type eastmoneyWireRow struct {
	Price     flexibleFloat `json:"f2"`
	ChangePct flexibleFloat `json:"f3"`
	Turnover  flexibleFloat `json:"f6"`
	Code      string        `json:"f12"`
	Market    int           `json:"f13"`
	Name      string        `json:"f14"`
	QuoteUnix int64         `json:"f124"`
}

func (p *EastmoneyProvider) FetchRankings(ctx context.Context, kind RankingKind) ([]Quote, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultEastmoneyURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse market URL: %w", err)
	}
	query := parsed.Query()
	query.Set("pn", "1")
	query.Set("pz", "200")
	query.Set("po", "1")
	query.Set("np", "1")
	query.Set("fltt", "2")
	query.Set("invt", "2")
	query.Set("fid", "f6")
	query.Set("fields", "f2,f3,f6,f12,f13,f14,f124")
	switch kind {
	case KindStock:
		query.Set("fs", "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23")
	case KindETF:
		query.Set("fs", "b:MK0021")
	default:
		return nil, fmt.Errorf("unsupported ranking kind %q", kind)
	}
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create market request: %w", err)
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch closing ranking: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market source returned HTTP %d", response.StatusCode)
	}
	var wire struct {
		Data *struct {
			Diff []eastmoneyWireRow `json:"diff"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode market response: %w", err)
	}
	if wire.Data == nil || len(wire.Data.Diff) == 0 {
		return nil, fmt.Errorf("market source returned no rows")
	}
	rows := make([]eastmoneyRow, 0, len(wire.Data.Diff))
	for _, item := range wire.Data.Diff {
		rows = append(rows, eastmoneyRow{
			Price: float64(item.Price), ChangePct: float64(item.ChangePct), Turnover: float64(item.Turnover),
			Code: item.Code, Market: item.Market, Name: item.Name, QuoteUnix: item.QuoteUnix,
		})
	}
	limit := 20
	if kind == KindETF {
		limit = 10
	}
	return normalizeRanking(rows, kind, limit), nil
}

func (p *EastmoneyProvider) FetchQuotes(ctx context.Context, keys []InstrumentKey) ([]Quote, error) {
	if len(keys) == 0 {
		return []Quote{}, nil
	}
	quoteURL := p.QuoteURL
	if quoteURL == "" {
		quoteURL = defaultEastmoneyQuoteURL
	}
	parsed, err := url.Parse(quoteURL)
	if err != nil {
		return nil, fmt.Errorf("parse quote URL: %w", err)
	}
	secids := make([]string, 0, len(keys))
	originals := make(map[string]InstrumentKey, len(keys))
	for _, key := range keys {
		marketID := "0"
		remoteCode := key.Code
		switch key.Market {
		case "HK":
			marketID = "116"
			remoteCode = strings.TrimSuffix(strings.ToUpper(key.Code), ".HK")
			if len(remoteCode) < 5 {
				remoteCode = strings.Repeat("0", 5-len(remoteCode)) + remoteCode
			}
		case "SH":
			marketID = "1"
		case "SZ":
			marketID = "0"
		default:
			continue
		}
		secids = append(secids, marketID+"."+remoteCode)
		originals[marketID+":"+remoteCode] = key
	}
	query := parsed.Query()
	query.Set("secids", strings.Join(secids, ","))
	query.Set("fields", "f2,f3,f6,f12,f13,f14,f124")
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create quote request: %w", err)
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch closing quotes: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quote source returned HTTP %d", response.StatusCode)
	}
	var wire struct {
		Data *struct {
			Diff []eastmoneyWireRow `json:"diff"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode quote response: %w", err)
	}
	if wire.Data == nil {
		return nil, fmt.Errorf("quote source returned no data")
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	quotes := make([]Quote, 0, len(wire.Data.Diff))
	for _, row := range wire.Data.Diff {
		key, ok := originals[strconv.Itoa(row.Market)+":"+row.Code]
		if !ok {
			continue
		}
		priceMinor := int64(math.Round(float64(row.Price)))
		if row.Market == 116 {
			priceMinor = int64(math.Round(float64(row.Price) / 10))
		}
		sourceTime := time.Unix(row.QuoteUnix, 0).In(location)
		quotes = append(quotes, Quote{
			TradeDate: sourceTime.Format("2006-01-02"), Market: key.Market, Code: key.Code, Name: row.Name,
			AssetType: KindStock, CloseMinor: priceMinor, ChangeBP: int(math.Round(float64(row.ChangePct))),
			TurnoverFen: int64(math.Round(float64(row.Turnover) * 100)), Source: "eastmoney-public-close", SourceTime: sourceTime.UTC(),
		})
	}
	return quotes, nil
}

func normalizeRanking(rows []eastmoneyRow, kind RankingKind, limit int) []Quote {
	location, _ := time.LoadLocation("Asia/Shanghai")
	quotes := make([]Quote, 0, len(rows))
	for _, row := range rows {
		nameUpper := strings.ToUpper(strings.TrimSpace(row.Name))
		if kind == KindStock && (strings.Contains(nameUpper, "ST") || strings.Contains(nameUpper, "退")) {
			continue
		}
		if row.Price < 0 || row.Turnover < 0 || row.Code == "" {
			continue
		}
		market := "SZ"
		if row.Market == 1 {
			market = "SH"
		}
		sourceTime := time.Unix(row.QuoteUnix, 0).In(location)
		quotes = append(quotes, Quote{
			TradeDate: sourceTime.Format("2006-01-02"), Market: market, Code: row.Code, Name: row.Name, AssetType: kind,
			CloseMinor: int64(math.Round(row.Price * 100)), ChangeBP: int(math.Round(row.ChangePct * 100)),
			TurnoverFen: int64(math.Round(row.Turnover * 100)), Source: "eastmoney-public-close", SourceTime: sourceTime.UTC(),
		})
	}
	sort.SliceStable(quotes, func(i, j int) bool {
		if quotes[i].TurnoverFen == quotes[j].TurnoverFen {
			return quotes[i].Code < quotes[j].Code
		}
		return quotes[i].TurnoverFen > quotes[j].TurnoverFen
	})
	if len(quotes) > limit {
		quotes = quotes[:limit]
	}
	return quotes
}
