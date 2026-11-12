package dashboardview

import (
	"strconv"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
)

// readyReadiness is the readiness that permits analytics reads.
const readyReadiness = "ready"

// SnapshotIdentity names one published dataset snapshot. Reads whose identity
// differs from the latest status are stale; an identity change drops cached
// usage and facets, while a revision change only refetches them.
//
// The web omits "/generation" when a response omits generation; Go values
// always carry it, and current backends always send it.
func SnapshotIdentity(instanceID, databaseID, datasetID string, generation int64) string {
	return instanceID + "/" + databaseID + "/" + datasetID + "/" + strconv.FormatInt(generation, decimalBase)
}

// StatusIdentity is the snapshot identity a status observation reports.
func StatusIdentity(status application.Status) string {
	return SnapshotIdentity(status.InstanceID, status.Metadata.DatabaseID, status.Metadata.DatasetID, status.Metadata.Generation)
}

// UsageIdentity is the snapshot identity a usage read answered from.
func UsageIdentity(usage application.Usage) string {
	return SnapshotIdentity(usage.InstanceID, usage.DatabaseID, usage.DatasetID, usage.Generation)
}

// FacetsIdentity is the snapshot identity a facets read answered from.
func FacetsIdentity(facets application.Facets) string {
	return SnapshotIdentity(facets.InstanceID, facets.DatabaseID, facets.DatasetID, facets.Generation)
}

// SyncInput is one observation of backend status and collector progress.
type SyncInput struct {
	// Status is the latest successful status read, nil before the first.
	Status *application.Status
	// StatusReadAtMs is when Status was read, in Unix milliseconds.
	StatusReadAtMs int64
	// DatasetID is the dataset the window's instance read described.
	DatasetID string
	// ProgressEnabled is true when the instance shows collector progress.
	ProgressEnabled bool
	// Attempts is the latest successful progress read, kept after a failure.
	Attempts []collectorprogress.Attempt
	// ProgressPending is true until the first progress read completes.
	ProgressPending bool
	// ProgressFailed is true while the latest progress read failed.
	ProgressFailed bool
	// CheckedAcceptance is the AcceptedTransition whose status refetch completed.
	CheckedAcceptance string
}

// SyncState is the derived synchronization state of one window.
type SyncState struct {
	// CollectionStage is the first active attempt's stage, empty when none or unknown.
	CollectionStage string
	// CollectionPending covers loading progress, an active attempt, or a status
	// refetch after acceptance.
	CollectionPending bool
	CollectionUnknown bool
	CollectionFailed  bool
	// ProcessingPending means published data trails accepted input.
	ProcessingPending bool
	// ProcessingBackoff means failed processing waits for a later retry.
	ProcessingBackoff bool
	Loading           bool
	Revision          int64
	Identity          string
	// AnalyticsEnabled permits usage and facets reads of the window's dataset.
	AnalyticsEnabled bool
	// CheckingAcceptance asks the owner to refetch status, then record
	// AcceptedTransition as checked.
	CheckingAcceptance bool
}

// AcceptedTransition identifies the newest attempt when it was accepted, or is
// empty. A new transition requires one status refetch before collection
// counts as finished.
func AcceptedTransition(attempts []collectorprogress.Attempt) string {
	latest := latestAttempt(attempts)
	if latest == nil || latest.Stage != StageAccepted {
		return ""
	}
	return latest.AttemptID + "/" + strconv.FormatInt(latest.UpdatedAtMS, decimalBase)
}

// latestAttempt is the most recently updated attempt; ties favor the later entry.
func latestAttempt(attempts []collectorprogress.Attempt) *collectorprogress.Attempt {
	var latest *collectorprogress.Attempt
	for index := range attempts {
		if latest == nil || attempts[index].UpdatedAtMS >= latest.UpdatedAtMS {
			latest = &attempts[index]
		}
	}
	return latest
}

// DeriveSync derives a window's synchronization state like the web's useDashboardSync.
func DeriveSync(input SyncInput) SyncState {
	attempts := input.Attempts
	if !input.ProgressEnabled {
		attempts = nil
	}
	var active *collectorprogress.Attempt
	for index := range attempts {
		if ActiveStage(attempts[index].Stage) {
			active = &attempts[index]
			break
		}
	}
	transition := AcceptedTransition(attempts)
	state := SyncState{
		CollectionUnknown:  input.ProgressEnabled && input.ProgressFailed,
		CheckingAcceptance: transition != "" && transition != input.CheckedAcceptance,
	}
	state.CollectionPending = input.ProgressEnabled && !state.CollectionUnknown &&
		(input.ProgressPending || active != nil || state.CheckingAcceptance)
	if active != nil && !state.CollectionUnknown {
		state.CollectionStage = active.Stage
	}
	if latest := latestAttempt(attempts); latest != nil {
		state.CollectionFailed = latest.Stage == StageFailed || latest.Stage == StageInterrupted
	}
	if status := input.Status; status != nil {
		state.ProcessingPending = status.Pending > 0 || status.Metadata.Generation != status.Metadata.TargetGeneration
		state.ProcessingBackoff = status.Failed > 0 && status.FailedRetryAtMs > input.StatusReadAtMs
		state.Revision, state.Identity = status.Metadata.Revision, StatusIdentity(*status)
		state.AnalyticsEnabled = status.Readiness == readyReadiness && status.Metadata.DatasetID == input.DatasetID
	}
	state.Loading = state.CollectionPending || state.ProcessingPending || input.Status == nil
	return state
}

// StatusState is the tone of the header status.
type StatusState string

const (
	StatusReady     StatusState = "ready"
	StatusLoading   StatusState = "loading"
	StatusAttention StatusState = "attention"
)

// StatusValue is the header status label and tone.
type StatusValue struct {
	Label string
	State StatusState
}

// StatusInput is everything the header status depends on.
type StatusInput struct {
	Sync SyncState
	// ConnectionFailed covers a failed connection or status read.
	ConnectionFailed bool
	RefreshFailed    bool
	RefreshPending   bool
	UsageFailed      bool
	FacetsFailed     bool
	// UsageCapability and FacetsCapability are the instance's advertised reads.
	UsageCapability  bool
	FacetsCapability bool
	// UsageFetching is true while a usage read is in flight or none has succeeded yet.
	UsageFetching bool
	// UsageLoaded is true once usage data exists for the window.
	UsageLoaded bool
	// FacetsPending is true until facets exist for the current query.
	FacetsPending bool
}

// DashboardStatus picks the single most important status, in the web's priority:
// connection, refresh, read failures, collection, processing, then loading.
func DashboardStatus(input StatusInput) StatusValue {
	sync := input.Sync
	switch {
	case input.ConnectionFailed:
		return StatusValue{"Connection unavailable", StatusAttention}
	case input.RefreshFailed:
		return StatusValue{"Refresh failed", StatusAttention}
	case input.RefreshPending:
		return StatusValue{"Refreshing usage…", StatusLoading}
	case input.UsageFailed:
		return StatusValue{"Usage couldn’t load", StatusAttention}
	case input.FacetsFailed:
		return StatusValue{"Filters couldn’t load", StatusAttention}
	case sync.CollectionUnknown:
		return StatusValue{"Collection status unavailable", StatusAttention}
	case sync.CollectionFailed:
		return StatusValue{"Collection needs attention", StatusAttention}
	case sync.CollectionStage != "":
		return StatusValue{CollectorStageLabel(sync.CollectionStage), StatusLoading}
	case sync.ProcessingPending && sync.ProcessingBackoff:
		return StatusValue{"Processing retry pending", StatusAttention}
	case sync.ProcessingPending:
		return StatusValue{"Processing usage", StatusLoading}
	case sync.Loading:
		return StatusValue{"Loading…", StatusLoading}
	case sync.AnalyticsEnabled && input.UsageCapability && input.UsageFetching && input.UsageLoaded:
		return StatusValue{"Refreshing usage…", StatusLoading}
	case sync.AnalyticsEnabled && input.UsageCapability && input.UsageFetching:
		return StatusValue{"Loading…", StatusLoading}
	case sync.AnalyticsEnabled && input.FacetsCapability && input.FacetsPending:
		return StatusValue{"Loading filters…", StatusLoading}
	}
	return StatusValue{"Ready", StatusReady}
}
