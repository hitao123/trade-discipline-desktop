package market

import "time"

// AShareCalendar is deliberately finite. Outside its published coverage callers
// must degrade instead of guessing weekdays are trading days. 2026 closures are
// from the SSE 2026 holiday trading arrangement (Shanghai Stock Exchange).
type AShareCalendar struct{ holidays map[string]bool }

func New2026AShareCalendar() AShareCalendar {
	return AShareCalendar{holidays: map[string]bool{
		"2026-01-01": true, "2026-01-02": true, "2026-01-03": true,
		"2026-02-16": true, "2026-02-17": true, "2026-02-18": true, "2026-02-19": true, "2026-02-20": true, "2026-02-23": true,
		"2026-04-06": true, "2026-05-01": true, "2026-05-04": true, "2026-05-05": true,
		"2026-06-19": true, "2026-09-25": true, "2026-10-01": true, "2026-10-02": true, "2026-10-05": true, "2026-10-06": true, "2026-10-07": true,
	}}
}
func (c AShareCalendar) IsTradingDay(date string) (bool, bool) {
	if len(date) != 10 || date < "2026-01-01" || date > "2026-12-31" {
		return false, false
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false, false
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday || c.holidays[date] {
		return false, true
	}
	return true, true
}
func (c AShareCalendar) PrevTradingDay(date string) (string, bool) {
	if _, known := c.IsTradingDay(date); !known {
		return "", false
	}
	t, _ := time.Parse("2006-01-02", date)
	for {
		t = t.AddDate(0, 0, -1)
		day := t.Format("2006-01-02")
		if ok, known := c.IsTradingDay(day); !known {
			return "", false
		} else if ok {
			return day, true
		}
	}
}
