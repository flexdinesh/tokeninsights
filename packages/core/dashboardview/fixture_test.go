package dashboardview_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// Shared with the web tests; see packages/contracts/fixtures/dashboard-view.
var fixtureDirectory = filepath.Join("..", "..", "contracts", "fixtures", "dashboard-view")

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func instant(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestFormatFixtures(t *testing.T) {
	var fixture struct {
		CompactCount, ExactCount []struct {
			Value    int64
			Expected string
		}
		TokenShare []struct {
			Tokens, Total int64
			Expected      string
		}
		ServerZoneLabel []struct{ Timezone, Expected string }
		ServerDate      []struct{ Instant, Timezone, Expected string }
		ServerDateTime  []struct{ Instant, Timezone, Expected string }
		TabLabels       map[dashboardview.Tab]string
	}
	readFixture(t, "format.json", &fixture)
	for _, c := range fixture.CompactCount {
		if got := dashboardview.CompactCount(c.Value); got != c.Expected {
			t.Errorf("CompactCount(%d) = %q, want %q", c.Value, got, c.Expected)
		}
	}
	for _, c := range fixture.ExactCount {
		if got := dashboardview.ExactCount(c.Value); got != c.Expected {
			t.Errorf("ExactCount(%d) = %q, want %q", c.Value, got, c.Expected)
		}
	}
	for _, c := range fixture.TokenShare {
		if got := dashboardview.TokenShare(c.Tokens, c.Total); got != c.Expected {
			t.Errorf("TokenShare(%d, %d) = %q, want %q", c.Tokens, c.Total, got, c.Expected)
		}
	}
	for _, c := range fixture.ServerZoneLabel {
		if got := dashboardview.ServerZoneLabel(c.Timezone); got != c.Expected {
			t.Errorf("ServerZoneLabel(%q) = %q, want %q", c.Timezone, got, c.Expected)
		}
	}
	for _, c := range fixture.ServerDate {
		if got := dashboardview.FormatServerDate(instant(t, c.Instant).UnixMilli(), c.Timezone); got != c.Expected {
			t.Errorf("FormatServerDate(%s, %q) = %q, want %q", c.Instant, c.Timezone, got, c.Expected)
		}
	}
	for _, c := range fixture.ServerDateTime {
		if got := dashboardview.FormatServerDateTime(instant(t, c.Instant).UnixMilli(), c.Timezone); got != c.Expected {
			t.Errorf("FormatServerDateTime(%s, %q) = %q, want %q", c.Instant, c.Timezone, got, c.Expected)
		}
	}
	tabs := dashboardview.Tabs()
	if len(fixture.TabLabels) != len(tabs) {
		t.Fatalf("fixture labels %d tabs, Tabs() has %d", len(fixture.TabLabels), len(tabs))
	}
	for _, tab := range tabs {
		if got := dashboardview.TabLabel(tab); got != fixture.TabLabels[tab] {
			t.Errorf("TabLabel(%s) = %q, want %q", tab, got, fixture.TabLabels[tab])
		}
	}
}

func TestDateRangeFixtures(t *testing.T) {
	var fixture struct {
		IsCalendarRange []struct {
			Period, From, To string
			Expected         bool
		}
		CalendarRange []struct {
			Period, Timezone, Now string
			Expected              dashboardview.DateRange
		}
		ShiftCalendarRange []struct {
			Period, From string
			Direction    int
			Expected     dashboardview.DateRange
		}
		CalendarRangeLabel []struct{ Period, From, To, Expected string }
	}
	readFixture(t, "date-range.json", &fixture)
	for _, c := range fixture.IsCalendarRange {
		if got := dashboardview.IsCalendarRange(c.Period, c.From, c.To); got != c.Expected {
			t.Errorf("IsCalendarRange(%q, %q, %q) = %v, want %v", c.Period, c.From, c.To, got, c.Expected)
		}
	}
	for _, c := range fixture.CalendarRange {
		if got := dashboardview.CalendarRange(c.Period, c.Timezone, instant(t, c.Now)); got != c.Expected {
			t.Errorf("CalendarRange(%q, %q, %s) = %+v, want %+v", c.Period, c.Timezone, c.Now, got, c.Expected)
		}
	}
	for _, c := range fixture.ShiftCalendarRange {
		got, err := dashboardview.ShiftCalendarRange(c.Period, c.From, c.Direction)
		if err != nil || got != c.Expected {
			t.Errorf("ShiftCalendarRange(%q, %q, %d) = %+v, %v; want %+v", c.Period, c.From, c.Direction, got, err, c.Expected)
		}
	}
	for _, c := range fixture.CalendarRangeLabel {
		got, err := dashboardview.CalendarRangeLabel(c.Period, c.From, c.To)
		if err != nil || got != c.Expected {
			t.Errorf("CalendarRangeLabel(%q, %q, %q) = %q, %v; want %q", c.Period, c.From, c.To, got, err, c.Expected)
		}
	}
}

func TestFilterFixtures(t *testing.T) {
	var fixture struct {
		Options []struct {
			Name                string
			Values              []dashboardview.FilterOption
			Selected            []string
			MissingName, Search string
			Expected            []string
		}
	}
	readFixture(t, "filters.json", &fixture)
	for _, c := range fixture.Options {
		t.Run(c.Name, func(t *testing.T) {
			options := dashboardview.FilterOptions(c.Values, c.Selected, c.MissingName, c.Search)
			names := make([]string, len(options))
			for index, option := range options {
				names[index] = option.Name
			}
			if !slices.Equal(names, c.Expected) {
				t.Errorf("names = %q, want %q", names, c.Expected)
			}
		})
	}
}
