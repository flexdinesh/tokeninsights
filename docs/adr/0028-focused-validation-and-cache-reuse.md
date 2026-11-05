# Focused validation and cache reuse

Status: Accepted. Date: 5 November 2026. User approved retaining mandatory
correctness contracts and moving performance benchmarks to explicit commands.

## Decision

This updates ADR 0022's execution and coverage policy. Default `check:pr` runs
format, static, tooling, web, Go race, distribution, browser and deployment
groups. Benchmarks remain available through explicit group selection and
`bench:smoke`/`bench:storage`. Correctness assertions embedded in benchmarks
must have independent retained coverage before default benchmark removal.

Keep semantic feature and contract tests likely to drift: accounting/calendar
oracles, metadata privacy, stable identities, exact retries/receipts, real SQLite
rollback/reopen/concurrency, connection rejection, resource ownership and
cancellation. Preserve real OpenSSH/TCP, browser flows and installed products.
Delete duplicate implementation-spelling, retired-artifact and literal fixture
assertions. Similar names alone are not proof of redundancy. Record removals
and retained coverage in `docs/agents/test-coverage.md`.

Formatting is a barrier. Independent missing groups then run concurrently under
the existing machine-wide heavy/light admission slots. Private group homes,
source snapshots, databases, outputs and service credentials remain isolated.
Failure cancels this invocation and joins its children before resources close.
Never stop or clean a shared engine on cancellation.

Persistent named Go compiler/module, golangci-lint and pnpm download caches
accelerate preparation and incremental compilation. Toolchain/platform-scoped
names and explicit paths avoid coupling to changing test homes. Dependencies
are prepared from manifests before application source. Filtered production
inputs share immutable web assets and compatible CLI/backend builds; release
linker flags retain an independent product build. Narrow group inputs prevent
backend edits invalidating web-only tests.

Caches are disposable accelerators, never evidence. Committed clean HEAD,
semantic inputs, complete status/logs, leases and artifact retention still
govern reusable success. A forced failed/cancelled attempt supersedes its earlier
passing receipt. Go tests retain `-count=1`; an invocation identity is attached
after immutable preparation to execute checks and services freshly when needed.

Retain run start/completion and group queue/preparation/execution/total durations.
Compare equivalent forced correctness runs and unchanged-input receipt reuse;
do not infer speed from deleting test counts alone or clear shared caches to
manufacture a cold baseline. Narrow input sets risk stale evidence; assert
relevant invalidation and unrelated-input reuse. Bounded concurrency avoids
trading wall time for unbounded memory or CPU contention.

## Consequences

Performance regressions require explicit benchmark runs. Correctness remains
mandatory. No runtime, token semantics, storage schema, connection policy or
release enforcement changes. Dagger engine ownership and local isolation from
ADRs 0022/0027 remain in force.

Unresolved questions: none.
