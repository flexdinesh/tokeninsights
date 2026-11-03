package publication

import (
	"testing"
)

func fixtureFact() Fact {
	f := Fact{Harness: "pi", Session: Session{Harness: "pi", NativeID: "fixture-session-A", FirstOccurredAtMs: 1767225600000, LastOccurredAtMs: 1767225600000},
		Message: &Message{NativeID: "fixture-request-A", OccurredAtMs: 1767225600000}, OccurredAtMs: 1767225600000,
		Provider: "unknown", ProviderSource: "unknown", Model: "unknown", UsageScope: "message", Quality: "exact", Countable: true,
		InputTokens: 80, OutputTokens: 20, TotalTokens: 100}
	SetIDs(&f)
	return f
}

func TestIdentityNativeTupleVectors(t *testing.T) {
	f := fixtureFact()
	// SHA-256 vectors authored from independent Python JSON-array encoding.
	if f.Session.ID != "819dab917372043e6013522d5af4c59310321899aec4910515e4a242527a7d84" || f.Message.ID != "b211a1b6b1b68a2a88c32caa3c8fa4e6c6cc647115b47a8fba878f65f91d0e9d" || f.ID != "dbbdd82e66a44eda576e1f5cbfe531465198b525eeb6e70f874f4b27896a2db9" {
		t.Fatalf("identity vectors differ: session=%s message=%s fact=%s", f.Session.ID, f.Message.ID, f.ID)
	}
	other := f
	other.Session.NativeID = "fixture-session-A|fixture-request-A"
	other.Message = &Message{NativeID: "different"}
	left := FactID(other)
	other.Session.NativeID = "fixture-session-A"
	other.Message.NativeID = "fixture-request-A|different"
	if left == FactID(other) {
		t.Fatal("ambiguous delimiter encoding")
	}
	other = f
	other.OutputTokens = 21
	other.TotalTokens = 101
	other.OccurredAtMs++
	if FactID(other) != f.ID || PayloadHash(other) == PayloadHash(f) {
		t.Fatal("identity confused with canonical value")
	}
}

func TestPayloadHashIgnoresMergeableReferenceTimes(t *testing.T) {
	f := fixtureFact()
	before := PayloadHash(f)
	f.Session.FirstOccurredAtMs--
	f.Session.LastOccurredAtMs++
	f.Message.OccurredAtMs--
	if PayloadHash(f) != before {
		t.Fatal("reference range changed immutable fact hash")
	}
	f.OccurredAtMs++
	if PayloadHash(f) == before {
		t.Fatal("source snapshot occurrence excluded")
	}
}

func TestCodexImmutableWitnessKeepsSameMillisecondEventsDistinct(t *testing.T) {
	one := fixtureFact()
	one.Harness, one.Session.Harness = "codex", "codex"
	one.Message.NativeID = "fixture-turn:1767225600000:snapshot-one"
	SetIDs(&one)
	two := one
	two.Message = &Message{NativeID: "fixture-turn:1767225600000:snapshot-two", OccurredAtMs: one.OccurredAtMs}
	two.InputTokens, two.TotalTokens = 81, 101
	SetIDs(&two)
	if one.ID == two.ID {
		t.Fatal("distinct source-event witnesses collapsed")
	}
	if err := ValidateFact(one); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFact(two); err != nil {
		t.Fatal(err)
	}
}

func TestNumericIdentityAndRevisionValidation(t *testing.T) {
	f := fixtureFact()
	f.InputTokens, f.OutputTokens, f.TotalTokens = SafeInteger, 0, SafeInteger
	if err := ValidateFact(f); err != nil {
		t.Fatal(err)
	}
	f.OutputTokens = 1
	if err := ValidateFact(f); err == nil {
		t.Fatal("component sum overflow accepted")
	}
	f = fixtureFact()
	f.Session.ID = "wrong"
	if err := ValidateFact(f); err == nil {
		t.Fatal("unverified session accepted")
	}
	f = fixtureFact()
	f.Revision = &SourceRevision{Rule: ClaudeRevisionRule, Value: f.OccurredAtMs}
	if err := ValidateFact(f); err == nil {
		t.Fatal("Pi source revision accepted")
	}
	f.Harness, f.Session.Harness, f.NativeRequestID = "claude-code", "claude-code", "fixture-native-request"
	SetIDs(&f)
	if err := ValidateFact(f); err != nil {
		t.Fatal(err)
	}
	f.Revision.Value++
	if err := ValidateFact(f); err == nil {
		t.Fatal("stream sequence accepted as source occurrence")
	}
}
