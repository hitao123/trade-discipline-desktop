package market

import "time"

type HealthState string

const (
	HealthLive        HealthState = "live"
	HealthDelayed     HealthState = "delayed"
	HealthCached      HealthState = "cached"
	HealthUnavailable HealthState = "unavailable"
)

type ComponentHealth struct {
	State            HealthState `json:"state"`
	Source           string      `json:"source,omitempty"`
	SourceTime       *time.Time  `json:"sourceTime,omitempty"`
	LastSuccessfulAt *time.Time  `json:"lastSuccessfulAt,omitempty"`
	Message          string      `json:"message"`
	DetailCode       string      `json:"detailCode,omitempty"`
}

func AssessFreshness(now time.Time, tradeDate string, sourceTime time.Time, maxAge time.Duration) ComponentHealth {
	if tradeDate == "" || sourceTime.IsZero() {
		return ComponentHealth{State: HealthUnavailable, Message: "暂不可用", DetailCode: "SOURCE_TIME_MISSING"}
	}
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	localNow := now.In(shanghai)
	stamp := sourceTime.UTC()
	health := ComponentHealth{SourceTime: &stamp}
	if tradeDate != localNow.Format("2006-01-02") {
		health.State = HealthCached
		health.Message = "本地缓存"
		health.DetailCode = "SOURCE_TRADE_DATE_STALE"
		return health
	}
	age := now.Sub(sourceTime)
	if age < 0 || age > maxAge {
		health.State = HealthDelayed
		health.Message = "当日延迟"
		health.DetailCode = "SOURCE_TIME_DELAYED"
		return health
	}
	health.State = HealthLive
	health.Message = "实时"
	return health
}
