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

func NewCapabilities(databaseID string) Capabilities {
	return Capabilities{ProtocolVersion: ProtocolVersion, IdentityVersion: IdentityVersion, SemanticsVersion: SemanticsVersion,
		DatabaseID: databaseID, MaxBodyBytes: MaxBodyBytes, MaxEntries: MaxEntries, MaxStringBytes: MaxStringBytes, MaxInteger: SafeInteger}
}

func ValidateBatch(b Batch) error {
	if b.ProtocolVersion != ProtocolVersion || b.IdentityVersion != IdentityVersion || b.SemanticsVersion != SemanticsVersion {
		return invalid("incompatible", "version")
	}
	if !validString(b.DatabaseID, true) || !validString(b.StreamID, true) || !validString(b.BatchID, true) || !validString(b.Hostname, false) {
		return invalid("invalid_request", "batchIdentity")
	}
	if b.FromSequence < 1 || !validInteger(b.FromSequence) || !validInteger(b.ToSequence) || b.ToSequence < b.FromSequence ||
		len(b.Entries) < 1 || len(b.Entries) > MaxEntries || b.ToSequence-b.FromSequence+1 != int64(len(b.Entries)) {
		return invalid("invalid_request", "sequenceRange")
	}
	for i, entry := range b.Entries {
		if entry.Sequence != b.FromSequence+int64(i) {
			return invalid("invalid_request", "sequence")
		}
		if err := ValidateFact(entry.Fact); err != nil {
			return err
		}
	}
	return nil
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
	if !validInteger(f.Session.FirstOccurredAtMs) || !validInteger(f.Session.LastOccurredAtMs) ||
		f.Session.FirstOccurredAtMs > f.Session.LastOccurredAtMs || !validInteger(f.OccurredAtMs) ||
		f.OccurredAtMs < f.Session.FirstOccurredAtMs || f.OccurredAtMs > f.Session.LastOccurredAtMs {
		return invalid("invalid_request", "occurredAtMs")
	}
	if !validString(f.NativeRequestID, false) || (f.Message == nil && f.NativeRequestID == "") {
		return invalid("invalid_identity", "nativeIdentity")
	}
	if f.Message != nil && (!validString(f.Message.NativeID, true) || !validInteger(f.Message.OccurredAtMs) ||
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
		f.Revision.Rule != ClaudeRevisionRule || f.Revision.Value != f.OccurredAtMs) {
		return invalid("invalid_revision", "revision")
	}
	if f.Location != nil {
		l := *f.Location
		if (l.DirectoryKey == "" && l.RepositoryKey == "") || l.ID != LocationID(l) ||
			!validString(l.DirectoryKey, false) || !validString(l.DirectoryName, false) ||
			!validString(l.RepositoryKey, false) || !validString(l.RepositoryName, false) || !validString(l.RepositorySource, false) ||
			(l.DirectoryKey == "" && l.DirectoryName != "") || (l.RepositoryKey == "" && (l.RepositoryName != "" || l.RepositorySource != "")) ||
			strings.HasPrefix(l.DirectoryName, "/") || strings.Contains(l.DirectoryName, ":\\") {
			return invalid("invalid_request", "location")
		}
	}
	return nil
}

func ValidateReceipt(r Receipt, b Batch, request []byte) error {
	if r.DatabaseID != b.DatabaseID || r.StreamID != b.StreamID || r.BatchID != b.BatchID ||
		r.FromSequence != b.FromSequence || r.ToSequence != b.ToSequence || r.RequestHash != RequestHash(request) {
		return invalid("receipt_mismatch", "binding")
	}
	if !validInteger(r.Inserted) || !validInteger(r.Updated) || !validInteger(r.Noop) || !validInteger(r.CommittedAtMs) ||
		!validInteger(r.Revision) || r.Inserted > int64(len(b.Entries)) || r.Updated > int64(len(b.Entries)) ||
		r.Noop > int64(len(b.Entries)) || r.Inserted+r.Updated+r.Noop != int64(len(b.Entries)) {
		return invalid("receipt_mismatch", "counts")
	}
	return nil
}
