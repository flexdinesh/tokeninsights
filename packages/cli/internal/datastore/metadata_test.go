package datastore

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func TestMetadataRejectsInvalidGenerationContracts(t *testing.T) {
	for _, test := range []struct {
		name, change, code string
	}{
		{"missing active", "DELETE FROM analytics_generations", sql.ErrNoRows.Error()},
		{"missing target", "UPDATE ingestion_metadata SET target_generation=2", sql.ErrNoRows.Error()},
		{"inactive current", "UPDATE analytics_generations SET state='retained'", "incompatible_server_data"},
		{"multiple active", "INSERT INTO analytics_generations SELECT dataset_id,2,processor_version,'active',created_at_ms,0 FROM analytics_generations", "incompatible_server_data"},
		{"inactive target", "INSERT INTO analytics_generations SELECT dataset_id,2,processor_version,'retained',created_at_ms,0 FROM analytics_generations; UPDATE ingestion_metadata SET target_generation=2", "incompatible_server_data"},
		{"newer current processor", "UPDATE analytics_generations SET processor_version=processor_version+1", "newer_processor_version"},
		{"newer retained processor", "INSERT INTO analytics_generations SELECT dataset_id,2,processor_version+1,'retained',created_at_ms,0 FROM analytics_generations", "newer_processor_version"},
		{"negative revision", "UPDATE ingestion_metadata SET input_revision=-1", "incompatible_server_data"},
		{"backward generation", "UPDATE ingestion_metadata SET target_generation=0", "incompatible_server_data"},
		{"empty identity", "UPDATE ingestion_metadata SET database_id=''", "incompatible_server_data"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			if _, err := store.SQL().ExecContext(t.Context(), sqlutil.Bind(test.change)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Metadata(t.Context()); err == nil || err.Error() != test.code {
				t.Fatalf("metadata error: %v, want %s", err, test.code)
			}
		})
	}
}

func BenchmarkMetadataRead(b *testing.B) {
	for _, generations := range []int{1, 1000} {
		b.Run(fmt.Sprintf("Generations%d", generations), func(b *testing.B) {
			store, err := Open(b.Context(), filepath.Join(b.TempDir(), "server.sqlite"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			if _, err := store.SQL().ExecContext(b.Context(), sqlutil.Bind("INSERT INTO analytics_generations SELECT ?,range,?,'retained',1,0 FROM range(2,?)"), DatasetID, evidence.ProcessorVersion, generations+1); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				metadata, err := store.Metadata(b.Context())
				if err != nil || metadata.DatasetID != DatasetID || metadata.Generation != 1 || metadata.TargetGeneration != 1 {
					b.Fatal("invalid snapshot", metadata, err)
				}
			}
		})
	}
}

func TestMetadataGenerationGuardsAreDatasetScoped(t *testing.T) {
	root, alice, bob := hostedStores(t)
	if _, err := root.SQL().ExecContext(t.Context(), sqlutil.Bind("UPDATE analytics_generations SET processor_version=? WHERE dataset_id=?"), evidence.ProcessorVersion+1, bob.DatasetID()); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Metadata(t.Context()); err != nil {
		t.Fatal("other dataset changed local generation checks", err)
	}
	if _, err := bob.Metadata(t.Context()); err == nil || err.Error() != "newer_processor_version" {
		t.Fatal("newer dataset accepted", err)
	}
	if _, err := root.ForDataset("missing").Metadata(t.Context()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing dataset accepted", err)
	}
}

func TestMetadataUsesCallersSnapshotAcrossGenerationReplacement(t *testing.T) {
	store := testStore(t)
	tx, err := store.BeginRead(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	before, err := ReadMetadata(t.Context(), tx)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := store.Reprocess(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	old, err := ReadMetadata(t.Context(), tx)
	if err != nil || old != before {
		t.Fatal("snapshot changed", before, old, err)
	}
	current, err := store.Metadata(t.Context())
	if err != nil || current.Generation != generation || current.TargetGeneration != generation || current.Generation == before.Generation {
		t.Fatal("replacement not visible to new reader", current, err)
	}
}
