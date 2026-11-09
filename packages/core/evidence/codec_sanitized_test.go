package evidence

import (
	"errors"
	"reflect"
	"testing"
)

// Retain the previous comparison as an independent oracle: the byte-equality
// fast path must accept and reject exactly the same data with the same codes.
func previousSanitizedOnly(harness, format string, data []byte) error {
	clean, _, err := Sanitize(harness, format, data)
	if err != nil {
		return err
	}
	left, err := DecodeData(clean)
	if err != nil {
		return err
	}
	right, err := DecodeData(data)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(left, right) {
		return errors.New("private_or_unknown_field")
	}
	return nil
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func assertSanitizedOnlyEquivalent(t *testing.T, harness, format string, data []byte) {
	t.Helper()
	got, want := errorCode(sanitizedOnly(harness, format, data)), errorCode(previousSanitizedOnly(harness, format, data))
	if got != want {
		t.Fatalf("%s/%s %q: got %q want %q", harness, format, data, got, want)
	}
}

var sanitizedOnlySeeds = []string{
	`{}`, `null`, `{} {}`, ``, `[]`, `{"type":"session","id":"session"}`,
	`{"id":"session","type":"session"}`, ` {"type":"session","id":"session"} `,
	`{"type":"session","id":"session","content":"SECRET"}`,
	`{"type":"session","id":1.0}`, `{"type":"session","id":"session","id":"other"}`,
	`{"payload":{"info":{"last_token_usage":{"input_tokens":9007199254740993}},"source":{"subagent":false}}}`,
	`{"message":{"role":"assistant","usage":{"input":null},"content":"SECRET"}}`,
	`{"type":"message","message":{"role":"assistant","model":"model","provider":"provider","usage":{"input":100,"output":20,"cacheRead":2,"cacheWrite":3,"totalTokens":125}}}`,
}

func TestSanitizedOnlyMatchesPreviousComparison(t *testing.T) {
	accepted := 0
	for _, seed := range sanitizedOnlySeeds {
		for _, source := range whitelistFormats {
			assertSanitizedOnlyEquivalent(t, source.harness, source.format, []byte(seed))
			clean, _, err := Sanitize(source.harness, source.format, []byte(seed))
			if err != nil {
				continue
			}
			// Sanitized bytes take the fast path and must still be accepted.
			assertSanitizedOnlyEquivalent(t, source.harness, source.format, clean)
			if sanitizedOnly(source.harness, source.format, clean) == nil {
				accepted++
			}
		}
	}
	if accepted == 0 {
		t.Fatal("fast path never exercised")
	}
	if err := sanitizedOnly("pi", "pi-jsonl", []byte(`{"type":"session","id":"session","content":"SECRET"}`)); errorCode(err) != "private_or_unknown_field" {
		t.Fatal("private field accepted", err)
	}
}

func FuzzSanitizedOnlyMatchesPreviousComparison(f *testing.F) {
	for _, seed := range sanitizedOnlySeeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, source := range whitelistFormats {
			assertSanitizedOnlyEquivalent(t, source.harness, source.format, data)
			if clean, _, err := Sanitize(source.harness, source.format, data); err == nil {
				assertSanitizedOnlyEquivalent(t, source.harness, source.format, clean)
			}
		}
	})
}
