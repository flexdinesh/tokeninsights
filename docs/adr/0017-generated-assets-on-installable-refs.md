# Generate dashboard assets on installable refs

Status: Accepted. Date: 18 October 2026.

## Decision

Ignore generated `packages/cli/internal/server/static` on `main`. Keep browser
source, public fonts and licenses committed. This supersedes the committed-bundle
requirement in ADR 0010. Go still embeds the generated dashboard; production
requires no JavaScript runtime.

Build tooling owns generation and source publication. Local build, test, lint and
benchmark tasks generate assets before invoking Go. Direct Go commands require
`pnpm run build:web` first; fresh `@main`, feature-branch and source-SHA installs
fail because their embed input is absent. Docker builds generate assets in a
separate frontend stage and retain only the Go executable in the runtime image.

After native CI passes for a main push, CI creates an asset commit whose first
parent is that exact source and publishes it to `edge`. The highest stable module
tag is an additional parent when necessary, allowing Go to derive a pseudo-version
above that release. Publication is serialized and uses an observed-ref lease;
superseded main candidates and already-published descendants skip. Workflow-level
cancellation starts before native checks, so an older native completion cannot
cancel a newer publication.

Manual stable release resolves `main` once and selects the existing patch-series
version. A separate asset commit receives `packages/cli/vX.Y.Z`; only the tag is
pushed. Native archives are built from this commit and verified before publication.
The source branch never receives it. Tag ownership uses the first parent, with
historical tags directly on source commits still supported. Existing tags never
move. An explicit `retry_tag` resumes a release created by this workflow after
main advances. Historical tags on main remain installable but reject this recovery
path before setup because their original build layouts may be incompatible.

Stable source installs use `@latest` or `@vX.Y.Z`; development uses `@edge`.
The edge branch is mutable and proxies may cache lookups. No development archive
channel is added. Plugin bundles retain their existing harness distribution path.

## Verification

Local pre-push verifies Git publication and real source installs using an isolated
bare remote, Git configuration, Go module cache and binaries. Cover first edge,
release retry, release ancestry/version ordering, stale candidates, embedded HTTP
assets and missing assets on main. CI retains native platform checks and adds
publication only; it does not duplicate locally reproducible checks.

Repository rulesets must permit the workflow identity to update `edge` and create
module version tags. No repository settings are changed by this implementation.

Unresolved questions: none.
