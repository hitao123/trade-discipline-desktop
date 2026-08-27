package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type ExecutionEmotion = domain.ExecutionEmotion

type QuickExecutionDraft struct {
	InstrumentID            string           `json:"instrumentId"`
	Side                    string           `json:"side"`
	Quantity                int              `json:"quantity"`
	LocalPriceMinor         int64            `json:"localPriceMinor"`
	LocalPriceTenThousandth int64            `json:"localPriceTenThousandth,omitempty"`
	SettlementFen           int64            `json:"settlementFen"`
	Emotion                 ExecutionEmotion `json:"emotion"`
	PlanID                  string           `json:"planId,omitempty"`
	ExecutedAt              time.Time        `json:"executedAt,omitempty"`
	BrokerReference         string           `json:"brokerReference,omitempty"`
	ExitCode                string           `json:"exitCode,omitempty"`
	Evidence                string           `json:"evidence,omitempty"`
}

func (s *Service) RecordQuickExecution(ctx context.Context, draft QuickExecutionDraft) (ExecutionReceipt, error) {
	if draft.LocalPriceTenThousandth <= 0 && draft.LocalPriceMinor > 0 {
		draft.LocalPriceTenThousandth = draft.LocalPriceMinor * 100
	}
	if draft.Quantity <= 0 || draft.LocalPriceTenThousandth <= 0 {
		return ExecutionReceipt{}, fmt.Errorf("成交数量和成交均价必须大于 0")
	}
	if int64(draft.Quantity) > math.MaxInt64/draft.LocalPriceTenThousandth {
		return ExecutionReceipt{}, fmt.Errorf("本币成交额超出可记录范围")
	}
	product := draft.LocalPriceTenThousandth * int64(draft.Quantity)
	localAmountMinor := (product + 50) / 100
	draft.LocalPriceMinor = (draft.LocalPriceTenThousandth + 50) / 100
	instruments, err := s.store.ListInstruments(ctx)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	var instrument *store.InstrumentRow
	for index := range instruments {
		if instruments[index].ID == draft.InstrumentID {
			instrument = &instruments[index]
			break
		}
	}
	if instrument == nil {
		return ExecutionReceipt{}, fmt.Errorf("证券不存在或已停用")
	}
	if draft.SettlementFen == 0 {
		if instrument.Currency == "HKD" {
			return ExecutionReceipt{}, fmt.Errorf("港股请填写券商显示的实际人民币扣款或到账")
		}
		if draft.Side == "buy" {
			draft.SettlementFen = -localAmountMinor
		} else if draft.Side == "sell" {
			draft.SettlementFen = localAmountMinor
		}
	}
	return s.recordExecution(ctx, ExecutionDraft{
		PlanID: draft.PlanID, InstrumentID: draft.InstrumentID, Side: draft.Side, ExecutedAt: draft.ExecutedAt,
		Quantity: draft.Quantity, LocalPriceMinor: draft.LocalPriceMinor, LocalPriceTenThousandth: draft.LocalPriceTenThousandth, LocalAmountMinor: localAmountMinor,
		SettlementFen: draft.SettlementFen, ExitCode: draft.ExitCode, Evidence: draft.Evidence, BrokerReference: draft.BrokerReference,
		Emotion: draft.Emotion,
	}, true)
}

func (s *Service) PendingPostTradeReviews(ctx context.Context, periodStart, periodEnd string) ([]store.PostTradeReview, error) {
	start, end, err := postTradeReviewPeriod(periodStart, periodEnd, s.now())
	if err != nil {
		return nil, err
	}
	return s.store.PendingPostTradeReviews(ctx, start, end)
}

func (s *Service) CompletePostTradeReview(ctx context.Context, executionID, note string) (store.PostTradeReview, error) {
	if strings.TrimSpace(executionID) == "" {
		return store.PostTradeReview{}, fmt.Errorf("成交记录不能为空")
	}
	return s.store.CompletePostTradeReview(ctx, executionID, note, s.now())
}

func postTradeReviewPeriod(periodStart, periodEnd string, now time.Time) (time.Time, time.Time, error) {
	if periodStart == "" && periodEnd == "" {
		return time.Unix(0, 0).UTC(), now.UTC().AddDate(10, 0, 0), nil
	}
	start, err := parseShanghaiDate(periodStart)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("待复盘开始日期无效")
	}
	end, err := parseShanghaiDate(periodEnd)
	if err != nil || end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("待复盘结束日期无效")
	}
	return start, end.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
}

func parseShanghaiDate(value string) (time.Time, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("2006-01-02", value, location)
}
