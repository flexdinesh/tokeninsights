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
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@v0.1.3
```

Development version directly from `main`:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@main
```

Go resolves `@latest` from stable module tags (`packages/cli/vX.Y.Z`), independently of GitHub's Latest badge. `@main` resolves the moving `main` branch to a Go pseudo-version, or a stable version when that commit is tagged. It does not wait for CI. Module proxies may briefly cache branch lookups; rerun the install after a push to update.

## Release

Required secret:

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge the release-ready code to `main`.
2. Run the **Release** workflow from GitHub Actions with branch `main` selected. Dispatches from other refs are skipped. The workflow checks out the latest `main` when it starts; that commit is verified and released even if `main` advances during the run.
3. The workflow selects the next patch version, builds macOS/Linux amd64/arm64 archives, writes `checksums.txt`, then pushes the Go module tag and publishes a stable GitHub Release explicitly marked Latest. New versions and GitHub releases are published only through this manual workflow.
4. The workflow generates `Formula/tokeninsights.rb` from the local release checksums and opens or updates a pull request against `flexdinesh/homebrew-tap`.

`.release-version` contains the active `major.minor` release series. It is currently `0.1`; releases increment the highest existing patch in that series. A rerun from the same commit reuses that commit's existing tag and updates its release assets and Latest badge.

To begin a new minor or major series, change `.release-version`. For example, changing it to `0.2` makes the next release `packages/cli/v0.2.0`; changing it to `1.0` makes the next release `packages/cli/v1.0.0`. Later releases automatically increment that series' patch number.

React assets are committed under `packages/cli/internal/server/static` and embedded with `go:embed` in every binary, including stable and main Go installs. Local pre-push verification rebuilds the frontend and checks asset drift. Formatting, schema/API consistency, full/race tests, benchmarks, browser tests and container contracts run in local pre-push verification. CI/release native jobs build both binaries and test storage on each supported OS/architecture, covering platforms unavailable on one developer host. Release validates its generated archives and Homebrew formula; preparation does not repeat local checks. Frontend changes must include regenerated assets (`pnpm run build:web`). The private `@tokeninsights/build-tools` workspace package under `tools/build` owns generated-asset checks and Homebrew formula generation; it is build/release-time tooling only.

Release artifacts contain only the two native Go binaries and documentation. Production hosts need no Node.js, npm, pnpm, `node_modules`, repository JavaScript tooling, or separate web files. Go serves the embedded browser JavaScript as bytes; it executes only in the browser, and the Go runtime never invokes a JavaScript runtime.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

## Verify Locally

### Database Compatibility

Collector SQLite 20, token SQLite 1, PostgreSQL token/account 1, application SQLite 2 and jobs SQLite 1 have
distinct roles. Only current contracts are supported. Incompatible, newer or corrupt
contracts reject without mutation. There are no migrations or legacy imports.

Sync captures evidence and waits for durable acceptance; processing runs
asynchronously. Local maintenance uses `data reprocess|wait` with command-owned storage.

Verification preserves native contribution identities, all token components, exact
receipt replay, source continuity, separate estimates, dataset-local generation
cutover and colliding multi-user identities. Verify auth/revocation, capability gates,
local progress and container persistence/shutdown. Both binaries must work without
Node/npm/pnpm. See [design](design.md) for the complete contract.

### Checks

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

## Version

```sh
tokeninsights --version
```

## Separate server executable

The existing release archives and Homebrew formula contain both `tokeninsights`
and `tokeninsights-server`. Go users install remote explicitly with
`go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights-server@latest`.
Both embed committed assets and run without Node/npm/pnpm. Remote runs only when
explicitly launched and requires `--server-db-path`; installing it starts nothing.

Native CI/release Linux/macOS amd64/arm64 jobs build with `CGO_ENABLED=0` and test on matching runners. See [design](design.md) for current acceptance/migration.

Live PostgreSQL verification: `pnpm test` and `pnpm test:race` start a pinned
PostgreSQL 18 Docker container and remove it afterward. Alternatively set
`TOKENINSIGHTS_TEST_POSTGRES_DSN` to a test server with CREATEDB privileges; tests
create random databases and delete only those. Never use production credentials.
Direct `go test` skips PostgreSQL cases without that variable; root verification
always supplies a live database and fails if it cannot start one.
