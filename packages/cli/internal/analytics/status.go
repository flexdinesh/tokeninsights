package analytics

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

// Facets contains filter choices from one dataset and one read snapshot.
type Facets struct {
	Providers, Models, Harnesses, Sessions []string
	Repositories, Directories              []db.LocationOption
	Revision, Generation, InputRevision    int64
	DatabaseID, DatasetID                  string
}

// ProcessingStatus describes one dataset without exposing storage to HTTP adapters.
type ProcessingStatus struct {
	Metadata        datastore.Metadata
	Pending         int64
	Failed          int64 // Pending scopes with a durable error; completed scopes are excluded.
	FailedRetryAtMs int64 // Earliest retry time among failed pending scopes.
	Hostname        string
}

func Status(ctx context.Context, store *datastore.Store) (ProcessingStatus, error) {
	result := ProcessingStatus{Hostname: "unknown"}
	tx, err := store.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result.Metadata, err = datastore.ReadMetadataForDataset(ctx, tx, store.DatasetID())
	if err != nil {
		return result, err
	}
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*), COUNT(*) FILTER (WHERE error_code<>''), COALESCE(MIN(retry_at_ms) FILTER (WHERE error_code<>''),0) FROM processing.scopes WHERE dataset_id=? AND (processed_revision<>revision OR generation<>?)", store.DatasetID(), result.Metadata.TargetGeneration).Scan(&result.Pending, &result.Failed, &result.FailedRetryAtMs)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}
