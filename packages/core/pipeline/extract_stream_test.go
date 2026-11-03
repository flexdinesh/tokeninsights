package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestStreamingDiscriminatorGrammarAndIrrelevance(t *testing.T) {
	cases := []struct {
		body       string
		harness    Harness
		irrelevant bool
	}{
		{`{"type":"response_item","payload":{"output":"private"}}`, HarnessCodex, true},
		{`{"payload":{"output":"private","type":"function_call_output"},"type":"response_item"}`, HarnessCodex, true},
		{`{"payload":{"output":"fake \\\"type\\\":\\\"response_item\\\""},"type":"session_meta"}`, HarnessCodex, false},
		{`{"type":"response_item","type":"event_msg","payload":{"type":"token_count"}}`, HarnessCodex, false},
		{`{"type":"response_item","type":"response_item"}`, HarnessCodex, false},
		{`{"payload":{"type":"agent_message"},"type":"event_msg"}`, HarnessCodex, true},
		{`{"payload":{"type":"task_started"},"type":"event_msg"}`, HarnessCodex, false},
		{`{"payload":{"type":"token_count"},"type":"event_msg"}`, HarnessCodex, false},
		{`{"payload":{"type":"new_kind"},"type":"event_msg"}`, HarnessCodex, false},
		{`{"payload":{"type":"agent_message"},"payload":{"type":"token_count"},"type":"event_msg"}`, HarnessCodex, false},
		{`{"private":{"type":"response_item"},"type":"session_meta"}`, HarnessCodex, false},
		{`{"type":"message","message":{"role":"user"}}`, HarnessPi, true},
		{`{"type":"message","message":{"role":"assistant"}}`, HarnessPi, false},
		{`{"type":"user"}`, HarnessClaudeCode, true},
		{`{"type":"assistant"}`, HarnessClaudeCode, false},
		{`{"type":"unknown"}`, HarnessClaudeCode, false},
		{`{"ty\u0070e":"response_\u0069tem","data":[null,true,false,0,-1,1e3,-1.2E-3]}`, HarnessCodex, true},
	}
	for _, test := range cases {
		t.Run(test.body, func(t *testing.T) {
			scanner := streamDiscriminator{reader: bufio.NewReaderSize(strings.NewReader(test.body), 16)}
			if err := scanner.object("", 0); err != nil {
				t.Fatal(err)
			}
			if err := scanner.finish(); err != nil {
				t.Fatal(err)
			}
			if got := scanner.irrelevant(test.harness); got != test.irrelevant {
				t.Fatalf("irrelevant=%v want %v", got, test.irrelevant)
			}
		})
	}
}

func FuzzStreamingDiscriminatorGrammar(f *testing.F) {
	for _, body := range []string{
		`{}`, `[]`, `null`, `{"a":1}`, `{"a":[1,true,false,null,-1.3e+10,{},[]]}`, `{"type":"response_item"}`,
		`{"a":"\\\"\/\b\f\n\r\t\uD800\uDC00"}`, `{"a":01}`, `{"a":1.}`, `{"a":1e}`, `{"a":+1}`,
		`{"a":[,]}`, `{"a":[1,]}`, `{"a":truee}`, `{"a":"bad\x"}`, `{"a":"bad\u12"}`, `{"a":1,"a":2}`, `{"a":{}}{}`,
	} {
		f.Add([]byte(body))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		scanner := streamDiscriminator{reader: bufio.NewReaderSize(bytes.NewReader(body), 16)}
		valid := scanner.object("", 0) == nil && scanner.finish() == nil
		trimmed := bytes.TrimSpace(body)
		want := json.Valid(body) && len(trimmed) > 0 && trimmed[0] == '{'
		if valid != want {
			t.Fatalf("stream valid=%v json object valid=%v for %q", valid, want, body)
		}
	})
}

func TestCaptureLineOversizedHashBoundaryAndTail(t *testing.T) {
	padding := strings.Repeat("x", maxEvidenceLineBytes)
	for _, test := range []struct {
		name, body                    string
		skipped, partial, limit, tail bool
	}{
		{"irrelevant-before", `{"type":"response_item","payload":{"output":"` + padding + `"}}` + "\n", true, false, false, false},
		{"irrelevant-after", `{"payload":{"output":"` + padding + `"},"type":"response_item"}` + "\n", true, false, false, false},
		{"irrelevant-tail", `{"payload":{"output":"` + padding + `"},"type":"response_item"}`, true, false, false, true},
		{"partial-tail", `{"type":"response_item","payload":{"output":"` + padding, false, true, false, true},
		{"usage", `{"type":"event_msg","payload":{"type":"token_count","private":"` + padding + `"}}` + "\n", false, false, true, false},
		{"unknown", `{"type":"new_kind","private":"` + padding + `"}` + "\n", false, false, true, false},
		{"fake-discriminator", `{"private":"\"type\":\"response_item\"` + padding + `","type":"session_meta"}` + "\n", false, false, true, false},
		{"invalid", `{"type":"response_item","private":"` + padding + `",}` + "\n", false, false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			hash := sha256.New()
			_, _ = hash.Write([]byte("prefix"))
			before := hex.EncodeToString(hash.Sum(nil))
			suffix := ""
			if !test.tail {
				suffix = "{\"next\":true}\n"
			}
			reader := bufio.NewReaderSize(strings.NewReader(test.body+suffix), 1024)
			got, err := readCaptureLine(t.Context(), reader, HarnessCodex, hash)
			if got.skipped != test.skipped || got.partial != test.partial || errors.Is(err, errSourceRecordLimit) != test.limit || errors.Is(err, io.EOF) != test.tail {
				t.Fatalf("got %+v err %v", got, err)
			}
			if test.skipped {
				expected := sha256.Sum256([]byte("prefix" + test.body))
				if got.length != int64(len(test.body)) || hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(expected[:]) {
					t.Fatal("drained byte count/hash changed")
				}
			}
			if test.partial && hex.EncodeToString(hash.Sum(nil)) != before {
				t.Fatal("partial tail polluted capture hash")
			}
			if !test.tail && !test.limit {
				next, err := reader.ReadString('\n')
				if err != nil || next != suffix {
					t.Fatal("next record consumed", next, err)
				}
			}
		})
	}
}

type failingCaptureReader struct {
	body *strings.Reader
	err  error
}

func (r failingCaptureReader) Read(body []byte) (int, error) {
	count, err := r.body.Read(body)
	if errors.Is(err, io.EOF) {
		return count, r.err
	}
	return count, err
}
func TestCaptureLineReadFailureAndCancellationAreNotLimits(t *testing.T) {
	synthetic := errors.New("temporary source failure")
	reader := bufio.NewReader(failingCaptureReader{body: strings.NewReader(`{"type":"response_item","private":"` + strings.Repeat("x", maxEvidenceLineBytes)), err: synthetic})
	if _, err := readCaptureLine(t.Context(), reader, HarnessCodex, sha256.New()); !errors.Is(err, synthetic) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readCaptureLine(ctx, bufio.NewReader(strings.NewReader("{}\n")), HarnessCodex, sha256.New()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestStreamingDiscriminatorDepthBoundaryMatchesJSON(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"empty-arrays-at-limit", `{"a":` + strings.Repeat("[", maxStreamingJSONDepth-1) + strings.Repeat("]", maxStreamingJSONDepth-1) + `}`, true},
		{"empty-arrays-over-limit", `{"a":` + strings.Repeat("[", maxStreamingJSONDepth) + strings.Repeat("]", maxStreamingJSONDepth) + `}`, false},
		{"empty-objects-at-limit", strings.Repeat(`{"a":`, maxStreamingJSONDepth-1) + `{}` + strings.Repeat("}", maxStreamingJSONDepth-1), true},
		{"empty-objects-over-limit", strings.Repeat(`{"a":`, maxStreamingJSONDepth) + `{}` + strings.Repeat("}", maxStreamingJSONDepth), false},
		{"primitive-at-limit", `{"a":` + strings.Repeat("[", maxStreamingJSONDepth-1) + `0` + strings.Repeat("]", maxStreamingJSONDepth-1) + `}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if json.Valid([]byte(test.body)) != test.valid {
				t.Fatal("fixture disagrees with standard JSON depth bound")
			}
			scanner := streamDiscriminator{reader: bufio.NewReader(strings.NewReader(test.body))}
			valid := scanner.object("", 0) == nil && scanner.finish() == nil
			if valid != test.valid {
				t.Fatalf("stream valid=%v want=%v", valid, test.valid)
			}
		})
	}
}
