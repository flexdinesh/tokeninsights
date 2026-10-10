package datastore

import (
	"context"
	"database/sql"
	"sync"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
)

// AttachPostgres binds the shared token transactions to an already validated,
// exclusively owned PostgreSQL deployment. The caller owns storage lifetime.
// This is an adapter construction API; domain consumers only receive ports.
func AttachPostgres(ctx context.Context, reader *sql.DB, writer *sync.Mutex, beginWrite func(context.Context) (*sql.Tx, error), ready func(context.Context) error) (*Store, error) {
	s := &Store{database: reader, writer: writer, worker: dataengine.NewWorker(), admissions: make(chan struct{}, 4), datasetID: DatasetID, kind: KindHosted, root: true, nextDataset: new(string), selection: &sync.Mutex{}, beginWrite: beginWrite, checkStorage: ready, postgres: true, closeStorage: func() error { return nil }}
	id, err := s.DatabaseIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if err := inspectDatasets(ctx, reader, id, KindHosted); err != nil {
		return nil, err
	}
	if err := s.prepareAllGenerations(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// PostgreSQL reports the physical query syntax to the SQL analytics adapter.
// Mode policy and public contracts remain independent of this adapter detail.
func (s *Store) PostgreSQL() bool { return s.postgres }
