package publication

import (
	"bytes"
	"encoding/json"
	"strings"
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

func fixtureBatch() Batch {
	return Batch{ProtocolVersion: ProtocolVersion, IdentityVersion: IdentityVersion, SemanticsVersion: SemanticsVersion,
		DatabaseID: "fixture-database", StreamID: "fixture-stream", BatchID: "fixture-batch", FromSequence: 1, ToSequence: 1,
		Entries: []Entry{{Sequence: 1, Fact: fixtureFact()}}}
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

func TestCanonicalHashSeparatesValueFromRequestEncoding(t *testing.T) {
	b := fixtureBatch()
	body, err := EncodeBatch(b)
	if err != nil {
		t.Fatal(err)
	}
	var reordered map[string]json.RawMessage
	if err := json.Unmarshal(body, &reordered); err != nil {
		t.Fatal(err)
	}
	alternative, err := json.Marshal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBatch(alternative)
	if err != nil {
		t.Fatal(err)
	}
	if RequestHash(body) == RequestHash(alternative) {
		t.Fatal("request identity ignored exact bytes")
	}
	if PayloadHash(decoded.Entries[0].Fact) != PayloadHash(b.Entries[0].Fact) {
		t.Fatal("JSON order changed canonical equality")
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

func TestStrictBatchDecoderRejectsInvalidAndPrivateFields(t *testing.T) {
	body, err := EncodeBatch(fixtureBatch())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBatch(body); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"duplicate":         bytes.Replace(body, []byte(`"batchId":"fixture-batch"`), []byte(`"batchId":"fixture-batch","batchId":"other"`), 1),
		"escaped duplicate": bytes.Replace(body, []byte(`"batchId":"fixture-batch"`), []byte(`"batchId":"fixture-batch","batch\u0049d":"other"`), 1),
		"private":           bytes.Replace(body, []byte(`"quality":"exact"`), []byte(`"quality":"exact","promptText":"SYNTHETIC_PRIVATE_MARKER"`), 1),
		"missing counter":   bytes.Replace(body, []byte(`"outputTokens":20,`), nil, 1),
		"null counter":      bytes.Replace(body, []byte(`"outputTokens":20`), []byte(`"outputTokens":null`), 1),
		"fraction":          bytes.Replace(body, []byte(`"outputTokens":20`), []byte(`"outputTokens":20.5`), 1),
		"numeric string":    bytes.Replace(body, []byte(`"outputTokens":20`), []byte(`"outputTokens":"20"`), 1),
		"unsafe integer":    bytes.Replace(body, []byte(`"outputTokens":20`), []byte(`"outputTokens":9007199254740992`), 1),
		"trailing":          append(bytes.Clone(body), []byte(` {}`)...),
		"case variant":      bytes.Replace(body, []byte(`"outputTokens":20`), []byte(`"OutputTokens":20`), 1),
		"body limit":        bytes.Repeat([]byte(" "), MaxBodyBytes+1),
	}
	for name, malformed := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeBatch(malformed); err == nil {
				t.Fatal("invalid batch accepted")
			} else if strings.Contains(err.Error(), "SYNTHETIC_PRIVATE_MARKER") {
				t.Fatal("private payload leaked in diagnostic")
			}
		})
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

func TestReceiptBindingRejectsEveryForeignAcknowledgement(t *testing.T) {
	b := fixtureBatch()
	body, err := EncodeBatch(b)
	if err != nil {
		t.Fatal(err)
	}
	r := Receipt{DatabaseID: b.DatabaseID, StreamID: b.StreamID, BatchID: b.BatchID, FromSequence: b.FromSequence, ToSequence: b.ToSequence,
		RequestHash: RequestHash(body), Inserted: 1, CommittedAtMs: 1767225600000, Revision: 1}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReceipt(encoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceipt(r, b, body); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Receipt){func(r *Receipt) { r.DatabaseID = "other" }, func(r *Receipt) { r.StreamID = "other" }, func(r *Receipt) { r.BatchID = "other" },
		func(r *Receipt) { r.FromSequence++ }, func(r *Receipt) { r.ToSequence++ }, func(r *Receipt) { r.RequestHash = "other" }, func(r *Receipt) { r.Inserted = 0 }} {
		copy := r
		change(&copy)
		if err := ValidateReceipt(copy, b, body); err == nil {
			t.Fatal("unbound receipt accepted")
		}
	}
}
