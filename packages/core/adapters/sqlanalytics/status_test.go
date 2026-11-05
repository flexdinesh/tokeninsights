package sqlanalytics

import (
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"path/filepath"
	"testing"
)

func TestStatusCountsOnlyFailedPendingScopes(t *testing.T) {
	for _, test := range []struct {
		name                     string
		scopes                   string
		pending, failed, retryAt int64
	}{
		{"completed", "('default','pi:session:complete',1,1,1,'processing_failed',1234)", 0, 0, 0},
		{"pending", "('default','pi:session:failed',1,0,0,'processing_failed',1234),('default','pi:session:pending',1,0,0,'',0)", 2, 1, 1234},
		{"due", "('default','pi:session:failed',1,0,0,'processing_failed',1234),('default','pi:session:due',1,0,0,'processing_failed',0)", 2, 2, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			if _, err := store.SQL().Exec("UPDATE ingestion_metadata SET input_revision=1"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SQL().Exec("INSERT INTO processing_scopes(dataset_id,scope,revision,processed_revision,generation,error_code,retry_at_ms) VALUES " + test.scopes); err != nil {
				t.Fatal(err)
			}
			status, err := Status(t.Context(), store)
			if err != nil || status.Metadata.DatasetID != datastore.DatasetID || status.Pending != test.pending || status.Failed != test.failed || status.FailedRetryAtMs != test.retryAt {
				t.Fatal(status, err)
			}
		})
	}
}
