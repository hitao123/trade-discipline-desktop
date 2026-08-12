package domain

import "fmt"

func Replay(initialCashFen int64, events []ExecutionEvent, cashEvents []CashEvent) (PortfolioState, error) {
	reversedExecution := make(map[string]bool)
	for _, event := range events {
		if event.EventType == ExecutionReversal {
			reversedExecution[event.OriginalEventID] = true
		}
	}
	reversedCash := make(map[string]bool)
	for _, event := range cashEvents {
		if event.OriginalEventID != "" {
			reversedCash[event.OriginalEventID] = true
		}
	}

	state := PortfolioState{
		AvailableCashFen: initialCashFen,
		Positions:        make(map[string]PositionState),
	}
	for _, event := range cashEvents {
		if event.OriginalEventID == "" && !reversedCash[event.ID] {
			state.AvailableCashFen += event.AmountFen
		}
	}
	for _, event := range events {
		if event.EventType == ExecutionReversal || reversedExecution[event.ID] {
			continue
		}
		position := state.Positions[event.InstrumentID]
		position.InstrumentID = event.InstrumentID
		position.Code = event.Code
		position.IsChinaTech = event.IsChinaTech
		switch event.EventType {
		case ExecutionBuy:
			if event.Quantity <= 0 || event.SettlementFen >= 0 {
				return PortfolioState{}, fmt.Errorf("invalid buy event %s", event.ID)
			}
			position.Quantity += event.Quantity
			position.CostFen += -event.SettlementFen
			state.AvailableCashFen += event.SettlementFen
		case ExecutionSell:
			if event.Quantity <= 0 || event.SettlementFen <= 0 {
				return PortfolioState{}, fmt.Errorf("invalid sell event %s", event.ID)
			}
			if event.Quantity > position.Quantity {
				return PortfolioState{}, fmt.Errorf("sell %d exceeds holding %d for %s", event.Quantity, position.Quantity, event.InstrumentID)
			}
			soldCost := position.CostFen * int64(event.Quantity) / int64(position.Quantity)
			position.Quantity -= event.Quantity
			position.CostFen -= soldCost
			position.RealizedPnLFen += event.SettlementFen - soldCost
			state.AvailableCashFen += event.SettlementFen
		default:
			return PortfolioState{}, fmt.Errorf("unknown execution type %q", event.EventType)
		}
		if position.Quantity == 0 {
			position.CostFen = 0
		}
		state.Positions[event.InstrumentID] = position
	}
	for id, position := range state.Positions {
		if position.Quantity == 0 && position.RealizedPnLFen == 0 {
			delete(state.Positions, id)
			continue
		}
		if position.IsChinaTech {
			state.ChinaTechExposureFen += position.CostFen
			state.ChinaTechUnrealizedPnLFen += position.UnrealizedPnLFen
		}
		if position.RealizedPnLFen < 0 {
			state.CumulativeLossFen += -position.RealizedPnLFen
		}
	}
	return state, nil
}
