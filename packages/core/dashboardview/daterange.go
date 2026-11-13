package dashboardview

import (
	"errors"
	"regexp"
	"strconv"
	"time"
)

// Periods of analytics.Selection.
const (
	PeriodToday     = "today"
	PeriodYesterday = "yesterday"
	PeriodWeek      = "week"
	PeriodMonth     = "month"
	PeriodYear      = "year"
	PeriodAll       = "all"
)

// DateRange is an inclusive pair of server-calendar dates (YYYY-MM-DD).
type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

const (
	daysPerWeek = 7
	maxMonthDay = 31
	isoDate     = time.DateOnly
)

// ErrInvalidDate reports a bound that is not a YYYY-MM-DD calendar date.
var ErrInvalidDate = errors.New("invalid date: use YYYY-MM-DD")

var isoDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// parseDate accepts dates the way the web's Date parser does: days 29–31 roll
// into the next month ("2026-02-30" is March 2), other out-of-range parts reject.
func parseDate(value string) (time.Time, error) {
	if !isoDatePattern.MatchString(value) {
		return time.Time{}, ErrInvalidDate
	}
	year, _ := strconv.Atoi(value[0:4])
	month, _ := strconv.Atoi(value[5:7])
	day, _ := strconv.Atoi(value[8:10])
	if month < int(time.January) || month > int(time.December) || day < 1 || day > maxMonthDay {
		return time.Time{}, ErrInvalidDate
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// rangeFromDate returns the period containing date. Weeks run Monday to Sunday;
// day and unknown periods are the date itself.
func rangeFromDate(period string, date time.Time) DateRange {
	start, end := date, date
	switch period {
	case PeriodWeek:
		start = date.AddDate(0, 0, -((int(date.Weekday()) - int(time.Monday) + daysPerWeek) % daysPerWeek))
		end = start.AddDate(0, 0, daysPerWeek-1)
	case PeriodMonth:
		start = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, -1)
	case PeriodYear:
		start = time.Date(date.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(1, 0, -1)
	}
	return DateRange{From: start.Format(isoDate), To: end.Format(isoDate)}
}

func calendarPeriod(period string) bool {
	switch period {
	case PeriodToday, PeriodYesterday, PeriodWeek, PeriodMonth, PeriodYear:
		return true
	}
	return false
}

// IsCalendarRange reports whether from/to are exactly one complete period with
// real, ordered dates. A restored calendar mode survives only when this holds.
func IsCalendarRange(period, from, to string) bool {
	if !calendarPeriod(period) || !isoDatePattern.MatchString(to) || from > to {
		return false
	}
	date, err := parseDate(from)
	if err != nil || date.Format(isoDate) != from {
		return false
	}
	return rangeFromDate(period, date) == DateRange{From: from, To: to}
}

// CalendarRange is the period containing now on the server calendar. Only the
// server's calendar date matters; elapsed hours never select a day or week.
func CalendarRange(period, timezone string, now time.Time) DateRange {
	local := ResolveServerZone(timezone).In(now)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if period == PeriodYesterday {
		date = date.AddDate(0, 0, -1)
	}
	return rangeFromDate(period, date)
}

// ShiftCalendarRange moves the period containing from by direction (−1 or 1)
// whole days, weeks, months or years.
func ShiftCalendarRange(period, from string, direction int) (DateRange, error) {
	date, err := parseDate(from)
	if err != nil {
		return DateRange{}, err
	}
	start, _ := parseDate(rangeFromDate(period, date).From)
	switch period {
	case PeriodYear:
		start = start.AddDate(direction, 0, 0)
	case PeriodMonth:
		start = start.AddDate(0, direction, 0)
	case PeriodWeek:
		start = start.AddDate(0, 0, direction*daysPerWeek)
	default:
		start = start.AddDate(0, 0, direction)
	}
	return rangeFromDate(period, start), nil
}

// rangeSeparator joins the two ends of a date range label (an en dash).
const rangeSeparator = " – "

// CalendarRangeLabel labels a visible calendar range without the viewer's
// timezone: "2026", "January 2026", "Jan 5 – 11, 2026", "Sep 28 – Oct 4, 2026",
// "Dec 29, 2025 – Jan 4, 2026" or a single day "Jan 5, 2026".
func CalendarRangeLabel(period, from, to string) (string, error) {
	start, err := parseDate(from)
	if err != nil {
		return "", err
	}
	switch period {
	case PeriodYear:
		return strconv.Itoa(start.Year()), nil
	case PeriodMonth:
		return start.Month().String() + " " + strconv.Itoa(start.Year()), nil
	case PeriodWeek:
		end, err := parseDate(to)
		if err != nil {
			return "", err
		}
		return dayRangeLabel(start, end), nil
	}
	return shortDate(start), nil
}

// dayRangeLabel follows Intl en formatRange for year/month/day: shared fields
// are written once.
func dayRangeLabel(start, end time.Time) string {
	switch {
	case start.Equal(end):
		return shortDate(start)
	case start.Year() != end.Year():
		return shortDate(start) + rangeSeparator + shortDate(end)
	case start.Month() != end.Month():
		return monthAbbreviation(start.Month()) + " " + strconv.Itoa(start.Day()) + rangeSeparator + shortDate(end)
	}
	return monthAbbreviation(start.Month()) + " " + strconv.Itoa(start.Day()) + rangeSeparator +
		strconv.Itoa(end.Day()) + ", " + strconv.Itoa(end.Year())
}
