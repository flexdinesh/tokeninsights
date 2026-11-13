package views

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Results texts, as the web's DashboardResults.
const (
	resultsLabel         = "Usage results"
	excludedResultsLabel = "Excluded usage results"

	rebuildingUsage   = "Rebuilding saved usage. Existing totals remain available."
	processingPrefix  = "Processing usage · "
	processingSuffix  = " pending."
	retryScheduled    = "Some usage needs a processing retry. Retry scheduled at "
	retryingUsage     = "Retrying usage processing."
	savedTotals       = " Saved totals remain available."
	unresolvedSuffix  = " observations need more evidence."
	retryRequest      = "Retry Request"
	requestFailed     = "Request failed"
	dataChanged       = "Data changed. Refreshing service status."
	refreshFailed     = "Refresh failed. Saved usage remains available. Try Refresh again."
	collectionUnknown = "Collection status couldn’t load. Saved usage remains available."
	collectionFailed  = "Saved usage remains available. Use Refresh to retry collection."

	noExcludedUsage = "No excluded usage saved."
	noIncludedUsage = "No usage included in your total yet."
	noUsage         = "No usage saved yet. Use Refresh to collect usage."

	loadingDashboard = "Loading dashboard"
	updatingResults  = "Updating view results"
)

// DashboardResults builds the results of the window's query, as the web's
// DashboardResults: processing and read-failure notes, the loading
// skeleton or empty-state message, then the summary readouts above the
// chart and table.
func DashboardResults(c *ui.Context, w *model.Window) ui.Element {
	query, _ := w.Query()
	estimated := query.Quality == dashboardview.QualityEstimated
	label := resultsLabel
	if estimated {
		label = excludedResultsLabel
	}
	region := ui.Column(c).MinWidth(0).Role(ui.RoleRegion).Label(label)
	table := ui.Local(region, "table", newTableState)
	return region.Children(func() {
		results(c, w, estimated, table)
	})
}

func results(c *ui.Context, w *model.Window, estimated bool, table *tableState) {
	sync := w.SyncState()
	status, _ := w.Status()
	usage, hasData := w.Usage()
	usageErr := w.UsageError()
	enabled := sync.AnalyticsEnabled
	if sync.ProcessingPending {
		note(c, processingNote(status), ui.RoleStatus)
	}
	if status.Failed > 0 {
		instance, _ := w.Instance()
		note(c, failedNote(status, sync.ProcessingBackoff, instance.Timezone), ui.RoleStatus)
	}
	if hasData && usage.Unresolved > 0 {
		note(c, fmt.Sprint(usage.Unresolved)+unresolvedSuffix, ui.RoleNone)
	}
	if err := w.ConnectionError(); err != nil {
		ErrorBanner(c.Key("connection"), errorMessage(err), w.RetryConnection)
	}
	if enabled && usageErr != nil {
		ErrorBanner(c.Key("usage"), errorMessage(usageErr), w.RetryUsage)
	}
	// The filter values' banner joins here with the filters.
	if w.RefreshError() != nil {
		note(c, refreshFailed, ui.RoleAlert)
	}
	if sync.CollectionUnknown {
		ErrorBanner(c.Key("collection"), collectionUnknown, w.RetryProgress)
	}
	if sync.CollectionFailed {
		note(c, collectionFailed, ui.RoleAlert)
	}
	waitingForFirstData := sync.Loading && hasData && usage.Summary.SyncedSessions == 0
	if enabled && (!hasData || waitingForFirstData) && usageErr == nil {
		DashboardSkeleton(c)
	}
	if enabled && hasData && usage.Summary.SyncedSessions == 0 && usageErr == nil && !sync.Loading &&
		!sync.CollectionFailed && !sync.CollectionUnknown && status.Failed == 0 {
		// Excluded usage, read with review mode, refines this message.
		note(c, emptyMessage(estimated, false), ui.RoleStatus)
	}
	if enabled && hasData && !waitingForFirstData {
		ui.Column(c).Busy(w.UsageFetching()).Children(func() {
			SummaryCards(c, usage.Summary, estimated)
			placeholder, matches := w.UsagePlaceholder(), w.UsageMatchesView()
			switch {
			case placeholder && !matches && usageErr == nil:
				RouteResultsSkeleton(c)
			case !placeholder || matches:
				usageChart(c, w)
			}
			if !placeholder || matches {
				resultsTable(c, w, table)
			}
		})
	}
}

// usageChart is where the usage chart goes, under the readouts; it shows
// the chart once its view arrives.
func usageChart(*ui.Context, *model.Window) {}

// processingNote says that published usage trails accepted input: a
// rebuild for a new generation, else the entries pending.
func processingNote(status application.Status) string {
	if status.Metadata.Generation != status.Metadata.TargetGeneration {
		return rebuildingUsage
	}
	return processingPrefix + dashboardview.ExactCount(status.Pending) + processingSuffix
}

// failedNote says that failed processing retries, at its scheduled time
// in the server's zone when it waits.
func failedNote(status application.Status, backoff bool, timezone string) string {
	if backoff {
		return retryScheduled + dashboardview.FormatServerDateTime(status.FailedRetryAtMs, timezone) + "." + savedTotals
	}
	return retryingUsage + savedTotals
}

// emptyMessage says why there is no usage: none excluded while reviewing
// it, none included when excluded usage exists, else none saved.
func emptyMessage(estimated, excludedExists bool) string {
	switch {
	case estimated:
		return noExcludedUsage
	case excludedExists:
		return noIncludedUsage
	}
	return noUsage
}

// errorMessage is what a banner says for a failed read.
func errorMessage(err error) string {
	switch {
	case errors.Is(err, model.ErrDataChanged):
		return dataChanged
	case err.Error() == "":
		return requestFailed
	}
	return err.Error()
}

// note is one line of the results' plain body text, as the web's
// paragraphs, read out as role says.
func note(c *ui.Context, text string, role ui.Role) {
	line := theme.Body(ui.Text(c, text))
	if role != ui.RoleNone {
		line.Role(role)
	}
}

// ErrorBanner is a destructive alert saying message, with a Retry Request
// button calling retry, as the web's. Give it a key.
func ErrorBanner(c *ui.Context, message string, retry func()) ui.Element {
	alert := kit.Alert(c, kit.AlertContent{Destructive: true, Icon: icons.CircleAlert(), Description: message}, func() {
		if kit.Button(c, retryRequest, kit.ButtonStyle{Variant: kit.Outline}).Clicked() {
			retry()
		}
	})
	return alert.Margin(theme.Unit*3, 0)
}

// DashboardSkeleton holds the readouts' and the chart's places while the
// first usage loads. The window's minimum width is wider than the web's
// compact breakpoint, so its readouts stay in one row.
func DashboardSkeleton(c *ui.Context) ui.Element {
	box := ui.Column(c).Role(ui.RoleStatus).Label(loadingDashboard)
	return box.Children(func() {
		ui.Row(c).Gap(theme.Unit*4).PaddingY(theme.Unit*2).Margin(0, 0, theme.Unit*2, 0).Children(func() {
			for i := range summaryReadoutCount {
				share := float32(1)
				if i == 0 {
					share = theme.ReadoutPrimaryShare
				}
				kit.Skeleton(c.Key(i)).Height(theme.SkeletonReadout).Basis(0).Grow(share)
			}
		})
		kit.Skeleton(c).Height(theme.SkeletonChart)
	})
}

// RouteResultsSkeleton holds the chart's and the table's places while a
// tab switch reads the new view.
func RouteResultsSkeleton(c *ui.Context) ui.Element {
	box := ui.Column(c).Role(ui.RoleStatus).Label(updatingResults)
	return box.Children(func() {
		kit.Skeleton(c).Height(theme.SkeletonChart)
		kit.Skeleton(c).Height(theme.SkeletonTable).Margin(theme.Unit*2, 0, 0, 0)
	})
}
