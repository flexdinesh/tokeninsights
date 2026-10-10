package datastore

import (
	"context"
	"database/sql"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func (s *Store) DatabaseIdentity(ctx context.Context) (string, error) {
	var id string
	err := s.database.QueryRowContext(ctx, sqlutil.Bind("SELECT database_id FROM ingestion_instance WHERE id=1")).Scan(&id)
	return id, err
}
func (s *Store) DatasetExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.database.QueryRowContext(ctx, sqlutil.Bind("SELECT EXISTS(SELECT 1 FROM ingestion_metadata WHERE dataset_id=?)"), id).Scan(&exists)
	return exists, err
}
func (s *Store) EnsureDataset(ctx context.Context, id string) error {
	return s.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT EXISTS(SELECT 1 FROM ingestion_metadata WHERE dataset_id=?)"), id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		return s.CreateDatasetInTx(ctx, tx, id)
	})
}
func (s *Store) ReprocessDataset(ctx context.Context, id string) (int64, error) {
	return s.ForDataset(id).Reprocess(ctx)
}
