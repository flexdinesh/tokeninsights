package appstore

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlitecore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
)

// InspectCompatibility classifies an existing application file read-only for one kind.
// Pairing is still verified when opened.
func InspectCompatibility(ctx context.Context, path, kind string) (storagecompat.Result, error) {
	schema, version := schemaFor(kind)
	return sqlitecore.Inspect(ctx, path, ApplicationID, version, schema)
}
