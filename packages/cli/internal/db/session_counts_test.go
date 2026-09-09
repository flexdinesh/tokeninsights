package db

import (
	"context"
	"testing"
	"time"
)

func TestViewerSessionCounts(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	ctx := context.Background()
	counts, err := ViewerSessionCounts(ctx, database, Filter{})
	if err != nil || counts != (SessionCounts{}) {
		t.Fatalf("empty database counts = %+v, err = %v", counts, err)
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)
	insertCanonicalToken(t, database, start.UnixMilli(), "opencode", "shared", "openai", "model-a", 10, 5, 0, 0, 0, 15)
	insertCanonicalToken(t, database, start.Add(time.Hour).UnixMilli(), "opencode", "shared", "anthropic", "model-b", 10, 5, 0, 0, 0, 15)
	insertCanonicalToken(t, database, start.UnixMilli(), "codex", "shared", "openai", "model-a", 10, 5, 0, 0, 0, 15)
	insertCanonicalToken(t, database, start.AddDate(0, 0, -1).UnixMilli(), "opencode", "old", "openai", "model-a", 10, 5, 0, 0, 0, 15)
	insertCanonicalToken(t, database, end.UnixMilli(), "opencode", "next", "openai", "model-a", 10, 5, 0, 0, 0, 15)
	insertCanonicalToken(t, database, start.UnixMilli(), "pi", "suppressed", "unknown", "unknown", 10, 5, 0, 0, 0, 15)
	if _, err := database.Exec("UPDATE canonical_token_usage SET is_countable = 0 WHERE harness = 'pi'"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO canonical_sessions
		(semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms)
		VALUES ('empty', 'pi', 'empty', 0, 0)`); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		filter Filter
		shown  int64
	}{
		{name: "all time counts canonical identities once", shown: 4},
		{name: "month has inclusive start and exclusive end", filter: Filter{Start: start, End: end}, shown: 2},
		{name: "harness", filter: Filter{Harnesses: []string{"opencode"}}, shown: 3},
		{name: "provider and model", filter: Filter{Providers: []string{"anthropic"}, Models: []string{"model-b"}}, shown: 1},
		{name: "session IDs across harnesses", filter: Filter{SessionIDs: []string{"shared"}}, shown: 2},
		{name: "custom local dates", filter: Filter{DayFrom: "2026-09-01", DayTo: "2026-09-01"}, shown: 2},
		{name: "combined filters", filter: Filter{Start: start, End: end, Harnesses: []string{"opencode"}, Models: []string{"model-a"}}, shown: 1},
		{name: "no matches", filter: Filter{Models: []string{"missing"}}, shown: 0},
		{name: "suppressed facts and empty sessions excluded", filter: Filter{Harnesses: []string{"pi"}}, shown: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ViewerSessionCounts(ctx, database, test.filter)
			want := SessionCounts{Shown: test.shown, Synced: 4}
			if err != nil || got != want {
				t.Fatalf("counts = %+v, want %+v; err = %v", got, want, err)
			}
		})
	}
}
