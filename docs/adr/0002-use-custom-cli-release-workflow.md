# Use a custom CLI release workflow

TokenInsights uses a GitHub Actions release workflow instead of GoReleaser because the CLI module remains rooted at `packages/cli` and stable Go install tags must use the `packages/cli/vX.Y.Z` prefix. The custom workflow preserves that Go-compatible tag shape, builds the macOS and Linux archives directly with `go build`, injects version metadata with linker flags, generates checksums, and publishes a GitHub Release without requiring root SemVer tags or paid monorepo release features.
