package pipeline

import (
	"context"
	"path"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// ExtractLocation is explicit local enrichment; it never interprets counters.
func ExtractLocation(ctx context.Context, options SyncOptions, directory, remote, project string) *evidence.Location {
	location, _ := resolveFactLocation(ctx, options, directory, remote, project)
	if location == nil {
		return nil
	}
	label := locationLabel(path.Base(strings.ReplaceAll(location.DirectoryName, "\\", "/")))
	result := &evidence.Location{DirectoryKey: location.DirectoryKey, DirectoryName: label, RepositoryKey: location.RepositoryKey, RepositoryName: location.RepositoryName, RepositorySource: location.RepositorySource}
	if result.DirectoryKey == "" {
		result.DirectoryName = ""
	}
	if !evidence.SafeMetadata(result.DirectoryName) {
		result.DirectoryName = "unknown"
	}
	if !evidence.SafeMetadata(result.RepositoryName) {
		result.RepositoryName = "unknown"
	}
	return result
}
