package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultEastmoneyHistoryURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
const defaultTencentHistoryURL = "https://web.ifzq.gtimg.cn/appstock/app/kline/kline"
const defaultEastmoneyDataCenterURL = "https://datacenter-web.eastmoney.com/api/data/v1/get"
const eastmoneySouthboundMillionYuanToFen = 100_000_000

func (p *EastmoneyProvider) FetchDailyBars(ctx context.Context, key InstrumentKey, limit int) ([]DailyBar, error) {
	bars, primaryErr := p.fetchEastmoneyDailyBars(ctx, key, limit)
	if primaryErr == nil {
		return bars, nil
	}
	fallbackURL := p.FallbackHistoryURL
	if fallbackURL == "" {
		if p.HistoryURL != "" {
			return nil, primaryErr
		}
		fallbackURL = defaultTencentHistoryURL
	}
	bars, fallbackErr := p.fetchTencentDailyBars(ctx, key, limit, fallbackURL)
	if fallbackErr != nil {
		return nil, fmt.Errorf("%v；备用公开行情源也失败: %w", primaryErr, fallbackErr)
	}
	return bars, nil
}

func (p *EastmoneyProvider) fetchEastmoneyDailyBars(ctx context.Context, key InstrumentKey, limit int) ([]DailyBar, error) {
	secid, err := eastmoneySecID(key)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}
	endpoint := p.HistoryURL
	if endpoint == "" {
		endpoint = defaultEastmoneyHistoryURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse history URL: %w", err)
	}
	query := parsed.Query()
	query.Set("secid", secid)
	query.Set("klt", "101")
	query.Set("fqt", "0")
	query.Set("lmt", strconv.Itoa(limit))
	query.Set("end", "20500101")
	query.Set("fields1", "f1,f2,f3,f4,f5,f6")
	query.Set("fields2", "f51,f52,f53,f54,f55,f56,f57")
	parsed.RawQuery = query.Encode()

	var wire struct {
		Data *struct {
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, parsed.String(), "https://quote.eastmoney.com/", &wire); err != nil {
		return nil, fmt.Errorf("fetch daily bars: %w", err)
	}
	if wire.Data == nil || len(wire.Data.KLines) == 0 {
		return nil, fmt.Errorf("history source returned no rows")
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	bars := make([]DailyBar, 0, len(wire.Data.KLines))
	for _, raw := range wire.Data.KLines {
		columns := strings.Split(raw, ",")
		if len(columns) < 7 {
			return nil, fmt.Errorf("history row has %d columns", len(columns))
		}
		if !closedMarketDate(columns[0], key.Market, now) {
			continue
		}
		closeValue, err := strconv.ParseFloat(columns[2], 64)
		if err != nil || closeValue < 0 {
			return nil, fmt.Errorf("invalid close for %s: %q", columns[0], columns[2])
		}
		turnover, err := strconv.ParseFloat(columns[6], 64)
		if err != nil || turnover < 0 {
			return nil, fmt.Errorf("invalid turnover for %s: %q", columns[0], columns[6])
		}
		bars = append(bars, DailyBar{
			TradeDate: columns[0], Market: key.Market, Code: key.Code,
			CloseMinor: int64(math.Round(closeValue * 100)), TurnoverFen: int64(math.Round(turnover * 100)),
			Source: "eastmoney-public-history", SourceTime: marketCloseTime(columns[0], key.Market),
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("history source returned no closed rows")
	}
	return bars, nil
}

func (p *EastmoneyProvider) fetchTencentDailyBars(ctx context.Context, key InstrumentKey, limit int, endpoint string) ([]DailyBar, error) {
	symbol, err := tencentHistorySymbol(key)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse fallback history URL: %w", err)
	}
	query := parsed.Query()
	query.Set("param", fmt.Sprintf("%s,day,,,%d", symbol, limit))
	parsed.RawQuery = query.Encode()

	var wire struct {
		Code int `json:"code"`
		Data map[string]struct {
			Day []json.RawMessage `json:"day"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, parsed.String(), "https://gu.qq.com/", &wire); err != nil {
		return nil, fmt.Errorf("fetch fallback daily bars: %w", err)
	}
	rows := wire.Data[symbol].Day
	if wire.Code != 0 || len(rows) == 0 {
		return nil, fmt.Errorf("fallback history source returned no rows")
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	bars := make([]DailyBar, 0, len(rows))
	for _, rawRow := range rows {
		var columns []json.RawMessage
		if err := json.Unmarshal(rawRow, &columns); err != nil {
			continue
		}
		if len(columns) < 6 {
			return nil, fmt.Errorf("fallback history row has %d columns", len(columns))
		}
		var tradeDate string
		if err := json.Unmarshal(columns[0], &tradeDate); err != nil {
			continue
		}
		if !closedMarketDate(tradeDate, key.Market, now) {
			continue
		}
		var closeValue flexibleFloat
		if err := json.Unmarshal(columns[2], &closeValue); err != nil || closeValue < 0 {
			return nil, fmt.Errorf("invalid fallback close for %s", tradeDate)
		}
		bars = append(bars, DailyBar{
			TradeDate: tradeDate, Market: key.Market, Code: key.Code,
			CloseMinor: int64(math.Round(float64(closeValue) * 100)), TurnoverFen: 0,
			Source: "tencent-public-history", SourceTime: marketCloseTime(tradeDate, key.Market),
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("fallback history source returned no closed rows")
	}
	if len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	return bars, nil
}

func tencentHistorySymbol(key InstrumentKey) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(key.Code))
	switch key.Market {
	case "SH":
		return "sh" + code, nil
	case "SZ":
		return "sz" + code, nil
	case "HK":
		code = strings.TrimSuffix(code, ".HK")
		if len(code) < 5 {
			code = strings.Repeat("0", 5-len(code)) + code
		}
		return "hk" + code, nil
	default:
		return "", fmt.Errorf("unsupported fallback history market %q", key.Market)
	}
}

func (p *EastmoneyProvider) FetchMarketMetrics(ctx context.Context, limit int) MetricBatch {
	type metricResult struct {
		metric MetricKind
		points []MetricPoint
		err    error
	}
	results := make(chan metricResult, 4)
	go func() {
		bars, err := p.fetchEastmoneyDailyBars(ctx, InstrumentKey{Market: "SH", Code: "000001"}, limit)
		results <- metricResult{metric: MetricSHTurnover, points: turnoverMetricPoints(bars, MetricSHTurnover), err: err}
	}()
	go func() {
		bars, err := p.fetchEastmoneyDailyBars(ctx, InstrumentKey{Market: "SZ", Code: "399106"}, limit)
		results <- metricResult{metric: MetricSZTurnover, points: turnoverMetricPoints(bars, MetricSZTurnover), err: err}
	}()
	go func() {
		points, err := p.fetchSouthboundChannel(ctx, "002", MetricSouthboundSHNetBuy, limit)
		results <- metricResult{metric: MetricSouthboundSHNetBuy, points: points, err: err}
	}()
	go func() {
		points, err := p.fetchSouthboundChannel(ctx, "004", MetricSouthboundSZNetBuy, limit)
		results <- metricResult{metric: MetricSouthboundSZNetBuy, points: points, err: err}
	}()

	batch := MetricBatch{Errors: make(map[MetricKind]string)}
	byMetric := make(map[MetricKind][]MetricPoint)
	for range 4 {
		result := <-results
		if result.err != nil {
			batch.Errors[result.metric] = result.err.Error()
			continue
		}
		byMetric[result.metric] = result.points
		batch.Points = append(batch.Points, result.points...)
	}
	if _, failed := batch.Errors[MetricSHTurnover]; !failed {
		if _, failed := batch.Errors[MetricSZTurnover]; !failed {
			batch.Points = append(batch.Points, combineMetricPoints(byMetric[MetricSHTurnover], byMetric[MetricSZTurnover], MetricAShareTurnover)...)
		}
	}
	if _, failed := batch.Errors[MetricSouthboundSHNetBuy]; !failed {
		if _, failed := batch.Errors[MetricSouthboundSZNetBuy]; !failed {
			batch.Points = append(batch.Points, combineMetricPoints(byMetric[MetricSouthboundSHNetBuy], byMetric[MetricSouthboundSZNetBuy], MetricSouthboundNetBuy)...)
		}
	}
	if len(batch.Errors) == 0 {
		batch.Errors = nil
	}
	sort.Slice(batch.Points, func(i, j int) bool {
		if batch.Points[i].TradeDate == batch.Points[j].TradeDate {
			return batch.Points[i].Metric < batch.Points[j].Metric
		}
		return batch.Points[i].TradeDate < batch.Points[j].TradeDate
	})
	return batch
}

func (p *EastmoneyProvider) fetchSouthboundChannel(ctx context.Context, mutualType string, metric MetricKind, limit int) ([]MetricPoint, error) {
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}
	endpoint := p.DataCenterURL
	if endpoint == "" {
		endpoint = defaultEastmoneyDataCenterURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse southbound URL: %w", err)
	}
	query := parsed.Query()
	query.Set("reportName", "RPT_MUTUAL_DEAL_HISTORY")
	query.Set("columns", "TRADE_DATE,NET_DEAL_AMT,MUTUAL_TYPE")
	query.Set("filter", fmt.Sprintf(`(MUTUAL_TYPE="%s")`, mutualType))
	query.Set("pageNumber", "1")
	query.Set("pageSize", strconv.Itoa(limit))
	query.Set("sortColumns", "TRADE_DATE")
	query.Set("sortTypes", "-1")
	query.Set("source", "WEB")
	query.Set("client", "WEB")
	parsed.RawQuery = query.Encode()
	var wire struct {
		Result *struct {
			Data []struct {
				TradeDate  string        `json:"TRADE_DATE"`
				NetDealAmt flexibleFloat `json:"NET_DEAL_AMT"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := p.getJSON(ctx, parsed.String(), "https://data.eastmoney.com/hsgt/hsgtV2.html", &wire); err != nil {
		return nil, fmt.Errorf("fetch southbound %s: %w", mutualType, err)
	}
	if wire.Result == nil || len(wire.Result.Data) == 0 {
		return nil, fmt.Errorf("southbound %s source returned no rows", mutualType)
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	points := make([]MetricPoint, 0, len(wire.Result.Data))
	for _, row := range wire.Result.Data {
		tradeDate := row.TradeDate
		if len(tradeDate) >= 10 {
			tradeDate = tradeDate[:10]
		}
		if !closedMarketDate(tradeDate, "HK", now) {
			continue
		}
		points = append(points, MetricPoint{
			TradeDate: tradeDate, Metric: metric, ValueFen: int64(math.Round(float64(row.NetDealAmt) * eastmoneySouthboundMillionYuanToFen)),
			Source: "eastmoney-public-southbound", SourceTime: marketCloseTime(tradeDate, "HK"),
		})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("southbound %s source returned no closed rows", mutualType)
	}
	return points, nil
}

func turnoverMetricPoints(bars []DailyBar, metric MetricKind) []MetricPoint {
	points := make([]MetricPoint, 0, len(bars))
	for _, bar := range bars {
		points = append(points, MetricPoint{
			TradeDate: bar.TradeDate, Metric: metric, ValueFen: bar.TurnoverFen,
			Source: bar.Source, SourceTime: bar.SourceTime,
		})
	}
	return points
}

func combineMetricPoints(left, right []MetricPoint, metric MetricKind) []MetricPoint {
	rightByDate := make(map[string]MetricPoint, len(right))
	for _, point := range right {
		rightByDate[point.TradeDate] = point
	}
	combined := make([]MetricPoint, 0)
	for _, point := range left {
		other, ok := rightByDate[point.TradeDate]
		if !ok {
			continue
		}
		sourceTime := point.SourceTime
		if other.SourceTime.After(sourceTime) {
			sourceTime = other.SourceTime
		}
		combined = append(combined, MetricPoint{
			TradeDate: point.TradeDate, Metric: metric, ValueFen: point.ValueFen + other.ValueFen,
			Source: "eastmoney-public-derived", SourceTime: sourceTime,
		})
	}
	return combined
}

func eastmoneySecID(key InstrumentKey) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(key.Code))
	switch key.Market {
	case "SH":
		return "1." + code, nil
	case "SZ":
		return "0." + code, nil
	case "HK":
		code = strings.TrimSuffix(code, ".HK")
		if len(code) < 5 {
			code = strings.Repeat("0", 5-len(code)) + code
		}
		return "116." + code, nil
	default:
		return "", fmt.Errorf("unsupported history market %q", key.Market)
	}
}

func closedMarketDate(tradeDate, market string, now time.Time) bool {
	location, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(location)
	if tradeDate != localNow.Format("2006-01-02") {
		return true
	}
	minutes := localNow.Hour()*60 + localNow.Minute()
	if market == "HK" {
		return minutes >= 16*60+20
	}
	return minutes >= 15*60
}

func marketCloseTime(tradeDate, market string) time.Time {
	location, _ := time.LoadLocation("Asia/Shanghai")
	hour := 15
	if market == "HK" {
		hour = 16
	}
	parsed, _ := time.ParseInLocation("2006-01-02 15", fmt.Sprintf("%s %02d", tradeDate, hour), location)
	return parsed.UTC()
}

func (p *EastmoneyProvider) getJSON(ctx context.Context, endpoint, referer string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/124 Safari/537.36")
	request.Header.Set("Referer", referer)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	var response *http.Response
	for attempt := 1; attempt <= 3; attempt++ {
		response, err = client.Do(request.Clone(ctx))
		if err == nil {
			break
		}
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		transient := isTransientMarketEOF(err)
		if attempt == 3 && transient {
			return fmt.Errorf("公开行情源连续 3 次临时断开连接，请稍后再试: %w", err)
		}
		if !transient {
			return err
		}
		if err := waitForMarketRetry(ctx, attempt); err != nil {
			return err
		}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("source returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func isTransientMarketEOF(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && !networkError.Timeout()
}

func waitForMarketRetry(ctx context.Context, failedAttempt int) error {
	delay := time.Duration(failedAttempt*failedAttempt) * 200 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
