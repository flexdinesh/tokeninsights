package server

import (
	"context"
	"fmt"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

// DirectQuery adapts the same query validation, analytics and presentation used
// by REST without constructing HTTP requests or opening a listener.
type DirectQuery struct {
	source             analytics.Repository
	policy             serverfeatures.Policy
	instance, hostname string
	defaults           viewer.Selection
}

// NewDirectQuery advertises the composition's policy, like the HTTP instance route.
func NewDirectQuery(source analytics.Repository, policy serverfeatures.Policy, instance, hostname string) *DirectQuery {
	return &DirectQuery{source: source, policy: policy, instance: instance, hostname: hostname, defaults: viewer.Selection{Period: "month", Bucket: "day"}}
}
func (d *DirectQuery) Instance(ctx context.Context) (api.InstanceResponseV2, error) {
	status, err := d.source.Status(ctx)
	if err != nil {
		return api.InstanceResponseV2{}, err
	}
	hostname := status.Hostname
	if d.hostname != "" {
		hostname = d.hostname
	}
	// Direct readers are read-only; ingestion uses its own in-process destination.
	return instanceResponse(d.policy, []string{accounts.Read}, status, d.instance, hostname, d.defaults), nil
}

func (d *DirectQuery) Usage(ctx context.Context, params api.GetUsageParams) (api.UsageResponseV2, error) {
	return d.usage(ctx, params, 0)
}

// AllUsage is composed for local viewers only; REST retains bounded pages.
func (d *DirectQuery) AllUsage(ctx context.Context, params api.GetUsageParams, maxRows int) (api.UsageResponseV2, error) {
	if maxRows < 1 {
		return api.UsageResponseV2{}, fmt.Errorf("invalid analytics row limit")
	}
	return d.usage(ctx, params, maxRows)
}

func (d *DirectQuery) usage(ctx context.Context, params api.GetUsageParams, maxRows int) (api.UsageResponseV2, error) {
	q, err := parseQuery(api.UsageValues(params))
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	var data analytics.Dashboard
	if maxRows > 0 {
		data, err = d.source.AllDashboard(ctx, q, time.Now(), maxRows)
	} else {
		data, err = d.source.Dashboard(ctx, q, time.Now())
	}
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	result := apiDashboard(data)
	result.InstanceId, result.DataEpoch = d.instance, data.DatabaseID
	return result, nil
}

func (d *DirectQuery) Facets(ctx context.Context, params api.GetUsageFacetsParams) (api.UsageFacetsResponseV2, error) {
	values := api.FacetValues(params)
	q, err := parseQuery(values)
	if err != nil {
		return api.UsageFacetsResponseV2{}, err
	}
	facets, err := d.source.Facets(ctx, q, values.Get("search"), time.Now())
	result := apiFacets(facets)
	result.InstanceId = d.instance
	return result, err
}

func (d *DirectQuery) Status(ctx context.Context) (api.StatusResponseV2, error) {
	status, err := d.source.Status(ctx)
	if err != nil {
		return api.StatusResponseV2{}, err
	}
	m := status.Metadata
	return api.StatusResponseV2{InstanceId: d.instance, DataEpoch: m.DatabaseID, DatasetId: m.DatasetID, DataReadiness: "ready", Revision: m.Revision, Generation: m.Generation, TargetGeneration: m.TargetGeneration, InputRevision: m.InputRevision, Pending: status.Pending, Failed: &status.Failed, FailedRetryAtMs: &status.FailedRetryAtMs}, nil
}
