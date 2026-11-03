// Package publication defines processed facts and stable native identities.
package publication

const (
	MaxBodyBytes         = 1 << 20
	MaxEntries           = 256
	MaxStringBytes       = 256
	SafeInteger    int64 = 9007199254740991
	// Leave one day before SQLite's year-10000 boundary for local date buckets.
	MaxTimestampMs     int64 = 253402214399999 // 9999-12-30T23:59:59.999Z
	ClaudeRevisionRule       = "claude-source-timestamp-v1"
)

type Session struct {
	ID                string `json:"id"`
	Harness           string `json:"harness"`
	NativeID          string `json:"nativeId"`
	FirstOccurredAtMs int64  `json:"firstOccurredAtMs"`
	LastOccurredAtMs  int64  `json:"lastOccurredAtMs"`
}

type Message struct {
	ID           string `json:"id"`
	NativeID     string `json:"nativeId"`
	OccurredAtMs int64  `json:"occurredAtMs"`
}

type Location struct {
	ID               string `json:"id"`
	DirectoryKey     string `json:"directoryKey,omitempty"`
	DirectoryName    string `json:"directoryName,omitempty"`
	RepositoryKey    string `json:"repositoryKey,omitempty"`
	RepositoryName   string `json:"repositoryName,omitempty"`
	RepositorySource string `json:"repositorySource,omitempty"`
}

type SourceRevision struct {
	Rule  string `json:"rule"`
	Value int64  `json:"value"`
}

type Fact struct {
	ID               string          `json:"id"`
	Harness          string          `json:"harness"`
	Session          Session         `json:"session"`
	Message          *Message        `json:"message,omitempty"`
	NativeRequestID  string          `json:"nativeRequestId,omitempty"`
	OccurredAtMs     int64           `json:"occurredAtMs"`
	Provider         string          `json:"provider"`
	ProviderSource   string          `json:"providerSource"`
	Model            string          `json:"model"`
	UsageScope       string          `json:"usageScope"`
	Quality          string          `json:"quality"`
	Countable        bool            `json:"countable"`
	InputTokens      int64           `json:"inputTokens"`
	OutputTokens     int64           `json:"outputTokens"`
	ReasoningTokens  int64           `json:"reasoningTokens"`
	CacheReadTokens  int64           `json:"cacheReadTokens"`
	CacheWriteTokens int64           `json:"cacheWriteTokens"`
	TotalTokens      int64           `json:"totalTokens"`
	Location         *Location       `json:"location,omitempty"`
	Revision         *SourceRevision `json:"revision,omitempty"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	BatchID string `json:"batchId,omitempty"`
	FactID  string `json:"factId,omitempty"`
}

// ValidationError carries safe field names, never incoming values or source content.
type ValidationError struct{ Code, Field string }

func (e *ValidationError) Error() string { return e.Code + ": " + e.Field }
