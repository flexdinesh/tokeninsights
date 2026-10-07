package processor_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

func stored(harness, format, data string, context ...string) evidence.Stored {
	record := evidence.Record{Harness: harness, Format: format, SourceID: "source", Lineage: "lineage", Ordinal: 3, Data: json.RawMessage(data)}
	for i, data := range context {
		record.Context = append(record.Context, evidence.Context{Ordinal: int64(i + 1), Data: json.RawMessage(data)})
	}
	return evidence.Stored{ID: evidence.EvidenceID(record), Scope: evidence.Scope(record), Record: record}
}

func piRecord(input string) evidence.Stored {
	return stored("pi", "pi-session-jsonl", `{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai-codex","model":"gpt-5","usage":{"input":`+input+`,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`, `{"type":"session","id":"session"}`)
}

func TestNativeComponentsAndMetadata(t *testing.T) {
	fixtures := []struct {
		name, provider, providerSource string
		record                         evidence.Stored
		cacheWrite, total              int64
	}{
		{"pi", "openai", "explicit", piRecord("10"), 4, 24},
		{"opencode-v1", "openai", "explicit", stored("opencode", "opencode-v1", `{"id":"message","session_id":"session","time_created":1700000000000,"data":{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5,"reasoning":2,"cache":{"read":3,"write":4}}}}`), 4, 24},
		{"opencode-v2", "openai", "explicit", stored("opencode", "opencode-v2", `{"id":"message","session_id":"session","type":"assistant","time_created":1700000000000,"data":{"model":{"providerID":"openai","modelID":"gpt-5"},"tokens":{"input":10,"output":5,"reasoning":2,"cache":{"read":3,"write":4}}}}`), 4, 24},
		{"claude-code", "maybe-anthropic", "inferred", stored("claude-code", "claude-code-session-jsonl", `{"type":"assistant","sessionId":"session","requestId":"request","timestamp":"2023-11-14T22:13:20Z","message":{"role":"assistant","id":"message","model":"claude-sonnet","usage":{"input_tokens":10,"output_tokens":7,"output_tokens_details":{"thinking_tokens":2},"cache_read_input_tokens":3,"cache_creation_input_tokens":4,"total_tokens":24}}}`), 4, 24},
		{"codex", "openai", "explicit", stored("codex", "codex-session-jsonl", `{"type":"event_msg","timestamp":"2023-11-14T22:13:20Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":13,"cached_input_tokens":3,"output_tokens":7,"reasoning_output_tokens":2,"total_tokens":20},"total_token_usage":{"input_tokens":13,"cached_input_tokens":3,"output_tokens":7,"reasoning_output_tokens":2,"total_tokens":20}}}}`, `{"type":"session_meta","payload":{"id":"session","model_provider":"openai"}}`, `{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-5"}}`), 0, 20},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			input := []evidence.Stored{fixture.record}
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := processor.Process(t.Context(), input)
			if err != nil || len(projection.Contributions) != 1 || len(projection.Estimates) != 0 {
				t.Fatalf("projection: %+v, error: %v", projection, err)
			}
			fact := projection.Contributions[0].Fact
			got := []int64{fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens, fact.TotalTokens}
			want := []int64{10, 5, 2, 3, fixture.cacheWrite, fixture.total}
			if !reflect.DeepEqual(got, want) || fact.Provider != fixture.provider || fact.ProviderSource != fixture.providerSource || fact.Session.NativeID != "session" {
				t.Fatalf("native components/metadata changed: %+v", fact)
			}
			again, err := processor.Process(t.Context(), input)
			if err != nil || !reflect.DeepEqual(projection, again) {
				t.Fatal("interpretation is nondeterministic", err)
			}
			after, err := json.Marshal(input)
			if err != nil || string(before) != string(after) {
				t.Fatal("interpretation mutated accepted evidence", err)
			}
		})
	}
}

func TestPortableIdentityDedupeAndConflictWithdrawal(t *testing.T) {
	one := piRecord("10")
	two := one
	two.Record.SourceID, two.Record.Lineage = "another-collector", "another-lineage"
	two.ID = evidence.EvidenceID(two.Record)
	projection, err := processor.Process(t.Context(), []evidence.Stored{one, two})
	if err != nil || len(projection.Contributions) != 1 || len(projection.Contributions[0].EvidenceIDs) != 2 {
		t.Fatal("same native usage counted twice", projection, err)
	}
	first, err := processor.Process(t.Context(), []evidence.Stored{one})
	if err != nil || first.Contributions[0].Fact.ID != projection.Contributions[0].Fact.ID {
		t.Fatal("collector-local identity changed native contribution", err)
	}
	conflict := piRecord("11")
	conflict.Record.Data = json.RawMessage(strings.ReplaceAll(string(conflict.Record.Data), `"totalTokens":24`, `"totalTokens":25`))
	conflict.ID = evidence.EvidenceID(conflict.Record)
	projection, err = processor.Process(t.Context(), []evidence.Stored{one, conflict})
	if err != nil || len(projection.Contributions) != 0 || len(projection.Estimates) != 1 {
		t.Fatal("conflicting native counters remained confirmed", projection, err)
	}
	for _, outcome := range projection.Outcomes {
		if outcome.Disposition != "ambiguous" || outcome.Code != "conflicting_native_identity" {
			t.Fatal("conflict outcome lost", outcome)
		}
	}
}

func TestMissingSessionAndCancellation(t *testing.T) {
	record := piRecord("10")
	record.Record.Context = nil
	projection, err := processor.Process(t.Context(), []evidence.Stored{record})
	if err != nil || len(projection.Contributions) != 0 || projection.Outcomes[0].Code != "unusable_usage" {
		t.Fatal("missing native session counted", projection, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := processor.Process(ctx, []evidence.Stored{record}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}

func TestLateCodexParentProvesCopyWithoutInflatingUsage(t *testing.T) {
	usage := `{"type":"event_msg","timestamp":"2026-01-01T00:00:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0,"total_tokens":120},"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0,"total_tokens":120}}}}`
	turn := `{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-5"}}`
	child := stored("codex", "codex-jsonl", usage, `{"type":"session_meta","payload":{"id":"child","model_provider":"openai","forked_from_id":"parent"}}`, turn)
	parent := stored("codex", "codex-jsonl", usage, `{"type":"session_meta","payload":{"id":"parent","model_provider":"openai"}}`, turn)
	projection, err := processor.Process(t.Context(), []evidence.Stored{child})
	if err != nil || len(projection.Contributions) != 0 || len(projection.Estimates) != 1 || projection.Estimates[0].Code != "missing_parent" || projection.Estimates[0].Fact.TotalTokens != 120 {
		t.Fatal("unproven child confirmed or ambiguity lost", projection, err)
	}
	for _, records := range [][]evidence.Stored{{child, parent}, {parent, child}} {
		projection, err = processor.Process(t.Context(), records)
		if err != nil || len(projection.Contributions) != 1 || len(projection.Estimates) != 0 || projection.Contributions[0].Fact.TotalTokens != 120 || projection.Contributions[0].Fact.Session.NativeID != "parent" {
			t.Fatal("copied parent inflated confirmed usage", projection, err)
		}
		for _, outcome := range projection.Outcomes {
			if outcome.EvidenceID == child.ID && outcome.Disposition != "duplicate" {
				t.Fatal("copy proof missing", outcome)
			}
		}
	}
}

func TestProcessorBoundary(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(name, "/internal/") && !strings.HasSuffix(name, "/evidence") && !strings.HasSuffix(name, "/publication") {
				t.Errorf("%s imports host/storage package %s", path, name)
			}
			for _, forbidden := range []string{"database", "os", "io", "net", "path", "syscall"} {
				if name == forbidden || strings.HasPrefix(name, forbidden+"/") {
					t.Errorf("%s imports impure package %s", path, name)
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if ok && pkg.Name == "time" && selector.Sel.Name == "Now" {
				t.Errorf("%s reads the wall clock", path)
			}
			return true
		})
	}
}
