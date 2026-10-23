package datastore

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlitecore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
)

// InspectCompatibility classifies an existing token SQLite file read-only. Compatible files
// still pass full metadata validation when opened.
func InspectCompatibility(ctx context.Context, path string) (storagecompat.Result, error) {
	return sqlitecore.Inspect(ctx, path, ApplicationID, SchemaVersion, Schema)
}
