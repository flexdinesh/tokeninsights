# Authenticated deployment, maintenance and service retirement

Status: complete
Dependencies: 02,03,05

Server container defaults to authenticated distributed mode. Require canonical
HTTPS origin and bearer ingestion; preserve read-token browser session exchange,
private operator socket, liveness/readiness and graceful shutdown. Single container
owns writable DuckDB. No collector source access, runtime Node or broker dependency.

Remove public service start/stop/restart workflow and personal remote deployment from
new UX. Retain required legacy import/reprocess maintenance with explicit finite
collector/data commands and private remote admin. Existing managed owner must be
stopped explicitly before opening new local runtime; never kill arbitrary processes.
Provide actionable migration instructions and transitional stop path if necessary.

Update Dockerfile/Compose, development server/fixture scripts, build smoke checks,
CLI help, README, packages/cli README, CONTEXT, docs/design/deployment and ADR status.
Do not relabel existing personal token history as a hosted user's dataset implicitly.
Stop server for backup and preserve app/data/WAL/recovery files as coordinated set.
Document rollback with retained copies and matching old binary, never schema downgrade.

Tests: native server startup/config errors before mutation, auth/admin/browser login,
container build/readiness/shutdown, direct Go build and native run with JS tools absent
from PATH; fixtures/dev tooling use new composition. Full checks listed in PRD.

Implementation: implemented in this worktree. See PRD validation record.

## Unresolved questions

None.
