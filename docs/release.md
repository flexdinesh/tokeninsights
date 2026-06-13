# Releases

## Install

Stable:

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

1. Merge the release-ready code to `main`.
2. Run the **Release TokenInsights CLI** workflow from GitHub Actions.
3. The workflow creates the next `packages/cli/v0.0.x` tag, builds archives, writes `checksums.txt`, and publishes a GitHub Release.

The first release is `packages/cli/v0.0.1`.

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
