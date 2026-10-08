# Application SQLite and recoverable account provisioning

Status: complete
Dependencies: 01

Introduce app.sqlite with application role/version and paired token database ID.
Tables: application_metadata, users (stable user/dataset, display name, enabled,
provisioning state, created time), tokens (digest/permissions/expiry/revocation),
sessions (digest/source token/expiry/revocation), migration state. Token permission
and session policies remain unchanged. Local default user binds dataset=default.

Create semantic account repository operations, with SQLite transaction ownership
inside adapter. Accounts service must not depend on DuckDB SQL. Dataset creation is
an idempotent data-store operation. User provisioning persists inactive user/fixed
dataset first, creates dataset, then activates; retry/restart resumes same identity.
Disabled or provisioning users never authenticate or mint tokens. Revocation is read
from SQLite on every authentication, including source-token browser invalidation.

Migrate existing accounts through a staged copy under exclusive server ownership.
Copy all IDs/digests/times/statuses; validate counts and dataset references before
activation. Persist pair identity and completed migration marker. Once switched,
never import stale DuckDB accounts over changed/revoked SQLite records. Interrupted
copy resumes or rebuilds staging; retain recoverable old copy. Reject mismatched app
and token databases without mutation. No distributed transaction claim.

Add schema source/embed/version checks; choose new role application ID and version 1.
Bump DuckDB contract only when removing account structures or changing its contract;
retain read-only old migration source. Existing raw/read protocol versions unchanged.
Provide explicit app path flag/env/config; derive default beside configured data path.
Validate role/path aliases and hardlinks before mutation. Docker persists both files.

Tests: existing auth/isolation/expiry cleanup against SQLite, default user,
interrupted account migration and each provisioning boundary, token rotation,
revocation after restart, backup mismatch, staged failure leaves verified source.

Implementation: implemented in this worktree. See PRD validation record.

## Unresolved questions

None.
