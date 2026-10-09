package querymodel

import "time"

type Filter struct {
	Start          time.Time
	End            time.Time
	SessionIDs     []string
	Providers      []string
	Models         []string
	Harnesses      []string
	RepositoryKeys []string
	DirectoryKeys  []string
	DayFrom        string
	DayTo          string
}
type LocationOption struct {
	Key  string
	Name string
}
type ViewerSummaryRow struct {
	TotalTokens      int64 `json:"total"`
	InputTokens      int64 `json:"input"`
	OutputTokens     int64 `json:"output"`
	ReasoningTokens  int64 `json:"reasoning"`
	CacheReadTokens  int64 `json:"cacheRead"`
	CacheWriteTokens int64 `json:"cacheWrite"`
	SessionCount     int64 `json:"sessions"`
	SyncedSessions   int64 `json:"syncedSessions"`
}
type SessionCounts struct {
	Shown  int64
	Synced int64
}
type SyncStatus struct {
	Running  bool
	Revision int64
}
type DayCoverage struct {
	Total          *int64 `json:"total"`
	Day            string `json:"day"`
	Status         string `json:"status"`
	CheckedAtMs    int64  `json:"checkedAt"`
	PendingSources int64  `json:"pendingSources"`
	FailedSources  int64  `json:"failedSources"`
	HasUsage       bool   `json:"hasUsage"`
}
type RepoGroup string

const (
	RepoGroupRepository RepoGroup = "repository"
	RepoGroupDirectory  RepoGroup = "directory"
)

func LocationDisplayName(option LocationOption) string {
	if option.Name == "" {
		return "unknown"
	}
	return option.Name
}
