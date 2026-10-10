package duckdb

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

func Status(ctx context.Context, store *datastore.Store) (analytics.ProcessingStatus, error) {
	result := analytics.ProcessingStatus{Hostname: "unknown"}
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
