# Releases

## Install

Stable:

```sh
brew install flexdinesh/tap/tokeninsights
```

Alternative stable install with Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

Specific stable version:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@v0.0.1
```

Development version from `dev`:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

## Release

Required secret:

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge the release-ready code to `main`.
2. Run the **Release TokenInsights CLI** workflow from GitHub Actions.
3. The workflow creates the next `packages/cli/v0.0.x` tag, builds archives, writes `checksums.txt`, and publishes a GitHub Release.
4. The workflow generates `Formula/tokeninsights.rb` from the local release checksums and opens or updates a pull request against `flexdinesh/homebrew-tap`.

The first release is `packages/cli/v0.0.1`.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

## Verify Locally

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

## Version

```sh
tokeninsights --version
```
