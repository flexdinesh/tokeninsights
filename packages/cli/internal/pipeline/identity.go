package pipeline

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

// Native tuples use unambiguous encoding; source IDs may contain separators.
func nativeTupleHash(parts ...string) string { return processor.NativeTupleHash(parts...) }

// Source-session provenance is allowlisted host metadata. Filenames remain a
// useful local fallback, but cannot identify portable published native sessions.
type sourceIdentityMetadata struct {
	SessionSource string  `json:"session_source,omitempty"`
	RequestID     *string `json:"request_id,omitempty"`
}

func sourceIdentityJSON(sessionSource string, requestID *string) *string {
	return processor.SourceIdentityJSON(sessionSource, requestID)
}
func sourceSessionIdentity(metadata *string) string { return processor.SourceSessionIdentity(metadata) }
