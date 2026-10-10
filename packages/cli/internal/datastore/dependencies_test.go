package datastore

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func codexRecord(session, parent string) evidence.Record {
	parentField := ""
	if parent != "" {
		parentField = fmt.Sprintf(`,"forked_from_id":%q`, parent)
	}
	return evidence.Record{Harness: "codex", Format: "codex-jsonl", SourceID: session, Lineage: "lineage", Ordinal: 3,
		Data:    json.RawMessage(`{"type":"event_msg","timestamp":"2026-01-01T00:00:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0,"total_tokens":120},"total_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0,"total_tokens":120}}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"model_provider":"openai"%s}}`, session, parentField))}, {Ordinal: 2, Data: json.RawMessage(`{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-5"}}`)}}}
}

func TestLateParentReprocessesAmbiguousCopiedEvidence(t *testing.T) {
	store := testStore(t)
	child := codexRecord("child", "parent")
	first, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "child-stream", "batch", child))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics_confirmed"); got != 0 {
		t.Fatal("unproven child counted", got)
	}
	if got := total(t, store, "analytics_estimated"); got != 120 {
		t.Fatal("usable ambiguity missing", got)
	}
	before, err := store.Receipt(t.Context(), "child-stream", "batch")
	if err != nil || before.Processing.Items[0].Disposition != "ambiguous" {
		t.Fatal(before, err)
	}
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "parent-stream", "batch", codexRecord("parent", ""))); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Receipt(t.Context(), "child-stream", "batch")
	if err != nil || pending.Processing.Pending != 1 {
		t.Fatal("late dependency not invalidated", pending, err)
	}
	drain(t, store)
	if got := total(t, store, "analytics_confirmed"); got != 120 {
		t.Fatal("copied history inflated totals", got)
	}
	if got := total(t, store, "analytics_estimated"); got != 0 {
		t.Fatal("proven copy remained estimated", got)
	}
	after, err := store.Receipt(t.Context(), "child-stream", "batch")
	if err != nil || after.Receipt != first.Receipt || after.Processing.Items[0].Disposition != "duplicate" {
		t.Fatal(after, err)
	}
}

func TestConflictingNativeAliasesRemainEstimated(t *testing.T) {
	store := testStore(t)
	record := evidence.Record{Harness: "claude-code", Format: "claude-code-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"assistant","sessionId":"session","session_id":"other","requestId":"request","timestamp":"2026-01-01T00:00:00Z","message":{"id":"message","role":"assistant","model":"claude","usage":{"input_tokens":100,"output_tokens":20}}}`)}
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", record)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics_confirmed"); got != 0 {
		t.Fatal("contradictory identity counted", got)
	}
	status, err := store.Receipt(t.Context(), "stream", "batch")
	if err != nil || status.Processing.Items[0].Code != "conflicting_native_fields" {
		t.Fatal(status, err)
	}
}

func TestProcessingFailureBackoffAllowsIndependentScope(t *testing.T) {
	store := testStore(t)
	broken := piRecord("broken", 100)
	good := piRecord("good", 200)
	good.Context[0].Data = json.RawMessage(`{"type":"session","id":"z-good"}`)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", broken, good)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec(sqlutil.Bind("UPDATE raw_evidence SET record_json='invalid' WHERE scope=?"), evidence.Scope(broken)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProcessNext(t.Context()); err == nil {
		t.Fatal("corrupt stored evidence silently consumed")
	}
	var attempts int
	var code string
	if err := store.SQL().QueryRow(sqlutil.Bind("SELECT attempts,error_code FROM processing_scopes WHERE scope=?"), evidence.Scope(broken)).Scan(&attempts, &code); err != nil || attempts != 1 || code != "processing_failed" {
		t.Fatal(attempts, code, err)
	}
	if worked, err := store.ProcessNext(t.Context()); err != nil || !worked {
		t.Fatal("independent scope blocked", worked, err)
	}
	if got := total(t, store, "analytics_confirmed"); got != 220 {
		t.Fatal(got)
	}
}

// A deliberate new generation should immediately retry previously failed scopes.
func TestReprocessClearsObsoleteFailureBackoff(t *testing.T) {
	store := testStore(t)
	record := piRecord("message", 100)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", record)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec(sqlutil.Bind("UPDATE processing_scopes SET attempts=6,retry_at_ms=9007199254740991,error_code='processing_failed'")); err != nil {
		t.Fatal(err)
	}
	if worked, err := store.ProcessNext(t.Context()); err != nil || worked {
		t.Fatal("backoff ignored", worked, err)
	}
	if _, err := store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	status, err := store.Receipt(t.Context(), "stream", "batch")
	if err != nil || status.Processing.Pending != 0 || total(t, store, "analytics_confirmed") != 120 {
		t.Fatal("new generation retained old backoff", status, err)
	}
}
