package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Retain the previous independent traversal as a byte-exact regression oracle:
// source duplicates, malformed nesting and native scalar types must not change.
func previousLeaf(object map[string]json.RawMessage, parts []string) (json.RawMessage, bool) {
	value, found := object[parts[0]]
	if !found || len(parts) == 1 {
		return value, found
	}
	child, err := decodeObject(value)
	if err != nil {
		return nil, false
	}
	return previousLeaf(child, parts[1:])
}

func previousSanitize(harness, format string, body []byte) (json.RawMessage, []string, error) {
	allowed, ok := paths(harness, format)
	if !ok {
		return nil, nil, errors.New("unsupported_source_format")
	}
	object, err := decodeObject(body)
	if err != nil {
		return nil, nil, err
	}
	result := map[string]interface{}{}
	diagnosticSet := map[string]bool{}
	for _, path := range allowed {
		parts := strings.Split(path, ".")
		value, found := previousLeaf(object, parts)
		if !found {
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || trimmed[0] == '{' || trimmed[0] == '[' {
			diagnosticSet["invalid_field_type"] = true
			setLeaf(result, parts, json.RawMessage("null"))
			continue
		}
		if trimmed[0] == '"' {
			var text string
			if json.Unmarshal(trimmed, &text) != nil || !SafeMetadata(text) {
				diagnosticSet["unsafe_metadata"] = true
				setLeaf(result, parts, json.RawMessage("null"))
				continue
			}
		}
		// Metadata bounds apply to decoded UTF-8, already checked above. JSON
		// quotes/escapes must not shorten the permitted native identifier.
		if trimmed[0] != '"' && len(value) > MaxStringBytes {
			diagnosticSet["field_limit"] = true
			setLeaf(result, parts, json.RawMessage("null"))
			continue
		}
		setLeaf(result, parts, value)
	}
	if harness == "codex" {
		if value, found := previousLeaf(object, []string{"payload", "source", "subagent"}); found && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			diagnosticSet["native_subagent_present"] = true
		}
	}
	var diagnostics []string
	for code := range diagnosticSet {
		diagnostics = append(diagnostics, code)
	}
	sort.Strings(diagnostics)
	out, err := json.Marshal(result)
	return out, diagnostics, err
}

var whitelistFormats = []struct{ harness, format string }{
	{"opencode", "opencode-v1"}, {"opencode", "opencode-v2"},
	{"opencode", "opencode-session"}, {"opencode", "opencode-project"},
	{"pi", "pi-jsonl"}, {"claude-code", "claude-code-jsonl"},
	{"codex", "codex-jsonl"}, {"unknown", "unknown"},
}

func assertSanitizeEquivalent(t *testing.T, harness, format string, body []byte) {
	t.Helper()
	got, diagnostics, err := Sanitize(harness, format, body)
	want, wantDiagnostics, wantErr := previousSanitize(harness, format, body)
	errorCode, wantErrorCode := "", ""
	if err != nil {
		errorCode = err.Error()
	}
	if wantErr != nil {
		wantErrorCode = wantErr.Error()
	}
	if !bytes.Equal(got, want) || !reflect.DeepEqual(diagnostics, wantDiagnostics) || errorCode != wantErrorCode {
		t.Fatalf("sanitization changed: got %s / %v / %s, want %s / %v / %s", got, diagnostics, errorCode, want, wantDiagnostics, wantErrorCode)
	}
}

func TestSanitizeCachedDecodingPreservesSourceSemantics(t *testing.T) {
	scalars := []string{
		`null`, `true`, `false`, `0`, `-1`, `9007199254740993`, `1.234567890123456789e+40`,
		`"native"`, `"9007199254740993"`, `"/private/path"`, `"model\nprivate"`,
		`"private@example.test"`, `"https://private.test"`, `{"private":"SECRET"}`, `["SECRET"]`,
		`"` + strings.Repeat("x", MaxStringBytes+1) + `"`,
		`"` + strings.Repeat("é", MaxStringBytes/len("é")) + `"`,
		`1` + strings.Repeat("0", MaxStringBytes),
	}
	malformed := []string{
		`null`, `[]`, `1`, `"record"`, `{`, `{} {}`, `{"type":NaN}`,
		`{"message":null,"payload":null,"data":null}`,
		`{"message":[],"payload":42,"data":"SECRET"}`,
		`{"message":{"usage":false},"payload":{"info":[]},"data":{"tokens":null}}`,
		`{"type":"discarded","type":"message","message":{"role":"user","role":"assistant","usage":{"input":1,"input":2}},"payload":{"source":{"subagent":false}}}`,
		`{"message":{"model":"\ud800"},"payload":{"source":{"subagent":null}}}`,
	}
	for _, source := range whitelistFormats {
		t.Run(source.format, func(t *testing.T) {
			for _, body := range malformed {
				assertSanitizeEquivalent(t, source.harness, source.format, []byte(body))
			}
			allowed, _ := paths(source.harness, source.format)
			for _, path := range allowed {
				for _, scalar := range scalars {
					input := map[string]interface{}{"private": map[string]interface{}{"content": "SECRET"}}
					setLeaf(input, strings.Split(path, "."), json.RawMessage(scalar))
					body, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					assertSanitizeEquivalent(t, source.harness, source.format, body)
				}
			}
		})
	}
}

func TestSanitizeRetainsSourceDuplicateLastValue(t *testing.T) {
	got, diagnostics, err := Sanitize("pi", "pi-jsonl", []byte(`{"type":"user","type":"message","message":{"role":"user","role":"assistant","usage":{"input":1,"input":9007199254740993},"private":"SECRET"}}`))
	want := `{"message":{"role":"assistant","usage":{"input":9007199254740993}},"type":"message"}`
	if err != nil || len(diagnostics) != 0 || string(got) != want {
		t.Fatal(string(got), diagnostics, err)
	}
	record := Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: got}
	if err := ValidateRecord(record); err != nil {
		t.Fatal(err)
	}
	record.Data = json.RawMessage(`{"type":"message","type":"session"}`)
	var decoded json.RawMessage
	if err := StrictDecode(record.Data, &decoded); err == nil {
		t.Fatal("wire duplicate admitted")
	}
}

func TestSanitizeNativeSubagentPresence(t *testing.T) {
	for _, source := range []struct {
		value   string
		present bool
	}{
		{`null`, false}, {`{}`, true}, {`[]`, true}, {`false`, true}, {`"PRIVATE"`, true},
		{`{"thread_spawn":{"parent_thread_id":"parent","private":"SECRET"}}`, true},
	} {
		got, diagnostics, err := Sanitize("codex", "codex-jsonl", []byte(`{"payload":{"source":{"subagent":`+source.value+`}}}`))
		if err != nil {
			t.Fatal(err)
		}
		present := false
		for _, code := range diagnostics {
			present = present || code == "native_subagent_present"
		}
		if present != source.present || bytes.Contains(got, []byte("PRIVATE")) || bytes.Contains(got, []byte("SECRET")) {
			t.Fatal(string(got), diagnostics)
		}
	}
}

func TestCachedRecordIdentityAndSelection(t *testing.T) {
	for _, source := range []struct {
		harness, format, data, context, native, id string
		usage                                      bool
	}{
		{"opencode", "opencode-v1", `{"id":"message","session_id":"session","data":{"role":"assistant"}}`, ``, "session", "message", true},
		{"opencode", "opencode-v2", `{"id":"message","session_id":"session","type":"assistant"}`, ``, "session", "message", true},
		{"opencode", "opencode-session", `{"id":"session"}`, ``, "session", "session", false},
		{"pi", "pi-jsonl", `{"type":"message","id":"message","message":{"role":"assistant"}}`, `{"type":"session","id":"session"}`, "session", "message", true},
		{"pi", "pi-jsonl", `{"type":"message","message":{"role":"user"}}`, `{"type":"session","id":"session"}`, "session", "", false},
		{"claude-code", "claude-code-jsonl", `{"type":"assistant","sessionId":null,"session_id":"session","requestId":"request","message":{"role":"assistant","id":"message"}}`, ``, "session", "message:request:", true},
		{"claude-code", "claude-code-jsonl", `{"type":"assistant","sessionId":"session","session_id":"other","uuid":"uuid","message":{"role":"assistant","id":"message"}}`, ``, "session", "uuid", true},
		{"codex", "codex-jsonl", `{"type":"event_msg","payload":{"type":"token_count"}}`, `{"type":"session_meta","payload":{"id":"session"}}`, "session", "", true},
		{"codex", "codex-jsonl", `{"type":"session_meta","payload":{"id":"session"}}`, ``, "session", "", false},
		{"codex", "codex-jsonl", `{"type":"response_item","payload":{"type":"token_count"}}`, ``, "", "", false},
		{"pi", "pi-jsonl", `null`, `[]`, "", "", false},
	} {
		record := Record{Harness: source.harness, Format: source.format, SourceID: "source", Lineage: "lineage", Data: json.RawMessage(source.data)}
		if source.context != "" {
			record.Context = []Context{{Data: json.RawMessage(source.context)}}
		}
		scope := source.harness + ":source:" + Tuple("source", "lineage")
		if source.native != "" {
			scope = SessionScope(source.harness, source.native)
		}
		if got := Scope(record); got != scope {
			t.Fatalf("scope: %s, want %s", got, scope)
		}
		key := ""
		if source.id != "" {
			key = Tuple(source.harness, source.format, scope, source.id)
		}
		if got := NativeRecordKey(record); got != key {
			t.Fatalf("native key: %s, want %s", got, key)
		}
		if got := UsageRecord(source.harness, source.format, record.Data); got != source.usage {
			t.Fatalf("usage selection: %v, want %v", got, source.usage)
		}
	}
}

func FuzzSanitizeCachedDecoding(f *testing.F) {
	for _, body := range []string{`{}`, `null`, `{} {}`, `{"payload":{"info":{"last_token_usage":{"input_tokens":9007199254740993}},"source":{"subagent":false}}}`, `{"message":{"role":"assistant","usage":{"input":null},"content":"SECRET"}}`} {
		f.Add([]byte(body))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, source := range whitelistFormats {
			assertSanitizeEquivalent(t, source.harness, source.format, body)
		}
	})
}

func BenchmarkSanitizeNestedPrivatePayload(b *testing.B) {
	private := strings.Repeat("PRIVATE", 8*1024)
	cases := []struct{ harness, format, body string }{
		{"opencode", "opencode-v2", `{"id":"message","session_id":"session","type":"assistant","data":{"model":{"id":"model","providerID":"provider"},"tokens":{"input":100,"output":20,"reasoning":1,"cache":{"read":2,"write":3}},"time":{"created":1,"completed":2},"content":"` + private + `"}}`},
		{"pi", "pi-jsonl", `{"type":"message","message":{"role":"assistant","model":"model","provider":"provider","usage":{"input":100,"output":20,"reasoning":1,"cacheRead":2,"cacheWrite":3,"totalTokens":126},"content":"` + private + `"}}`},
		{"claude-code", "claude-code-jsonl", `{"type":"assistant","message":{"role":"assistant","model":"model","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens_details":{"thinking_tokens":1}},"content":"` + private + `"}}`},
		{"codex", "codex-jsonl", `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":20,"reasoning_output_tokens":1,"cached_input_tokens":2,"total_tokens":121},"total_token_usage":{"input_tokens":1000,"output_tokens":200,"reasoning_output_tokens":10,"cached_input_tokens":20,"total_tokens":1210}},"private":"` + private + `"}}`},
	}
	for _, source := range cases {
		body := []byte(source.body)
		for _, algorithm := range []struct {
			name string
			run  func(string, string, []byte) (json.RawMessage, []string, error)
		}{{"previous", previousSanitize}, {"cached", Sanitize}} {
			b.Run(source.harness+"/"+algorithm.name, func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				for b.Loop() {
					if _, _, err := algorithm.run(source.harness, source.format, body); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
