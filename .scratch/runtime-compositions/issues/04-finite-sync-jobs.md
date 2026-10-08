# Durable background sync and local owner handoff

Status: complete
Dependencies: 02

Use a separate local operational SQLite queue derived from the canonical collector
path: <collector-db-path>.jobs.sqlite. This avoids the collector writer lock held
during network submission blocking quick enqueue. Version/role validate this file;
it stores local invocation metadata, never evidence or credentials. Schema fields:
job ID, mode, collector/data identity, destination URL, expected dataset/database
when known, credential fingerprint/reference, harness selector, bounded safe options,
state, claim owner, attempt count, timestamps, fixed error, accepted counts/receipts.
Local source configuration remains local and is never uploaded.

Foreground parent validates configuration, persists invocation, re-execs the native
binary with private inherited configuration/readiness pipes and detached stdio,
waits for startup acknowledgement under a deadline, then returns. Reuse audited
native detachment mechanics, not the managed server lifecycle. Persisted jobs never
contain bearer tokens; startup pipes are bounded and close on all failures. Worker
validates job/config binding and authenticates dataset before capture. A resumed job
whose credential identity cannot be safely reconciled requires explicit retry with
current settings; never silently change account/destination.

A finite worker claims work under an OS-backed queue-owner lock, retries transient
submission at bounded intervals respecting Retry-After, records terminal/interrupted
state, exits when its admitted work completes or deadline expires. No idle daemon.
Job state distinguishes queued/running/accepted/failed/interrupted. Subsequent explicit
sync recovers abandoned claims and retained immutable delivery; no reboot guarantee.
Retain bounded recent terminal diagnostics, never prune pending evidence/jobs.

Local sync queues a request when a foreground viewer owns data. Viewer consumes
requests and invokes collector/ingestor in its own process. Lock ordering: enqueue
commits without token ownership; owner acquires job claim, releases queue transaction,
then captures/accepts; do not hold queue transaction during collector/network work.
Startup must recheck queue under ownership before exiting to avoid lost wakeups;
pending requests survive races even if next explicit command is needed after exit.
Different harness requests remain distinct; completion during scan creates follow-up.
One active viewer means no remote query IPC is needed.

Tests: child spawn/readiness failure, terminal return before remote completion,
parent exit, kill after acceptance, no credentials in jobs/logs/argv, overlapping
invocations, config/account changes, busy owner, handover/cancellation and recovery.

Implementation: implemented in this worktree. See PRD validation record.

## Unresolved questions

None.
