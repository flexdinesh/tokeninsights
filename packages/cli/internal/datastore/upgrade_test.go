package datastore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

// Write a real schema1 file from populated schema2 fixtures; both production
// schema contracts are used, with identical rows and byte-preserved v2 requests.
func schema1Copy(t *testing.T, current *Store, path string) {
	t.Helper()
	database, err := connect(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.ExecContext(t.Context(), schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err := current.SQL().Exec("CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	source := "'" + strings.ReplaceAll(current.path, "'", "''") + "'"
	if _, err := database.ExecContext(t.Context(), "ATTACH "+source+" AS current_data (READ_ONLY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), "INSERT INTO ingestion.metadata SELECT id,role,1,database_id,dataset_id,generation,target_generation,input_revision,revision,last_ingestion_at_ms,created_at_ms FROM current_data.ingestion.metadata"); err != nil {
		t.Fatal(err)
	}
	for _, table := range datasetTables {
		if _, err := database.ExecContext(t.Context(), "INSERT INTO "+table+" SELECT * EXCLUDE(dataset_id) FROM current_data."+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec("DETACH current_data; CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
}

func legacyRawBody(t *testing.T, store *Store, stream, batch string, records ...evidence.Record) []byte {
	t.Helper()
	var value evidence.Batch
	if err := json.Unmarshal(batchBody(t, store, stream, batch, records...), &value); err != nil {
		t.Fatal(err)
	}
	value.ProtocolVersion = evidence.LegacyProtocolVersion
	value.DatasetID = ""
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSchema1UpgradePreservesBytesHistoryAndInterruptedGeneration(t *testing.T) {
	original := testStore(t)
	first := piRecord("one", 100)
	first.Data = json.RawMessage(`{"type":"message","id":"one","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20,"reasoning":5,"cacheRead":7,"cacheWrite":11}}}`)
	second := piRecord("two", 200)
	second.Context[0].Data = json.RawMessage(`{"type":"session","id":"other"}`)
	m, err := original.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	stored := []evidence.Stored{}
	for _, record := range []evidence.Record{first, second} {
		stored = append(stored, evidence.Stored{ID: evidence.EvidenceID(record), Scope: evidence.Scope(record), Record: record})
	}
	projection, err := processor.Process(t.Context(), stored)
	if err != nil {
		t.Fatal(err)
	}
	legacyBatch := publication.Batch{ProtocolVersion: 1, IdentityVersion: 1, SemanticsVersion: 1, DatabaseID: m.DatabaseID, StreamID: "legacy-stream", BatchID: "legacy-batch", FromSequence: 1, ToSequence: int64(len(projection.Contributions)), Hostname: "fixture"}
	for i, contribution := range projection.Contributions {
		legacyBatch.Entries = append(legacyBatch.Entries, publication.Entry{Sequence: int64(i + 1), Fact: contribution.Fact})
	}
	legacyBody, err := publication.EncodeBatch(legacyBatch)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceipt, err := original.AcceptLegacy(t.Context(), legacyBody)
	if err != nil {
		t.Fatal(err)
	}
	body := legacyRawBody(t, original, "stream", "batch", first, second)
	accepted, err := original.Accept(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	drain(t, original)
	before, err := original.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := original.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	var receiptBefore string
	if err := original.SQL().QueryRow("SELECT receipt_json FROM ingestion.batches").Scan(&receiptBefore); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.duckdb")
	schema1Copy(t, original, path)
	digest, err := fileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upgraded.Close() }()
	metadata, err := upgraded.Metadata(t.Context())
	if err != nil || metadata.DatabaseID != before.DatabaseID || metadata.Generation != 1 || metadata.TargetGeneration != 2 || metadata.InputRevision != before.InputRevision || metadata.Revision != before.Revision {
		t.Fatal("upgrade changed history/generation", before, metadata, err)
	}
	var requestAfter []byte
	var receiptAfter string
	if err := upgraded.SQL().QueryRow("SELECT request_bytes,receipt_json FROM ingestion.batches").Scan(&requestAfter, &receiptAfter); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, requestAfter) || receiptBefore != receiptAfter {
		t.Fatal("upgrade changed immutable bytes")
	}
	if total(t, upgraded, "analytics.confirmed") != 358 {
		t.Fatal("upgrade lost active totals")
	}
	replay, err := upgraded.Accept(t.Context(), body)
	if err != nil || replay.Receipt != accepted.Receipt {
		t.Fatal("old request no longer replayable", replay, err)
	}
	legacyReplay, err := upgraded.AcceptLegacy(t.Context(), legacyBody)
	if err != nil || legacyReplay != legacyReceipt {
		t.Fatal("legacy receipt changed during upgrade", legacyReplay, err)
	}
	var covered int
	if err := upgraded.SQL().QueryRow("SELECT COUNT(*) FROM analytics.legacy_coverage WHERE dataset_id='default' AND generation=1").Scan(&covered); err != nil || covered != 2 {
		t.Fatal("legacy identity coverage changed", covered, err)
	}
	backupDigest, err := fileDigest(path + ".schema1-backup")
	if err != nil || backupDigest != digest {
		t.Fatal("recoverable original changed", err)
	}
	drain(t, upgraded)
	metadata, err = upgraded.Metadata(t.Context())
	if err != nil || metadata.Generation != 2 || total(t, upgraded, "analytics.confirmed") != 358 {
		t.Fatal("interrupted generation failed to resume", metadata, err)
	}
	var components [6]int64
	if err := upgraded.SQL().QueryRow("SELECT CAST(SUM(input_tokens) AS BIGINT),CAST(SUM(output_tokens) AS BIGINT),CAST(SUM(reasoning_tokens) AS BIGINT),CAST(SUM(cache_read_tokens) AS BIGINT),CAST(SUM(cache_write_tokens) AS BIGINT),CAST(SUM(total_tokens) AS BIGINT) FROM analytics.confirmed").Scan(&components[0], &components[1], &components[2], &components[3], &components[4], &components[5]); err != nil {
		t.Fatal(err)
	}
	if components != [6]int64{300, 35, 5, 7, 11, 358} {
		t.Fatal("components changed", components)
	}
}

func TestRejectedUpgradeAndKindDoNotMutateSource(t *testing.T) {
	original := testStore(t)
	path := filepath.Join(t.TempDir(), "server.duckdb")
	schema1Copy(t, original, path)
	before, err := fileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := OpenKind(t.Context(), path, KindHosted); err == nil {
		_ = store.Close()
		t.Fatal("personal upgraded as hosted")
	}
	after, err := fileDigest(path)
	if err != nil || before != after {
		t.Fatal("kind rejection mutated database", err)
	}
	if _, err := os.Stat(path + ".schema1-backup"); !os.IsNotExist(err) {
		t.Fatal("rejected upgrade created backup", err)
	}
	database, err := connect(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("ALTER TABLE raw.evidence ADD COLUMN unexpected VARCHAR"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err = fileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := Open(t.Context(), path); err == nil {
		_ = store.Close()
		t.Fatal("incompatible schema upgraded")
	}
	after, err = fileDigest(path)
	if err != nil || before != after {
		t.Fatal("incompatible rejection mutated source", err)
	}
}

func TestSchema2ServerKindCannotChangeOnReopen(t *testing.T) {
	for _, kind := range []string{KindPersonal, KindHosted} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.duckdb")
			store, err := OpenKind(t.Context(), path, kind)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := fileDigest(path)
			if err != nil {
				t.Fatal(err)
			}
			other := KindPersonal
			if kind == KindPersonal {
				other = KindHosted
			}
			if store, err := OpenKind(t.Context(), path, other); err == nil {
				_ = store.Close()
				t.Fatal("kind changed")
			}
			after, err := fileDigest(path)
			if err != nil || before != after {
				t.Fatal("kind rejection mutated database", err)
			}
		})
	}
}

func TestUpgradeRecoversRetainedOriginalAndRejectsBackupConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "retained-original"
		if conflict {
			name = "conflicting-backup"
		}
		t.Run(name, func(t *testing.T) {
			current := testStore(t)
			path := filepath.Join(t.TempDir(), "server.duckdb")
			schema1Copy(t, current, path)
			before, err := fileDigest(path)
			if err != nil {
				t.Fatal(err)
			}
			backup := path + ".schema1-backup"
			if conflict {
				if err := os.WriteFile(backup, []byte("different recoverable file"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Link(path, backup); err != nil {
				t.Fatal(err)
			}
			store, err := Open(t.Context(), path)
			if conflict {
				if err == nil {
					_ = store.Close()
					t.Fatal("different recoverable original overwritten")
				}
				after, readErr := fileDigest(path)
				if readErr != nil || before != after {
					t.Fatal("conflicting backup changed source", readErr)
				}
				contents, readErr := os.ReadFile(backup)
				if readErr != nil || string(contents) != "different recoverable file" {
					t.Fatal("conflicting backup overwritten", readErr)
				}
				return
			}
			if err != nil {
				t.Fatal("interrupted publication did not recover", err)
			}
			defer func() { _ = store.Close() }()
			after, readErr := fileDigest(backup)
			if readErr != nil || before != after {
				t.Fatal("retained original changed", readErr)
			}
		})
	}
}

func TestInconsistentGenerationMetadataRejectsWithoutMutation(t *testing.T) {
	corruptions := []struct{ name, query string }{
		{"missing-active", "DELETE FROM analytics.generations WHERE generation=1"},
		{"retained-active", "UPDATE analytics.generations SET state='retained' WHERE generation=1"},
		{"multiple-active", "UPDATE analytics.generations SET state='active' WHERE generation=2"},
		{"retained-target", "UPDATE analytics.generations SET state='retained' WHERE generation=2"},
	}
	for _, version := range []string{"schema1", "schema2"} {
		for _, corruption := range corruptions {
			t.Run(version+"/"+corruption.name, func(t *testing.T) {
				current := testStore(t)
				if _, err := current.Accept(t.Context(), batchBody(t, current, "stream", "batch", piRecord("message", 100))); err != nil {
					t.Fatal(err)
				}
				if _, err := current.Reprocess(t.Context()); err != nil {
					t.Fatal(err)
				}
				path := current.path
				if version == "schema1" {
					path = filepath.Join(t.TempDir(), "server.duckdb")
					schema1Copy(t, current, path)
				} else if err := current.Close(); err != nil {
					t.Fatal(err)
				}
				database, err := connect(path, false)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := database.Exec(corruption.query); err != nil {
					t.Fatal(err)
				}
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
				before, err := fileDigest(path)
				if err != nil {
					t.Fatal(err)
				}
				if opened, err := Open(t.Context(), path); err == nil {
					_ = opened.Close()
					t.Fatal("inconsistent generation accepted")
				}
				after, err := fileDigest(path)
				if err != nil || before != after {
					t.Fatal("inconsistent generation rejection mutated source", err)
				}
			})
		}
	}
}
