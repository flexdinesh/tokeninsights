package pipeline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestNormalizeSchema17PreviewAndUpgradePreserveDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"evidence_batches", "evidence_destinations", "evidence_outbox", "evidence_sources", "evidence_state"} {
		if _, err := database.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := os.ReadFile("../db/schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, old, _ := strings.Cut(string(schema), "-- Sanitized raw outbox.")
	old = strings.ReplaceAll(old, "  dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256),\n", "")
	old = strings.ReplaceAll(old, "  protocol_version INTEGER NOT NULL DEFAULT 3 CHECK (protocol_version IN (2, 3)),\n", "")
	old = strings.ReplaceAll(old, "database_id,dataset_id,protocol_version,first_sequence", "database_id,first_sequence")
	old = strings.ReplaceAll(old, "user_version = 18", "user_version = 17")
	if _, err := database.Exec("-- Sanitized raw outbox." + old); err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}
	recordBytes, _ := json.Marshal(record)
	batch := evidence.Batch{ProtocolVersion: 2, ExtractorVersion: 1, DatabaseID: "server", StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}}
	request, _ := json.MarshalIndent(batch, "", "  ")
	for _, statement := range []string{
		"INSERT INTO evidence_state VALUES(1,'stream',1,1)",
		"INSERT INTO evidence_sources VALUES('source-key','source','lineage','pi-jsonl',1,12,1,'prefix','{}',1)",
		"INSERT INTO evidence_destinations(destination_id,endpoint,database_id) VALUES('destination','http://remote','server')",
		"INSERT INTO raw_token_usage(raw_fact_key,harness,source_id,source_kind,collector,parser,observed_at_ms,occurred_at_ms,session_id,message_id,usage_scope,quality,input_tokens,output_tokens,total_tokens,metadata_json) VALUES('fact','pi','source','jsonl','fixture','fixture',1,1,'session','message','message','exact',2,3,5,'{\"session_source\":\"native\"}')",
		"INSERT INTO normalization_work_queue(raw_fact_id,domain,enqueued_at_ms) VALUES(1,'token_usage',1)",
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec("INSERT INTO evidence_outbox(sequence,observation_key,harness,record_json,created_at_ms) VALUES(1,?,'pi',?,1)", evidence.Hash(recordBytes), string(recordBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes) VALUES('batch','destination','stream','server',1,1,?,?)", evidence.Hash(request), request); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	options := NormalizeOptions{DBPath: path, Sources: &SourceConfig{}, DryRun: true, Harnesses: []Harness{HarnessPi}}
	preview, err := Normalize(t.Context(), options)
	if err != nil || preview.Canonical != 1 {
		t.Fatal(preview, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("preview changed schema17 file", err)
	}
	options.DryRun = false
	result, err := Normalize(t.Context(), options)
	if err != nil || result.Canonical != 1 {
		t.Fatal(result, err)
	}
	database, err = db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var version, protocol, cursor, offset, count int
	var saved []byte
	var hash, dataset, stream string
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 18 {
		t.Fatal(version, err)
	}
	if err := database.QueryRow("SELECT request_bytes,request_hash,dataset_id,protocol_version,stream_id FROM evidence_batches").Scan(&saved, &hash, &dataset, &protocol, &stream); err != nil || !bytes.Equal(saved, request) || hash != evidence.Hash(request) || dataset != "default" || protocol != 2 || stream != "stream" {
		t.Fatal("normalization changed immutable delivery", err)
	}
	if err := database.QueryRow("SELECT acknowledged_sequence FROM evidence_destinations").Scan(&cursor); err != nil || cursor != 0 {
		t.Fatal(cursor, err)
	}
	if err := database.QueryRow("SELECT byte_offset FROM evidence_sources").Scan(&offset); err != nil || offset != 12 {
		t.Fatal(offset, err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := database.QueryRow("SELECT total_tokens FROM canonical_token_usage").Scan(&count); err != nil || count != 5 {
		t.Fatal("normalization semantics changed", count, err)
	}
}
