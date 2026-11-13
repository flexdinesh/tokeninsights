package views

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Collector progress texts, as the web's CollectorProgress.
const (
	latestCollection  = "Latest collection"
	progressLabel     = "Collector progress"
	harnessListLabel  = "Collection by harness"
	harnessListTip    = "This run only; saved usage remains included."
	harnessSkippedTip = "Sources unchanged or none found; saved usage included."
)

// CollectorProgress builds the collection attempts of progress, the web's
// CollectorProgress: each active attempt, then the latest finished one
// under "Latest collection", each with its stage and counts above its
// harness states. It builds nothing without attempts to show.
func CollectorProgress(c *ui.Context, progress collectorprogress.Snapshot) {
	visible := dashboardview.VisibleAttempts(progress.Attempts)
	if len(visible) == 0 {
		return
	}
	t := theme.Current(c)
	section := ui.Column(c).Gap(theme.Unit*3).Margin(theme.Unit, 0, 0, 0).TextColor(t.InkSubtle).
		Role(ui.RoleGroup).Label(progressLabel)
	section.Children(func() {
		for _, attempt := range visible {
			ui.Column(c.Key(attempt.AttemptID)).Gap(theme.Unit).Children(func() {
				if !dashboardview.ActiveStage(attempt.Stage) {
					theme.Sub(ui.Text(c, latestCollection)).TextColor(t.Ink).Role(ui.RoleHeading).Level(3)
				}
				theme.Label(ui.Text(c, AttemptLine(attempt)))
				harnessStates(c, attempt.Harnesses)
			})
		}
	})
}

// AttemptLine is an attempt's stage with its accepted and, when known,
// remaining entries, e.g. "Collecting usage · 1,204 accepted · 36 remaining".
func AttemptLine(attempt collectorprogress.Attempt) string {
	line := dashboardview.AttemptLabel(attempt) + " · " + dashboardview.ExactCount(attempt.AcknowledgedEntries) + " accepted"
	if attempt.PendingKnown && attempt.Pending > 0 {
		line += " · " + dashboardview.ExactCount(attempt.Pending) + " remaining"
	}
	return line
}

// harnessStates lists each harness's state in this attempt, by harness
// name as the web's JSON order.
func harnessStates(c *ui.Context, harnesses map[string]string) {
	names := make([]string, 0, len(harnesses))
	for name := range harnesses {
		names = append(names, name)
	}
	slices.Sort(names)
	list := ui.Row(c).Wrap().GapX(theme.Unit * 4).GapY(theme.Unit * 2).Role(ui.RoleList).Label(harnessListLabel)
	list.Children(func() {
		for _, name := range names {
			state := harnesses[name]
			item := theme.Label(ui.Text(c.Key(name), name+": "+dashboardview.HarnessStateLabel(state))).Role(ui.RoleListItem)
			if state == dashboardview.HarnessSkipped {
				kit.Tooltip(c, item, harnessSkippedTip)
			}
		}
	})
	kit.Tooltip(c, list, harnessListTip)
}
