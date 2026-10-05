# Collector/server implementation gates

Status: Runtime path complete in PR #53; repository verification passed.

See [PRD](PRD.md), [execution plan](EXECUTION-PLAN.md), [approved contract](STORAGE-CONTRACT-PROPOSAL.md), [architecture](../../docs/collector-server-architecture.md), [failure matrix](../../docs/collector-ingestion-tests.md), and [validation evidence](VALIDATION.md).

The user chose fresh collector/server files rebuilt from retained sources. Preserve legacy `tokeninsights.sqlite`; no history import or legacy alias migration. Collector/server role checks run before recovery or mutation.

| Work | State |
| --- | --- |
| T01 native identities | OpenCode/Claude regressions fixed; CFI001–009 preserved. Weak identity has explicit publication diagnostics. |
| T02 contract | Approved; separate schemas and normalized wire protocol implemented. |
| T03 collector journal | Atomic canonical+journal writes, immutable saved batches, independent destination progress implemented and focused-tested. |
| T04/T05 protocol/server | Strict decoder and bounded transactional ingestion implemented; receipt, dedupe, conflict and privacy tests execute real SQLite/HTTP. |
| T06/T07 composition/CLI | Shared local/remote core; default all-harness manual sync, publish-only, host-only resets, fresh paths implemented. |
| T08/T09 viewers | REST TUI and browser Reload implemented; no implicit source collection. |
| T10 plugins | Native packaging/runner completion tracked in [plugin plan](PLUGIN-PLAN.md); isolated real-host installation remains deferred. |
| T11 compatibility | Fresh role databases; legacy preserved; wrong-role/newer schemas reject without destructive recovery. |
| T12 failures | Production contract tests plus 12 named subprocess kill/reopen barriers implemented. |
| T13 docs/tooling | Current runtime docs, separate dev fixtures, embedded assets and repository gates complete. |

## Gate 1: Source identities

Real adapters compare native identity, canonical timestamps, token components and totals across repeated/full parse and fresh databases. CFI007 preserves distinct equal-counter OpenCode requests; CFI008 replaces one native Claude request snapshot. CFI009 preserves ambiguous raw evidence; it does not invent two published facts. Missing occurrence/native identity or unsupported changed values create explicit publication diagnostics.

## Gate 2: Durable publication

Canonical mutation and journal snapshot share one transaction. Unchanged normalization creates no journal entry; mergeable reference-envelope changes remain deliverable. Saved request bytes never change during retry. Receipt validation precedes the atomic cursor/receipt commit. Acknowledged prefixes persist across suffix failure.

## Gate 3: Canonical server

Normalized self-contained batches share one ingestion core in local and remote compositions. Validation, canonical references/facts, analytics revision and receipt commit atomically. Duplicate batch replay returns its original receipt; a fresh stream repeats no contribution. Claude timestamp evidence governs revisions; stream sequence is never source precedence. Missing producer artifacts cannot cause server reconstruction or historical deletion.

## Gate 4: Workflow and viewers

`sync` defaults to all harnesses. Collection and delivery summaries report their separate outcomes. `--publish-only` requires no source discovery; explicit server URL never boots local service. Producer resets affect only collector storage. TUI/browser query through REST; optional `view --sync` invokes caller-side collection explicitly. Reload observes saved state only.

## Gate 5: Verification/release

Focused tests are evidence for their own packages only. Complete root formatting/lint, full tests, schema/API checks, generated assets, native build/runtime, race and browser gates. Record exact outcomes in [VALIDATION.md](VALIDATION.md); do not claim final completion until those gates pass. Mutation experiments and real-host plugin installation are separate evidence and must not be claimed unless performed.

## Unresolved questions

None blocking approved implementation. Future work: isolated plugin host verification and remote setup/authentication.
