// Package duckdb adapts embedded token storage and SQL analytics to server ports.
package duckdb

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type Source struct{ Store *datastore.Store }

func (s Source) Kind() string {
	if s.Store == nil {
		return ""
	}
	return s.Store.Kind()
}
func (s Source) Receiver(datasetID string) evidence.Receiver { return s.Store.ForDataset(datasetID) }
func (s Source) Queries(datasetID string) analytics.Repository {
	return Queries{Store: s.Store.ForDataset(datasetID)}
}

// Open serializes embedded initialization. Composition separately holds the
// database's lifetime lock until workers, requests and storage have closed.
func Open(ctx context.Context, path string, options datastore.Options) (*datastore.Store, error) {
	release, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return nil, err
	}
	defer release()
	return datastore.OpenWithOptions(ctx, path, options)
}

var _ analytics.Repository = Queries{}
var _ evidence.Receiver = (*datastore.Store)(nil)
