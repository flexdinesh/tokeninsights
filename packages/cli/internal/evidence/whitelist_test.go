package evidence

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWhitelistDropsPrivateContentPreservesNativeScalars(t *testing.T) {
	input := []byte(`{"type":"message","id":"native","timestamp":null,"cwd":"/private/alice/project","message":{"role":"assistant","model":"model","content":[{"text":"PRIVATE_PROMPT"}],"toolArguments":"SECRET","usage":{"input":9007199254740993,"output":"20","cacheRead":null,"totalTokens":-1},"headers":{"Authorization":"SECRET"}}}`)
	clean, diagnostics, err := Sanitize("pi", "pi-jsonl", input)
	if err != nil || len(diagnostics) != 0 {
		t.Fatal(err, diagnostics)
	}
	for _, private := range []string{"PRIVATE_PROMPT", "SECRET", "alice", "cwd", "content", "headers", "toolArguments"} {
		if bytes.Contains(clean, []byte(private)) {
			t.Fatal("private data survived", private, string(clean))
		}
	}
	for _, native := range []string{"9007199254740993", `"output":"20"`, `"timestamp":null`, `"cacheRead":null`, `"totalTokens":-1`} {
		if !bytes.Contains(clean, []byte(native)) {
			t.Fatal("source scalar changed", native, string(clean))
		}
	}
	record := Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: clean}
	if err := ValidateRecord(record); err != nil {
		t.Fatal(err)
	}
	record.Data = input
	if err := ValidateRecord(record); err == nil {
		t.Fatal("server admitted unknown/private fields")
	}
}

func TestUnsafeAllowedMetadataNullsAndFixedDiagnostic(t *testing.T) {
	for _, value := range []string{"/private/path", "person@example.test", "https://private.test", strings.Repeat("x", MaxStringBytes+1), "model\nsecret"} {
		body, _ := json.Marshal(map[string]interface{}{"type": "message", "message": map[string]interface{}{"model": value}})
		clean, diagnostics, err := Sanitize("pi", "pi-jsonl", body)
		if err != nil || len(diagnostics) != 1 || bytes.Contains(clean, []byte(value)) || !bytes.Contains(clean, []byte(`"model":null`)) {
			t.Fatal(string(clean), diagnostics, err)
		}
	}
}

func TestStrictDecodeRejectsDuplicateFieldsAndFutureContext(t *testing.T) {
	for _, body := range []string{`{"id":1,"id":2}`, `{"data":{"id":1,"id":2}}`, `{} {}`, `{"unknown":1}`} {
		var target struct {
			ID   int `json:"id"`
			Data struct {
				ID int `json:"id"`
			} `json:"data"`
		}
		if err := StrictDecode([]byte(body), &target); err == nil {
			t.Fatal("invalid input accepted", body)
		}
	}
	record := Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"message"}`), Context: []Context{{Ordinal: 2, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	if err := ValidateRecord(record); err == nil {
		t.Fatal("future context accepted")
	}
}

func TestMetadataLimitCountsDecodedUTF8Bytes(t *testing.T) {
	for _, native := range []string{strings.Repeat("x", MaxStringBytes), strings.Repeat("é", MaxStringBytes/len("é")), strings.Repeat("<", MaxStringBytes)} {
		body, err := json.Marshal(map[string]interface{}{"type": "session", "id": native})
		if err != nil {
			t.Fatal(err)
		}
		clean, diagnostics, err := Sanitize("pi", "pi-jsonl", body)
		if err != nil || len(diagnostics) != 0 || String(clean, "id") != native {
			t.Fatal("valid native metadata changed", string(clean), diagnostics, err)
		}
		record := Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: clean}
		if err := ValidateRecord(record); err != nil {
			t.Fatal("boundary metadata rejected", err)
		}
	}
}
