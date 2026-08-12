package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type WatchlistDraft struct {
	Market     string `json:"market"`
	Code       string `json:"code"`
	Reason     string `json:"reason"`
	SourceType string `json:"sourceType"`
}

func (s *Service) AddWatchlist(ctx context.Context, draft WatchlistDraft) (store.WatchlistRow, error) {
	draft.Market = strings.ToUpper(strings.TrimSpace(draft.Market))
	draft.Code = strings.ToUpper(strings.TrimSpace(draft.Code))
	if draft.Market != "HK" && draft.Market != "SH" && draft.Market != "SZ" {
		return store.WatchlistRow{}, fmt.Errorf("市场只能是 HK、SH 或 SZ")
	}
	if strings.TrimSpace(draft.Reason) == "" {
		return store.WatchlistRow{}, fmt.Errorf("观察理由不能为空")
	}
	if draft.SourceType == "" {
		draft.SourceType = "manual"
	}
	return s.store.AddWatchlist(ctx, draft.Market, draft.Code, draft.SourceType, strings.TrimSpace(draft.Reason), s.now())
}

func (s *Service) ListWatchlist(ctx context.Context) ([]store.WatchlistRow, error) {
	return s.store.ListWatchlist(ctx)
}
