# Use Go module-prefixed CLI release tags

The Go module for the TokenInsights CLI remains rooted at `packages/cli` to preserve the monorepo layout. Stable CLI release tags therefore use the Go-compatible `packages/cli/vX.Y.Z` form instead of plain `vX.Y.Z` or shorter `cli/vX.Y.Z` tags, so `go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@vX.Y.Z` resolves correctly.
