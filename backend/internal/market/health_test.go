package market

import (
	"testing"
	"time"
)

func TestAssessFreshnessRejectsPreviousTradeDate(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, shanghai)
	sourceTime := time.Date(2026, 9, 30, 15, 0, 0, 0, shanghai)

	health := AssessFreshness(now, "2026-09-30", sourceTime, 5*time.Minute)

	if health.State != HealthCached {
		t.Fatalf("state=%q want=%q", health.State, HealthCached)
	}
}

func TestAssessFreshnessClassifiesCurrentTradingDayByAge(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, shanghai)
	tests := []struct {
		name       string
		sourceTime time.Time
		want       HealthState
	}{
		{name: "fresh", sourceTime: now.Add(-4 * time.Minute), want: HealthLive},
		{name: "at boundary", sourceTime: now.Add(-5 * time.Minute), want: HealthLive},
		{name: "too old", sourceTime: now.Add(-6 * time.Minute), want: HealthDelayed},
		{name: "future timestamp", sourceTime: now.Add(time.Minute), want: HealthDelayed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			health := AssessFreshness(now, "2026-08-21", test.sourceTime, 5*time.Minute)
			if health.State != test.want {
				t.Fatalf("state=%q want=%q", health.State, test.want)
			}
		})
	}
}
