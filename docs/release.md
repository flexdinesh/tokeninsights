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

React assets are committed under `packages/cli/internal/server/static` and embedded with `go:embed` in every binary, including stable and main Go installs. Local pre-push verification rebuilds the frontend and checks asset drift. CI/release run only `mise run check:ci`: formatting, SQLite contract consistency, and native build. Test suites and browser installation stay local. Frontend changes must include regenerated assets (`pnpm run build:web`). The private `@tokeninsights/build-tools` workspace package under `tools/build` owns generated-asset checks and Homebrew formula generation; it is build/release-time tooling only.

Release artifacts contain only the native Go binary and documentation. Production hosts need no Node.js, npm, pnpm, `node_modules`, repository JavaScript tooling, or separate web files. Go serves the embedded browser JavaScript as bytes; it executes only in the browser, and the Go runtime never invokes a JavaScript runtime.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

## Verify Locally

### Database Compatibility

Collector and server storage have separate SQLite application IDs and schemas.
Only collector schema 16 and server schema 2 are accepted; older, unknown,
wrong-role, corrupt, and newer schema contracts fail before mutation. The former
`tokeninsights.sqlite` stays untouched. Releases do not import legacy history or
migrate previous schemas. Release version numbers do not drive recovery.

Within the current collector schema, an older data generation can rebuild from
retained sources, while a current-generation pending rebuild resumes using its
saved source-scope fingerprint. Current data generation is 6; newer generations
are rejected. Failed rebuilds preserve committed collector work and pending
state. Retry with the original `--collector-db-path`, source directory, and source
environment settings. Changed scopes are rejected; `--dry-run` never repairs.
Only retained sources reconstruct collector history. Server storage never uses
collector recovery, and resets cannot retract committed server facts.

Everyday commands are `service`, `sync`, and `tui`. Advanced operations are
`collector normalize`, `collector reset-canonical`, and `collector reset-all`.
The TUI and browser read committed REST data; `tui --sync` explicitly collects
first. Previous commands and `--db-path` / `--no-sync` are removed. Explicit
collector resets require current-role/current-schema storage or a brand-new empty
file. Role paths default to `collector.sqlite` and `server.sqlite`.

Release verification covers rejection without mutation for previous schemas and
wrong roles; fresh/current preservation; current-schema rebuild/resume and scope
rejection; read-only viewer and dry-run behavior; and stable fact/receipt replay
when collector storage is rebuilt. Keep adapter accounting, ancestry, source
continuity, and every token component pinned by synthetic fixtures.

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
