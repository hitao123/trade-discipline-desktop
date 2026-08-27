package domain

import "time"

type UserMode string

const (
	UserModeLegacy  UserMode = "legacy"
	UserModeGeneric UserMode = "generic"
)

type OnboardingStatus string

const (
	OnboardingPending   OnboardingStatus = "pending"
	OnboardingCompleted OnboardingStatus = "completed"
)

type HoldingHorizon string

const (
	HorizonUnder6M HoldingHorizon = "under_6m"
	Horizon6To12M  HoldingHorizon = "6_to_12m"
	Horizon1To3Y   HoldingHorizon = "1_to_3y"
	HorizonOver3Y  HoldingHorizon = "over_3y"
	HorizonLegacy  HoldingHorizon = "legacy_unspecified"
)

type MarketScope string

const (
	MarketAShareStock MarketScope = "ashare_stock"
	MarketAShareETF   MarketScope = "ashare_etf"
	MarketHK          MarketScope = "hk"
)

type UserProfile struct {
	ID                   string           `json:"id"`
	Mode                 UserMode         `json:"mode"`
	OnboardingStatus     OnboardingStatus `json:"onboardingStatus"`
	InvestableCapitalFen int64            `json:"investableCapitalFen"`
	MaxLossFen           int64            `json:"maxLossFen"`
	HoldingHorizon       HoldingHorizon   `json:"holdingHorizon"`
	EnabledMarkets       []MarketScope    `json:"enabledMarkets"`
	CompletedAt          *time.Time       `json:"completedAt,omitempty"`
	UpdatedAt            time.Time        `json:"updatedAt"`
}
