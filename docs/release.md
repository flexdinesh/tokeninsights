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

React assets are committed under `packages/cli/internal/server/static` and embedded with `go:embed` in every binary, including Go installs and snapshot archives. CI/release verification rebuilds the frontend and checks for asset drift before packaging. Frontend changes must include regenerated assets (`pnpm run build:web`). The private `@tokeninsights/build-tools` workspace package under `tools/build` owns generated-asset checks and Homebrew formula generation; it is release-time tooling only.

Release artifacts contain only the native Go binary and documentation. Production hosts need no Node.js, npm, pnpm, `node_modules`, repository JavaScript tooling, or separate web files. Go serves the embedded browser JavaScript as bytes; it executes only in the browser, and the Go runtime never invokes a JavaScript runtime.

The tap branch is deterministic per version, such as `tokeninsights-v0.0.1`, so rerunning the release updates the same tap pull request. If the tap pull request cannot be created or updated, the release workflow fails after publishing the GitHub Release so the Homebrew update can be repaired manually.

The tap repository owns Homebrew-native validation. Its CI should run style, audit, install, and formula test checks for changed formulae before merging the generated pull request.

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
