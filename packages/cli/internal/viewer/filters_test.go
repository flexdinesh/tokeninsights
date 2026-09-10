package viewer

import (
	"testing"
	"time"
)

func TestCalendarRangesAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = location
	defer func() { time.Local = previous }()
	for _, test := range []struct {
		date  string
		hours time.Duration
	}{{"2026-03-08", 23}, {"2026-11-01", 25}} {
		now, err := time.ParseInLocation(time.DateOnly, test.date, location)
		if err != nil {
			t.Fatal(err)
		}
		start, end := PeriodStart(now, "today"), PeriodEnd(now, "today")
		if end.Sub(start) != test.hours*time.Hour {
			t.Fatalf("%s duration %v", test.date, end.Sub(start))
		}
		if PeriodStart(now, "week").Weekday() != time.Monday {
			t.Fatal("week must begin on Monday")
		}
		if PeriodEnd(now, "week").Weekday() != time.Monday {
			t.Fatal("week end must use calendar arithmetic")
		}
	}
}

func TestCustomBoundsReplacePreset(t *testing.T) {
	s := Selection{Period: "today", Bucket: "day", From: "2026-01-01"}
	f := s.Filter(time.Now())
	if !f.Start.IsZero() || !f.End.IsZero() || f.DayFrom != "2026-01-01" {
		t.Fatalf("custom bound intersects preset: %+v", f)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.To = "2025-12-31"
	if s.Validate() == nil {
		t.Fatal("reversed dates accepted")
	}
}
