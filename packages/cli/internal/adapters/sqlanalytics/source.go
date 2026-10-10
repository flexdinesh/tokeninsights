// Package sqlanalytics adapts relational token storage and SQL analytics to server ports.
package sqlanalytics

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type Source struct{ Store *datastore.Store }

func (s Source) Kind() string {
	if s.Store == nil {
		return ""
	}
	return s.Store.Kind()
}
func (s Source) Receiver(datasetID string) evidence.Receiver { return s.Store.ForDataset(datasetID) }
func (s Source) Queries(datasetID string) analytics.Repository {
	return Queries{Store: s.Store.ForDataset(datasetID)}
}

var _ analytics.Repository = Queries{}
var _ evidence.Receiver = (*datastore.Store)(nil)
