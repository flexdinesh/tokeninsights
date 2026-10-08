package server

import (
	"context"
	"io"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

// DirectQuery adapts the same query validation, analytics and presentation used
// by REST without constructing HTTP requests or opening a listener.
type DirectQuery struct{ app *app }

func NewDirectQuery(ctx context.Context, source analytics.Repository, instance string) *DirectQuery {
	a := newApp(ctx, Options{InstanceID: instance, Defaults: viewer.Selection{Period: "month", Bucket: "day"}}, io.Discard)
	a.queries = source
	return &DirectQuery{app: a}
}

func (d *DirectQuery) Instance(ctx context.Context) (api.InstanceResponse, error) {
	status, err := d.app.queries.Status(ctx)
	if err != nil {
		return api.InstanceResponse{}, err
	}
	return api.InstanceResponse{ApiVersion: api.V1, DataEpoch: status.Metadata.DatabaseID, DataReadiness: api.InstanceResponseDataReadinessReady, InstanceId: d.app.options.InstanceID, ServerVersion: version.Version, Hostname: "unknown", Timezone: reportingTimezone(time.Local, time.Now()), Defaults: apiSelection(d.app.options.Defaults)}, nil
}

func (d *DirectQuery) Usage(ctx context.Context, params api.GetUsageParams) (api.UsageResponse, error) {
	q, err := parseQuery(api.UsageValues(params))
	if err != nil {
		return api.UsageResponse{}, err
	}
	data, err := d.app.queries.Dashboard(ctx, q, time.Now())
	if err != nil {
		return api.UsageResponse{}, err
	}
	result := apiDashboard(data)
	result.InstanceId, result.DataEpoch = d.app.options.InstanceID, data.DatabaseID
	return result, nil
}

func (d *DirectQuery) Facets(ctx context.Context, params api.GetUsageFacetsParams) (api.UsageFacetsResponse, error) {
	values := api.FacetValues(params)
	q, err := parseQuery(values)
	if err != nil {
		return api.UsageFacetsResponse{}, err
	}
	facets, err := d.app.queries.Facets(ctx, q, values.Get("search"), time.Now())
	result := apiFacets(facets)
	result.InstanceId = d.app.options.InstanceID
	return result, err
}

func (d *DirectQuery) Status(ctx context.Context) (api.SyncResponse, error) {
	status, err := d.app.queries.Status(ctx)
	if err != nil {
		return api.SyncResponse{}, err
	}
	m := status.Metadata
	return apiSyncState(syncState{InstanceID: d.app.options.InstanceID, DataEpoch: m.DatabaseID, DataReadiness: "ready", Revision: uint64(m.Revision), Generation: m.Generation, InputRevision: m.InputRevision, Pending: status.Pending, Running: status.Pending > 0, Phase: "ready", Harnesses: map[string]string{}}), nil
}
