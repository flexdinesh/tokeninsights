package duckdb

import (
	"context"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

// Queries is the embedded analytics adapter. Its store is scoped by composition
// after authentication; consumers do not open paths or execute SQL.
type Queries struct{ Store *datastore.Store }

func (d Queries) Dashboard(ctx context.Context, q analytics.Query, now time.Time) (analytics.Dashboard, error) {
	return LoadDashboard(ctx, d.Store, q, now)
}

// AllDashboard reads bounded complete results within one publication snapshot.
func (d Queries) AllDashboard(ctx context.Context, q analytics.Query, now time.Time, maxRows int) (analytics.Dashboard, error) {
	return LoadAllDashboard(ctx, d.Store, q, now, maxRows)
}

func (d Queries) Facets(ctx context.Context, q analytics.Query, search string, now time.Time) (analytics.Facets, error) {
	return LoadFacets(ctx, d.Store, q, search, now)
}

func (d Queries) Status(ctx context.Context) (analytics.ProcessingStatus, error) {
	return Status(ctx, d.Store)
}
