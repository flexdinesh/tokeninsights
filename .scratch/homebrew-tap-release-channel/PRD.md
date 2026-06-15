# Homebrew Tap Release Channel PRD

Status: ready-for-agent

## Problem Statement

TokenInsights currently has a stable Go install path and a GitHub Release workflow that builds platform archives, but users who expect a Homebrew install path cannot install or upgrade the TokenInsights CLI through `brew`.

The release process also does not update a Homebrew tap, so any future formula would drift unless a maintainer manually translated release artifacts and checksums into a tap update.

## Solution

Add Homebrew as the primary stable install path for the TokenInsights CLI through the generic `flexdinesh/homebrew-tap` repository.

The TokenInsights release workflow will continue to publish GitHub Release artifacts from the existing Go module-prefixed `packages/cli/vX.Y.Z` tags. After publishing the release, it will generate an explicit Homebrew formula for all existing release platforms and open a reviewed pull request against `flexdinesh/homebrew-tap`. The tap PR uses a deterministic branch per TokenInsights version, reuses the same branch on reruns, and fails the release workflow if PR creation fails.

Users should see `brew install flexdinesh/tap/tokeninsights` as the primary stable install command. `go install` remains available as an alternative and development-friendly path.

## User Stories

1. As a TokenInsights user, I want to install the CLI with Homebrew, so that I can use the package manager already on my machine.
2. As a TokenInsights user, I want one documented stable install command, so that I do not need to choose between release channels.
3. As a TokenInsights user, I want Homebrew installs to use released binaries, so that installation is fast and does not require a local Go toolchain.
4. As a TokenInsights user on Apple Silicon macOS, I want the Homebrew formula to select the arm64 macOS archive, so that I get the correct native binary.
5. As a TokenInsights user on Intel macOS, I want the Homebrew formula to select the amd64 macOS archive, so that I get a compatible binary.
6. As a TokenInsights user on Linux x86_64, I want the Homebrew formula to select the amd64 Linux archive, so that Linuxbrew works without manual downloads.
7. As a TokenInsights user on Linux ARM64, I want the Homebrew formula to select the arm64 Linux archive, so that Linuxbrew works on ARM hosts.
8. As a TokenInsights user, I want Homebrew installs to verify archive checksums, so that downloads are integrity checked before installation.
9. As a TokenInsights user, I want `tokeninsights --version` to report the released version after Homebrew install, so that I can confirm what is installed.
10. As a TokenInsights user, I want `brew upgrade tokeninsights` to pick up reviewed tap updates, so that upgrades follow normal Homebrew behavior.
11. As a TokenInsights user, I want the Homebrew formula to install the `tokeninsights` binary into Homebrew's bin path, so that the command works without extra PATH setup.
12. As a TokenInsights user, I want the Homebrew install docs to be prominent, so that I can discover the easiest supported install path.
13. As a TokenInsights user who prefers Go tooling, I want `go install` docs to remain available, so that I can install directly from the Go module when needed.
14. As a TokenInsights maintainer, I want the release workflow to generate the formula from the artifacts it just built, so that formula URLs and SHA256s cannot drift from the release.
15. As a TokenInsights maintainer, I want the formula generator to parse local release checksums, so that the release job does not depend on GitHub asset propagation timing.
16. As a TokenInsights maintainer, I want module-prefixed release tags to remain the single release identity, so that Go install behavior and Homebrew behavior point at the same release.
17. As a TokenInsights maintainer, I want the generated formula URLs to handle `packages/cli/vX.Y.Z` tags correctly, so that GitHub Release assets download reliably despite slashes in tag names.
18. As a TokenInsights maintainer, I want the generated formula to use explicit URL and SHA256 blocks per platform, so that tap PR diffs are easy to review.
19. As a TokenInsights maintainer, I want formula generation to live in a focused script, so that release workflow YAML stays readable.
20. As a TokenInsights maintainer, I want a focused generator test, so that checksum parsing and formula output are protected from regressions.
21. As a TokenInsights maintainer, I want release automation to open a pull request to the tap instead of pushing directly, so that tap CI and review run before public Homebrew updates land.
22. As a TokenInsights maintainer, I want tap PR automation to use a narrowly scoped credential, so that cross-repo write access is limited to the tap repo.
23. As a TokenInsights maintainer, I want release reruns for the same version to reuse one deterministic tap branch, so that duplicate tap PRs are not created.
24. As a TokenInsights maintainer, I want the release workflow to fail if the tap PR cannot be opened, so that missing Homebrew updates are visible.
25. As a TokenInsights maintainer, I want minimal local formula validation before PR creation, so that obvious generation mistakes fail early.
26. As a TokenInsights maintainer, I want full Homebrew audit, style, install, and test checks to run in the tap repo, so that Homebrew-specific validation stays with the tap.
27. As a TokenInsights maintainer, I want the required release secret documented, so that future maintainers can configure release automation without reverse-engineering it.
28. As a TokenInsights maintainer, I want tap CI expectations documented, so that the external tap repo can be set up consistently.
29. As a TokenInsights maintainer, I want the release process to avoid schema, storage, and Viewer Aggregation changes, so that packaging work does not affect TokenInsights data behavior.
30. As a future maintainer, I want an ADR explaining why Homebrew uses a generic tap and reviewed PR updates, so that the release-channel shape is not accidentally replaced.

## Implementation Decisions

- Homebrew is the primary stable install path for TokenInsights.
- The canonical Homebrew install command is `brew install flexdinesh/tap/tokeninsights`.
- The Homebrew tap repository is the generic `flexdinesh/homebrew-tap`, not a TokenInsights-specific tap repository.
- The TokenInsights formula in that tap is named `tokeninsights`.
- The TokenInsights release workflow remains the owner of tap updates for TokenInsights releases.
- The release workflow publishes GitHub Release artifacts first, then opens a tap pull request.
- The release workflow must use a fine-grained `HOMEBREW_TAP_TOKEN` secret for cross-repo tap writes and pull request creation.
- Tap PR automation should use the least practical permissions: contents write and pull request write for the tap repository.
- The release workflow should fail when tap PR creation fails.
- Reruns for the same TokenInsights version should reuse a deterministic tap branch, such as one derived from `tokeninsights-vX.Y.Z`, and should update or reuse the existing PR.
- The Homebrew formula installs prebuilt release archives rather than building from source.
- The formula supports all current release targets: macOS Intel, macOS Apple Silicon, Linux x86_64, and Linux ARM64.
- The formula uses the existing `packages/cli/vX.Y.Z` release tags and must encode the slash in GitHub Release download URLs.
- The formula should be explicit: each platform branch has its own release asset URL and SHA256.
- The formula installs the released `tokeninsights` binary.
- The formula test is a version smoke test that runs the installed binary and verifies the output includes the formula version.
- Formula generation should live in a small Node script under the existing scripts area.
- The generator should read the local `checksums.txt` produced by the release job rather than fetching checksums from the published GitHub Release.
- The release workflow should perform minimal local validation before opening the tap PR, including checksum completeness and formula syntax where practical.
- Full Homebrew validation belongs in the tap repository CI.
- Install docs should present Homebrew before `go install`.
- `go install` remains documented as an alternative stable install path and development-friendly path.
- This work does not change TokenInsights storage, schema, token semantics, pipeline behavior, or Viewer Aggregation behavior.
- No schema approval is required because the work is release packaging and documentation only.

## Testing Decisions

- Test the formula generator at the script boundary with fixture checksum input and generated formula output.
- The generator test should assert the rendered formula includes the expected version.
- The generator test should assert GitHub Release URLs include the encoded module-prefixed tag.
- The generator test should assert all four platform archives are represented with their expected SHA256s.
- The generator test should assert the formula installs `tokeninsights`.
- The generator test should assert the formula includes a version smoke test.
- The generator should fail when any expected platform checksum is missing.
- Release workflow logic should stay thin enough that the generator test covers most string and checksum behavior.
- Documentation changes should be reviewed through normal repository checks.
- Tap CI should run Homebrew-native checks such as style, audit, install, and test for changed formulae.
- Repository verification for this work should include the existing test and build commands.
- No schema validation changes or data pipeline tests are required beyond the existing build command's normal checks.

## Out of Scope

- Creating or scaffolding the external `flexdinesh/homebrew-tap` repository from this repo.
- Implementing the tap repo CI workflow in this repo.
- Publishing to Homebrew core.
- Adding bottles or Homebrew bottle publishing.
- Replacing the existing GitHub Release workflow with GoReleaser.
- Introducing plain root `vX.Y.Z` tags.
- Changing the Go module layout.
- Changing release artifact names unless required by a later packaging decision.
- Adding Windows release artifacts.
- Adding casks, services, shell completions, manpages, or external Homebrew commands.
- Auto-merging tap pull requests.
- Changing TokenInsights schema, raw storage, normalization, canonical facts, Viewer Aggregation behavior, or CLI command semantics.
- Adding cost tracking or any new product metric domain.

## Further Notes

- ADR 0003 records the release-channel decision.
- Existing ADRs require stable CLI release tags to keep the `packages/cli/vX.Y.Z` shape and explain why the project uses a custom release workflow.
- Homebrew's tap naming convention means the GitHub repo `flexdinesh/homebrew-tap` is used as tap `flexdinesh/tap`.
- The fully qualified install command includes owner, tap, and formula: `brew install flexdinesh/tap/tokeninsights`.
