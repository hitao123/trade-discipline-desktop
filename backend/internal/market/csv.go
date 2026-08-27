package market

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

var csvHeader = []string{"trade_date", "market", "code", "name", "asset_type", "close", "change_pct", "turnover"}

type CSVRowError struct {
	Line    int      `json:"line"`
	Message string   `json:"message"`
	Raw     []string `json:"raw"`
}

type CSVDuplicate struct {
	Line int    `json:"line"`
	Key  string `json:"key"`
}

type CSVPreview struct {
	Valid      []Quote        `json:"valid"`
	Errors     []CSVRowError  `json:"errors"`
	Duplicates []CSVDuplicate `json:"duplicates"`
	StockTop20 []Quote        `json:"stockTop20"`
	ETFTop20   []Quote        `json:"etfTop20"`
}

func PreviewCSV(reader io.Reader) CSVPreview {
	parser := csv.NewReader(reader)
	parser.FieldsPerRecord = -1
	parser.TrimLeadingSpace = true
	preview := CSVPreview{Valid: []Quote{}, Errors: []CSVRowError{}, Duplicates: []CSVDuplicate{}}
	header, err := parser.Read()
	if err != nil {
		preview.Errors = append(preview.Errors, CSVRowError{Line: 1, Message: "无法读取 CSV 表头"})
		return preview
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	if strings.Join(header, ",") != strings.Join(csvHeader, ",") {
		preview.Errors = append(preview.Errors, CSVRowError{Line: 1, Message: "表头必须为 trade_date,market,code,name,asset_type,close,change_pct,turnover", Raw: header})
		return preview
	}
	seen := make(map[string]bool)
	for line := 2; ; line++ {
		record, err := parser.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			preview.Errors = append(preview.Errors, CSVRowError{Line: line, Message: "CSV 行格式错误"})
			continue
		}
		quote, message := parseCSVRecord(record)
		if message != "" {
			preview.Errors = append(preview.Errors, CSVRowError{Line: line, Message: message, Raw: record})
			continue
		}
		key := strings.Join([]string{quote.TradeDate, quote.Market, quote.Code, string(quote.AssetType)}, "|")
		if seen[key] {
			preview.Duplicates = append(preview.Duplicates, CSVDuplicate{Line: line, Key: key})
			continue
		}
		seen[key] = true
		if quote.AssetType == KindStock && (strings.Contains(strings.ToUpper(quote.Name), "ST") || strings.Contains(quote.Name, "退")) {
			continue
		}
		preview.Valid = append(preview.Valid, quote)
	}
	preview.StockTop20 = topQuotes(preview.Valid, KindStock, StockRankingLimit)
	preview.ETFTop20 = topQuotes(preview.Valid, KindETF, ETFRankingLimit)
	return preview
}

func parseCSVRecord(record []string) (Quote, string) {
	if len(record) != len(csvHeader) {
		return Quote{}, "字段数量必须为 8"
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(record[0]))
	if err != nil {
		return Quote{}, "trade_date 必须使用 YYYY-MM-DD"
	}
	market := strings.ToUpper(strings.TrimSpace(record[1]))
	if market != "SH" && market != "SZ" {
		return Quote{}, "market 只能是 SH 或 SZ"
	}
	assetType := RankingKind(strings.ToLower(strings.TrimSpace(record[4])))
	if assetType != KindStock && assetType != KindETF {
		return Quote{}, "asset_type 只能是 stock 或 etf"
	}
	closeValue, err := strconv.ParseFloat(strings.TrimSpace(record[5]), 64)
	if err != nil || closeValue < 0 {
		return Quote{}, "close 必须是非负数字"
	}
	changeValue, err := strconv.ParseFloat(strings.TrimSpace(record[6]), 64)
	if err != nil {
		return Quote{}, "change_pct 必须是数字"
	}
	turnoverValue, err := strconv.ParseFloat(strings.TrimSpace(record[7]), 64)
	if err != nil || turnoverValue < 0 {
		return Quote{}, "turnover 必须是非负人民币金额"
	}
	return Quote{
		TradeDate: date.Format("2006-01-02"), Market: market, Code: strings.TrimSpace(record[2]), Name: strings.TrimSpace(record[3]), AssetType: assetType,
		CloseMinor: int64(math.Round(closeValue * 100)), ChangeBP: int(math.Round(changeValue * 100)), TurnoverFen: int64(math.Round(turnoverValue * 100)),
		Source: "csv", SourceTime: time.Now().UTC(),
	}, ""
}

func topQuotes(all []Quote, kind RankingKind, limit int) []Quote {
	filtered := make([]Quote, 0)
	for _, quote := range all {
		if quote.AssetType == kind && (kind != KindETF || visibleETF(quote.Code, quote.Name)) {
			filtered = append(filtered, quote)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].TurnoverFen > filtered[j].TurnoverFen })
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	for index := range filtered {
		if filtered[index].Source == "" {
			filtered[index].Source = fmt.Sprintf("csv-%s", kind)
		}
	}
	return filtered
}
