package pipeline

import (
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/capture"
)

type Harness = capture.Harness

const (
	HarnessOpenCode   = capture.HarnessOpenCode
	HarnessPi         = capture.HarnessPi
	HarnessCodex      = capture.HarnessCodex
	HarnessClaudeCode = capture.HarnessClaudeCode
)

var SupportedHarnesses = capture.SupportedHarnesses

// ParseHarness accepts exactly one supported harness identifier.
func ParseHarness(value string) (Harness, bool) { return capture.ParseHarness(value) }

type Source struct {
	Harness       Harness
	ID            string
	Kind          string
	Path          string
	RawSourceID   string
	AlwaysRefresh bool
}

type DiscoverOptions struct {
	Sources           *SourceConfig
	SourceDir         string
	HarnessSubdirOnly bool
}

type sessionLocation struct {
	SessionID *string
	Location  *Location
}

type SyncOptions struct {
	Sources          *SourceConfig
	DBPath           string
	Harnesses        []Harness
	DryRun           bool
	FullRefresh      bool
	SourceDir        string
	Now              time.Time
	Progress         func(SyncProgressEvent)
	CaptureProgress  func(CaptureProgressEvent)
	locationResolver *locationResolver
	workers          int
	// beforePrepare observes a worker starting a source job; tests only.
	beforePrepare func(job int)
}

type SyncProgressStatus string

const (
	SyncProgressDiscovering SyncProgressStatus = "discovering"
	SyncProgressSyncing     SyncProgressStatus = "syncing"
	SyncProgressSkipped     SyncProgressStatus = "skipped"
	SyncProgressSynced      SyncProgressStatus = "synced"
	SyncProgressFailed      SyncProgressStatus = "failed"
	SyncProgressLoading     SyncProgressStatus = "loading dashboard"
	SyncProgressWaiting     SyncProgressStatus = "waiting"
)

type SyncProgressEvent struct {
	Harness     Harness
	Status      SyncProgressStatus
	Published   bool
	Quarantined int
}

type Summary struct {
	RequestedHarnesses int
	Synced             int
	Skipped            int
	Failed             int
	RawFacts           int
	Quarantined        int
	Errors             []error
}
