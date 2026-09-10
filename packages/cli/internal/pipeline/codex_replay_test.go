package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func codexReplayHeader(session, parent string) string {
	if parent == "" {
		return fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"model_provider":"openai"}}`, session)
	}
	return fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"model_provider":"openai","forked_from_id":%q}}`, session, parent)
}

func codexReplayTurn(turn string) string {
	return fmt.Sprintf(`{"type":"turn_context","payload":{"turn_id":%q,"model":"gpt-5"}}`, turn)
}

func codexReplayUsage(second, last, total int) string {
	return fmt.Sprintf(`{"timestamp":"2026-01-01T00:00:%02dZ","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d},"total_token_usage":{"input_tokens":%d}}}}`, second, last, total)
}

func codexReplaySource(t *testing.T, root, session string, lines ...string) string {
	t.Helper()
	path := filepath.Join(root, "rollout-2026-01-01T00-00-00-"+session+".jsonl")
	writeJSONL(t, path, lines...)
	return path
}

func codexReplayDiscover(t *testing.T, root string) (*codexJSONLAdapter, map[string]Source) {
	t.Helper()
	a := &codexJSONLAdapter{}
	sources, err := a.Discover(context.Background(), DiscoverOptions{SourceDir: root})
	if err != nil {
		t.Fatal(err)
	}
	bySession := make(map[string]Source)
	for _, source := range sources {
		bySession[a.metadata[source.Path].sessionID] = source
	}
	return a, bySession
}

func codexReplayParse(t *testing.T, a *codexJSONLAdapter, source Source) ([]RawTokenFact, []Diagnostic) {
	t.Helper()
	facts, diagnostics, err := a.Parse(context.Background(), source, SyncOptions{Parser: codexJSONLParserV2, Collector: "fixture", Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return facts, diagnostics
}

func hasCodexDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

// Cancel at deterministic parser checkpoints, without timing or goroutines.
type codexCancellationContext struct {
	context.Context
	check func()
}

func (ctx codexCancellationContext) Err() error {
	ctx.check()
	return ctx.Context.Err()
}

func TestCodexMetadataScanCancellation(t *testing.T) {
	root := t.TempDir()
	path := codexReplaySource(t, root, "filename", `{}`, `{}`, `{}`, codexReplayHeader("late-header", ""))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	checks := 0
	ctx := codexCancellationContext{Context: base, check: func() {
		checks++
		if checks == 3 {
			cancel()
		}
	}}
	metadata, err := codexReadSourceMetadata(ctx, Source{Path: path})
	if !errors.Is(err, context.Canceled) || metadata.sessionID != "filename" {
		t.Fatalf("scan reached late header despite cancellation: %+v %v", metadata, err)
	}
}

func TestCodexDiscoveryMetadataLoopCancellation(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "session", codexReplayHeader("session", ""))
	a := &codexJSONLAdapter{}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := codexCancellationContext{Context: base, check: func() {
		// Discovery allocates the index after walking the source boundary.
		if a.metadata != nil {
			cancel()
		}
	}}
	sources, err := a.Discover(ctx, DiscoverOptions{SourceDir: root})
	if !errors.Is(err, context.Canceled) || len(sources) != 0 || len(a.metadata) != 0 {
		t.Fatalf("metadata discovery ignored cancellation: %+v %v", sources, err)
	}
}

func TestCodexCanceledParseOverridesCachedResults(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("empty=%t/cached=%t", empty, cached), func(t *testing.T) {
				var lines []string
				if !empty {
					lines = []string{codexReplayHeader("session", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10)}
				}
				path := codexReplaySource(t, t.TempDir(), "session", lines...)
				source := Source{Path: path, Kind: codexSessionJSONLSourceKind}
				a := &codexJSONLAdapter{}
				if cached {
					codexReplayParse(t, a, source)
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				facts, diagnostics, err := a.Parse(ctx, source, SyncOptions{})
				if !errors.Is(err, context.Canceled) || len(facts) != 0 || len(diagnostics) != 0 {
					t.Fatalf("canceled parse returned successful data: %+v %+v %v", facts, diagnostics, err)
				}
				if _, found := a.cache[path]; found != cached {
					t.Fatal("cancellation changed parse cache")
				}
				// Cancellation must not poison the adapter for subsequent work.
				codexReplayParse(t, a, source)
			})
		}
	}
}

func TestCodexAncestorCancellationNeverBecomesReadWarning(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10))
	codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("turn"), codexReplayUsage(2, 10, 10))
	// Sweep cancellation checkpoints through child parsing, ancestor parsing,
	// replay resolution and cache publication. Stop once parsing completes.
	for cancelAt := 1; ; cancelAt++ {
		a, sources := codexReplayDiscover(t, root)
		base, cancel := context.WithCancel(context.Background())
		checks := 0
		ctx := codexCancellationContext{Context: base, check: func() {
			checks++
			if checks == cancelAt {
				cancel()
			}
		}}
		facts, diagnostics, err := a.Parse(ctx, sources["child"], SyncOptions{})
		canceled := base.Err() != nil
		cancel()
		if !canceled {
			if err != nil || len(facts) != 1 || *facts[0].SessionID != "parent" {
				t.Fatalf("uncanceled replay failed: %+v %v", facts, err)
			}
			break
		}
		if !errors.Is(err, context.Canceled) || len(facts) != 0 || len(diagnostics) != 0 {
			t.Fatalf("checkpoint %d downgraded cancellation: %+v %+v %v", cancelAt, facts, diagnostics, err)
		}
		if _, found := a.cache[sources["child"].Path]; found {
			t.Fatalf("checkpoint %d cached canceled child", cancelAt)
		}
	}
}

func TestCodexMetadataReadFailureRemainsPerSource(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "rollout-2026-01-01T00-00-00-missing.jsonl"), Kind: codexSessionJSONLSourceKind}
	metadata, err := codexReadSourceMetadata(context.Background(), source)
	if err != nil || metadata.sessionID != "missing" {
		t.Fatalf("header lookup promoted ordinary read failure: %+v %v", metadata, err)
	}
	a := &codexJSONLAdapter{}
	if _, _, err := a.Parse(context.Background(), source, SyncOptions{}); !os.IsNotExist(err) {
		t.Fatalf("source ingestion lost read failure: %v", err)
	}
}

func TestCodexReplayRetimestampedHistoryReusesOriginalFacts(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("reset=%t", reset), func(t *testing.T) {
			root := t.TempDir()
			codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("parent-turn"), codexReplayUsage(2, 10, 10), codexReplayUsage(3, 20, 30))
			total := 37
			if reset {
				total = 7
			}
			codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("parent-turn"), codexReplayUsage(12, 10, 10), codexReplayUsage(13, 20, 30), codexReplayTurn("child-turn"), codexReplayUsage(14, 7, total))
			for _, childFirst := range []bool{true, false} {
				a, sources := codexReplayDiscover(t, root)
				if !sources["child"].AlwaysRefresh || sources["parent"].AlwaysRefresh {
					t.Fatal("fork refresh policy missing")
				}
				var child, parent []RawTokenFact
				var childDiagnostics, parentDiagnostics []Diagnostic
				if childFirst {
					child, childDiagnostics = codexReplayParse(t, a, sources["child"])
					parent, parentDiagnostics = codexReplayParse(t, a, sources["parent"])
				} else {
					parent, parentDiagnostics = codexReplayParse(t, a, sources["parent"])
					child, childDiagnostics = codexReplayParse(t, a, sources["child"])
				}
				wantDiagnostics := []Diagnostic{{Harness: HarnessCodex, Severity: "info", Code: "codex_jsonl_replay_resolved", Message: "matched inherited Codex token usage to original session facts"}}
				if !reflect.DeepEqual(childDiagnostics, wantDiagnostics) {
					t.Fatalf("resolved diagnostic changed: %+v", childDiagnostics)
				}
				if len(parentDiagnostics) != 0 {
					t.Fatalf("ordinary parent acquired replay diagnostics: %+v", parentDiagnostics)
				}
				if len(child) != 3 || len(parent) != 2 || !reflect.DeepEqual(child[:2], parent) {
					t.Fatalf("original metadata lost: child=%+v parent=%+v", child, parent)
				}
				if *child[2].SessionID != "child" || *child[2].InputTokens != 7 || child[0].DedupeKey != "" {
					t.Fatalf("child attribution: %+v", child)
				}
			}
		})
	}
}

func TestCodexReplayChainChildrenAndParseCache(t *testing.T) {
	root := t.TempDir()
	parentPath := codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
	childPath := codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("original"), codexReplayUsage(2, 10, 10), codexReplayTurn("own"), codexReplayUsage(3, 7, 17))
	codexReplaySource(t, root, "sibling", codexReplayHeader("sibling", "parent"), codexReplayTurn("original"), codexReplayUsage(4, 10, 10))
	codexReplaySource(t, root, "grandchild", codexReplayHeader("grandchild", "child"), codexReplayTurn("original"), codexReplayUsage(5, 10, 10), codexReplayTurn("own"), codexReplayUsage(6, 7, 17))
	a, sources := codexReplayDiscover(t, root)
	grandchild, grandchildDiagnostics := codexReplayParse(t, a, sources["grandchild"])
	// Removing files after their first linked parse proves later reads use the
	// cache, including normal ingestion after optional ancestor parsing.
	for _, path := range []string{parentPath, childPath} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	child, childDiagnostics := codexReplayParse(t, a, sources["child"])
	parent, _ := codexReplayParse(t, a, sources["parent"])
	sibling, siblingDiagnostics := codexReplayParse(t, a, sources["sibling"])
	for _, diagnostics := range [][]Diagnostic{grandchildDiagnostics, childDiagnostics, siblingDiagnostics} {
		if len(diagnostics) != 1 || !hasCodexDiagnostic(diagnostics, "codex_jsonl_replay_resolved") {
			t.Fatalf("resolved diagnostic must occur once per source: %+v", diagnostics)
		}
	}
	if len(parent) != 1 || len(child) != 2 || !reflect.DeepEqual(grandchild, child) || !reflect.DeepEqual(parent, sibling) {
		t.Fatal("ancestor originals differ across children")
	}
	updated, _, err := a.Parse(context.Background(), sources["child"], SyncOptions{Parser: "new-observation", Collector: "new-collector", Now: time.Unix(2, 0)})
	if err != nil || updated[0].Parser != "new-observation" || updated[0].ObservedAtMs != 2000 || updated[0].Collector != "new-collector" {
		t.Fatalf("cached observation provenance not updated: %+v %v", updated, err)
	}
}

func TestCodexReplayMissingParentResetsOnlyAtExplicitTurn(t *testing.T) {
	root := t.TempDir()
	path := codexReplaySource(t, root, "child", codexReplayHeader("child", "missing"), codexReplayTurn("inherited"), codexReplayUsage(1, 20, 30), codexReplayUsage(2, 7, 7), codexReplayTurn("own"), codexReplayUsage(3, 7, 7), codexReplayUsage(4, 7, 7))
	// Zero-initialized direct parsing has no parent-discovery context.
	a := &codexJSONLAdapter{}
	facts, diagnostics := codexReplayParse(t, a, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	if len(facts) != 2 || *facts[1].InputTokens != 7 {
		t.Fatalf("new turn lost to inherited counters: %+v", facts)
	}
	for _, code := range []string{"codex_jsonl_replay_missing_parent", "codex_jsonl_replay_boundary_uncertain", "codex_jsonl_stale_token_snapshot", "codex_jsonl_duplicate_token_snapshot"} {
		if !hasCodexDiagnostic(diagnostics, code) {
			t.Fatalf("missing %s: %+v", code, diagnostics)
		}
	}
}

func TestCodexReplayRequiresCompleteExactIdentity(t *testing.T) {
	cases := []struct {
		name  string
		turn  string
		usage string
		want  int
	}{
		{"different turn", codexReplayTurn("other"), codexReplayUsage(2, 10, 10), 1},
		{"different model", `{"type":"turn_context","payload":{"turn_id":"original","model":"other"}}`, codexReplayUsage(2, 10, 10), 1},
		{"missing turn", `{"type":"turn_context","payload":{"model":"gpt-5"}}`, codexReplayUsage(2, 10, 10), 1},
		{"missing cumulative", codexReplayTurn("original"), `{"timestamp":"2026-01-01T00:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10}}}}`, 1},
		{"presence differs", codexReplayTurn("original"), strings.Replace(codexReplayUsage(2, 10, 10), `"last_token_usage":{"input_tokens":10}`, `"last_token_usage":{"input_tokens":10,"output_tokens":0}`, 1), 1},
		{"cumulative presence differs", codexReplayTurn("original"), strings.Replace(codexReplayUsage(2, 10, 10), `"total_token_usage":{"input_tokens":10}`, `"total_token_usage":{"input_tokens":10,"output_tokens":0}`, 1), 1},
		{"cache alias presence differs", codexReplayTurn("original"), strings.Replace(codexReplayUsage(2, 10, 10), `"last_token_usage":{"input_tokens":10}`, `"last_token_usage":{"input_tokens":10,"cache_read_input_tokens":0}`, 1), 1},
		{"numeric string", codexReplayTurn("original"), strings.ReplaceAll(codexReplayUsage(2, 10, 10), `:10`, `:"10"`), 1},
		{"fraction rejected", codexReplayTurn("original"), strings.ReplaceAll(codexReplayUsage(2, 10, 10), `:10`, `:10.5`), 0},
		{"overflow rejected", codexReplayTurn("original"), strings.ReplaceAll(codexReplayUsage(2, 10, 10), `:10`, `:9223372036854775808`), 0},
		{"negative retained unproven", codexReplayTurn("original"), codexReplayUsage(2, -10, -10), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
			codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), tc.turn, tc.usage)
			a, sources := codexReplayDiscover(t, root)
			facts, _ := codexReplayParse(t, a, sources["child"])
			if len(facts) != tc.want {
				t.Fatalf("facts=%+v", facts)
			}
			for _, fact := range facts {
				if *fact.SessionID != "child" {
					t.Fatalf("unproven history attributed to parent: %+v", fact)
				}
			}
		})
	}
}

func TestCodexReplayUnrelatedEqualCountersStayIndependent(t *testing.T) {
	root := t.TempDir()
	for _, session := range []string{"one", "two"} {
		codexReplaySource(t, root, session, codexReplayHeader(session, ""), codexReplayTurn("same-turn"), codexReplayUsage(1, 10, 10))
	}
	a, sources := codexReplayDiscover(t, root)
	one, _ := codexReplayParse(t, a, sources["one"])
	two, _ := codexReplayParse(t, a, sources["two"])
	if len(one) != 1 || len(two) != 1 || rawFactKey(one[0]) == rawFactKey(two[0]) {
		t.Fatal("unrelated sessions collapsed")
	}
}

func TestCodexReplayRequiresAcceptedParentCandidate(t *testing.T) {
	root := t.TempDir()
	// This exact snapshot occurs in the parent but was rejected as stale.
	codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("original"), codexReplayUsage(1, 20, 30), codexReplayUsage(2, 7, 7))
	codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("original"), codexReplayUsage(3, 7, 7))
	a, sources := codexReplayDiscover(t, root)
	child, _ := codexReplayParse(t, a, sources["child"])
	if len(child) != 1 || *child[0].SessionID != "child" {
		t.Fatal("rejected parent candidate proved replay")
	}
}

func TestCodexReplayAmbiguousLogicalOriginalsAreRetained(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "root", codexReplayHeader("root", ""), codexReplayTurn("root-turn"), codexReplayUsage(1, 1, 1))
	// A reused turn ID after a fork counter reset produces two accepted,
	// countable originals with identical replay metadata but different times.
	codexReplaySource(t, root, "parent", codexReplayHeader("parent", "root"), codexReplayTurn("reused"), codexReplayUsage(2, 10, 10), codexReplayTurn("other"), codexReplayUsage(3, 20, 30), codexReplayTurn("reused"), codexReplayUsage(4, 10, 10))
	codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("reused"), codexReplayUsage(5, 10, 10))
	for _, parentFirst := range []bool{false, true} {
		a, sources := codexReplayDiscover(t, root)
		if parentFirst {
			codexReplayParse(t, a, sources["parent"])
		}
		facts, diagnostics := codexReplayParse(t, a, sources["child"])
		if len(facts) != 1 || *facts[0].SessionID != "child" || !hasCodexDiagnostic(diagnostics, "codex_jsonl_replay_ambiguous_fact") {
			t.Fatalf("ambiguous originals collapsed: %+v %+v", facts, diagnostics)
		}
	}
}

func TestCodexReplayDiscoveryBoundariesAndReferenceForms(t *testing.T) {
	for _, reference := range []string{
		`"forked_from_id":"parent"`,
		`"parent_thread_id":"parent"`,
		`"source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}`,
		`"source":{"subagent":"review"}`,
	} {
		t.Run(reference, func(t *testing.T) {
			root := t.TempDir()
			codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
			childRoot := filepath.Join(root, "bounded")
			header := fmt.Sprintf(`{"type":"session_meta","payload":{"id":"child","model_provider":"openai",%s}}`, reference)
			codexReplaySource(t, childRoot, "child", header, codexReplayTurn("original"), codexReplayUsage(2, 10, 10))
			a, sources := codexReplayDiscover(t, childRoot)
			if len(sources) != 1 || !sources["child"].AlwaysRefresh {
				t.Fatal("discovery escaped root or lost fork flag")
			}
			child, diagnostics := codexReplayParse(t, a, sources["child"])
			if len(child) != 1 || *child[0].SessionID != "child" || !hasCodexDiagnostic(diagnostics, "codex_jsonl_replay_missing_parent") {
				t.Fatal("used undiscovered parent")
			}
		})
	}
}

func TestCodexReplayNullReferencesAndOrdinalDoNotInferFork(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "ordinary", `{"type":"session_meta","payload":{"id":"ordinary","model_provider":"openai","forked_from_id":null,"parent_thread_id":null,"source":{"subagent":null},"ordinal":5,"history_mode":"replay"}}`, codexReplayTurn("turn"), codexReplayUsage(1, 20, 30), codexReplayUsage(2, 7, 7))
	a, sources := codexReplayDiscover(t, root)
	facts, diagnostics := codexReplayParse(t, a, sources["ordinary"])
	if sources["ordinary"].AlwaysRefresh || len(facts) != 1 || !hasCodexDiagnostic(diagnostics, "codex_jsonl_stale_token_snapshot") {
		t.Fatal("inferred replay from ordinal or null references")
	}
}

func TestCodexReplayMissingProviderAndModelCannotProveCopies(t *testing.T) {
	for _, missing := range []string{"provider", "model"} {
		t.Run(missing, func(t *testing.T) {
			root := t.TempDir()
			header := codexReplayHeader("parent", "")
			turn := codexReplayTurn("original")
			if missing == "provider" {
				header = strings.Replace(header, `,"model_provider":"openai"`, "", 1)
			}
			if missing == "model" {
				turn = strings.Replace(turn, `,"model":"gpt-5"`, "", 1)
			}
			codexReplaySource(t, root, "parent", header, turn, codexReplayUsage(1, 10, 10))
			codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("original"), codexReplayUsage(2, 10, 10), codexReplayTurn("own"), codexReplayUsage(3, 7, 7))
			a, sources := codexReplayDiscover(t, root)
			child, _ := codexReplayParse(t, a, sources["child"])
			if len(child) != 2 || *child[0].SessionID != "child" || *child[1].InputTokens != 7 {
				t.Fatal("missing parent metadata proved replay or suppressed own turn")
			}
		})
	}
}

func TestCodexFilenameFallbackAndMissingTurnIdentity(t *testing.T) {
	root := t.TempDir()
	path := codexReplaySource(t, root, "fallback", `{"type":"turn_context","payload":{"model":"gpt-5"}}`, codexReplayUsage(1, 10, 10))
	facts, _ := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	if len(facts) != 1 || *facts[0].SessionID != "fallback" || *facts[0].MessageID != "line:2:1767225601000:42e9129b7e0064efbe1992158aadc330dd6dfe2078f17617d3ea381b734035cd" {
		t.Fatalf("filename/line fallback changed: %+v", facts)
	}
}

func TestCodexFirstUsableMetadataOwnsFactsBeforeHeader(t *testing.T) {
	root := t.TempDir()
	path := codexReplaySource(t, root, "filename", codexReplayTurn("turn"), codexReplayUsage(1, 10, 10), codexReplayHeader("owner", ""), codexReplayUsage(2, 20, 30), codexReplayHeader("copied", ""))
	facts, diagnostics := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	if len(facts) != 2 || !hasCodexDiagnostic(diagnostics, "codex_jsonl_session_id_mismatch") || !hasCodexDiagnostic(diagnostics, "codex_jsonl_multiple_session_meta") {
		t.Fatalf("metadata ownership diagnostics missing: %+v %+v", facts, diagnostics)
	}
	for _, fact := range facts {
		if *fact.SessionID != "owner" || fact.SourceID != stableHash("codex-session:owner") {
			t.Fatalf("first metadata did not own complete source: %+v", fact)
		}
	}
}

func TestCodexRejectsTrailingJSONAndPreservesIntegerPrecision(t *testing.T) {
	root := t.TempDir()
	large := strings.ReplaceAll(codexReplayUsage(1, 10, 10), ":10", ":9007199254740993")
	path := codexReplaySource(t, root, "session", codexReplayHeader("session", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10)+" garbage", large)
	facts, diagnostics := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	if len(facts) != 1 || *facts[0].InputTokens != 9007199254740993 || !hasCodexDiagnostic(diagnostics, "codex_jsonl_parse_error") {
		t.Fatalf("invalid JSON or rounded integer accepted: %+v %+v", facts, diagnostics)
	}
}

func TestCodexReplayMissingAmbiguousAndCyclicAncestry(t *testing.T) {
	for _, scenario := range []string{"missing", "conflict", "copy", "cycle", "invalid-reference", "read-failure"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			parentHeader := codexReplayHeader("parent", "")
			childHeader := codexReplayHeader("child", "parent")
			code := "codex_jsonl_replay_ambiguous_parent"
			switch scenario {
			case "cycle":
				parentHeader = codexReplayHeader("parent", "child")
				code = "codex_jsonl_replay_cycle"
			case "conflict":
				childHeader = `{"type":"session_meta","payload":{"id":"child","model_provider":"openai","forked_from_id":"parent","parent_thread_id":"other"}}`
			case "invalid-reference":
				childHeader = `{"type":"session_meta","payload":{"id":"child","model_provider":"openai","forked_from_id":"parent","parent_thread_id":3}}`
			case "missing":
				code = "codex_jsonl_replay_missing_parent"
			case "read-failure":
				code = "codex_jsonl_replay_parent_read_error"
			}
			parentPath := codexReplaySource(t, root, "parent", parentHeader, codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
			codexReplaySource(t, root, "child", childHeader, codexReplayTurn("original"), codexReplayUsage(2, 10, 10))
			if scenario == "copy" {
				codexReplaySource(t, root, "copy", parentHeader, codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
			}
			if scenario == "missing" {
				if err := os.Remove(parentPath); err != nil {
					t.Fatal(err)
				}
			}
			a, sources := codexReplayDiscover(t, root)
			if scenario == "read-failure" {
				if err := os.Remove(parentPath); err != nil {
					t.Fatal(err)
				}
			}
			facts, diagnostics := codexReplayParse(t, a, sources["child"])
			if len(facts) != 1 || *facts[0].SessionID != "child" || !hasCodexDiagnostic(diagnostics, code) {
				t.Fatalf("uncertain child lost: facts=%+v diagnostics=%+v", facts, diagnostics)
			}
			if scenario == "read-failure" {
				if _, _, err := a.Parse(context.Background(), sources["parent"], SyncOptions{}); err == nil {
					t.Fatal("real parent ingestion failure swallowed")
				}
			}
		})
	}
}

func TestCodexSnapshotIdentityAndPendingBackfill(t *testing.T) {
	root := t.TempDir()
	for _, pending := range []bool{false, true} {
		lines := []string{codexReplayHeader("session", "")}
		if !pending {
			lines = append(lines, codexReplayTurn("turn"))
		}
		lines = append(lines, codexReplayUsage(1, 10, 10), codexReplayUsage(1, 20, 30))
		if pending {
			lines = append(lines, codexReplayTurn("turn"))
		}
		path := codexReplaySource(t, root, "session", lines...)
		facts, _ := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
		if len(facts) != 2 || *facts[0].MessageID == *facts[1].MessageID {
			t.Fatalf("same-ms snapshots collapsed: %+v", facts)
		}
		// Literal protocol examples pin the typed, presence-preserving snapshot
		// format independently of parsing/backfill code.
		want := []string{
			"turn:1767225601000:42e9129b7e0064efbe1992158aadc330dd6dfe2078f17617d3ea381b734035cd",
			"turn:1767225601000:3a6e5eb1e1f3c3cf81f7f0f2132e0f003cd339443a168baabb98eca22a301b74",
		}
		for i, fact := range facts {
			if *fact.MessageID != want[i] {
				t.Fatalf("identity %d: got %s want %s", i, *fact.MessageID, want[i])
			}
		}
	}
}

func TestCodexNoCumulativeKeepsOriginalLineDuringBackfill(t *testing.T) {
	root := t.TempDir()
	usage := `{"timestamp":"2026-01-01T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10}}}}`
	path := codexReplaySource(t, root, "session", codexReplayHeader("session", ""), usage, usage, codexReplayTurn("turn"))
	facts, _ := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	if len(facts) != 2 || !strings.HasSuffix(*facts[0].MessageID, ":line:2") || !strings.HasSuffix(*facts[1].MessageID, ":line:3") {
		t.Fatalf("pending line discriminator lost: %+v", facts)
	}
}

func TestCodexReplayOwnershipAndMetadataPrivacy(t *testing.T) {
	root := t.TempDir()
	path := codexReplaySource(t, root, "child", codexReplayHeader("child", "missing"), codexReplayHeader("copied", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10), `{"type":"response_item","payload":{"text":"PRIVATE-CONTENT","arguments":"PRIVATE-CONTENT"}}`)
	facts, diagnostics := codexReplayParse(t, &codexJSONLAdapter{}, Source{Path: path, Kind: codexSessionJSONLSourceKind})
	encoded, err := json.Marshal(struct {
		Facts       []RawTokenFact
		Diagnostics []Diagnostic
	}{facts, diagnostics})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || *facts[0].SessionID != "child" || strings.Contains(string(encoded), "PRIVATE-CONTENT") || strings.Contains(string(encoded), root) || facts[0].MetadataJSON != nil {
		t.Fatalf("ownership/privacy failure: %s", encoded)
	}
}

func TestCodexAlwaysRefreshBypassesStateForDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		skip, err := shouldSkipSourceRefresh(context.Background(), nil, Source{AlwaysRefresh: true}, SyncOptions{DryRun: dryRun}, sourceRefreshMetadata{}, true)
		if err != nil || skip {
			t.Fatalf("fork skip=%t err=%v", skip, err)
		}
	}
}

func TestCodexReplayConformance(t *testing.T) {
	assertConformanceFixture(t, filepath.Join("testdata", "conformance", "codex-replay"), Summary{RequestedHarnesses: 4, Synced: 1, Skipped: 3, RawFacts: 3, Observations: 5, Canonical: 3, Diagnostics: 1})
}

func TestCodexReplayRepeatSyncAndDeferredNormalize(t *testing.T) {
	for _, normalize := range []bool{false, true} {
		t.Run(fmt.Sprintf("normalize=%t", normalize), func(t *testing.T) {
			root := t.TempDir()
			copyFixtureDir(t, filepath.Join("testdata", "conformance", "codex-replay", "source"), root)
			options := SyncOptions{DBPath: filepath.Join(t.TempDir(), "usage.sqlite"), SourceDir: root, Harnesses: []Harness{HarnessCodex}, Normalize: normalize, Now: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)}
			if _, err := Sync(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			database := openTestDB(t, options.DBPath)
			defer database.Close()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 3)
			if !normalize {
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 0)
				if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: options.DBPath}); err != nil {
					t.Fatal(err)
				}
			}
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 3)
			assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 37)
			options.Now = options.Now.Add(time.Hour)
			summary, err := Sync(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if summary.RawFacts != 0 || summary.Canonical != 0 {
				t.Fatalf("repeat duplicated facts: %+v", summary)
			}
			assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 3)
			assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 37)
		})
	}
}

func TestCodexReplaySkippedParentStillSuppliesOriginals(t *testing.T) {
	root := t.TempDir()
	parentPath := codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("original"), codexReplayUsage(1, 10, 10))
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(parentPath, old, old); err != nil {
		t.Fatal(err)
	}
	options := SyncOptions{DBPath: filepath.Join(t.TempDir(), "usage.sqlite"), SourceDir: root, Harnesses: []Harness{HarnessCodex}, Normalize: true, Now: old.Add(7 * 24 * time.Hour)}
	if _, err := Sync(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	childPath := codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("original"), codexReplayUsage(2, 10, 10), codexReplayTurn("own"), codexReplayUsage(3, 7, 17))
	if err := os.Chtimes(childPath, old, old); err != nil {
		t.Fatal(err)
	}
	options.Now = options.Now.Add(time.Hour)
	if _, err := Sync(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	database := openTestDB(t, options.DBPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 17)
	options.Now = options.Now.Add(time.Hour)
	summary, err := Sync(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Observations != 2 || summary.RawFacts != 0 {
		t.Fatalf("child was not refreshed independently of skipped parent: %+v", summary)
	}
	options.DryRun = true
	dry, err := Sync(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if dry.RawFacts != 2 {
		t.Fatalf("dry-run skipped old fork: %+v", dry)
	}
}
