package server

import (
	"net/url"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
)

type query = analytics.Query
type Row = analytics.Row
type dashboard = analytics.Dashboard

func parseQuery(values url.Values) (query, error) { return analytics.ParseQuery(values) }
