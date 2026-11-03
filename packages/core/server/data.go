package server

import (
	"net/url"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/server/api"
)

type query = analytics.Query
type Row = analytics.Row
type dashboard = analytics.Dashboard

func parseQuery(values url.Values) (query, error) { return api.ParseQuery(values) }
