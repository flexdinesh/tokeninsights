package cli

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

// Collection, processing, and the displayed publication are separate contracts.
// Neither an accepted attempt nor a drained queue alone means the UI is current.
type tuiRefresh struct {
	requested, collectionPending bool
	result                       *localruntime.CollectionResult
	startupAttemptID             string
	attempt                      *collectorprogress.Attempt
	capture                      map[string]collectorprogress.Capture
	statusKnown                  bool
	statusErr                    error
	status                       analytics.ProcessingStatus
	loaded                       bool
	displayedRevision            int64
	inputRevision                int64
	serverGeneration             int64
	token, displayedToken        string
	queryErr                     error
}

type collectionFinishedMsg struct{ result localruntime.CollectionResult }
type refreshAnimationTickMsg struct{}

func (m interactiveModel) startCollection() interactiveModel {
	m.refresh.requested, m.refresh.collectionPending = true, true
	m.refreshAnimating = true
	m.collectionDone = m.options.local.StartCollection(m.ctx, pipeline.SyncOptions{
		Harnesses: pipeline.SupportedHarnesses, Now: m.now,
	}, runViewCollector)
	if attempt := currentCollection(m.options.local.Progress.Snapshot()); attempt != nil {
		m.refresh.startupAttemptID = attempt.AttemptID
	}
	return m
}

func refreshAnimationCmd() tea.Cmd {
	return tea.Tick(syncAnimationInterval, func(time.Time) tea.Msg { return refreshAnimationTickMsg{} })
}

// An active attempt takes precedence; otherwise show the most recent attempt.
// Updated time breaks ties without letting an older heartbeat obscure new work.
func currentCollection(snapshot collectorprogress.Snapshot) *collectorprogress.Attempt {
	var latest, active *collectorprogress.Attempt
	for _, attempt := range snapshot.Attempts {
		if latest == nil || newerCollection(attempt, *latest) {
			copy := attempt
			latest = &copy
		}
		if attempt.Stage == "waiting" || attempt.Stage == "capturing" || attempt.Stage == "submitting" {
			if active == nil || newerCollection(attempt, *active) {
				copy := attempt
				active = &copy
			}
		}
	}
	if active != nil {
		return active
	}
	return latest
}

func newerCollection(a, b collectorprogress.Attempt) bool {
	if a.StartedAtMS != b.StartedAtMS {
		return a.StartedAtMS > b.StartedAtMS
	}
	if a.UpdatedAtMS != b.UpdatedAtMS {
		return a.UpdatedAtMS > b.UpdatedAtMS
	}
	return a.AttemptID > b.AttemptID
}

func (m interactiveModel) observeRefresh(msg sharedSyncMsg) interactiveModel {
	m.refresh.statusErr = nil
	if msg.collection != nil {
		if attempt := currentCollection(*msg.collection); attempt != nil {
			m.refresh.attempt = attempt
			m.refresh.capture = msg.capture[attempt.AttemptID]
		}
		if attempt := m.refresh.attempt; attempt != nil && attempt.Stage == "accepted" {
			m.refresh.token = fmt.Sprintf("%s/%d", attempt.AttemptID, attempt.FinishedAtMS)
		}
	}
	if msg.refreshStatus != nil {
		m.refresh.statusKnown = true
		m.refresh.status = *msg.refreshStatus
	}
	return m
}

func (m interactiveModel) refreshNeedsRead() bool {
	return m.refresh.statusKnown && (m.refresh.displayedRevision < m.refresh.status.Metadata.Revision ||
		m.refresh.displayedToken != m.refresh.token ||
		m.refresh.serverGeneration != m.refresh.status.Metadata.Generation ||
		m.refresh.inputRevision < m.refresh.status.Metadata.InputRevision)
}

func (m interactiveModel) collectionActive() bool {
	if m.refresh.collectionPending {
		return true
	}
	attempt := m.refresh.attempt
	return attempt != nil && (attempt.Stage == "waiting" || attempt.Stage == "capturing" || attempt.Stage == "submitting")
}

func (m interactiveModel) processingFailed() bool {
	return m.refresh.statusKnown && m.refresh.status.Failed > 0 && m.refresh.status.FailedRetryAtMs > time.Now().UnixMilli()
}

func (m interactiveModel) collectionFailed() bool {
	if attempt := m.refresh.attempt; attempt != nil {
		if attempt.AttemptID == m.refresh.startupAttemptID && m.refresh.result != nil && m.refresh.result.Result.Collection.Quarantined > 0 {
			return true
		}
		return attempt.Stage == "failed" || attempt.Stage == "interrupted"
	}
	return m.refresh.result != nil && (m.refresh.result.Err != nil || m.refresh.result.Result.Collection.Quarantined > 0)
}

func (m interactiveModel) refreshBusy() bool {
	if m.refresh.statusErr != nil || m.refresh.queryErr != nil || m.processingFailed() || (m.collectionFailed() && !m.collectionActive()) {
		return false
	}
	return m.collectionActive() || m.pendingRefresh || m.sharedSync.Running || m.refreshNeedsRead() ||
		(m.refresh.requested && m.refresh.result != nil && !m.refresh.statusKnown)
}

func (m interactiveModel) refreshLine() string {
	width := m.tableViewportWidth()
	text, compact := "Saved usage · r Reload", "Saved usage"
	busy, failed, refreshed := false, false, false
	switch {
	case m.refresh.queryErr != nil:
		text, compact, failed = "Display update failed · showing saved usage · r Retry", "Update failed · r Retry", true
	case m.refresh.statusErr != nil:
		text, compact, failed = "Refresh status unavailable · showing saved usage", "Refresh status unavailable", true
	case m.processingFailed():
		text, compact, failed = "Processing needs attention · showing saved usage · r Reload", "Processing needs attention", true
	case m.collectionActive():
		stage := "checking local sessions"
		if m.options.local != nil {
			stage = m.captureActivity()
		}
		if attempt := m.refresh.attempt; attempt != nil {
			switch attempt.Stage {
			case "waiting":
				stage = "waiting for another sync"
			case "submitting":
				stage = "submitting collected data"
			}
		}
		text, compact, busy = "Refreshing usage · "+stage+" · updates automatically", "Refreshing · auto-updating", true
		if m.options.local != nil {
			compact = "Refreshing sources · auto-updating"
		}
		if attempt := m.refresh.attempt; m.options.local != nil && attempt != nil && attempt.Stage == "submitting" && attempt.PendingKnown {
			text = fmt.Sprintf("Submitting · %d entries accepted · %d pending", attempt.AcknowledgedEntries, attempt.Pending)
			compact = fmt.Sprintf("Submitting · %d accepted · %d pending", attempt.AcknowledgedEntries, attempt.Pending)
		}
	case m.collectionFailed():
		text, compact, failed = "Refresh incomplete · showing saved usage · run tokeninsights sync", "Refresh incomplete · saved usage", true
		if m.refresh.result != nil && m.refresh.result.Result.Collection.Quarantined > 0 {
			text = "Refresh incomplete · retry: tokeninsights sync --full-refresh"
		}
	case m.pendingRefresh || m.sharedSync.Running:
		text, compact, busy = "Refreshing usage · processing collected data · updates automatically", "Processing · auto-updating", true
		if m.options.local != nil && m.refresh.statusKnown {
			text = fmt.Sprintf("Refreshing usage · processing · %d groups pending", m.refresh.status.Pending)
			compact = fmt.Sprintf("Processing · %d groups pending", m.refresh.status.Pending)
		}
	case m.refreshNeedsRead() || (m.refresh.requested && m.refresh.result != nil && !m.refresh.statusKnown) || (m.reloadInFlight && m.refresh.loaded):
		text, compact, busy = "Updating displayed usage…", "Updating usage…", true
	case m.refresh.statusKnown && m.refresh.loaded && m.refresh.token != "":
		text, compact = "Usage refreshed", "Usage refreshed"
		refreshed = true
		if m.refresh.result != nil && m.refresh.attempt != nil && m.refresh.attempt.AttemptID == m.refresh.startupAttemptID && m.refresh.result.Result.Accepted == 0 && m.refresh.result.Result.Collection.RawFacts == 0 {
			text, compact = "Usage checked · no new usage", "No new usage"
			refreshed = false
		}
	case m.options.local != nil && !m.options.syncOnStart:
		text, compact = "Saved usage · startup collection off · r Reload", "Saved usage · collection off"
	}
	style := hintStyle
	if busy {
		prefix := syncSpinnerFrame(m.syncFrame) + " "
		text, compact = prefix+text, prefix+compact
		style = syncBusyStyle
	} else if failed {
		text, compact = "! "+text, "! "+compact
		style = syncFailStyle
	} else if refreshed {
		style = syncOKStyle
	}
	if ansi.StringWidth(text) > width {
		text = compact
	}
	return style.Render(truncateCell(text, width))
}

func (m interactiveModel) focusedRowKey() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rowFocusKey(m.rows[m.cursor])
}

func (m interactiveModel) rowFocusKey(row renderRow) string {
	switch m.activeTab {
	case tabModels:
		return row.model
	case tabProviders:
		return row.provider
	case tabHarnesses:
		return row.harness
	case tabSessions:
		return row.sessionID
	default:
		return row.bucket
	}
}

func (m interactiveModel) restoreRowFocus(key string) interactiveModel {
	if key != "" {
		for index, row := range m.rows {
			if m.rowFocusKey(row) == key {
				m.cursor = index
				break
			}
		}
	}
	return m
}
