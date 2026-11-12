package dashboardview

import (
	"slices"
	"time"
)

// PeriodCustom is the date filter's Custom choice: explicit bounds rather
// than a selection period.
const PeriodCustom = "custom"

// Ends of a custom range label when a bound is empty, and their joint, as
// the web's DateFilter writes them.
const (
	customBeginning = "Beginning"
	customNow       = "Now"
	customJoint     = " → "
	// defaultPresetLabel labels a period without a preset, as the web's.
	defaultPresetLabel = "Daily"
)

// CustomDates reports whether q has explicit bounds that are not a
// navigated calendar period: the web's custom date range.
func (q Query) CustomDates() bool {
	return (q.From != "" || q.To != "") && q.DateMode != DateModeCalendar
}

// DateFilterLabel is the date filter's label, as the web's DateFilter:
// "from → to" for custom dates, an empty bound reading Beginning or Now,
// else the period's preset label, Daily for a period without one.
func DateFilterLabel(q Query) string {
	if q.CustomDates() {
		from, to := q.From, q.To
		if from == "" {
			from = customBeginning
		}
		if to == "" {
			to = customNow
		}
		return from + customJoint + to
	}
	options := PeriodOptions()
	if i := slices.IndexFunc(options, func(o PeriodOption) bool { return o.Period == q.Period }); i >= 0 {
		return options[i].Label
	}
	return defaultPresetLabel
}

// DatePreset is the date filter's choice for q: PeriodCustom for custom
// dates, Daily (PeriodToday) for the legacy yesterday, else the period.
func DatePreset(q Query) string {
	if q.CustomDates() {
		return PeriodCustom
	}
	return dailyPeriod(q.Period)
}

// dailyPeriod shows the legacy yesterday as Daily.
func dailyPeriod(period string) string {
	if period == PeriodYesterday {
		return PeriodToday
	}
	return period
}

// PresetChange chooses a preset period, clearing the bounds and the
// calendar mode, as the web's preset choice.
func PresetChange(period string) SelectionChange {
	return boundsChange(&period, "", "", "")
}

// CustomRangeChange applies custom bounds (YYYY-MM-DD, either empty),
// clearing the calendar mode and keeping the period.
func CustomRangeChange(from, to string) SelectionChange {
	return boundsChange(nil, from, to, "")
}

func boundsChange(period *string, from, to string, mode DateMode) SelectionChange {
	return SelectionChange{Period: period, From: &from, To: &to, DateMode: &mode}
}

// CustomRangeOrderInvalid reports whether custom bounds run backwards:
// both set and from after to.
func CustomRangeOrderInvalid(from, to string) bool {
	return from != "" && to != "" && from > to
}

// CustomRangeApplicable reports whether custom bounds can apply: at least
// one is set and they are in order.
func CustomRangeApplicable(from, to string) bool {
	return (from != "" || to != "") && !CustomRangeOrderInvalid(from, to)
}

// Directions of period navigation.
const (
	PreviousPeriod = -1
	NextPeriod     = 1
)

// unitDay names one step of daily navigation.
const unitDay = "day"

// PeriodNavigation is the web's DateNavigation for one query: the
// calendar period shown and the current one on the server calendar.
type PeriodNavigation struct {
	// Period is the query's period.
	Period string
	// Unit names one step: day, week, month or year.
	Unit string
	// Range is the period shown, Current the period containing now.
	Range, Current DateRange
	// Label is Range's CalendarRangeLabel.
	Label string
}

// NavigatePeriod is the period navigation of q on the server calendar of
// timezone at now. ok is false where the web hides it: custom dates, All
// time, and bounds that are not dates.
func NavigatePeriod(q Query, timezone string, now time.Time) (PeriodNavigation, bool) {
	if q.CustomDates() || q.Period == PeriodAll {
		return PeriodNavigation{}, false
	}
	n := PeriodNavigation{
		Period:  q.Period,
		Unit:    q.Period,
		Range:   CalendarRange(q.Period, timezone, now),
		Current: CalendarRange(dailyPeriod(q.Period), timezone, now),
	}
	if dailyPeriod(q.Period) == PeriodToday {
		n.Unit = unitDay
	}
	if q.From != "" && q.To != "" {
		n.Range = DateRange{From: q.From, To: q.To}
	}
	label, err := CalendarRangeLabel(q.Period, n.Range.From, n.Range.To)
	if err != nil {
		return PeriodNavigation{}, false
	}
	n.Label = label
	return n, true
}

// AtCurrent reports whether the period shown is the current one or later,
// where the web disables Next.
func (n PeriodNavigation) AtCurrent() bool { return n.Range.From >= n.Current.From }

// Shift is the selection change moving the period shown one step in
// direction (PreviousPeriod or NextPeriod), as the web's move: reaching
// the current period clears the bounds, any other is a calendar period;
// yesterday navigates as Daily.
func (n PeriodNavigation) Shift(direction int) (SelectionChange, error) {
	next, err := ShiftCalendarRange(n.Period, n.Range.From, direction)
	if err != nil {
		return SelectionChange{}, err
	}
	period := dailyPeriod(n.Period)
	if next.From == n.Current.From {
		return boundsChange(&period, "", "", ""), nil
	}
	return boundsChange(&period, next.From, next.To, DateModeCalendar), nil
}
