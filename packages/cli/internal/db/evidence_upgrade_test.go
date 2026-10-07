package db

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

func evidencePredecessor(t *testing.T, version int) (string, *sql.DB) {
	t.Helper()
	if version == 17 {
		return maintenanceSchema17(t)
	}
	database, path := newTestDB(t)
	execLifecycleSQL(t, database, "DROP TABLE evidence_quarantine")
	if version == 16 {
		for _, table := range []string{TableEvidenceBatches, TableEvidenceDestinations, TableEvidenceOutbox, TableEvidenceSources, TableEvidenceState} {
			execLifecycleSQL(t, database, "DROP TABLE "+table)
		}
	}
	execLifecycleSQL(t, database, fmt.Sprintf("PRAGMA user_version=%d", version))
	return path, database
}

func TestEvidenceUpgradePreservesDeliveryAndCaptureAcrossPredecessors(t *testing.T) {
	for _, version := range []int{16, 17, 18} {
		t.Run(fmt.Sprintf("schema%d", version), func(t *testing.T) {
			path, database := evidencePredecessor(t, version)
			defer func() { _ = database.Close() }()
			insertCanonicalToken(t, database, 1767225600000, "pi", "legacy-session", "provider", "model", 10, 20, 3, 4, 5, 42)
			hash := strings.Repeat("a", 64)
			legacyRequest := []byte("{\n  \"protocolVersion\": 1\n}")
			legacyReceipt := []byte("{ \"accepted\": true }")
			for _, statement := range []string{
				`INSERT INTO publication_journal(sequence,fact_id,payload_hash,payload_json,identity_version,semantics_version,created_at_ms) VALUES(1,'fact',printf('%064d',0),'{}',1,1,1)`,
				`INSERT INTO publication_entities(fact_id,payload_hash,sequence) VALUES('fact',printf('%064d',0),1)`,
				`INSERT INTO publication_destinations(destination_id,endpoint,database_id,acknowledged_sequence,last_receipt_json,acknowledged_at_ms) VALUES('legacy','http://remote','database',7,'{}',1)`,
			} {
				execLifecycleSQL(t, database, statement)
			}
			if _, err := database.Exec(`INSERT INTO publication_batches(batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes,receipt_bytes,acknowledged_at_ms) VALUES('saved-v1','legacy','legacy-stream','database',7,7,?,?,?,1)`, hash, legacyRequest, legacyReceipt); err != nil {
				t.Fatal(err)
			}
			var legacyStream string
			if err := database.QueryRow("SELECT stream_id FROM publication_state").Scan(&legacyStream); err != nil {
				t.Fatal(err)
			}
			rawRequest := []byte("{\n \"protocolVersion\": 2\n}")
			if version >= 17 {
				for _, statement := range []string{
					`INSERT INTO evidence_state VALUES(1,'raw-stream',1,1)`,
					`INSERT INTO evidence_sources VALUES('source','source-id','lineage','codex-jsonl',1,99,3,'prefix','[]',1)`,
					`INSERT INTO evidence_outbox(sequence,observation_key,harness,record_json,created_at_ms) VALUES(8,'observation','codex','{}',1)`,
					`INSERT INTO evidence_destinations(destination_id,endpoint,database_id,acknowledged_sequence,acknowledged_at_ms) VALUES('raw','http://remote','database',7,1)`,
				} {
					execLifecycleSQL(t, database, statement)
				}
				rawInsert := `INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes) VALUES('saved-v2','raw','raw-stream','database',8,8,?,?)`
				if version == 18 {
					rawInsert = `INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes) VALUES('saved-v2','raw','raw-stream','database',2,8,8,?,?)`
				}
				if _, err := database.Exec(rawInsert, hash, rawRequest); err != nil {
					t.Fatal(err)
				}
				if version == 18 {
					execLifecycleSQL(t, database, `INSERT INTO evidence_destinations(destination_id,endpoint,database_id,dataset_id,acknowledged_sequence) VALUES('hosted','https://hosted','database','user-dataset',8)`)
					if _, err := database.Exec(`INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes,receipt_bytes,acknowledged_at_ms) VALUES('saved-v3','hosted','raw-stream','database','user-dataset',3,8,8,?,?,?,1)`, hash, []byte("{ \"protocolVersion\": 3 }"), []byte("{ \"datasetId\": \"user-dataset\" }")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := InspectEvidenceCompatibility(t.Context(), path); err != nil {
				t.Fatal("predecessor not recognized read-only", err)
			}
			if err := UpgradeEvidence(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectCompatibility(t.Context(), path); err != nil {
				t.Fatal("upgraded contract invalid", err)
			}
			var current int
			var afterStream string
			if err := database.QueryRow("PRAGMA user_version").Scan(&current); err != nil || current != SupportedSchemaVersion {
				t.Fatal(current, err)
			}
			if err := database.QueryRow("SELECT stream_id FROM publication_state").Scan(&afterStream); err != nil || afterStream != legacyStream {
				t.Fatal("publication identity changed", afterStream, err)
			}
			assertSavedRequest(t, database, "publication_batches", "saved-v1", legacyRequest, legacyReceipt, hash)
			var input, output, reasoning, cacheRead, cacheWrite, total int64
			if err := database.QueryRow("SELECT input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens FROM canonical_token_usage").Scan(&input, &output, &reasoning, &cacheRead, &cacheWrite, &total); err != nil || input != 10 || output != 20 || reasoning != 3 || cacheRead != 4 || cacheWrite != 5 || total != 42 {
				t.Fatal("legacy token components changed", input, output, reasoning, cacheRead, cacheWrite, total, err)
			}
			var legacyCursor int
			if err := database.QueryRow("SELECT acknowledged_sequence FROM publication_destinations WHERE destination_id='legacy'").Scan(&legacyCursor); err != nil || legacyCursor != 7 {
				t.Fatal("legacy cursor changed", legacyCursor, err)
			}
			if version >= 17 {
				assertSavedRequest(t, database, "evidence_batches", "saved-v2", rawRequest, nil, hash)
				var rawStream string
				var outboxSequence int
				var rawJSON string
				if err := database.QueryRow("SELECT stream_id FROM evidence_state").Scan(&rawStream); err != nil || rawStream != "raw-stream" {
					t.Fatal("raw stream changed", rawStream, err)
				}
				if err := database.QueryRow("SELECT sequence,record_json FROM evidence_outbox").Scan(&outboxSequence, &rawJSON); err != nil || outboxSequence != 8 || rawJSON != "{}" {
					t.Fatal("raw outbox changed", outboxSequence, rawJSON, err)
				}
				var cursor, offset, ordinal int
				var protocol int
				if err := database.QueryRow("SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id='raw'").Scan(&cursor); err != nil || cursor != 7 {
					t.Fatal("raw cursor changed", cursor, err)
				}
				if err := database.QueryRow("SELECT byte_offset,ordinal FROM evidence_sources WHERE source_key='source'").Scan(&offset, &ordinal); err != nil || offset != 99 || ordinal != 3 {
					t.Fatal("capture continuity changed", offset, ordinal, err)
				}
				if err := database.QueryRow("SELECT protocol_version FROM evidence_batches WHERE batch_id='saved-v2'").Scan(&protocol); err != nil || protocol != 2 {
					t.Fatal("saved protocol changed", protocol, err)
				}
				if version == 18 {
					assertSavedRequest(t, database, "evidence_batches", "saved-v3", []byte("{ \"protocolVersion\": 3 }"), []byte("{ \"datasetId\": \"user-dataset\" }"), hash)
				}
				if _, err := database.Exec("UPDATE evidence_batches SET request_bytes='changed' WHERE batch_id='saved-v2'"); err == nil {
					t.Fatal("upgrade removed immutable raw request protection")
				}
			}
			if _, err := database.Exec("UPDATE publication_journal SET payload_json='{}'"); err == nil {
				t.Fatal("upgrade removed immutable legacy journal protection")
			}
			if err := UpgradeEvidence(t.Context(), path); err != nil {
				t.Fatal("repeat upgrade failed", err)
			}
		})
	}
}

func assertSavedRequest(t *testing.T, database *sql.DB, table, batchID string, request, receipt []byte, hash string) {
	t.Helper()
	var gotRequest, gotReceipt []byte
	var gotHash string
	if err := database.QueryRow("SELECT request_bytes,receipt_bytes,request_hash FROM "+table+" WHERE batch_id=?", batchID).Scan(&gotRequest, &gotReceipt, &gotHash); err != nil || !bytes.Equal(gotRequest, request) || !bytes.Equal(gotReceipt, receipt) || gotHash != hash {
		t.Fatal("immutable delivery bytes changed", table, batchID, err)
	}
}

func TestEvidenceUpgradeRejectsInvalidPredecessorsWithoutMutation(t *testing.T) {
	for name, corrupt := range map[string]string{
		"missing column":   "ALTER TABLE evidence_sources RENAME COLUMN prefix_hash TO unexpected_hash",
		"unknown table":    "CREATE TABLE private_notes(note TEXT)",
		"newer generation": fmt.Sprintf("UPDATE database_lifecycle SET data_generation=%d", CurrentDataGeneration+1),
		"newer schema":     fmt.Sprintf("PRAGMA user_version=%d", SupportedSchemaVersion+1),
		"wrong role":       "PRAGMA application_id=0",
	} {
		t.Run(name, func(t *testing.T) {
			path, database := evidencePredecessor(t, 18)
			execLifecycleSQL(t, database, corrupt)
			execLifecycleSQL(t, database, "PRAGMA wal_checkpoint(TRUNCATE)")
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := UpgradeEvidence(t.Context(), path); err == nil {
				t.Fatal("invalid collector upgraded")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected collector was changed", err)
			}
		})
	}
}
