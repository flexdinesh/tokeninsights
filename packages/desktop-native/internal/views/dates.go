package views

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Date filter texts, as the web's DateFilter and DateNavigation.
const (
	dateRangeTitle   = "Date range"
	dateRangePrefix  = "Date range: "
	datePresetsLabel = "Date presets"
	customLabel      = "Custom"
	fromLabel        = "From"
	fromDateLabel    = "From date"
	toLabel          = "To"
	toDateLabel      = "To date"
	customHint       = "Inclusive dates in the server’s local timezone. Either bound can be empty."
	rangeOrderError  = "From must not be after to."
	applyRangeLabel  = "Apply range"
	// fromEmpty and toEmpty show in empty bounds, as the date filter's
	// label reads them.
	fromEmpty = "Beginning"
	toEmpty   = "Now"

	periodNavigationLabel = "Period navigation"
	previousPrefix        = "Previous "
	nextPrefix            = "Next "
)

// datePresetColumns lays the date presets out in two columns, as the web's.
const datePresetColumns = 2

// dateFilter is the date filter's state, kept by its trigger: the
// popover, and the custom bounds being edited, which the query's custom
// bounds start from as it opens.
type dateFilter struct {
	open, custom bool
	from, to     time.Time
}

// reset starts editing from query: its custom bounds, or none.
func (s *dateFilter) reset(query dashboardview.Query) {
	s.custom = query.CustomDates()
	s.from, s.to = time.Time{}, time.Time{}
	if s.custom {
		s.from, s.to = parseCalendarDate(query.From), parseCalendarDate(query.To)
	}
}

// datePresets are the date filter's choices: the periods, then Custom.
func datePresets() []kit.Option[string] {
	periods := dashboardview.PeriodOptions()
	options := make([]kit.Option[string], 0, len(periods)+1)
	for _, p := range periods {
		options = append(options, kit.Option[string]{Value: p.Period, Label: p.Label})
	}
	return append(options, kit.Option[string]{Value: dashboardview.PeriodCustom, Label: customLabel})
}

// DateFilter is the web's DateFilter: a trigger showing the period or the
// custom range, opening a popover of presets, which apply at once, and
// Custom, whose two date fields apply with Apply range. Bounds are server
// calendar dates. Give it a key.
func DateFilter(c *ui.Context, w *model.Window) ui.Element {
	query, _ := w.Query()
	label := dashboardview.DateFilterLabel(query)
	b := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact}, func() {
		kit.Icon(c, icons.CalendarDays())
		ui.Text(c, label).SingleLine()
		kit.Icon(c, icons.ChevronDown())
	})
	b.Label(dateRangePrefix + label).HasPopup(ui.PopupDialog)
	s := ui.Local(b, "date", func() dateFilter { return dateFilter{} })
	if b.Clicked() {
		s.open = !s.open
		if s.open {
			s.reset(query)
		}
	}
	b.Expanded(s.open)
	panel := kit.PopoverWith(c, b, &s.open, kit.PopoverOptions{End: true}, func() {
		dateRangePanel(c, w, s, query)
	})
	panel.Label(dateRangeTitle)
	return b
}

// dateRangePanel is the date filter's popover: the presets, then the
// custom bounds while Custom is chosen.
func dateRangePanel(c *ui.Context, w *model.Window, s *dateFilter, query dashboardview.Query) {
	kit.PopoverHeader(c, dateRangeTitle, nil)
	choice := dashboardview.DatePreset(query)
	if s.custom {
		choice = dashboardview.PeriodCustom
	}
	presets := kit.Segmented(c.Key("presets"), datePresetsLabel, &choice, datePresets(), kit.ButtonStyle{Variant: kit.Outline})
	presets.Grid().Columns(datePresetColumns).Gap(theme.Unit*2).Margin(theme.Unit*2, 0, theme.Unit*3, 0)
	if presets.Changed() {
		if choice == dashboardview.PeriodCustom {
			s.custom = true
		} else {
			w.UpdateSelection(dashboardview.PresetChange(choice))
			s.open = false
		}
	}
	if s.custom {
		customRange(c, w, s)
	}
}

// customRange edits the custom bounds: From and To beside each other, the
// order error, and Apply range, which applies at least one bound in order.
func customRange(c *ui.Context, w *model.Window, s *dateFilter) {
	t := theme.Current(c)
	from, to := formatCalendarDate(s.from), formatCalendarDate(s.to)
	invalid := dashboardview.CustomRangeOrderInvalid(from, to)
	today := parseCalendarDate(w.ServerToday())
	ui.Grid(c).Columns(datePresetColumns).Gap(theme.Unit * 3).Children(func() {
		dateField(c.Key("from"), fromLabel, fromDateLabel, &s.from, kit.DateInputOptions{Placeholder: fromEmpty, Today: today, Invalid: invalid})
		dateField(c.Key("to"), toLabel, toDateLabel, &s.to, kit.DateInputOptions{Placeholder: toEmpty, Today: today, Invalid: invalid})
	})
	theme.Label(ui.Text(c, customHint)).TextColor(t.InkSubtle).Margin(theme.Unit*2, 0)
	if invalid {
		theme.Label(ui.Text(c, rangeOrderError)).TextColor(t.StatusDanger).Role(ui.RoleAlert).Margin(0, 0, theme.Unit*2, 0)
	}
	apply := kit.Button(c.Key("apply"), applyRangeLabel, kit.ButtonStyle{Disabled: !dashboardview.CustomRangeApplicable(from, to)})
	if apply.FillWidth().Clicked() {
		w.UpdateSelection(dashboardview.CustomRangeChange(from, to))
		s.open = false
	}
}

// dateField is a labelled custom bound.
func dateField(c *ui.Context, label, name string, date *time.Time, o kit.DateInputOptions) {
	kit.Field(c, label, func() {
		kit.DateInput(c, date, o).Label(name).FillWidth()
	}).MinWidth(0)
}

// parseCalendarDate reads a YYYY-MM-DD bound as midnight UTC, or the zero
// time, the empty bound, for "" or a value that is not a date.
func parseCalendarDate(value string) time.Time {
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}
	}
	return day
}

// formatCalendarDate writes a bound as YYYY-MM-DD, or "" while empty.
func formatCalendarDate(day time.Time) string {
	if day.IsZero() {
		return ""
	}
	return day.Format(time.DateOnly)
}

// DateNavigation is the web's DateNavigation: previous and next arrows
// around the label of the calendar period shown, Next disabled at the
// current period. It hides for All time and custom dates. Give it a key.
func DateNavigation(c *ui.Context, w *model.Window) ui.Element {
	n, ok := w.PeriodNavigation()
	if !ok {
		return ui.Element{}
	}
	nav := ui.Row(c).AlignItems(ui.Center).Gap(theme.Unit * 2).MarginStart(ui.Auto).MinWidth(0).
		Role(ui.RoleNavigation).Label(periodNavigationLabel)
	return nav.Children(func() {
		style := kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact}
		if kit.IconButton(c.Key("previous"), icons.ChevronLeft(), previousPrefix+n.Unit, style).Clicked() {
			w.ShiftPeriod(dashboardview.PreviousPeriod)
		}
		theme.Label(ui.Text(c, n.Label)).TextAlign(ui.Center).FontFeatures("tnum").Role(ui.RoleStatus)
		style.Disabled = n.AtCurrent()
		if kit.IconButton(c.Key("next"), icons.ChevronRight(), nextPrefix+n.Unit, style).Clicked() {
			w.ShiftPeriod(dashboardview.NextPeriod)
		}
	})
}
