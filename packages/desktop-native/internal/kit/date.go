package kit

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// DateInputOptions complete a DateInput: what it shows while empty, the
// day Today chooses, and whether its date is invalid.
type DateInputOptions struct {
	// Placeholder shows while the date is empty.
	Placeholder string
	// Today is the day the Today button chooses and the calendar opens on
	// while the date is empty.
	Today time.Time
	// Invalid draws the field edge in danger and marks the field invalid;
	// set it here rather than on the returned element so the look follows
	// in the same frame.
	Invalid bool
}

// Date input labels, as a browser's date picker's buttons.
const (
	dateClear = "Clear"
	dateToday = "Today"
)

// fallbackLocale formats dates while the window names no language, as
// MyGo's date input does.
const fallbackLocale = "en"

// DateInput builds an optional date field on the field surface with the
// strong field edge, as the web's date input: it shows *date as the
// user's regional formats write it in numbers, or o.Placeholder while
// *date is the zero time, the empty date. A click, Enter or Space opens
// MyGo's calendar below it, where choosing a day sets *date (midnight
// UTC, a calendar date) and closes it, with the focus back on the field;
// Clear empties *date and Today chooses o.Today, as a browser's date
// picker. Escape closes the calendar. Give it a key with c.Key and a
// Label; query Changed on the result.
func DateInput(c *ui.Context, date *time.Time, o DateInputOptions) ui.Element {
	t := theme.Current(c)
	b := ui.ButtonBase(c)
	open := ui.Local(b, "open", func() bool { return false })
	// shown is the calendar's day: *date, or o.Today while empty.
	shown := ui.Local(b, "shown", func() time.Time { return time.Time{} })
	if b.Clicked() {
		*open = !*open
		*shown = *date
		if date.IsZero() {
			*shown = o.Today
		}
	}
	text, ink := o.Placeholder, t.InkSubtle
	if !date.IsZero() {
		text, ink = formatDate(c, *date), t.Ink
	}
	edge := t.LineStrong
	if o.Invalid {
		edge = t.StatusDanger
	}
	b.Role(ui.RolePopUpButton).HasPopup(ui.PopupGrid).Expanded(*open).Value(text).Invalid(o.Invalid).
		MinHeight(theme.ControlHeight).Padding(theme.Unit, theme.Unit*3).Gap(theme.Unit*2).Justify(ui.SpaceBetween).
		Background(t.SurfaceField).Border(theme.Rule, edge)
	focusRing(c, b, b.FocusVisible())
	theme.Body(b).Children(func() {
		ui.Text(c, text).SingleLine().MinWidth(0).TextColor(ink).FontFeatures("tnum")
		icon(c, icons.CalendarDays()).TextColor(t.InkSubtle)
	})
	set := func(day time.Time) {
		if !day.Equal(*date) {
			*date = day
			b.SetChanged()
		}
		*open = false
		b.Focus()
	}
	panel := ui.PopoverBase(c, b, open, func(panel ui.Element) {
		overlay(t, panel).Margin(theme.MenuOffset, 0, 0, 0).Padding(theme.Unit * 2).Gap(theme.Unit * 2)
		if ui.Calendar(c, shown).Browse().AutoFocus().Changed() {
			set(calendarDate(*shown))
		}
		ui.Row(c).Justify(ui.SpaceBetween).Children(func() {
			if Button(c.Key("clear"), dateClear, ButtonStyle{Variant: Link, Size: SizeCompact, Disabled: date.IsZero()}).Clicked() {
				set(time.Time{})
			}
			if Button(c.Key("today"), dateToday, ButtonStyle{Variant: Link, Size: SizeCompact}).Clicked() {
				set(calendarDate(o.Today))
			}
		})
	})
	// The calendar takes the focus as it opens: dismissed with it, the
	// calendar gives the focus back to the field.
	if panel.Valid() && panel.Dismissed() && panel.FocusWithin() {
		b.Focus()
	}
	return b
}

// calendarDate is day's calendar date at midnight UTC.
func calendarDate(day time.Time) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// formatDate writes day in numbers in the window's language, as MyGo's
// date input does: 11/5/2026 in English.
func formatDate(c *ui.Context, day time.Time) string {
	locale := c.Locale()
	if locale == "" {
		locale = fallbackLocale
	}
	return i18n.FormatDate(locale, day, i18n.DateOptions{Skeleton: "yMd"})
}
