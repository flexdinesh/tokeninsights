package evidence

import (
	"encoding/json"
	"testing"
)

func TestDatasetRequiredOnlyInProtocol3(t *testing.T) {
	batch := Batch{ProtocolVersion: ProtocolVersion, ExtractorVersion: ExtractorVersion, DatabaseID: "database", StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []Entry{{Sequence: 1, Record: Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}}
	body, _ := json.Marshal(batch)
	if _, err := DecodeBatch(body); err == nil {
		t.Fatal("v3 accepted missing dataset")
	}
	batch.DatasetID = "user-a"
	body, _ = json.Marshal(batch)
	if got, err := DecodeBatch(body); err != nil || got.EffectiveDatasetID() != "user-a" {
		t.Fatal(got, err)
	}
	batch.ProtocolVersion = LegacyProtocolVersion
	batch.DatasetID = ""
	body, _ = json.Marshal(batch)
	if got, err := DecodeBatch(body); err != nil || got.EffectiveDatasetID() != "default" {
		t.Fatal(got, err)
	}
	batch.DatasetID = "user-a"
	body, _ = json.Marshal(batch)
	if _, err := DecodeBatch(body); err == nil {
		t.Fatal("v2 accepted dataset")
	}
}
