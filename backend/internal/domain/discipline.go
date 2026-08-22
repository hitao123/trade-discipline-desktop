package domain

import "time"

type AlertKind string

const (
	AlertRiskExit   AlertKind = "risk_exit"
	AlertTargetZone AlertKind = "target_zone"
)

type ReviewDecision string

const (
	ReviewHold ReviewDecision = "hold"
	ReviewTrim ReviewDecision = "trim"
	ReviewSell ReviewDecision = "sell"
	ReviewWait ReviewDecision = "wait"
)

type MonitorSettings struct {
	Interval string `json:"interval"`
}

type MonitorStatus struct {
	Enabled          bool       `json:"enabled"`
	Interval         string     `json:"interval"`
	LastAttemptAt    *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessfulAt *time.Time `json:"lastSuccessfulAt,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
}

type PreTradeConfirmation struct {
	ID               string    `json:"id"`
	PlanID           string    `json:"planId"`
	PlanSnapshotJSON string    `json:"-"`
	StartedAt        time.Time `json:"startedAt"`
	ConfirmedAt      time.Time `json:"confirmedAt"`
}

type PriceAlertEvent struct {
	ID                string     `json:"id"`
	PlanID            string     `json:"planId"`
	InstrumentID      string     `json:"instrumentId"`
	Kind              AlertKind  `json:"kind"`
	TriggerPriceMinor int64      `json:"triggerPriceMinor"`
	ThresholdMinor    int64      `json:"thresholdMinor"`
	PlanSnapshotJSON  string     `json:"-"`
	Source            string     `json:"source"`
	SourceTime        time.Time  `json:"sourceTime"`
	TriggeredAt       time.Time  `json:"triggeredAt"`
	NotifiedAt        *time.Time `json:"notifiedAt,omitempty"`
}

type PositionReviewEvent struct {
	ID        string         `json:"id"`
	AlertID   string         `json:"alertId"`
	Decision  ReviewDecision `json:"decision"`
	Reason    string         `json:"reason"`
	CreatedAt time.Time      `json:"createdAt"`
}
