package main

import (
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
)

func TestNativeQueryPreservesFiltersAndUsesConfirmedModelTotals(t *testing.T) {
	query := nativeQuery(filters{Period: "custom", From: "2026-03-01", To: "2026-03-31", Provider: "unknown", Model: "model/a", Harness: "claude-code"})
	if _, err := application.NormalizeQuery(query); err != nil {
		t.Fatal(err)
	}
	want := analytics.Selection{Period: "all", Bucket: "day", From: "2026-03-01", To: "2026-03-31", Providers: []string{"unknown"}, Models: []string{"model/a"}, Harnesses: []string{"claude-code"}}
	if !reflect.DeepEqual(query.Selection, want) || query.Tab != "models" || query.Quality == "estimated" {
		t.Fatalf("query %+v", query)
	}
}

func TestNativeSummaryUsesFullFilterMetadataAndEveryTokenComponent(t *testing.T) {
	data := application.Usage{Dashboard: analytics.Dashboard{
		Summary: analytics.ViewerSummaryRow{TotalTokens: 1000, InputTokens: 100, OutputTokens: 200, ReasoningTokens: 300, CacheReadTokens: 150, CacheWriteTokens: 250, SessionCount: 12},
		Rows:    []analytics.Row{{Provider: "maybe-anthropic", Model: "unknown", Harness: "claude-code", Total: 30, Input: 1, Output: 2, Reasoning: 3, CacheRead: 4, CacheWrite: 20, Sessions: 1}},
	}}
	got := usageSnapshot(data)
	wantTotals := totals{Total: 1000, Input: 100, Output: 200, Reasoning: 300, CacheRead: 150, CacheWrite: 250, Sessions: 12}
	if got.Totals != wantTotals {
		t.Fatalf("summary %+v; want %+v", got.Totals, wantTotals)
	}
	wantRow := usageRow{Provider: "maybe-anthropic", Model: "unknown", Harness: "claude-code", Tokens: totals{Total: 30, Input: 1, Output: 2, Reasoning: 3, CacheRead: 4, CacheWrite: 20, Sessions: 1}}
	if len(got.Rows) != 1 || got.Rows[0] != wantRow {
		t.Fatalf("rows %+v", got.Rows)
	}
}

func TestNativeCollectionStatusUsesLatestAttemptAndDetailedCounts(t *testing.T) {
	progress := collectorprogress.DetailSnapshot{
		Collection: collectorprogress.Snapshot{Attempts: []collectorprogress.Attempt{
			{AttemptID: "old", Stage: "failed"}, {AttemptID: "current", Stage: "capturing"},
		}},
		Captures: map[string]map[string]collectorprogress.Capture{"current": {"codex": {TotalKnown: true, Total: 12, Checked: 5}, "pi": {TotalKnown: true, Total: 8, Checked: 3}}},
	}
	if text, failed := collectionText(progress); failed || text != "Collecting: 8 of 20 sources checked" {
		t.Fatalf("status %q, %v", text, failed)
	}
	progress.Collection.Attempts[1].Stage = "interrupted"
	if _, failed := collectionText(progress); !failed {
		t.Fatal("interruption omitted retry")
	}
	progress.Collection.Attempts[1].Stage = "accepted"
	if text, failed := collectionText(progress); failed || text != "Collection complete" {
		t.Fatalf("status %q, %v", text, failed)
	}
}
