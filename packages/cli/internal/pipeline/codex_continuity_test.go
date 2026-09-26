package pipeline

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func codexPreparedState(t *testing.T, prepared preparedSource) sourceState {
	t.Helper()
	if prepared.parseErr != nil || prepared.cursor == nil {
		t.Fatalf("fork did not establish continuity: %+v", prepared)
	}
	return sourceState{hasCursor: true, cursor: *prepared.cursor, hasRefresh: true,
		refresh: sourceRefreshState{collector: prepared.cursor.collector, parser: prepared.cursor.parser,
			sourceMtimeMs: prepared.metadata.mtimeMs, sourceSizeBytes: prepared.metadata.sizeBytes}}
}

func TestCodexForkContinuityUsesCompleteAncestry(t *testing.T) {
	root := t.TempDir()
	grandparent := codexReplaySource(t, root, "grandparent", codexReplayHeader("grandparent", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10))
	codexReplaySource(t, root, "parent", codexReplayHeader("parent", "grandparent"), codexReplayTurn("turn"), codexReplayUsage(2, 10, 10))
	codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("turn"), codexReplayUsage(3, 10, 10))
	options := SyncOptions{Collector: "fixture", Parser: codexJSONLParserV3, Now: time.Unix(1, 0), locationResolver: &locationResolver{}}
	adapter, sources := codexReplayDiscover(t, root)
	prepared := prepareCodexSource(context.Background(), adapter, sources["child"], options, sourceState{})
	state := codexPreparedState(t, prepared)
	if len(prepared.facts) != 1 || *prepared.facts[0].SessionID != "grandparent" || len(prepared.dependencies) != 3 {
		t.Fatalf("wrong initial replay proof: %+v", prepared)
	}
	adapter, sources = codexReplayDiscover(t, root)
	unchanged := prepareCodexSource(context.Background(), adapter, sources["child"], options, state)
	if !unchanged.unchanged || len(adapter.cache) != 0 {
		t.Fatalf("unchanged ancestry reparsed: %+v", unchanged)
	}
	info, err := os.Stat(grandparent)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(grandparent)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := strings.ReplaceAll(string(data), `"input_tokens":10`, `"input_tokens":11`)
	if err := os.WriteFile(grandparent, []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(grandparent, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	adapter, sources = codexReplayDiscover(t, root)
	changed := prepareCodexSource(context.Background(), adapter, sources["child"], options, state)
	if changed.unchanged || changed.cursor == nil || changed.cursor.boundaryHash == state.cursor.boundaryHash {
		t.Fatalf("same-size ancestor rewrite reused stale marker: %+v", changed)
	}
	if err := os.Remove(grandparent); err != nil {
		t.Fatal(err)
	}
	adapter, sources = codexReplayDiscover(t, root)
	missing := prepareCodexSource(context.Background(), adapter, sources["child"], options, state)
	if missing.unchanged || missing.cursor != nil || !hasCodexDiagnostic(missing.diagnostics, "codex_jsonl_replay_missing_parent") {
		t.Fatalf("missing ancestry established marker: %+v", missing)
	}
}

func TestCodexForkContinuityRejectsIncompleteOrConflictingAncestry(t *testing.T) {
	for _, test := range []struct{ name, parent, child string }{
		{"incomplete tail", codexReplayHeader("parent", "") + "\n{", codexReplayHeader("child", "parent")},
		{"cycle", codexReplayHeader("parent", "child"), codexReplayHeader("child", "parent")},
		{"conflicting references", codexReplayHeader("parent", ""), `{"type":"session_meta","payload":{"id":"child","forked_from_id":"parent","parent_thread_id":"other"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			parentPath := codexReplaySource(t, root, "parent", test.parent)
			if test.name == "incomplete tail" {
				if err := os.WriteFile(parentPath, []byte(test.parent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			codexReplaySource(t, root, "child", test.child, codexReplayTurn("turn"), codexReplayUsage(1, 10, 10))
			adapter, sources := codexReplayDiscover(t, root)
			prepared := prepareCodexSource(context.Background(), adapter, sources["child"], SyncOptions{Collector: "fixture", Parser: codexJSONLParserV3}, sourceState{})
			if prepared.unchanged || prepared.cursor != nil {
				t.Fatalf("uncertain ancestry established marker: %+v", prepared)
			}
		})
	}
}

func TestCodexParallelParsingSharesAncestors(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "parent", codexReplayHeader("parent", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10))
	codexReplaySource(t, root, "child", codexReplayHeader("child", "parent"), codexReplayTurn("turn"), codexReplayUsage(2, 10, 10))
	codexReplaySource(t, root, "sibling", codexReplayHeader("sibling", "parent"), codexReplayTurn("turn"), codexReplayUsage(3, 10, 10))
	adapter, sources := codexReplayDiscover(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	for range 8 {
		for _, session := range []string{"parent", "child", "sibling"} {
			workers.Go(func() {
				facts, _, err := adapter.Parse(ctx, sources[session], SyncOptions{Collector: "fixture", Parser: codexJSONLParserV3, locationResolver: &locationResolver{}})
				if err != nil || len(facts) != 1 || *facts[0].SessionID != "parent" {
					t.Errorf("parallel replay failed: %+v %v", facts, err)
				}
			})
		}
	}
	workers.Wait()
	if len(adapter.cache) != 3 {
		t.Fatalf("ancestor computations not shared: %d", len(adapter.cache))
	}
}

func TestLocationResolverParallelInspection(t *testing.T) {
	resolver := &locationResolver{}
	path := t.TempDir()
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() { resolver.inspect(context.Background(), path) })
	}
	workers.Wait()
	if len(resolver.git) != 1 {
		t.Fatalf("inspection not shared: %d", len(resolver.git))
	}
}
