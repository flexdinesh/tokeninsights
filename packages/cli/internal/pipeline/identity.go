package pipeline

import "encoding/json"

// Native tuples use unambiguous encoding; source IDs may contain separators.
func nativeTupleHash(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return stableHash(string(encoded))
}

// Source-session provenance is allowlisted host metadata. Filenames remain a
// useful local fallback, but cannot identify portable published native sessions.
type sourceIdentityMetadata struct {
	SessionSource string  `json:"session_source,omitempty"`
	RequestID     *string `json:"request_id,omitempty"`
}

func sourceIdentityJSON(sessionSource string, requestID *string) *string {
	encoded, _ := json.Marshal(sourceIdentityMetadata{SessionSource: sessionSource, RequestID: requestID})
	metadata := string(encoded)
	return &metadata
}

func sourceSessionIdentity(metadata *string) string {
	if metadata == nil {
		return ""
	}
	var identity sourceIdentityMetadata
	if json.Unmarshal([]byte(*metadata), &identity) != nil {
		return ""
	}
	return identity.SessionSource
}
