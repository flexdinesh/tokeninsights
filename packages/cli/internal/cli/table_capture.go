package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

const (
	captureHarnessWidth        = 11
	capturePhaseWidth          = 12
	captureCompactHarnessWidth = 8
	captureCompactPhaseWidth   = 7
)

// Capture detail belongs to the command-owned local viewer. Its height depends
// only on terminal geometry, never on attempt state or changing source counts.
func (m interactiveModel) pairedCaptureRows() bool {
	return m.height < deskRoomyHeight && m.tableViewportWidth() >= deskMetricMinWidth
}

func (m interactiveModel) captureRows() []string {
	if m.options.local == nil {
		return nil
	}
	width := m.tableViewportWidth()
	paired := m.pairedCaptureRows()
	cellWidth := width
	if paired {
		cellWidth = (width - tuiColGap) / 2
	}
	rows := make([]string, 0, len(pipeline.SupportedHarnesses))
	for _, harness := range pipeline.SupportedHarnesses {
		rows = append(rows, m.captureRow(harness, cellWidth, paired))
	}
	if !paired {
		return rows
	}
	return []string{
		padCaptureCell(rows[0], cellWidth) + strings.Repeat(" ", tuiColGap) + rows[1],
		padCaptureCell(rows[2], cellWidth) + strings.Repeat(" ", tuiColGap) + rows[3],
	}
}

func padCaptureCell(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func (m interactiveModel) captureRow(harness pipeline.Harness, width int, compact bool) string {
	label := syncHarnessDisplayName(harness)
	labelWidth, stateWidth := captureHarnessWidth, capturePhaseWidth
	if compact {
		labelWidth, stateWidth = captureCompactHarnessWidth, captureCompactPhaseWidth
		if harness == pipeline.HarnessClaudeCode {
			label = "Claude"
		}
	}
	state, style := m.captureState(string(harness))
	coarseState := state
	if compact {
		state = compactCaptureState(state)
	}
	detail, known := m.refresh.capture[string(harness)]
	metrics := "sources unknown"
	if detail.TotalKnown {
		count := fmt.Sprintf("%d/%d", detail.Checked, detail.Total)
		remaining := max(int64(0), detail.Total-detail.Checked)
		countWidth, remainingWidth := len(count), len(fmt.Sprint(remaining))
		for _, capture := range m.refresh.capture {
			if capture.TotalKnown {
				countWidth = max(countWidth, len(fmt.Sprintf("%d/%d", capture.Checked, capture.Total)))
				remainingWidth = max(remainingWidth, len(fmt.Sprint(max(int64(0), capture.Total-capture.Checked))))
			}
		}
		metrics = fmt.Sprintf("%*s sources · %*d left", countWidth, count, remainingWidth, remaining)
		if compact {
			metrics = fmt.Sprintf("%*s %*d left", countWidth, count, remainingWidth, remaining)
		}
	}
	if !known && (coarseState == "Disabled" || coarseState == "Not requested" || coarseState == "Idle") {
		metrics = ""
	}
	if detail.Failed > 0 || detail.Quarantined > 0 {
		var outcomes []string
		if detail.Failed > 0 {
			outcomes = append(outcomes, fmt.Sprintf("%d failed", detail.Failed))
		}
		if detail.Quarantined > 0 {
			outcomes = append(outcomes, fmt.Sprintf("%d quarantined", detail.Quarantined))
		}
		faults := strings.Join(outcomes, " · ")
		metrics += " · " + faults
		// Failure detail takes priority in a constrained cell. Never abbreviate
		// quarantine into a successful completion or silently omit its outcome.
		if compact {
			stateWidth = len(state)
			metrics = fmt.Sprintf("%d/%d · %s", detail.Checked, detail.Total, faults)
			if !detail.TotalKnown || labelWidth+stateWidth+2+ansi.StringWidth(metrics) > width {
				metrics = faults
			}
		} else if labelWidth+stateWidth+2+ansi.StringWidth(metrics) > width {
			stateWidth = len(state)
			metrics = fmt.Sprintf("%d/%d · %s", detail.Checked, detail.Total, faults)
			if !detail.TotalKnown {
				metrics = faults
			}
		}
	}
	text := fmt.Sprintf("%-*s %-*s %s", labelWidth, label, stateWidth, state, metrics)
	return style.Render(truncateCell(strings.TrimRight(text, " "), width))
}

func (m interactiveModel) captureState(harness string) (string, lipgloss.Style) {
	if detail, exists := m.refresh.capture[harness]; exists {
		incomplete := detail.Failed > 0 || detail.Quarantined > 0
		if incomplete && (detail.Phase == collectorprogress.CaptureComplete || detail.Phase == collectorprogress.CaptureFailed) {
			return "Incomplete", syncFailStyle
		}
		switch detail.Phase {
		case collectorprogress.CaptureDiscovering:
			return "Discovering", syncBusyStyle
		case collectorprogress.CaptureWaiting:
			return "Waiting", hintStyle
		case collectorprogress.CaptureReading:
			return "Reading", syncBusyStyle
		case collectorprogress.CaptureSaving:
			return "Saving", syncBusyStyle
		case collectorprogress.CaptureComplete:
			if detail.TotalKnown && detail.Total == 0 {
				return "No sources", hintStyle
			}
			if detail.TotalKnown && detail.Unchanged == detail.Total && detail.Captured == 0 {
				return "Unchanged", syncOKStyle
			}
			return "Complete", syncOKStyle
		case collectorprogress.CaptureFailed:
			return "Failed", syncFailStyle
		case collectorprogress.CaptureInterrupted:
			return "Interrupted", syncFailStyle
		}
	}
	if attempt := m.refresh.attempt; attempt != nil {
		switch attempt.Harnesses[harness] {
		case "waiting":
			return "Waiting", hintStyle
		case "running":
			return "Checking", syncBusyStyle
		case "complete":
			return "Complete", syncOKStyle
		case "failed":
			return "Failed", syncFailStyle
		case "skipped":
			return "Skipped", hintStyle
		default:
			if attempt.AttemptID == m.refresh.startupAttemptID && (attempt.Stage == "waiting" || attempt.Stage == "capturing") {
				return "Waiting", hintStyle
			}
			return "Not requested", hintStyle
		}
	}
	if !m.options.syncOnStart {
		return "Disabled", hintStyle
	}
	if m.refresh.collectionPending {
		return "Waiting", hintStyle
	}
	return "Idle", hintStyle
}

func compactCaptureState(state string) string {
	switch state {
	case "Discovering":
		return "Find"
	case "Waiting":
		return "Wait"
	case "Reading":
		return "Read"
	case "Saving":
		return "Save"
	case "Complete":
		return "Done"
	case "Interrupted":
		return "Stopped"
	case "Not requested":
		return "Skipped"
	case "Incomplete":
		return "!"
	case "No sources":
		return "Empty"
	case "Unchanged":
		return "No new"
	default:
		return state
	}
}

func (m interactiveModel) captureActivity() string {
	// Prefer admitted work over discovery or waiting in another harness.
	for _, phase := range []collectorprogress.CapturePhase{collectorprogress.CaptureReading, collectorprogress.CaptureSaving, collectorprogress.CaptureDiscovering, collectorprogress.CaptureWaiting} {
		for _, capture := range m.refresh.capture {
			if capture.Phase != phase {
				continue
			}
			switch phase {
			case collectorprogress.CaptureReading:
				return "reading local sources"
			case collectorprogress.CaptureSaving:
				return "saving captured sources"
			case collectorprogress.CaptureDiscovering:
				return "discovering local sources"
			case collectorprogress.CaptureWaiting:
				return "waiting to read local sources"
			}
		}
	}
	return "checking local sources"
}
