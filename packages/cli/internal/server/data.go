package server

import (
	"context"
	"net/url"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
)

type query = analytics.Query
type Row = analytics.Row
type dashboard = analytics.Dashboard

func parseQuery(values url.Values) (query, error) { return analytics.ParseQuery(values) }
func loadDashboard(ctx context.Context, path string, q query, now time.Time) (dashboard, error) {
	return analytics.LoadLegacyDashboard(ctx, path, q, now)
}
