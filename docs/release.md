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

Development version from the latest `main` push published after CI passes:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

Go resolves `@latest` from stable module tags (`packages/cli/vX.Y.Z`), independently of GitHub's Latest badge. `@dev` resolves the moving `dev` branch to a Go pseudo-version, or a stable version when that commit is tagged. Module proxies may briefly cache branch lookups; rerun the install after publication to update.

## Release

Required secret:

- `HOMEBREW_TAP_TOKEN`: fine-grained token with contents write and pull request write access to `flexdinesh/homebrew-tap`.

1. Merge the release-ready code to `main`.
2. Run the **Release** workflow from GitHub Actions with branch `main` selected. Dispatches from other refs are skipped. The workflow checks out the latest `main` when it starts; that commit is verified and released even if `main` advances during the run.
3. The workflow selects the next patch version, builds macOS/Linux amd64/arm64 archives, writes `checksums.txt`, then pushes the Go module tag and publishes a stable GitHub Release explicitly marked Latest. New versions and GitHub releases are published only through this manual workflow.
4. The workflow generates `Formula/tokeninsights.rb` from the local release checksums and opens or updates a pull request against `flexdinesh/homebrew-tap`.

`.release-version` contains the active `major.minor` release series. It is currently `0.1`; releases increment the highest existing patch in that series. A rerun from the same commit reuses that commit's existing tag and updates its release assets and Latest badge.

To begin a new minor or major series, change `.release-version`. For example, changing it to `0.2` makes the next release `packages/cli/v0.2.0`; changing it to `1.0` makes the next release `packages/cli/v1.0.0`. Later releases automatically increment that series' patch number.

React assets are committed under `packages/cli/internal/server/static` and embedded with `go:embed` in every binary, including stable and dev Go installs. CI/release verification rebuilds the frontend and checks for asset drift before publication. Frontend changes must include regenerated assets (`pnpm run build:web`). The private `@tokeninsights/build-tools` workspace package under `tools/build` owns generated-asset checks, dev branch publication, and Homebrew formula generation; it is build/release-time tooling only.

Release artifacts contain only the native Go binary and documentation. Production hosts need no Node.js, npm, pnpm, `node_modules`, repository JavaScript tooling, or separate web files. Go serves the embedded browser JavaScript as bytes; it executes only in the browser, and the Go runtime never invokes a JavaScript runtime.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

## Automatic dev publication

Every push to `main` runs CI. After verification passes, **Publish dev** points `dev` at that exact commit, making it available through `go install ...@dev`. Pull requests only verify; pushes to `dev` no longer trigger the obsolete snapshot packaging job. Dev publication creates no tags, GitHub releases, or Homebrew updates and leaves the stable Latest release unchanged.

`dev` is a generated distribution branch, not an independent development branch. The first publication replaces its obsolete history with `main`. A job whose commit has been superseded on `main` skips publication. An explicit Git lease rejects concurrent changes to `dev`; publication retries up to three times, rechecking both branches each time. Failed verification leaves `dev` unchanged. Rerun the latest `main` CI run to retry a failed publication. The job uses `GITHUB_TOKEN` with `contents: write`; branch rules must permit its update to `dev`.

## Verify Locally

### Database Compatibility

Schema V8 introduces singleton lifecycle state: current data generation 1, a durable rebuild-pending marker, and `rebuild_source_key`, a hash of the normalized source configuration. The key is NULL when ready and nonempty while pending; no paths are persisted. Bump schema version for table/column/constraint changes; bump data generation for breaking token semantics or raw/canonical identity changes that require reingestion. Compatible releases do neither. Release version numbers do not drive recovery.

The first normal sync, normalize, or TUI/web startup after an incompatible update automatically resets recognized older data transactionally inside the existing SQLite file, then reimports and normalizes all configured harnesses. Skipped generations need one rebuild to current. Fresh/current databases need no reset. Missing harnesses are normal skips; only retained local sources can reconstruct history. Unknown/corrupt/newer databases are rejected without automatic deletion.

Failed recovery retains partial imports, pending state, and the source-scope fingerprint. Retry with the original `--db-path`, `--source-dir` if used, and source environment settings to resume without resetting again. Default keys include all effective harness roots; custom all-harness keys use the normalized canonical absolute root. Mismatched attempts are rejected before data writes, including attempts to finish a custom-root rebuild through default-source dashboard startup or normalize. Full paths cannot be recovered from the hash. Read-only `--no-sync` never repairs data, and `--dry-run` previews without writes. Targeted default-source recovery expands to all defaults; all-harness custom roots stay bounded; single-harness custom roots defer recovery. Explicit `reset-all --confirm` remains available, including when changed Codex parent transcript availability requires historical reconciliation. `reset-canonical` cannot repair incompatible usage.

Release verification should cover legacy/old-generation rebuild, current/fresh preservation, failed-rebuild same-scope resume, mismatched-scope rejection, dry-run/no-sync nonmutation, source boundaries, and lifecycle validation inside analytics read snapshots. The Codex correction counts verified parent replay once using explicit ancestry and complete token metadata, including rewritten timestamps; uncertain history is retained with diagnostics. Forks/subagents reparse each sync and cache linked parent parses within that run.

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
