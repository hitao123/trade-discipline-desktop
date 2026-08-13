package market

import (
	"context"
	"time"
)

type RankingKind string

const (
	KindStock RankingKind = "stock"
	KindETF   RankingKind = "etf"
)

type InstrumentKey struct {
	Market string `json:"market"`
	Code   string `json:"code"`
}

type Quote struct {
	TradeDate   string      `json:"tradeDate"`
	Market      string      `json:"market"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	AssetType   RankingKind `json:"assetType"`
	CloseMinor  int64       `json:"closeMinor"`
	ChangeBP    int         `json:"changeBP"`
	TurnoverFen int64       `json:"turnoverFen"`
	Source      string      `json:"source"`
	SourceTime  time.Time   `json:"sourceTime"`
}

type DailyBar struct {
	TradeDate   string    `json:"tradeDate"`
	Market      string    `json:"market"`
	Code        string    `json:"code"`
	CloseMinor  int64     `json:"closeMinor"`
	TurnoverFen int64     `json:"turnoverFen"`
	Source      string    `json:"source"`
	SourceTime  time.Time `json:"sourceTime"`
}

type MetricKind string

const (
	MetricAShareTurnover     MetricKind = "ashare_turnover"
	MetricSouthboundNetBuy   MetricKind = "southbound_net_buy"
	MetricSHTurnover         MetricKind = "sh_turnover"
	MetricSZTurnover         MetricKind = "sz_turnover"
	MetricSouthboundSHNetBuy MetricKind = "southbound_sh_net_buy"
	MetricSouthboundSZNetBuy MetricKind = "southbound_sz_net_buy"
)

type MetricPoint struct {
	TradeDate  string     `json:"tradeDate"`
	Metric     MetricKind `json:"metric"`
	ValueFen   int64      `json:"valueFen"`
	Source     string     `json:"source"`
	SourceTime time.Time  `json:"sourceTime"`
}

type MetricBatch struct {
	Points []MetricPoint         `json:"points"`
	Errors map[MetricKind]string `json:"errors,omitempty"`
}

type Provider interface {
	FetchRankings(ctx context.Context, kind RankingKind) ([]Quote, error)
	FetchQuotes(ctx context.Context, keys []InstrumentKey) ([]Quote, error)
	FetchDailyBars(ctx context.Context, key InstrumentKey, limit int) ([]DailyBar, error)
	FetchMarketMetrics(ctx context.Context, limit int) MetricBatch
}
