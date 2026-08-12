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

type Provider interface {
	FetchRankings(ctx context.Context, kind RankingKind) ([]Quote, error)
	FetchQuotes(ctx context.Context, keys []InstrumentKey) ([]Quote, error)
}
