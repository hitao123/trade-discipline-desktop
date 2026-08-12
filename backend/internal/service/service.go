package service

import (
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type Service struct {
	store          *store.Store
	now            func() time.Time
	marketProvider market.Provider
}

func New(store *store.Store, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, now: now, marketProvider: &market.EastmoneyProvider{}}
}
