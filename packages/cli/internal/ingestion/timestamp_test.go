package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestInvalidTimestampBatchDoesNotMutateSavedData(t *testing.T) {
	for _, field := range []string{"snapshot", "session-first", "session-last", "message", "revision"} {
		t.Run(field, func(t *testing.T) {
			store, _ := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(store)))
			defer server.Close()
			existing := contractFact("existing")
			contractPost(t, server, contractBatch(t, store, "saved-stream", "saved-batch", existing), http.StatusOK)
			before, err := store.Metadata(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			body := contractBatch(t, store, "rejected-stream", "rejected-batch", contractFact("new-valid"), contractFact("new-invalid"))
			var batch publication.Batch
			if err := json.Unmarshal(body, &batch); err != nil {
				t.Fatal(err)
			}
			// Mutate wire JSON directly; production encoding must also reject it.
			fact := &batch.Entries[1].Fact
			const invalidTime int64 = 253402214400000
			switch field {
			case "snapshot":
				fact.OccurredAtMs = invalidTime
				fact.Session.LastOccurredAtMs = invalidTime
			case "session-first":
				fact.Session.FirstOccurredAtMs = -1
			case "session-last":
				fact.Session.LastOccurredAtMs = invalidTime
			case "message":
				fact.Message.OccurredAtMs = invalidTime
			case "revision":
				fact.Harness, fact.Session.Harness, fact.NativeRequestID = "claude-code", "claude-code", "request"
				fact.OccurredAtMs, fact.Session.FirstOccurredAtMs, fact.Session.LastOccurredAtMs = invalidTime, invalidTime, invalidTime
				fact.Revision = &publication.SourceRevision{Rule: publication.ClaudeRevisionRule, Value: invalidTime}
				publication.SetIDs(fact)
			}
			body, err = json.Marshal(batch)
			if err != nil {
				t.Fatal(err)
			}
			contractPost(t, server, body, http.StatusBadRequest)
			contractAssert(t, store, 1, existing)
			after, err := store.Metadata(context.Background())
			if err != nil || before != after {
				t.Fatalf("metadata changed: before=%+v after=%+v err=%v", before, after, err)
			}
			var first, last, message int64
			if err := store.SQL().QueryRow(`SELECT s.first_seen_at_ms,s.last_seen_at_ms,m.occurred_at_ms FROM canonical_sessions s JOIN canonical_messages m ON m.session_id=s.id`).Scan(&first, &last, &message); err != nil || first != contractTime || last != contractTime || message != contractTime {
				t.Fatalf("saved reference timestamps changed: first=%d last=%d message=%d error=%v", first, last, message, err)
			}
			for table, want := range map[string]int{"canonical_sessions": 1, "canonical_messages": 1, "usage_locations": 0, "ingestion_producers": 1} {
				var count int
				if err := store.SQL().QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil || count != want {
					t.Fatalf("%s count=%d want=%d error=%v", table, count, want, err)
				}
			}
		})
	}
}

func TestServerCanonicalTimestampConstraints(t *testing.T) {
	store, _ := contractStore(t)
	if _, err := NewCore(store).Ingest(context.Background(), contractBatch(t, store, "stream", "batch", contractFact("request"))); err != nil {
		t.Fatal(err)
	}
	for name, statement := range map[string]string{
		"session":  `UPDATE canonical_sessions SET first_seen_at_ms=?,last_seen_at_ms=?`,
		"message":  `UPDATE canonical_messages SET occurred_at_ms=?`,
		"fact":     `UPDATE canonical_token_usage SET recorded_at_ms=?`,
		"revision": `UPDATE canonical_token_usage SET revision_value=?`,
	} {
		for _, value := range []any{int64(-1), int64(0), int64(publication.MaxTimestampMs), int64(publication.MaxTimestampMs + 1), float64(0.5)} {
			t.Run(fmt.Sprintf("%s/%v", name, value), func(t *testing.T) {
				args := []any{value}
				if name == "session" {
					args = append(args, value)
				}
				_, err := store.SQL().Exec(statement, args...)
				integer, ok := value.(int64)
				wantValid := ok && publication.ValidTimestampMs(integer)
				if (err == nil) != wantValid {
					t.Fatalf("SQL error=%v wantValid=%v", err, wantValid)
				}
			})
		}
	}
}
