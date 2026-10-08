package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

func publicationSnapshot(t *testing.T, store *Store) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, table := range []string{"analytics.facts", "analytics.estimates", "analytics.provenance", "analytics.legacy_coverage", "processing.outcomes", "processing.scopes", "ingestion.metadata", "analytics.generations", "raw.evidence", "ingestion.batches", "ingestion.items", "ingestion.batch_items"} {
		var value string
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT COALESCE(CAST(to_json(list(t ORDER BY CAST(to_json(t) AS VARCHAR))) AS VARCHAR),'[]') FROM "+table+" t").Scan(&value); err != nil {
			t.Fatal(err)
		}
		result[table] = value
	}
	return result
}

func TestPublicationRollbackReopenAndRetryPreservesUsage(t *testing.T) {
	for _, failure := range []string{"constraint", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			store := testStore(t)
			record := func(id string) evidence.Record {
				value := piRecord(id, 10)
				value.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":%q,"message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":10,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`, id))
				return value
			}
			if _, err := store.Accept(t.Context(), batchBody(t, store, "initial", "initial", record("saved"))); err != nil {
				t.Fatal(err)
			}
			drain(t, store)
			// Cross a bulk-statement boundary before failing publication.
			const newRecords = 150
			records := make([]evidence.Record, newRecords)
			for i := range records {
				records[i] = record(fmt.Sprintf("new-%d", i))
			}
			body := batchBody(t, store, "later", "later", records...)
			accepted, err := store.Accept(t.Context(), body)
			if err != nil {
				t.Fatal(err)
			}
			work, found, err := store.LoadWork(t.Context())
			if err != nil || !found {
				t.Fatal("missing publication work", found, err)
			}
			projection, err := processor.Process(t.Context(), work.Records)
			if err != nil || len(projection.Contributions) != newRecords+1 {
				t.Fatal("missing native contributions", err)
			}
			before := publicationSnapshot(t, store)
			switch failure {
			case "constraint":
				invalid := projection
				// A genuine unique-index error after facts and coverage were written.
				estimate := evidence.Estimate{Fact: projection.Contributions[0].Fact, EvidenceID: work.Records[0].ID, Code: "test_failure"}
				invalid.Estimates = []evidence.Estimate{estimate, estimate}
				if published, err := store.PublishProjection(t.Context(), work, invalid); err == nil || published {
					t.Fatal("invalid publication committed", published, err)
				}
			case "cancellation":
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				prepared, err := prepareProjection(work, projection)
				if err != nil {
					t.Fatal(err)
				}
				err = store.WriteTransaction(ctx, func(tx *sql.Tx) error {
					if err := publishRows(ctx, tx, work, projection, prepared); err != nil {
						return err
					}
					cancel() // Interrupt after all projection rows, before commit.
					return ctx.Err()
				})
				if !errors.Is(err, context.Canceled) {
					t.Fatal("canceled publication committed", err)
				}
			}
			if after := publicationSnapshot(t, store); !reflect.DeepEqual(before, after) {
				for table, value := range before {
					if after[table] != value {
						t.Errorf("rollback changed %s", table)
					}
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(t.Context(), store.path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reopened.Close() }()
			if after := publicationSnapshot(t, reopened); !reflect.DeepEqual(before, after) {
				t.Fatal("reopen changed committed state")
			}
			drain(t, reopened)
			var components [6]int64
			if err := reopened.SQL().QueryRow("SELECT SUM(input_tokens),SUM(output_tokens),SUM(reasoning_tokens),SUM(cache_read_tokens),SUM(cache_write_tokens),SUM(total_tokens) FROM analytics.confirmed").Scan(&components[0], &components[1], &components[2], &components[3], &components[4], &components[5]); err != nil {
				t.Fatal(err)
			}
			count := int64(newRecords + 1)
			if components != [6]int64{count * 10, count * 5, count * 2, count * 3, count * 4, count * 24} {
				t.Fatal("retry lost or inflated native components", components)
			}
			for _, contribution := range projection.Contributions {
				var edges int64
				if err := reopened.SQL().QueryRow("SELECT COUNT(*) FROM analytics.provenance WHERE fact_id=?", contribution.Fact.ID).Scan(&edges); err != nil || edges != int64(len(contribution.EvidenceIDs)) {
					t.Fatal("retry changed fact identity or provenance", edges, err)
				}
			}
			response, err := reopened.Receipt(t.Context(), "later", "later")
			if err != nil || response.Receipt != accepted.Receipt || response.Processing.Pending != 0 || len(response.Processing.Items) != newRecords {
				t.Fatal("retry lost receipt or outcomes", response, err)
			}
			replay, err := reopened.Accept(t.Context(), body)
			if err != nil || replay.Receipt != accepted.Receipt || replay.Processing.Pending != 0 {
				t.Fatal("replay changed accepted delivery", replay, err)
			}
			if total(t, reopened, "analytics.confirmed") != count*24 {
				t.Fatal("replay inflated usage")
			}
		})
	}
}
