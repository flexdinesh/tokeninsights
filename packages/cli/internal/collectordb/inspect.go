package collectordb

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlitecore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
)

// InspectCompatibility classifies an existing collector file read-only.
func InspectCompatibility(ctx context.Context, path string) (storagecompat.Result, error) {
	return sqlitecore.Inspect(ctx, path, CollectorApplicationID, SupportedSchemaVersion, schema)
}
