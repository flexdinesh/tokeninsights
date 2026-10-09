package publication

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func invalid(code, field string) error { return &ValidationError{Code: code, Field: field} }

func validString(value string, required bool) bool {
	return (!required || strings.TrimSpace(value) != "") && len(value) <= MaxStringBytes && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func validInteger(value int64) bool { return value >= 0 && value <= SafeInteger }

// ValidTimestampMs accepts normalized Unix milliseconds with localtime headroom.
func ValidTimestampMs(value int64) bool { return value >= 0 && value <= MaxTimestampMs }

// Published location labels are basenames, independent of the server OS.
func validLocationLabel(value string) bool {
	return validString(value, false) && !strings.ContainsAny(value, `/\`)
}

func ValidateFact(f Fact) error {
	switch f.Harness {
	case "opencode", "pi", "codex", "claude-code":
	default:
		return invalid("invalid_request", "harness")
	}
	if f.Session.Harness != f.Harness || !validString(f.Session.NativeID, true) || f.Session.ID != SessionID(f.Harness, f.Session.NativeID) {
		return invalid("invalid_identity", "session")
	}
	if !ValidTimestampMs(f.Session.FirstOccurredAtMs) || !ValidTimestampMs(f.Session.LastOccurredAtMs) ||
		f.Session.FirstOccurredAtMs > f.Session.LastOccurredAtMs || !ValidTimestampMs(f.OccurredAtMs) ||
		f.OccurredAtMs < f.Session.FirstOccurredAtMs || f.OccurredAtMs > f.Session.LastOccurredAtMs {
		return invalid("invalid_request", "occurredAtMs")
	}
	if !validString(f.NativeRequestID, false) || (f.Message == nil && f.NativeRequestID == "") {
		return invalid("invalid_identity", "nativeIdentity")
	}
	if f.Message != nil && !ValidTimestampMs(f.Message.OccurredAtMs) {
		return invalid("invalid_request", "message.occurredAtMs")
	}
	if f.Message != nil && (!validString(f.Message.NativeID, true) ||
		f.Message.ID != MessageID(f.Harness, f.Session.NativeID, f.Message.NativeID)) {
		return invalid("invalid_identity", "message")
	}
	if f.UsageScope != "message" || f.ID != FactID(f) {
		return invalid("invalid_identity", "fact")
	}
	if !validString(f.Provider, true) || !validString(f.Model, true) {
		return invalid("invalid_request", "providerModel")
	}
	switch f.ProviderSource {
	case "explicit", "inferred", "unknown":
	default:
		return invalid("invalid_request", "providerSource")
	}
	switch f.Quality {
	case "exact", "derived", "estimated":
	default:
		return invalid("invalid_request", "quality")
	}
	components := []int64{f.InputTokens, f.OutputTokens, f.ReasoningTokens, f.CacheReadTokens, f.CacheWriteTokens}
	var sum int64
	for _, component := range components {
		if !validInteger(component) || component > SafeInteger-sum {
			return invalid("invalid_tokens", "components")
		}
		sum += component
	}
	if !validInteger(f.TotalTokens) || f.TotalTokens != sum {
		return invalid("invalid_tokens", "totalTokens")
	}
	if f.Revision != nil && (f.Harness != "claude-code" || f.NativeRequestID == "" || f.Message == nil ||
		f.Revision.Rule != ClaudeRevisionRule || !ValidTimestampMs(f.Revision.Value) || f.Revision.Value != f.OccurredAtMs) {
		return invalid("invalid_revision", "revision")
	}
	if f.Location != nil {
		l := *f.Location
		if (l.DirectoryKey == "" && l.RepositoryKey == "") || l.ID != LocationID(l) ||
			!validString(l.DirectoryKey, false) || !validLocationLabel(l.DirectoryName) ||
			!validString(l.RepositoryKey, false) || !validLocationLabel(l.RepositoryName) || !validString(l.RepositorySource, false) ||
			(l.DirectoryKey == "" && l.DirectoryName != "") || (l.RepositoryKey == "" && (l.RepositoryName != "" || l.RepositorySource != "")) {
			return invalid("invalid_request", "location")
		}
	}
	return nil
}
