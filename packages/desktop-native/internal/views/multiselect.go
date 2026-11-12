package views

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Multi-select texts, as the web's MultiSelect.
const (
	allValues      = "All"
	namesJoint     = ", "
	filterSuffix   = " filter"
	clearLabel     = "Clear"
	searchPrefix   = "Search "
	findPrefix     = "Find "
	findSuffix     = "…"
	loadingValues  = "Loading…"
	noValues       = "No matching values"
	retryPrefix    = "Retry "
	retrySuffix    = " search"
	firstMatches   = "First 100 matches. Search to narrow results."
	doneLabel      = "Done"
	labelSeparator = ":"
)

// MultiSelectOptions describe a usage filter of several values.
type MultiSelectOptions struct {
	// Label names the filter, as Harness.
	Label string
	// Values are the values offered; Selected the keys chosen, in order.
	Values   []dashboardview.FilterOption
	Selected []string
	// MissingName names a selected key the values no longer offer; empty
	// shows the key.
	MissingName string
	// Search, when set, is search text the caller owns and the backend
	// searches: the popover then notes that it offers the first matches.
	// Without it the filter keeps its own search of Values.
	Search *string
	// Loading is whether the values are on their way.
	Loading bool
	// Error, when set, says why the values of the search failed, beside a
	// button calling Retry.
	Error string
	Retry func()
	// OnChange applies the keys chosen, as each value is checked or
	// unchecked and as Clear empties them.
	OnChange func(selected []string)
}

// multiSelect is a multi-select's own state, kept by its trigger: the
// popover, its search when the caller owns none, and the options' list.
type multiSelect struct {
	open   bool
	search string
	list   ui.ListState
}

// MultiSelect is the web's MultiSelect: a trigger reading "Label: names"
// or "Label: All", with the count chosen, opening a popover to search
// and check values. Each check applies at once and the popover stays open
// until Done, Escape or a press outside. Options show in a virtualized
// list, ordered and searched as dashboardview.FilterOptions does. Give it
// a key.
func MultiSelect(c *ui.Context, o MultiSelectOptions) ui.Element {
	t := theme.Current(c)
	count := len(o.Selected)
	names := allValues
	name := o.Label
	if count > 0 {
		names = strings.Join(dashboardview.SelectedFilterNames(o.Values, o.Selected, o.MissingName), namesJoint)
		name += " " + strconv.Itoa(count)
	}
	style := kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact, Selected: count > 0}
	b := kit.ButtonContent(c, style, func() {
		ui.Text(c, o.Label+labelSeparator).SingleLine()
		ui.Text(c, names).SingleLine().TextColor(t.InkSubtle).MaxWidth(theme.FilterSelectionWidth).MinWidth(0).Shrink(1)
		if count > 0 {
			kit.Badge(c, strconv.Itoa(count), kit.Secondary)
		}
		kit.Icon(c, icons.ChevronDown())
	}).Gap(theme.Unit).MaxWidthPercent(100)
	b.Label(name).HasPopup(ui.PopupDialog)
	s := ui.Local(b, "multi", func() multiSelect { return multiSelect{} })
	if b.Clicked() {
		s.open = !s.open
	}
	b.Expanded(s.open)
	search := o.Search
	if search == nil {
		search = &s.search
	}
	panel := kit.PopoverWith(c, b, &s.open, kit.PopoverOptions{Start: true}, func() {
		multiSelectPanel(c, o, s, search)
	})
	panel.Label(o.Label + filterSuffix)
	return b
}

// multiSelectPanel is a multi-select's popover: the title and Clear, the
// search, the options, the search failure, the matches note and Done.
func multiSelectPanel(c *ui.Context, o MultiSelectOptions, s *multiSelect, search *string) {
	t := theme.Current(c)
	lower := strings.ToLower(o.Label)
	kit.PopoverHeader(c, o.Label, func() {
		if kit.Button(c.Key("clear"), clearLabel, kit.ButtonStyle{Variant: kit.Link, Size: kit.SizeCompact}).Clicked() {
			o.OnChange([]string{})
		}
	})
	kit.SearchFieldWith(c.Key("search"), search, kit.SearchFieldOptions{
		Placeholder: findPrefix + lower + findSuffix, Label: searchPrefix + lower, AutoFocus: true,
	}).Margin(theme.Unit*2, 0)
	options := dashboardview.FilterOptions(o.Values, o.Selected, o.MissingName, *search)
	s.list.Key = func(i int) any { return options[i].Key }
	list := ui.List(c.Key("options"), &s.list, len(options), func(i int) {
		option := options[i]
		checked := slices.Contains(o.Selected, option.Key)
		if kit.CheckboxOption(c, &checked, option.Name).FillWidth().Changed() {
			o.OnChange(toggled(o.Selected, option.Key, checked))
		}
	})
	list.MaxHeight(theme.FilterOptionsMaxHeight).Margin(0, 0, theme.Unit*2, 0).Busy(o.Loading).Label(o.Label + filterSuffix)
	list.Children(func() {
		if len(options) == 0 && o.Error == "" {
			empty := noValues
			if o.Loading {
				empty = loadingValues
			}
			theme.Label(ui.Text(c, empty)).TextColor(t.InkSubtle)
		}
	})
	if o.Error != "" {
		ui.Column(c).Role(ui.RoleAlert).Children(func() {
			theme.Label(ui.Text(c, o.Error)).TextColor(t.StatusDanger).Margin(0, 0, theme.Unit*2, 0)
			retry := kit.Button(c.Key("retry"), retryPrefix+lower+retrySuffix,
				kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact, Disabled: o.Loading})
			if retry.AlignSelf(ui.Start).Clicked() && o.Retry != nil {
				o.Retry()
			}
		})
	}
	if o.Search != nil {
		// The options' bottom margin already parts the note from them,
		// as the web's collapsing margins do.
		top := float32(0)
		if o.Error != "" {
			top = theme.Unit * 2
		}
		theme.Label(ui.Text(c, firstMatches)).TextColor(t.InkSubtle).Margin(top, 0, theme.Unit*2, 0)
	}
	done := kit.ButtonContent(c.Key("done"), kit.ButtonStyle{}, func() {
		kit.Icon(c, icons.Check())
		ui.Text(c, doneLabel).SingleLine()
	})
	if done.FillWidth().Label(doneLabel).Clicked() {
		s.open = false
	}
}

// toggled is selected with key added at its end when checked, else
// without it, as the web's option toggle.
func toggled(selected []string, key string, checked bool) []string {
	rest := slices.DeleteFunc(slices.Clone(selected), func(v string) bool { return v == key })
	if checked {
		rest = append(rest, key)
	}
	return rest
}
