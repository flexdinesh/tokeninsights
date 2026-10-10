package analytics

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

// Facets contains filter choices from one dataset and one read snapshot.
type Facets struct {
	Providers, Models, Harnesses, Sessions []string
	Repositories, Directories              []querymodel.LocationOption
	Revision, Generation, InputRevision    int64
	DatabaseID, DatasetID                  string
}

// ProcessingStatus describes one dataset without exposing storage to HTTP adapters.
type ProcessingStatus struct {
	Metadata        dataengine.Metadata
	Pending         int64
	Failed          int64 // Pending scopes with a durable error; completed scopes are excluded.
	FailedRetryAtMs int64 // Earliest retry time among failed pending scopes.
	Hostname        string
}
