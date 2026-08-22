package domain

import "time"

type AllocationItem struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	TargetWeightBP int    `json:"targetWeightBP"`
	TargetShares   int    `json:"targetShares,omitempty"`
	BuyRule        string `json:"buyRule"`
	SellRule       string `json:"sellRule"`
	Note           string `json:"note"`
}

type AllocationDraft struct {
	InitialCapitalFen int64            `json:"initialCapitalFen"`
	TargetReturnBP    int              `json:"targetReturnBP"`
	TargetDeadline    string           `json:"targetDeadline"`
	Items             []AllocationItem `json:"items"`
}

type AllocationVersion struct {
	ID        string          `json:"id"`
	ProfileID string          `json:"profileId"`
	Version   int             `json:"version"`
	Draft     AllocationDraft `json:"draft"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"createdAt"`
}

type AllocationValueEvent struct {
	ID         string    `json:"id"`
	ProfileID  string    `json:"profileId"`
	ItemKey    string    `json:"itemKey"`
	ValueFen   int64     `json:"valueFen"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observedAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

type AllocationSnapshot struct {
	ProfileID string                          `json:"profileId"`
	Version   AllocationVersion               `json:"version"`
	Values    map[string]AllocationValueEvent `json:"values"`
}

type AllocationItemProgress struct {
	Item                AllocationItem        `json:"item"`
	CurrentValueFen     int64                 `json:"currentValueFen"`
	CurrentWeightBP     int                   `json:"currentWeightBP"`
	BuildTargetFen      int64                 `json:"buildTargetFen"`
	BuildGapFen         int64                 `json:"buildGapFen"`
	RebalanceTargetFen  int64                 `json:"rebalanceTargetFen"`
	RebalanceGapFen     int64                 `json:"rebalanceGapFen"`
	LatestValueRecorded *AllocationValueEvent `json:"latestValueRecorded,omitempty"`
}

type AllocationOverview struct {
	ProfileID       string                   `json:"profileId"`
	Version         AllocationVersion        `json:"version"`
	CurrentTotalFen int64                    `json:"currentTotalFen"`
	TargetTotalFen  int64                    `json:"targetTotalFen"`
	ReturnBP        int                      `json:"returnBP"`
	GoalGapFen      int64                    `json:"goalGapFen"`
	Items           []AllocationItemProgress `json:"items"`
}
