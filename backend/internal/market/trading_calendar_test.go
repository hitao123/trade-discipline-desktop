package market

import "testing"

func TestPrevTradingDayCrossesLongHolidayAndRespectsCoverage(t *testing.T) {
	calendar := New2026AShareCalendar()
	if previous, known := calendar.PrevTradingDay("2026-02-24"); !known || previous != "2026-02-13" {
		t.Fatalf("long holiday previous=%q known=%v", previous, known)
	}
	for _, date := range []string{"2027-01-01", "2026-01-01"} {
		if previous, known := calendar.PrevTradingDay(date); known || previous != "" {
			t.Fatalf("out of coverage date=%s previous=%q known=%v", date, previous, known)
		}
	}
}
