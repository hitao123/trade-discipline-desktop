package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultSinaRankingURL = "https://vip.stock.finance.sina.com.cn/quotes_service/api/json_v2.php/Market_Center.getHQNodeData"
const defaultSinaIndexURL = "https://hq.sinajs.cn/list=sh000001"
const defaultSinaIndexMetricsURL = "https://hq.sinajs.cn/list=sh000001,sz399106"

// SinaProvider reads public A-share rankings. It is intentionally limited to
// observation data and is used only when the primary public source is unavailable.
type SinaProvider struct {
	RankingURL      string
	IndexURL        string
	IndexMetricsURL string
	Client          *http.Client
}

type sinaWireRow struct {
	Symbol        string        `json:"symbol"`
	Code          string        `json:"code"`
	Name          string        `json:"name"`
	Trade         flexibleFloat `json:"trade"`
	ChangePercent flexibleFloat `json:"changepercent"`
	Amount        flexibleFloat `json:"amount"`
	TickTime      string        `json:"ticktime"`
}

func (p SinaProvider) FetchRankings(ctx context.Context, kind RankingKind) ([]Quote, error) {
	tradeDate, marketStamp, err := p.fetchMarketTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	baseURL := p.RankingURL
	if baseURL == "" {
		baseURL = defaultSinaRankingURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse sina ranking URL: %w", err)
	}
	query := parsed.Query()
	query.Set("sort", "amount")
	query.Set("asc", "0")
	query.Set("num", "200")
	query.Set("page", "1")
	switch kind {
	case KindStock:
		query.Set("node", "hs_a")
	case KindETF:
		query.Set("node", "etf_hq_fund")
	default:
		return nil, fmt.Errorf("unsupported ranking kind %q", kind)
	}
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create sina ranking request: %w", err)
	}
	response, err := p.do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch sina ranking: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sina ranking returned HTTP %d", response.StatusCode)
	}
	var rows []sinaWireRow
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode sina ranking: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sina ranking returned no rows")
	}
	quotes := make([]Quote, 0, len(rows))
	location, _ := time.LoadLocation("Asia/Shanghai")
	for _, row := range rows {
		market := ""
		symbol := strings.ToLower(strings.TrimSpace(row.Symbol))
		switch {
		case strings.HasPrefix(symbol, "sh"):
			market = "SH"
		case strings.HasPrefix(symbol, "sz"):
			market = "SZ"
		}
		code := strings.TrimSpace(row.Code)
		if market == "" || code == "" || float64(row.Trade) < 0 || float64(row.Amount) < 0 {
			continue
		}
		name := strings.TrimSpace(row.Name)
		nameUpper := strings.ToUpper(name)
		if kind == KindStock && (strings.Contains(nameUpper, "ST") || strings.Contains(nameUpper, "退")) {
			continue
		}
		sourceTime := marketStamp
		if row.TickTime != "" {
			if parsedTime, parseErr := time.ParseInLocation("2006-01-02 15:04:05", tradeDate+" "+row.TickTime, location); parseErr == nil {
				sourceTime = parsedTime
			}
		}
		quotes = append(quotes, Quote{
			TradeDate: tradeDate, Market: market, Code: code, Name: name, AssetType: kind,
			CloseMinor:  int64(math.Round(float64(row.Trade) * 100)),
			ChangeBP:    int(math.Round(float64(row.ChangePercent) * 100)),
			TurnoverFen: int64(math.Round(float64(row.Amount) * 100)),
			Source:      "sina-public-ranking", SourceTime: sourceTime.UTC(),
		})
	}
	if len(quotes) == 0 {
		return nil, fmt.Errorf("sina ranking has no usable rows")
	}
	sort.SliceStable(quotes, func(i, j int) bool {
		if quotes[i].TurnoverFen == quotes[j].TurnoverFen {
			return quotes[i].Code < quotes[j].Code
		}
		return quotes[i].TurnoverFen > quotes[j].TurnoverFen
	})
	limit := 20
	if kind == KindETF {
		limit = 10
	}
	if len(quotes) > limit {
		quotes = quotes[:limit]
	}
	return quotes, nil
}

func (p SinaProvider) fetchMarketTimestamp(ctx context.Context) (string, time.Time, error) {
	indexURL := p.IndexURL
	if indexURL == "" {
		indexURL = defaultSinaIndexURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create sina index request: %w", err)
	}
	response, err := p.do(request)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch sina index timestamp: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("sina index returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read sina index timestamp: %w", err)
	}
	raw := string(body)
	first := strings.Index(raw, `"`)
	last := strings.LastIndex(raw, `"`)
	if first < 0 || last <= first {
		return "", time.Time{}, fmt.Errorf("parse sina index timestamp: missing quote payload")
	}
	return parseSinaTimestamp(strings.Split(raw[first+1:last], ","))
}

func (p SinaProvider) FetchAShareTurnoverMetrics(ctx context.Context) MetricBatch {
	endpoint := p.IndexMetricsURL
	if endpoint == "" {
		endpoint = defaultSinaIndexMetricsURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return sinaMetricError(fmt.Errorf("create sina index metrics request: %w", err))
	}
	response, err := p.do(request)
	if err != nil {
		return sinaMetricError(fmt.Errorf("fetch sina index metrics: %w", err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return sinaMetricError(fmt.Errorf("sina index metrics returned HTTP %d", response.StatusCode))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return sinaMetricError(fmt.Errorf("read sina index metrics: %w", err))
	}
	metricForSymbol := map[string]MetricKind{"sh000001": MetricSHTurnover, "sz399106": MetricSZTurnover}
	points := make(map[MetricKind]MetricPoint, 2)
	for _, line := range strings.Split(string(body), "\n") {
		symbol, payload, ok := sinaQuotePayload(line)
		metric, tracked := metricForSymbol[symbol]
		if !ok || !tracked {
			continue
		}
		parts := strings.Split(payload, ",")
		if len(parts) < 10 {
			return sinaMetricError(fmt.Errorf("parse sina %s metrics: insufficient fields", symbol))
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(parts[9]), 64)
		if err != nil || amount < 0 {
			return sinaMetricError(fmt.Errorf("parse sina %s turnover: %q", symbol, parts[9]))
		}
		tradeDate, sourceTime, err := parseSinaTimestamp(parts)
		if err != nil {
			return sinaMetricError(fmt.Errorf("parse sina %s timestamp: %w", symbol, err))
		}
		points[metric] = MetricPoint{TradeDate: tradeDate, Metric: metric, ValueFen: int64(math.Round(amount * 100)), Source: "sina-public-index", SourceTime: sourceTime.UTC()}
	}
	sh, hasSH := points[MetricSHTurnover]
	sz, hasSZ := points[MetricSZTurnover]
	if !hasSH || !hasSZ {
		return sinaMetricError(fmt.Errorf("sina index metrics missing %s", missingSinaMetric(hasSH, hasSZ)))
	}
	if sh.TradeDate != sz.TradeDate {
		return sinaMetricError(fmt.Errorf("sina index metrics trade dates do not match"))
	}
	sourceTime := sh.SourceTime
	if sz.SourceTime.After(sourceTime) {
		sourceTime = sz.SourceTime
	}
	return MetricBatch{Points: []MetricPoint{
		sh,
		sz,
		{TradeDate: sh.TradeDate, Metric: MetricAShareTurnover, ValueFen: sh.ValueFen + sz.ValueFen, Source: "sina-public-derived", SourceTime: sourceTime},
	}}
}

func sinaMetricError(err error) MetricBatch {
	return MetricBatch{Errors: map[MetricKind]string{MetricSHTurnover: err.Error(), MetricSZTurnover: err.Error()}}
}

func missingSinaMetric(hasSH, hasSZ bool) string {
	if !hasSH && !hasSZ {
		return "上证指数和深证成指"
	}
	if !hasSH {
		return "上证指数"
	}
	return "深证成指"
}

func sinaQuotePayload(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	const marker = "var hq_str_"
	if !strings.HasPrefix(line, marker) {
		return "", "", false
	}
	equals := strings.Index(line, "=")
	if equals <= len(marker) {
		return "", "", false
	}
	first := strings.Index(line[equals:], `"`)
	last := strings.LastIndex(line, `"`)
	if first < 0 || last <= equals+first {
		return "", "", false
	}
	return line[len(marker):equals], line[equals+first+1 : last], true
}

func parseSinaTimestamp(parts []string) (string, time.Time, error) {
	if len(parts) < 3 {
		return "", time.Time{}, fmt.Errorf("insufficient fields")
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	for index := 0; index+1 < len(parts); index++ {
		tradeDate := strings.TrimSpace(parts[index])
		clock := strings.TrimSpace(parts[index+1])
		if _, err := time.Parse("2006-01-02", tradeDate); err != nil {
			continue
		}
		if _, err := time.Parse("15:04:05", clock); err != nil {
			continue
		}
		stamp, err := time.ParseInLocation("2006-01-02 15:04:05", tradeDate+" "+clock, location)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("parse sina index timestamp: %w", err)
		}
		return tradeDate, stamp, nil
	}
	return "", time.Time{}, fmt.Errorf("date and time fields not found")
}

func (p SinaProvider) do(request *http.Request) (*http.Response, error) {
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Referer", "https://finance.sina.com.cn/")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120 Safari/537.36")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return client.Do(request)
}

func IsAShareTradingSession(now time.Time) bool {
	location, _ := time.LoadLocation("Asia/Shanghai")
	local := now.In(location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	minutes := local.Hour()*60 + local.Minute()
	return (minutes >= 9*60+30 && minutes <= 11*60+30) || (minutes >= 13*60 && minutes <= 15*60)
}
