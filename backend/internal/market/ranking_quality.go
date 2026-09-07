package market

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// RankingQuality describes whether a saved close snapshot is safe to compare.
// It is intentionally separate from freshness: an old verified close is useful
// history, while a fresh intraday response is not a close record.
type RankingQuality string

const (
	RankingQualityVerifiedClose    RankingQuality = "verified_close"
	RankingQualityLegacyUnverified RankingQuality = "legacy_unverified"
	RankingQualityManualUnverified RankingQuality = "manual_unverified"
	RankingQualityIncomplete       RankingQuality = "incomplete"
	RankingUniverseV1                             = "ashare-turnover-filter-v1"
)

type RankingQualityInfo struct {
	Quality         RankingQuality `json:"quality"`
	UniverseVersion string         `json:"universeVersion,omitempty"`
	Reason          string         `json:"qualityReason,omitempty"`
}

// AssessCloseRanking applies the conservative first-version admission rule.
// Providers already fetch a larger amount-sorted universe before filtering; this
// function verifies the resulting, normalized Top 20 rather than treating an
// arbitrary 20-row response as complete.
func AssessCloseRanking(now time.Time, kind RankingKind, quotes []Quote) RankingQualityInfo {
	bad := func(reason string) RankingQualityInfo {
		return RankingQualityInfo{Quality: RankingQualityIncomplete, UniverseVersion: RankingUniverseV1, Reason: reason}
	}
	limit := StockRankingLimit
	if kind == KindETF {
		limit = ETFRankingLimit
	}
	if len(quotes) != limit {
		return bad(fmt.Sprintf("需要完整 Top%d，当前仅 %d 条", limit, len(quotes)))
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(loc)
	if localNow.Hour() < 15 || (localNow.Hour() == 15 && localNow.Minute() < 10) {
		return bad("尚未到收盘校验时间")
	}
	tradeDate := quotes[0].TradeDate
	seen := make(map[string]struct{}, len(quotes))
	for _, quote := range quotes {
		if quote.TradeDate != tradeDate || quote.AssetType != kind {
			return bad("交易日或资产类型不一致")
		}
		key := strings.ToUpper(quote.Market) + ":" + strings.ToUpper(quote.Code)
		if _, ok := seen[key]; ok {
			return bad("榜单包含重复证券")
		}
		seen[key] = struct{}{}
		if quote.TurnoverFen <= 0 || quote.CloseMinor < 0 {
			return bad("成交额或价格无效")
		}
		stamp := quote.SourceTime.In(loc)
		if stamp.Format("2006-01-02") != tradeDate || stamp.Hour() < 15 || stamp.After(localNow) {
			return bad("来源时间不能证明收盘")
		}
	}
	if !sort.SliceIsSorted(quotes, func(i, j int) bool {
		if quotes[i].TurnoverFen == quotes[j].TurnoverFen {
			left, right := quotes[i].Market+quotes[i].Code, quotes[j].Market+quotes[j].Code
			return left < right
		}
		return quotes[i].TurnoverFen > quotes[j].TurnoverFen
	}) {
		return bad("成交额排序不稳定")
	}
	return RankingQualityInfo{Quality: RankingQualityVerifiedClose, UniverseVersion: RankingUniverseV1}
}
