package server

import (
	"context"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func loadDataDashboard(ctx context.Context, store *datastore.Store, q query, now time.Time) (dashboard, error) {
	return analytics.LoadDashboard(ctx, store, q, now)
}
func loadDataFacets(ctx context.Context, store *datastore.Store, q query, search string, now time.Time) (serverapi.UsageFacetsResponse, error) {
	facets, err := analytics.LoadFacets(ctx, store, q, search, now)
	if err != nil {
		return serverapi.UsageFacetsResponse{}, err
	}
	return apiFacets(facets), nil
}
