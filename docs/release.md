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

Latest `main` source that passed native CI:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@edge
```

Go resolves `@latest` from stable module tags (`packages/cli/vX.Y.Z`), independently of GitHub's Latest badge. `edge` is a moving source branch published after native CI succeeds for a main push. It contains that source plus generated dashboard assets. Its latest stable release ancestry makes its Go pseudo-version sort above that release. Module proxies may briefly cache branch lookups; rerun the install after publication to update.

`main`, feature branches and their source SHAs omit generated assets, so direct
versioned installs from them fail on the missing Go embed input. For local builds:

```sh
mise run setup
mise run build:web
cd packages/cli
GOWORK=off go build ./cmd/tokeninsights ./cmd/tokeninsights-server
```

mise build/test/lint tasks generate assets before invoking Go.
An existing binary runs without Node/npm/pnpm. Source installs report Go's resolved
module version; archive binaries include explicit release version/commit metadata.

## Release

Required secret:

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge the release-ready code to `main`.
2. Run the **Release** workflow from GitHub Actions with branch `main` selected. Dispatches from other refs are skipped. The workflow checks out the latest `main` when it starts; that commit is verified and released even if `main` advances during the run.
3. The workflow selects the next patch version, builds the dashboard, and creates an asset commit with the resolved main source as its first parent. Native jobs build macOS/Linux amd64/arm64 archives from that same commit. After verification, it pushes only the Go module tag, writes `checksums.txt`, and publishes a stable GitHub Release explicitly marked Latest. New versions and GitHub releases are published only through this manual workflow.
4. The workflow generates `Formula/tokeninsights.rb` from the local release checksums and opens or updates a pull request against `flexdinesh/homebrew-tap`.

`.release-version` contains the active `major.minor` release series. It is currently `0.1`; releases increment the highest existing patch in that series. A rerun against the same source reuses its existing tag, identified by the tag commit's first parent. Existing published tags never move. Completed release archives/checksums are downloaded, verified and reused during recovery.

To resume a release created by this workflow after main advances,
dispatch **Release** from `main` with `retry_tag` set to its full module tag.
Historical tags directly on main remain installable but cannot use this recovery path.
This uses the original tagged source
and version. Retrying an older version preserves the newer Latest release.
If an existing GitHub release has incomplete assets, recovery stops;
restore the original native workflow artifacts and checksum manifest before retrying.

To begin a new minor or major series, change `.release-version`. For example, changing it to `0.2` makes the next release `packages/cli/v0.2.0`; changing it to `1.0` makes the next release `packages/cli/v1.0.0`. Later releases automatically increment that series' patch number.

React assets under `packages/cli/internal/server/static` are ignored on `main` and embedded with `go:embed` after generation. CI alone commits them on `edge` and stable tags; source branch history stays free of bundle updates. Edge publication uses an observed-ref lease and rejects obsolete runs. Repository rulesets must allow the workflow identity to update `edge` and create `packages/cli/v*` tags. No `dev` archive channel is added. See [ADR 0017](adr/0017-generated-assets-on-installable-refs.md).

Formatting, schema/API consistency, full/race tests, benchmarks, browser tests,
container contracts and isolated source-install checks run in local pre-push.
CI/release native jobs build both binaries and test storage on each supported
OS/architecture, covering platforms unavailable on one developer host. Release
validates generated archives and the Homebrew formula. The private
`@tokeninsights/build-tools` workspace package owns asset generation, installable
refs, local publication tests and Homebrew formula generation; it is build tooling.

Release artifacts contain only the two native Go binaries and documentation. Production hosts need no Node.js, npm, pnpm, `node_modules`, repository JavaScript tooling, or separate web files. Go serves the embedded browser JavaScript as bytes; it executes only in the browser, and the Go runtime never invokes a JavaScript runtime.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

## Verify Locally

### Database Compatibility

Collector SQLite 20, token SQLite 1, PostgreSQL token/account 1/2, personal application
SQLite 2, hosted application SQLite 3 and jobs SQLite 1 have
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
mise run check-schema
mise run test
mise run build
mise run test:distribution
```

## Version

```sh
tokeninsights --version
```

## Separate server executable

The existing release archives and Homebrew formula contain both `tokeninsights`
and `tokeninsights-server`. Go users install remote explicitly with
`go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights-server@latest`.
Both embed generated assets from stable tags or `edge` and run without Node/npm/pnpm. Remote runs only when
explicitly launched and requires `--server-db-path`; installing it starts nothing.

Native CI/release Linux/macOS amd64/arm64 jobs build with `CGO_ENABLED=0` and test on matching runners. See [design](design.md) for current acceptance/migration.

Live PostgreSQL verification: `mise run test` and `mise run test:race` start a pinned
PostgreSQL 18 Docker container and remove it afterward. Alternatively set
`TOKENINSIGHTS_TEST_POSTGRES_DSN` to a test server with CREATEDB privileges; tests
create random databases and delete only those. Never use production credentials.
Direct `go test` skips PostgreSQL cases without that variable; root verification
always supplies a live database and fails if it cannot start one.
