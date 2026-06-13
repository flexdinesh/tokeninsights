package pipeline

import "time"

type Harness string

const (
	HarnessOpenCode Harness = "opencode"
	HarnessPi       Harness = "pi"
	HarnessCodex    Harness = "codex"
)

var SupportedHarnesses = []Harness{HarnessOpenCode, HarnessPi, HarnessCodex}

type Source struct {
	Harness Harness
	ID      string
	Kind    string
	Path    string
}

type DiscoverOptions struct {
	SourceDir         string
	HarnessSubdirOnly bool
}

type RawTokenFact struct {
	Harness          Harness
	SourceID         string
	SourceKind       string
	Collector        string
	Parser           string
	ObservedAtMs     int64
	OccurredAtMs     *int64
	SessionID        *string
	MessageID        *string
	Provider         *string
	Model            *string
	UsageScope       string
	Quality          string
	InputTokens      *int64
	OutputTokens     *int64
	ReasoningTokens  *int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	TotalTokens      *int64
	MetadataJSON     *string
}

type Diagnostic struct {
	Harness      Harness
	RawFactKey   string
	Severity     string
	Code         string
	Message      string
	MetadataJSON *string
}

type SyncOptions struct {
	DBPath    string
	Harnesses []Harness
	DryRun    bool
	Normalize bool
	SourceDir string
	Collector string
	Parser    string
	Now       time.Time
}

type NormalizeOptions struct {
	DBPath    string
	DryRun    bool
	Harnesses []Harness
	Now       time.Time
}

type Summary struct {
	RequestedHarnesses int
	Synced             int
	Skipped            int
	Failed             int
	RawFacts           int
	Observations       int
	Canonical          int
	Diagnostics        int
	Errors             []error
}
