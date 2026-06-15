# Generate Homebrew Formula From Release Artifacts

Status: ready-for-agent
Type: AFK

## Parent

[Homebrew Tap Release Channel PRD](../PRD.md)

## What to build

Add a focused formula generator that turns a TokenInsights release version, the existing module-prefixed release tag, and the locally generated release checksums into a complete Homebrew formula for `tokeninsights`.

The generated formula should install prebuilt GitHub Release archives for macOS Intel, macOS Apple Silicon, Linux x86_64, and Linux ARM64. It should use explicit URL and SHA256 blocks for each platform, install the `tokeninsights` binary, and include a version smoke test. The generator should fail when any expected release artifact checksum is missing.

## Acceptance criteria

- [ ] A script can generate a `tokeninsights` Homebrew formula from a version, release tag, checksum file, and output path.
- [ ] The generated formula uses the existing `packages/cli/vX.Y.Z` tag shape and encodes the tag correctly in GitHub Release download URLs.
- [ ] The generated formula includes explicit URL and SHA256 blocks for macOS Intel, macOS Apple Silicon, Linux x86_64, and Linux ARM64.
- [ ] The generated formula installs the released `tokeninsights` binary.
- [ ] The generated formula includes a version smoke test that verifies the installed binary reports the formula version.
- [ ] The generator reads local release checksums rather than fetching checksum data from GitHub.
- [ ] The generator fails with a clear error when any expected platform checksum is absent.
- [ ] The generator performs lightweight output validation, including checksum completeness.
- [ ] Focused tests cover fixture checksum input, encoded tag URLs, all four platform SHA256s, binary installation, and the formula smoke test.
- [ ] Existing repo test and build commands pass.
- [ ] No schema, storage, normalization, Viewer Aggregation, or CLI command behavior changes are made.

## Blocked by

None - can start immediately
