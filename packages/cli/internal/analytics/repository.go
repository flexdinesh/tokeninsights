package analytics

import (
	"context"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

// Repository reads one authorized dataset. Implementations must return rows,
// totals and publication metadata from a consistent snapshot. No query can
// select a different dataset through user-supplied filter values.
type Repository interface {
	Dashboard(context.Context, Query, time.Time) (Dashboard, error)
	Facets(context.Context, Query, string, time.Time) (Facets, error)
	Status(context.Context) (ProcessingStatus, error)
}

// DuckDB is the embedded analytics adapter. Its store is scoped by composition
// after authentication; consumers do not open paths or execute SQL.
type DuckDB struct{ Store *datastore.Store }

func (d DuckDB) Dashboard(ctx context.Context, q Query, now time.Time) (Dashboard, error) {
	return LoadDashboard(ctx, d.Store, q, now)
}

func (d DuckDB) Facets(ctx context.Context, q Query, search string, now time.Time) (Facets, error) {
	return LoadFacets(ctx, d.Store, q, search, now)
}

func (d DuckDB) Status(ctx context.Context) (ProcessingStatus, error) {
	return Status(ctx, d.Store)
}
