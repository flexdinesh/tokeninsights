package server

import (
	"testing"
	"time"
)

func TestReportingTimezonePreservesHistoricalDSTRules(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	summer := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	name := reportingTimezone(location, summer)
	if name != "America/New_York" {
		t.Fatal("lost historical timezone rules", name)
	}
	clientLocation, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	winter := time.Date(2026, time.January, 1, 4, 30, 0, 0, time.UTC)
	if day := winter.In(clientLocation).Format(time.DateOnly); day != "2025-12-31" {
		t.Fatal("historical date used summer offset", day)
	}
	if _, offset := winter.In(clientLocation).Zone(); offset != -5*3600 {
		t.Fatal("winter offset", offset)
	}
}

func TestReportingTimezoneFixedOffsetFallback(t *testing.T) {
	for _, test := range []struct {
		offset int
		want   string
	}{{19800, "UTC+05:30"}, {-12600, "UTC-03:30"}, {0, "UTC+00:00"}} {
		if got := reportingTimezone(time.FixedZone("unresolvable-fixture", test.offset), time.Now()); got != test.want {
			t.Fatalf("%d: %s", test.offset, got)
		}
	}
}
