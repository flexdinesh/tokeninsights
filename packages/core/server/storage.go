package server

import (
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
)

// DataSource borrows personal query storage selected by composition. HTTP never
// opens databases or changes the dataset from request input.
type DataSource interface {
	Kind() string
	Queries(datasetID string) analytics.Repository
}
