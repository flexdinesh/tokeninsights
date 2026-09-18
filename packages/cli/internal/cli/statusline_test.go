package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestStatuslineRendersOrderedItems(t *testing.T) {
	lastSync := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()
	statusline := newStatuslineModel("month", "day", "tokens", "workstation", lastSync)
	output := strings.TrimRight(ansi.Strip(statusline.View(140)), " ")
	want := "TokenInsights · daterange: month · bucket: day · sort: tokens · hostname: workstation · lastsynced: 2026-07-10 17:48"
	if output != want {
		t.Fatalf("got statusline %q, want %q", output, want)
	}
}

func TestStatuslineWithValueReturnsUpdatedCopy(t *testing.T) {
	statusline := newStatuslineModel("month", "day", "tokens", "workstation", 0)
	updated := statusline.withValue(statuslineDateRange, "yesterday")

	if got := statusline.value(statuslineDateRange); got != "month" {
		t.Fatalf("original daterange changed to %q", got)
	}
	if got := updated.value(statuslineDateRange); got != "yesterday" {
		t.Fatalf("updated daterange is %q, want yesterday", got)
	}
}

func TestStatuslineDateRangeLabel(t *testing.T) {
	tests := []struct {
		name    string
		options tableOptions
		want    string
	}{
		{name: "preset", options: tableOptions{period: periodAllTime}, want: "all time"},
		{name: "bounded", options: tableOptions{filters: filters{dayFrom: "2026-04-20", dayTo: "2026-04-25"}}, want: "2026-04-20..2026-04-25"},
		{name: "from", options: tableOptions{filters: filters{dayFrom: "2026-04-20"}}, want: "2026-04-20.."},
		{name: "to", options: tableOptions{filters: filters{dayTo: "2026-04-25"}}, want: "..2026-04-25"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := statuslineDateRangeLabel(test.options); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		name  string
		value string
		err   error
		want  string
	}{
		{name: "value", value: "  workstation  ", want: "workstation"},
		{name: "blank", value: "  ", want: "unknown"},
		{name: "error", value: "workstation", err: errors.New("hostname unavailable"), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeHostname(test.value, test.err); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestFormatLastSync(t *testing.T) {
	if got := formatLastSync(0); got != "never" {
		t.Fatalf("got %q, want never", got)
	}
	value := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()
	if got := formatLastSync(value); got != "2026-07-10 17:48" {
		t.Fatalf("got %q, want formatted local time", got)
	}
}

func TestStatuslineUsesExistingDistinctPaletteColors(t *testing.T) {
	if got := statuslineStyle(statuslineBrand).GetForeground(); got != themeBrand {
		t.Fatalf("brand foreground is %v, want brand", got)
	}
	for _, id := range []statuslineItemID{statuslineDateRange, statuslineBucket, statuslineSort, statuslineHostname, statuslineLastSynced} {
		if got := statuslineStyle(id).GetForeground(); got != themeMuted {
			t.Fatalf("item %d foreground is %v, want muted", id, got)
		}
	}
	if got := statuslineSeparatorStyle.GetForeground(); got != themeFaint {
		t.Fatalf("separator foreground is %v, want faint", got)
	}
}

func TestStatuslineFitsSingleRowWithoutDanglingSeparators(t *testing.T) {
	lastSync := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()
	statusline := newStatuslineModel("month", "day", "tokens", "a-very-long-workstation-hostname", lastSync)

	for _, width := range []int{120, 80, 60, 40, 20, 8, 3} {
		output := statusline.View(width)
		if strings.Contains(output, "\n") {
			t.Fatalf("width %d rendered more than one row: %q", width, output)
		}
		if got := ansi.StringWidth(output); got != width {
			t.Fatalf("width %d rendered %d cells", width, got)
		}
		plain := strings.TrimSpace(ansi.Strip(output))
		if strings.HasPrefix(plain, "·") || strings.HasSuffix(plain, "·") || strings.Contains(plain, "·  ·") {
			t.Fatalf("width %d rendered dangling separator: %q", width, plain)
		}
	}
}

func TestStatuslineTruncatesHostnameBeforeLastSync(t *testing.T) {
	lastSync := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()
	statusline := newStatuslineModel("month", "day", "tokens", "a-very-long-workstation-hostname", lastSync)
	withoutHostname := removeStatuslineItem(append([]statuslineItem(nil), statusline.items...), statuslineHostname)
	width := statuslineItemsWidth(withoutHostname)
	output := strings.TrimSpace(ansi.Strip(statusline.View(width)))

	if strings.Contains(output, "hostname:") {
		t.Fatalf("hostname should be omitted first at width %d: %q", width, output)
	}
	if !strings.Contains(output, "lastsynced: 2026-07-10 17:48") {
		t.Fatalf("last sync should remain after hostname is omitted: %q", output)
	}
}
