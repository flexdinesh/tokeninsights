# Hosted user management

Status: Accepted. Date: 13 October 2026. Schema/API changes explicitly approved.

## Decision

Identity belongs to `accounts`; mode policy belongs to composition. Keep one stable
internal user ID and one stable dataset ID per user. Passwords, API keys and browser
sessions authenticate that identity; credential changes never replace its dataset.
This supersedes the token-login portions of ADRs 0008/0009, source-token-bound session
contract in ADR 0011, and account/token bootstrap portions of ADR 0015. Their other
runtime, storage, deployment and plugin decisions remain current.

Personal composition automatically creates its existing default identity and exposes
no login, signup or account-management routes or UI. Preserve personal SQLite
application schema 2 unchanged. Hosted SQLite application schema becomes 3 in
`schema/app.hosted.sql`; PostgreSQL account schema becomes 2. Add unique usernames,
password hashes and named API keys; browser sessions no longer reference source
keys. Token schemas remain 1. Hosted mode has not been deployed; incompatible older
account schemas reject without mutation. No migrations or legacy auth adapters.

Hosted users sign in with username/password. Signup defaults off and is enabled by
`--signup` / `TOKENINSIGHTS_SIGNUP_ENABLED`. The top-right profile control opens
settings: username, change password, named API keys and sign out. Public
`GET /api/v2/auth` advertises kind, login methods and signup availability without
identity/data information. Authenticated `/api/v2/instance` keeps capabilities and
caller permissions distinct. The UI resolves policy once at its application
boundary; HTTP route mounting and authorization enforce it independently.

Normalize usernames by trimming and lowercasing: 3–32 ASCII characters, first
alphanumeric and remaining alphanumeric, underscore or hyphen. Passwords are
15–128 Unicode characters, with whitespace preserved. Shared account policy hashes
passwords with Argon2id, a unique salt, 19 MiB memory, two iterations and one lane.
Return bounded errors without credentials. Admission before decoding permits five
password attempts per minute per resolved client address and four concurrent
password operations globally. Deployment proxy policy remains at the hosted HTTP
boundary; domains do not inspect flags/environment or proxy headers.

Password login creates a 24-hour opaque host-only HttpOnly/SameSite=Lax cookie,
Secure for HTTPS browser origins. Sessions authorize reads and the current user's
settings, but never ingestion or administration of other users. Protect login,
signup, logout and account mutations with the existing same-origin policy. HTTP
remains supported for private deployments; HTTPS for external deployments remains
deployment-owned. Self-service password change requires the current password and
invalidates all browser sessions. Private password reset does the same. API keys
remain active until separately revoked; disabling a user blocks every credential.

Settings create/list/revoke named 256-bit random API keys with labels of at most
200 Unicode characters. Persist only secret
digests and show a created secret once. Keys authorize `read` and/or `ingest`; the
sync preset includes both so collectors can inspect receipts. There is no default
expiry. Revocation takes effect on the next authentication without affecting browser
sessions. API keys cannot manage keys or passwords. Self-service ownership derives
only from the authenticated session, never a request user/dataset selector.

Browser settings send the resolved dataset as optional `X-TokenInsights-Dataset-ID`
on account reads and mutations. Authenticate first, compare the expected dataset,
and reject mismatches with 409 before performing the operation; the header never
selects a dataset. The UI clears account caches and requires login on mismatch,
protecting settings when another tab changes the shared-cookie identity.

Provisioning preserves the existing recoverable sequence: persist inactive user
and fixed dataset ID, ensure the dataset through `accounts.Datasets`, then activate.
Reopen resumes interrupted provisioning; ready disabled accounts stay disabled.
Account and token transactions remain separate even within PostgreSQL.

Optional bootstrap uses `TOKENINSIGHTS_BOOTSTRAP_USERNAME` paired with
`TOKENINSIGHTS_BOOTSTRAP_PASSWORD` or `TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE`.
Executable settings validation owns secret-file reading; password values have no
argv representation. Before readiness, accounts initializes one fixed ordinary
user/dataset. Initialize its password once: restart/env changes never reset a
changed password or reenable a disabled account. A conflicting username rejects
rather than renaming an existing account. Bootstrap creates no API key. Legacy
`ADMIN_ACCOUNT`/`ADMIN_TOKEN` reject.

Administration and recovery remain on the private owner socket. Existing user
creation takes a username but leaves password login unavailable until set.
`admin user reset-password USER_ID` reads the new password from stdin. No public
administrator roles/UI, recovery emails or invitations are introduced.

Future GitHub/Google identities should attach to the same internal user through
explicit verified linking. Provider identity must remain independent of username,
dataset and email; matching an email alone must not link accounts. No OAuth code,
provider framework or external identity dependency is required now.

## Verification

Extend shared real SQLite/PostgreSQL account contracts for username uniqueness,
password authentication, interrupted provisioning, bootstrap/reopen, stable
identity, disabled users, session invalidation and immediate key revocation.
Verify users cannot access each other's keys, and key credentials cannot manage
accounts. HTTP tests cover signup policy, public discovery, local unavailable
routes, same-origin protection, bounded password admission and cookie policy.
Browser tests cover hosted login/settings and absence of personal account flows.
Native/container tests cover settings/secret inputs, bootstrap and replacement.
Format/lint, schema/API checks, focused/full tests and builds remain local gates;
CI coverage is unchanged.

Unresolved questions: none.
