package domain

import (
	"encoding/json"
	"testing"
)

func TestTradePlanDraftDecodesExistingPlanWithoutTargetExitRange(t *testing.T) {
	var draft TradePlanDraft
	if err := json.Unmarshal([]byte(`{"instrumentId":"hk-9988","riskExitMinor":9000}`), &draft); err != nil {
		t.Fatal(err)
	}
	if draft.TargetExitLowMinor != 0 || draft.TargetExitHighMinor != 0 {
		t.Fatalf("missing target range must remain disabled: %#v", draft)
	}
}
