package datastore

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

const publicationBenchmarkFacts = 10000

// BenchmarkPublication times complete production publication transactions. Opening,
// acceptance, work selection, interpretation and result verification are untimed.
func BenchmarkPublication(b *testing.B) {
	for _, components := range []int{1, 50, 500} {
		for _, replacement := range []bool{false, true} {
			name := "Initial"
			if replacement {
				name = "Replacement"
			}
			b.Run(fmt.Sprintf("Components%d/%s", components, name), func(b *testing.B) {
				b.ReportAllocs()
				b.StopTimer()
				var elapsed time.Duration
				for range b.N {
					store := testStore(b)
					start := 0
					if replacement {
						acceptPublicationRecords(b, store, components, 0, 1)
						for range components {
							if worked, err := store.ProcessNext(b.Context()); err != nil || !worked {
								b.Fatal("seed publication", worked, err)
							}
						}
						start = 1
					}
					acceptPublicationRecords(b, store, components, start, publicationBenchmarkFacts/components)
					before, err := store.Metadata(b.Context())
					if err != nil {
						b.Fatal(err)
					}
					expected := make(map[string]string, publicationBenchmarkFacts)
					for range components {
						work, found, err := store.LoadWork(b.Context())
						if err != nil || !found {
							b.Fatal("load publication", found, err)
						}
						projection, err := processor.Process(b.Context(), work.Records)
						if err != nil {
							b.Fatal(err)
						}
						for _, contribution := range projection.Contributions {
							if len(contribution.EvidenceIDs) != 1 {
								b.Fatal("fixture must have one evidence edge per fact")
							}
							expected[contribution.Fact.ID] = contribution.EvidenceIDs[0]
						}
						b.StartTimer()
						started := time.Now()
						published, err := store.PublishProjection(b.Context(), work, projection)
						elapsed += time.Since(started)
						b.StopTimer()
						if err != nil || !published {
							b.Fatal("publish", published, err)
						}
					}
					verifyPublication(b, store, expected, components, before.Revision)
					if err := store.Close(); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N*components), "ns/component")
			})
		}
	}
}

func acceptPublicationRecords(b *testing.B, store *Store, components, first, end int) {
	b.Helper()
	var records []evidence.Record
	batch := 0
	flush := func() {
		if len(records) == 0 {
			return
		}
		stream := fmt.Sprintf("phase-%d-batch-%d", first, batch)
		response, err := store.Accept(b.Context(), evidence.ProtocolVersion, batchBody(b, store, stream, "batch", records...))
		if err != nil || response.Receipt.Accepted != int64(len(records)) {
			b.Fatal("accept fixture", response, err)
		}
		batch++
		records = records[:0]
	}
	for component := range components {
		for message := first; message < end; message++ {
			id := fmt.Sprintf("message-%d", message)
			record := piRecord(id, 10)
			record.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, component))
			record.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":%q,"message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":10,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`, id))
			records = append(records, record)
			if len(records) == evidence.MaxEntries {
				flush()
			}
		}
	}
	flush()
}

func verifyPublication(b *testing.B, store *Store, expected map[string]string, components int, previousRevision int64) {
	b.Helper()
	if len(expected) != publicationBenchmarkFacts {
		b.Fatal("fixture lost fact identities", len(expected))
	}
	rows, err := store.SQL().QueryContext(b.Context(), `SELECT f.fact_id,p.evidence_id,f.input_tokens,f.output_tokens,f.reasoning_tokens,f.cache_read_tokens,f.cache_write_tokens,f.total_tokens
		FROM analytics.confirmed f JOIN analytics.provenance p USING(dataset_id,generation,fact_id)`)
	if err != nil {
		b.Fatal(err)
	}
	for rows.Next() {
		var id, evidenceID string
		var tokens [6]int64
		if err := rows.Scan(&id, &evidenceID, &tokens[0], &tokens[1], &tokens[2], &tokens[3], &tokens[4], &tokens[5]); err != nil {
			b.Fatal(err)
		}
		if expected[id] != evidenceID || tokens != [6]int64{10, 5, 2, 3, 4, 24} {
			b.Fatal("publication changed identity, provenance or accounting", id, tokens)
		}
		delete(expected, id)
	}
	if err := rows.Err(); err != nil {
		b.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		b.Fatal(err)
	}
	if len(expected) != 0 {
		b.Fatal("publication lost facts", len(expected))
	}
	var outcomes, facts, provenance, estimates, pending, sessions int
	err = store.SQL().QueryRowContext(b.Context(), `SELECT
		(SELECT COUNT(*) FROM processing.outcomes WHERE disposition='processed'),
		(SELECT COUNT(*) FROM analytics.facts), (SELECT COUNT(*) FROM analytics.provenance),
		(SELECT COUNT(*) FROM analytics.estimates),
		(SELECT COUNT(*) FROM processing.scopes WHERE processed_revision<>revision OR generation<>1),
		(SELECT COUNT(DISTINCT session_id) FROM analytics.confirmed)`).Scan(&outcomes, &facts, &provenance, &estimates, &pending, &sessions)
	if err != nil || outcomes != publicationBenchmarkFacts || facts != publicationBenchmarkFacts || provenance != publicationBenchmarkFacts || estimates != 0 || pending != 0 || sessions != components {
		b.Fatal("incomplete publication", outcomes, facts, provenance, estimates, pending, sessions, err)
	}
	metadata, err := store.Metadata(b.Context())
	if err != nil || metadata.Generation != 1 || metadata.TargetGeneration != 1 || metadata.Revision != previousRevision+int64(components) {
		b.Fatal("publication metadata", metadata, err)
	}
}
