package collector

import (
	"context"
	"errors"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

// RunLegacyForTest retains the original recovery/failure semantic oracles. New
// raw contract tests exercise Run; production has no legacy collection path.
func RunLegacyForTest(ctx context.Context, options Options) (Result, error) {
	var result Result
	if strings.TrimSpace(options.ServerURL) == "" {
		if err := ValidatePaths(options.CollectorDBPath, options.ServerDBPath); err != nil {
			return result, err
		}
	} else {
		if _, err := endpoint(options.ServerURL); err != nil {
			return result, err
		}
	}
	options.SyncOptions.DBPath = options.CollectorDBPath
	if !options.PublishOnly {
		result.Collection, result.CollectionError = pipeline.Sync(ctx, options.SyncOptions)
	}
	if options.SyncOptions.DryRun {
		return result, result.CollectionError
	}
	result.DeliveryError = publishLegacy(ctx, options, &result)
	return result, errors.Join(result.CollectionError, result.DeliveryError)
}
