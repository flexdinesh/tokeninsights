package storagecontract

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

// Group counts describe visible row identities, not facts or native sessions.
// Filtered summaries and the unfiltered synced-session total have distinct scope.
func queryGroupCounts(t *testing.T, store Tokens) {
	d := dataset(t, store, "group-counts")
	models := []string{"model-a", "model-a", "model-b", "model-c", "model-a", "model-b"}
	var records []evidence.Record
	for i, model := range models {
		r := record(fmt.Sprintf("message-%d", i))
		r.Ordinal = int64(i + 2)
		r.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i/2))
		provider := "provider-a"
		at := int64(1700000000000)
		if i < 4 {
			r.Location = &evidence.Location{DirectoryKey: fmt.Sprintf("directory-%d", i/2), DirectoryName: "project", RepositoryKey: "repository", RepositoryName: "repo", RepositorySource: "harness"}
		} else {
			provider = "provider-b"
			at += int64(24 * time.Hour / time.Millisecond)
		}
		r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":%d,"provider":%q,"model":%q,"usage":{"input":10,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`, i, at, provider, model))
		records = append(records, r)
	}
	accept(t, d, batch(t, d, "seed", records...))
	drain(t, d)
	other := dataset(t, store, "other")
	accept(t, other, batch(t, other, "seed", record("message-0")))
	drain(t, other)
	for _, generation := range []int64{1, 2} {
		if generation == 2 {
			if _, err := d.Reprocess(t.Context()); err != nil {
				t.Fatal(err)
			}
			drain(t, d)
		}
		t.Run(fmt.Sprintf("generation-%d", generation), func(t *testing.T) {
			for _, tc := range []struct {
				tab           string
				location      querymodel.RepoGroup
				all, filtered int
			}{
				{"models", "", 3, 1}, {"providers", "", 2, 2}, {"harnesses", "", 1, 1}, {"sessions", "", 3, 2},
				{"context", "", 5, 2}, {"tokens", "", 2, 2},
				{"repo", querymodel.RepoGroupRepository, 2, 2}, {"repo", querymodel.RepoGroupDirectory, 3, 2},
			} {
				t.Run(tc.tab+string(tc.location), func(t *testing.T) {
					for _, filter := range []struct {
						name, model     string
						facts, sessions int64
						groups          int
					}{
						{"all", "", 6, 3, tc.all}, {"filtered", "model-a", 3, 2, tc.filtered}, {"empty", "missing", 0, 0, 0},
					} {
						t.Run(filter.name, func(t *testing.T) {
							q := query()
							q.Tab = tc.tab
							q.LocationGroup = tc.location
							q.Page = 999
							q.PageSize = 1
							if q.Tab == "context" {
								q.Sort = "averageContext"
							}
							if filter.model != "" {
								q.Selection.Models = []string{filter.model}
							}
							got, err := d.Queries.Dashboard(t.Context(), q, time.Now())
							if err != nil {
								t.Fatal(err)
							}
							s := got.Summary
							wantSummary := querymodel.ViewerSummaryRow{
								TotalTokens: filter.facts * 24, InputTokens: filter.facts * 10,
								OutputTokens: filter.facts * 5, ReasoningTokens: filter.facts * 2,
								CacheReadTokens: filter.facts * 3, CacheWriteTokens: filter.facts * 4,
								SessionCount: filter.sessions, SyncedSessions: 3,
							}
							if got.RowCount != filter.groups || got.FactCount != filter.facts || got.Page != max(1, filter.groups) || len(got.Rows) != min(1, filter.groups) || got.Generation != generation || got.DatasetID != "group-counts" ||
								s != wantSummary {
								t.Fatal("group count, pagination or summary scope changed", got)
							}
							all, err := d.Queries.AllDashboard(t.Context(), q, time.Now(), max(1, filter.groups))
							if err != nil || len(all.Rows) != filter.groups || all.RowCount != filter.groups || all.Summary != s || all.FactCount != got.FactCount || all.Generation != generation {
								t.Fatal("complete rows disagree with count", all, err)
							}
							if filter.groups > 1 {
								if _, err := d.Queries.AllDashboard(t.Context(), q, time.Now(), filter.groups-1); err == nil {
									t.Fatal("complete row limit silently truncated")
								}
							}
						})
					}
				})
			}
		})
	}
}
