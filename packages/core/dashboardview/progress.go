package dashboardview

import "github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"

// Collector attempt stages (collectorprogress.Attempt.Stage).
const (
	StageWaiting     = "waiting"
	StageCapturing   = "capturing"
	StageSubmitting  = "submitting"
	StageAccepted    = "accepted"
	StageFailed      = "failed"
	StageInterrupted = "interrupted"
)

// submissionFailed is the attempt error code that overrides its stage label.
const submissionFailed = "submission_failed"

var stageLabels = map[string]string{
	StageWaiting: "Waiting to collect", StageCapturing: "Collecting usage", StageSubmitting: "Submitting usage",
	StageAccepted: "Usage accepted", StageFailed: "Collection failed", StageInterrupted: "Collection interrupted",
}

var harnessStateLabels = map[string]string{
	"waiting": "Waiting", "running": "Collecting", "complete": "Collected",
	"failed": "Collection failed", "skipped": "No new usage",
}

// CollectorStageLabel labels an attempt stage, e.g. "Collecting usage"; unknown stages are empty.
func CollectorStageLabel(stage string) string { return stageLabels[stage] }

// HarnessStateLabel labels one harness within an attempt, e.g. "No new usage".
func HarnessStateLabel(state string) string { return harnessStateLabels[state] }

// AttemptLabel labels an attempt, naming submission failures explicitly.
func AttemptLabel(attempt collectorprogress.Attempt) string {
	if attempt.ErrorCode == submissionFailed {
		return "Submission failed"
	}
	return CollectorStageLabel(attempt.Stage)
}

// ActiveStage reports whether a stage is still collecting or submitting.
func ActiveStage(stage string) bool {
	return stage == StageWaiting || stage == StageCapturing || stage == StageSubmitting
}

// VisibleAttempts are the attempts the progress panel lists: every active
// attempt, then the most recently updated terminal one.
func VisibleAttempts(attempts []collectorprogress.Attempt) []collectorprogress.Attempt {
	visible := []collectorprogress.Attempt{}
	var terminal *collectorprogress.Attempt
	for index := range attempts {
		attempt := &attempts[index]
		if ActiveStage(attempt.Stage) {
			visible = append(visible, *attempt)
		} else if terminal == nil || attempt.UpdatedAtMS >= terminal.UpdatedAtMS {
			terminal = attempt
		}
	}
	if terminal != nil {
		visible = append(visible, *terminal)
	}
	return visible
}
