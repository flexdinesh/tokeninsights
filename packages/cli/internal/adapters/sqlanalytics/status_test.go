package sqlanalytics

import (
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func TestStatusCountsOnlyFailedPendingScopesInAuthorizedDataset(t *testing.T) {
	store, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"), datastore.KindHosted)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	for _, dataset := range []string{"alice", "bob", "carol"} {
		if err := store.CreateDataset(t.Context(), dataset); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SQL().Exec(sqlutil.Bind("UPDATE ingestion_metadata SET input_revision=1")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec(sqlutil.Bind(`INSERT INTO processing_scopes(dataset_id,scope,revision,processed_revision,generation,error_code,retry_at_ms) VALUES
		('alice','pi:session:complete',1,1,1,'processing_failed',1234),
		('bob','pi:session:failed',1,0,0,'processing_failed',1234),
		('bob','pi:session:pending',1,0,0,'',0),
		('carol','pi:session:failed',1,0,0,'processing_failed',1234),
		('carol','pi:session:due',1,0,0,'processing_failed',0)`)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		dataset                  string
		pending, failed, retryAt int64
	}{{"alice", 0, 0, 0}, {"bob", 2, 1, 1234}, {"carol", 2, 2, 0}} {
		status, err := Status(t.Context(), store.ForDataset(test.dataset))
		if err != nil || status.Metadata.DatasetID != test.dataset || status.Pending != test.pending || status.Failed != test.failed || status.FailedRetryAtMs != test.retryAt {
			t.Fatal("status included completed or other-dataset failures", status, err)
		}
	}
}
