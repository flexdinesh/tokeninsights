package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func tupleID(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return RequestHash(encoded)
}

func SessionID(harness, nativeID string) string {
	return tupleID("session-v1", "default", harness, nativeID)
}

func MessageID(harness, sessionNativeID, messageNativeID string) string {
	return tupleID("message-v1", "default", harness, sessionNativeID, messageNativeID)
}

// FactID preserves Codex's adapter-derived immutable event witness in NativeID.
// No installation, transport, mutable label, or capture clock enters this tuple.
func FactID(f Fact) string {
	message := ""
	if f.Message != nil {
		message = f.Message.NativeID
	}
	return tupleID("fact-v1", "default", f.Harness, f.Session.NativeID, message, f.NativeRequestID, f.UsageScope)
}

func LocationID(l Location) string {
	return tupleID("location-v1", l.DirectoryKey, l.RepositoryKey)
}

func SetIDs(f *Fact) {
	f.Session.ID = SessionID(f.Harness, f.Session.NativeID)
	if f.Message != nil {
		f.Message.ID = MessageID(f.Harness, f.Session.NativeID, f.Message.NativeID)
	}
	if f.Location != nil {
		f.Location.ID = LocationID(*f.Location)
	}
	f.ID = FactID(*f)
}

func RequestHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// PayloadHash excludes mergeable reference occurrence envelopes and delivery
// metadata. A source snapshot occurrence remains part of the contribution.
func PayloadHash(f Fact) string {
	f.Session.FirstOccurredAtMs = 0
	f.Session.LastOccurredAtMs = 0
	if f.Message != nil {
		message := *f.Message
		message.OccurredAtMs = 0
		f.Message = &message
	}
	encoded, _ := json.Marshal(f)
	return RequestHash(encoded)
}
