// Package sqlite composes embedded token storage initialization.
package sqlite

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

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
