package pipeline

import (
	"time"
)

type Harness string

const (
	HarnessOpenCode   Harness = "opencode"
	HarnessPi         Harness = "pi"
	HarnessCodex      Harness = "codex"
	HarnessClaudeCode Harness = "claude-code"
)

var SupportedHarnesses = []Harness{HarnessOpenCode, HarnessPi, HarnessCodex, HarnessClaudeCode}

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
	locationResolver *locationResolver
	workers          int
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
