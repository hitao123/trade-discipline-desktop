package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type RankingComparisonEntry struct {
	Quote        market.Quote `json:"quote"`
	Rank         int          `json:"rank"`
	PreviousRank *int         `json:"previousRank"`
	RankDelta    *int         `json:"rankDelta"`
	ChangeState  string       `json:"changeState"`
	StreakDays   *int         `json:"streakDays"`
	StreakExact  bool         `json:"streakExact"`
	StreakReason string       `json:"streakReason,omitempty"`
}
type RankingComparison struct {
	Snapshot           *store.MarketSnapshotRow `json:"snapshot"`
	BaselineDate       *string                  `json:"baselineDate"`
	BaselineSnapshotID *string                  `json:"baselineSnapshotId"`
	Quality            market.RankingQuality    `json:"quality"`
	UniverseVersion    string                   `json:"universeVersion,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	Entries            []RankingComparisonEntry `json:"entries"`
}
type RankingDatesResult struct {
	Kind       market.RankingKind  `json:"kind"`
	Dates      []store.RankingDate `json:"dates"`
	LatestDate string              `json:"latestDate,omitempty"`
}

func validRankingKind(kind market.RankingKind) bool {
	return kind == market.KindStock || kind == market.KindETF
}
func (s *Service) RankingDates(ctx context.Context, kind market.RankingKind) (RankingDatesResult, error) {
	if !validRankingKind(kind) {
		return RankingDatesResult{}, fmt.Errorf("榜单类型仅支持 stock 或 etf")
	}
	dates, err := s.store.ListRankingDates(ctx, kind)
	if err != nil {
		return RankingDatesResult{}, err
	}
	result := RankingDatesResult{Kind: kind, Dates: dates}
	if len(dates) > 0 {
		result.LatestDate = dates[0].TradeDate
	}
	return result, nil
}
func (s *Service) RankingComparison(ctx context.Context, kind market.RankingKind, date string) (RankingComparison, error) {
	if !validRankingKind(kind) {
		return RankingComparison{}, fmt.Errorf("榜单类型仅支持 stock 或 etf")
	}
	if date == "" {
		dates, err := s.store.ListRankingDates(ctx, kind)
		if err != nil {
			return RankingComparison{}, err
		}
		if len(dates) == 0 {
			return RankingComparison{Reason: "no_history", Entries: []RankingComparisonEntry{}}, nil
		}
		date = dates[0].TradeDate
	}
	snapshot, err := s.store.RankingSnapshotAt(ctx, kind, date)
	if err == sql.ErrNoRows {
		return RankingComparison{}, fmt.Errorf("该日期没有本地榜单")
	}
	if err != nil {
		return RankingComparison{}, err
	}
	result := RankingComparison{Snapshot: &snapshot, Quality: snapshot.Quality, UniverseVersion: snapshot.UniverseVersion, Entries: []RankingComparisonEntry{}}
	calendar := market.New2026AShareCalendar()
	previousDate, known := calendar.PrevTradingDay(date)
	if !known {
		result.Reason = "calendar_unknown"
		return s.unknownRankingEntries(result), nil
	}
	result.BaselineDate = &previousDate
	if snapshot.Quality != market.RankingQualityVerifiedClose {
		result.Reason = "unverified"
		return s.unknownRankingEntries(result), nil
	}
	baseline, err := s.store.RankingSnapshotAt(ctx, kind, previousDate)
	if err == sql.ErrNoRows {
		result.Reason = "missing_baseline"
		return s.withStreaks(ctx, result, calendar), nil
	}
	if err != nil {
		return RankingComparison{}, err
	}
	if baseline.Quality != market.RankingQualityVerifiedClose {
		result.Reason = "missing_baseline"
		return s.withStreaks(ctx, result, calendar), nil
	}
	result.BaselineSnapshotID = &baseline.ID
	if baseline.UniverseVersion != snapshot.UniverseVersion {
		result.Reason = "universe_changed"
		return s.unknownRankingEntries(result), nil
	}
	prior := map[string]int{}
	for i, q := range baseline.Entries {
		prior[q.Market+":"+q.Code] = i + 1
	}
	for i, q := range snapshot.Entries {
		entry := RankingComparisonEntry{Quote: q, Rank: i + 1, ChangeState: "new"}
		if rank, ok := prior[q.Market+":"+q.Code]; ok {
			entry.PreviousRank = &rank
			delta := rank - (i + 1)
			entry.RankDelta = &delta
			entry.ChangeState = "unchanged"
			if delta > 0 {
				entry.ChangeState = "up"
			}
			if delta < 0 {
				entry.ChangeState = "down"
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return s.withStreaks(ctx, result, calendar), nil
}
func (s *Service) unknownRankingEntries(result RankingComparison) RankingComparison {
	for i, q := range result.Snapshot.Entries {
		result.Entries = append(result.Entries, RankingComparisonEntry{Quote: q, Rank: i + 1, ChangeState: "unknown"})
	}
	return result
}
func (s *Service) withStreaks(ctx context.Context, result RankingComparison, calendar market.AShareCalendar) RankingComparison {
	if result.Snapshot == nil || result.Snapshot.Quality != market.RankingQualityVerifiedClose {
		return result
	}
	for i := range result.Entries {
		days := 1
		exact := true
		reason := ""
		date := result.Snapshot.TradeDate
		key := result.Entries[i].Quote.Market + ":" + result.Entries[i].Quote.Code
		for {
			previous, known := calendar.PrevTradingDay(date)
			if !known {
				exact = false
				reason = "calendar_unknown"
				break
			}
			older, err := s.store.RankingSnapshotAt(ctx, result.Snapshot.Kind, previous)
			if err == sql.ErrNoRows {
				exact = false
				reason = "missing_history"
				break
			}
			if err != nil {
				exact = false
				reason = "history_unavailable"
				break
			}
			if older.Quality != market.RankingQualityVerifiedClose || older.UniverseVersion != result.Snapshot.UniverseVersion {
				exact = false
				reason = "history_unverified"
				break
			}
			found := false
			for _, q := range older.Entries {
				if q.Market+":"+q.Code == key {
					found = true
					break
				}
			}
			if !found {
				break
			}
			days++
			date = previous
		}
		result.Entries[i].StreakDays = &days
		result.Entries[i].StreakExact = exact
		result.Entries[i].StreakReason = reason
	}
	return result
}
