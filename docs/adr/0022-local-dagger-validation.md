# Local validation through Dagger

Status: Accepted. Date: 1 November 2026.

## Context

The mandatory pre-push suite serialized worktrees of one repository but did not
isolate targeted checks or other repositories. Tests and fixture servers shared
the host network. Successful verification could not be reused, and phase outputs
depended on previous tasks mutating the checkout.

## Decision

This supersedes ADR 0021's pre-push admission, graph ownership and uncached
verification policy. Mise remains the public command interface and defines leaf
commands. Dagger owns isolated preparation, dependency ordering, services and
execution. Validation is explicit before PR creation and subsequent updates.
Pushing alone does not run the full suite. Existing CI, release and deployment
workflows remain unchanged; no remote enforcement is introduced.

The aggregate preserves existing coverage, excluding the docs production build.
Coarse contracts, JavaScript, Go, benchmark, distribution, browser and deployment
groups receive conservative selected inputs. Immutable preparation and generated
assets may be shared; writable homes, temporary files, outputs and databases may
not. Clients and fixture servers run inside containers. PostgreSQL uses service
bindings and independent fixture databases. No application ports are published,
and runners receive no host Docker socket, host networking or host-service tunnel.

Validation defaults to an immutable committed HEAD snapshot with clean tracked
files. Independent Git metadata supports file enumeration; publication tests keep
their own fixture histories. Host worktree Git pointers and personal configuration
are excluded. Successful evidence records coverage, semantic inputs, toolchain,
platform and tested commit. Commit identity identifies evidence, not general cache
validity. History-only rebases can reuse unchanged inputs. Failed or interrupted
runs never qualify as successful validation. Force reruns bypass check-result
caches while allowing immutable dependency preparation to be reused.

Machine admission bounds expensive execution independently of task dependencies.
Logs and browser failure artifacts belong to individual runs. Cancellation stops
only the current invocation's work, and never its shared engine.

## Engine ownership

CLI, SDK and engine versions are pinned together. Each repository explicitly
manages a named engine outside the `dagger-engine-*` prefix and a dedicated cache
volume. Worktrees share that engine. Inspect ownership before setup; leave unknown
engines untouched. On Dagger 0.21.x connect directly with
`_EXPERIMENTAL_DAGGER_RUNNER_HOST` and set `DAGGER_LEAVE_OLD_ENGINE=true`, including
initialization. Automatic provisioning and cleanup are not the validation path.
Engine upgrades and cleanup require explicit coordination. Broad Docker/Dagger
pruning is prohibited. Persistent caches are shared accelerators, not validation
evidence or database storage.

## Deployment coverage exception

The user approved removing read-only-root enforcement coverage after probes on
Dagger 0.21.11, including a dedicated repository engine, demonstrated successful
root writes despite `withMountedDirectory(path: "/", readOnly: true)`.
Permissions are not presented as equivalent enforcement. Production Dockerfile
and deployment configuration are unchanged.

The real-image fixture preserves non-root execution, the healthcheck command,
secret-file inputs, actual process exit after SIGTERM, persistent state across
server process replacement, private admin socket operations, authentication,
revocation and accounting identity assertions. A test-only supervisor owns the
process lifecycle inside a Dagger service. Docker-in-Docker is excluded.

## Consequences

Local wrappers and agent instructions are bypassable; Git has no standard pre-PR
hook. Linux containers do not provide full native macOS test coverage. Existing
CI retains native Linux arm64 and macOS build/storage coverage. Input modeling,
resource bounds, service independence and artifact retention remain explicit
repository responsibilities rather than automatic Dagger guarantees.

Unresolved questions: none.
