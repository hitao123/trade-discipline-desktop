package market

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultTencentQuoteURL = "https://qt.gtimg.cn/"

// TencentQuoteProvider is a read-only fallback for prices already requested by
// the local portfolio. It does not discover securities or submit orders.
type TencentQuoteProvider struct {
	BaseURL string
	Client  *http.Client
}

func (p TencentQuoteProvider) FetchQuotes(ctx context.Context, keys []InstrumentKey) ([]Quote, error) {
	if len(keys) == 0 {
		return []Quote{}, nil
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultTencentQuoteURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Tencent quote URL: %w", err)
	}
	symbols := make([]string, 0, len(keys))
	keysBySymbol := make(map[string]InstrumentKey, len(keys))
	for _, key := range keys {
		symbol, ok := tencentQuoteSymbol(key)
		if !ok {
			continue
		}
		symbols = append(symbols, symbol)
		keysBySymbol[symbol] = key
	}
	if len(symbols) == 0 {
		return []Quote{}, nil
	}
	query := parsed.Query()
	query.Set("q", strings.Join(symbols, ","))
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Tencent quote request: %w", err)
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	request.Header.Set("Referer", "https://gu.qq.com/")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Tencent quotes: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tencent quote source returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read Tencent quote response: %w", err)
	}
	quotes := make([]Quote, 0, len(keys))
	location, _ := time.LoadLocation("Asia/Shanghai")
	for _, line := range strings.Split(string(body), "\n") {
		quote, ok := parseTencentQuoteLine(strings.TrimSpace(line), keysBySymbol, location)
		if ok {
			quotes = append(quotes, quote)
		}
	}
	if len(quotes) == 0 {
		return nil, fmt.Errorf("Tencent quote source returned no usable rows")
	}
	return quotes, nil
}

func tencentQuoteSymbol(key InstrumentKey) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(key.Code))
	switch key.Market {
	case "SH":
		return "sh" + code, true
	case "SZ":
		return "sz" + code, true
	case "HK":
		code = strings.TrimSuffix(code, ".hk")
		if len(code) < 5 {
			code = strings.Repeat("0", 5-len(code)) + code
		}
		return "hk" + code, true
	default:
		return "", false
	}
}

func parseTencentQuoteLine(line string, keys map[string]InstrumentKey, location *time.Location) (Quote, bool) {
	if !strings.HasPrefix(line, "v_") {
		return Quote{}, false
	}
	equals := strings.IndexByte(line, '=')
	if equals < 3 {
		return Quote{}, false
	}
	symbol := line[2:equals]
	key, ok := keys[symbol]
	if !ok {
		return Quote{}, false
	}
	payload := strings.Trim(strings.TrimSpace(line[equals+1:]), `";`)
	fields := strings.Split(payload, "~")
	if len(fields) < 4 {
		return Quote{}, false
	}
	price, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || price <= 0 {
		return Quote{}, false
	}
	var sourceTime time.Time
	for _, field := range fields[4:] {
		if len(field) != 14 {
			continue
		}
		parsed, parseErr := time.ParseInLocation("20060102150405", field, location)
		if parseErr == nil {
			sourceTime = parsed.UTC()
			break
		}
	}
	if sourceTime.IsZero() {
		return Quote{}, false
	}
	return Quote{
		TradeDate: sourceTime.In(location).Format("2006-01-02"), Market: key.Market, Code: key.Code,
		Name: fields[1], AssetType: KindStock, CloseMinor: int64(math.Round(price * 100)),
		Source: "tencent-public-quote", SourceTime: sourceTime,
	}, true
}
