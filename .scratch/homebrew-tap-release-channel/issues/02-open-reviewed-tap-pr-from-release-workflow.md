# Open Reviewed Tap PR From Release Workflow

Status: ready-for-agent
Type: AFK

## Parent

[Homebrew Tap Release Channel PRD](../PRD.md)

## What to build

Extend the TokenInsights release workflow so that, after publishing the GitHub Release, it generates the Homebrew formula and opens or updates a pull request against `flexdinesh/homebrew-tap`.

The workflow should use a narrowly scoped `HOMEBREW_TAP_TOKEN` secret for cross-repo access, push a deterministic branch for the released TokenInsights version, and reuse that branch and pull request on reruns. Failure to create or update the tap pull request should fail the release workflow so Homebrew drift is visible.

## Acceptance criteria

- [ ] The release workflow generates the Homebrew formula after release archives and local checksums have been produced.
- [ ] The release workflow publishes the GitHub Release before attempting the tap pull request.
- [ ] The tap update uses `HOMEBREW_TAP_TOKEN`, not the source repo's default token, for cross-repo write and pull request operations.
- [ ] The workflow targets the `flexdinesh/homebrew-tap` repository and updates the `tokeninsights` formula.
- [ ] The workflow uses a deterministic tap branch name derived from the TokenInsights version.
- [ ] Rerunning the release for the same version updates or reuses the existing tap branch and pull request rather than creating duplicate PRs.
- [ ] The tap pull request title and body clearly identify the TokenInsights version and source release tag.
- [ ] The workflow fails if formula generation, tap branch push, or tap PR creation/update fails.
- [ ] The workflow keeps full Homebrew audit/style/install validation in the tap repo rather than duplicating it here.
- [ ] Existing release behavior for selecting tags, building archives, generating checksums, and publishing GitHub Releases is preserved.
- [ ] Existing repo test and build commands pass.
- [ ] No schema, storage, normalization, Viewer Aggregation, or CLI command behavior changes are made.

## Blocked by

- [01 - Generate Homebrew Formula From Release Artifacts](01-generate-homebrew-formula-from-release-artifacts.md)
