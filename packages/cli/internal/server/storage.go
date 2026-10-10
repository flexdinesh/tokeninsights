package server

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// DataSource binds storage ports to an authenticated dataset. Composition selects
// the adapter once; HTTP never opens databases or selects a backend from a request.
// Receiver and Queries must use the same database/dataset identity. Scoping never
// creates a dataset or falls back to another dataset when it is absent.
type DataSource interface {
	Kind() string
	Receiver(datasetID string) evidence.Receiver
	Queries(datasetID string) analytics.Repository
}
