package domain

import "testing"

func TestReplayUsesActualRMBSettlementForWeightedCost(t *testing.T) {
	state, err := Replay(20_000_000, []ExecutionEvent{
		{ID: "buy-1", EventType: ExecutionBuy, InstrumentID: "hk-9988", Code: "9988.HK", Quantity: 100, SettlementFen: -1_205_000, IsChinaTech: true},
		{ID: "sell-1", EventType: ExecutionSell, InstrumentID: "hk-9988", Code: "9988.HK", Quantity: 40, SettlementFen: 520_000, IsChinaTech: true, ExitCode: "T"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	position := state.Positions["hk-9988"]
	if state.AvailableCashFen != 19_315_000 || position.CostFen != 723_000 || position.RealizedPnLFen != 38_000 || position.Quantity != 60 {
		t.Fatalf("unexpected state: %#v, position: %#v", state, position)
	}
}

func TestReplayReversalRestoresState(t *testing.T) {
	state, err := Replay(20_000_000, []ExecutionEvent{
		{ID: "buy-1", EventType: ExecutionBuy, InstrumentID: "hk-9988", Code: "9988.HK", Quantity: 100, SettlementFen: -1_205_000},
		{ID: "reverse-1", EventType: ExecutionReversal, OriginalEventID: "buy-1", InstrumentID: "hk-9988"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.AvailableCashFen != 20_000_000 || len(state.Positions) != 0 {
		t.Fatalf("unexpected state after reversal: %#v", state)
	}
}

func TestReplayAppliesCashEventsToAvailableCash(t *testing.T) {
	state, err := Replay(20_000_000, nil, []CashEvent{
		{ID: "in-1", AmountFen: 5_000_000},
		{ID: "out-1", AmountFen: -1_200_000},
		{ID: "div-1", AmountFen: 8_000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.AvailableCashFen != 23_808_000 {
		t.Fatalf("available cash=%d", state.AvailableCashFen)
	}
}

func TestReplayRejectsSellBeyondHolding(t *testing.T) {
	_, err := Replay(20_000_000, []ExecutionEvent{
		{ID: "sell-1", EventType: ExecutionSell, InstrumentID: "hk-9988", Quantity: 100, SettlementFen: 1_200_000, ExitCode: "T"},
	}, nil)
	if err == nil {
		t.Fatal("expected oversell failure")
	}
}
