# Document Homebrew Stable Release Channel

Status: ready-for-agent
Type: AFK

## Parent

[Homebrew Tap Release Channel PRD](../PRD.md)

## What to build

Update TokenInsights documentation so Homebrew is presented as the primary stable install path, while `go install` remains documented as an alternative and development-friendly install path.

The release docs should also document the required `HOMEBREW_TAP_TOKEN` secret, the reviewed tap PR flow, rerun behavior, and the expected CI responsibility of the external `flexdinesh/homebrew-tap` repository.

## Acceptance criteria

- [ ] User-facing install docs present `brew install flexdinesh/tap/tokeninsights` before `go install`.
- [ ] `go install` remains documented as an alternative stable install path.
- [ ] Development install instructions remain available for contributors.
- [ ] Release docs explain that the GitHub Release is published first and the tap PR is opened after artifact publication.
- [ ] Release docs document the `HOMEBREW_TAP_TOKEN` secret and its expected narrow scope.
- [ ] Release docs document deterministic tap branch reuse on release workflow reruns.
- [ ] Release docs state that tap PR failure fails the release workflow and must be resolved manually.
- [ ] Release docs describe expected tap repo CI: Homebrew style, audit, install, and formula test checks for changed formulae.
- [ ] Docs preserve the existing Go module-prefixed tag model and custom release workflow decisions.
- [ ] Docs do not imply that this repo scaffolds or owns the external tap repository CI.
- [ ] Existing repo test and build commands pass.
- [ ] No schema, storage, normalization, Viewer Aggregation, or CLI command behavior changes are made.

## Blocked by

- [01 - Generate Homebrew Formula From Release Artifacts](01-generate-homebrew-formula-from-release-artifacts.md)
- [02 - Open Reviewed Tap PR From Release Workflow](02-open-reviewed-tap-pr-from-release-workflow.md)
