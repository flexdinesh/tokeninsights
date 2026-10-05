# Prove stable harness identities

Status: needs-triage

## Problem

Canonical publication needs reproducible identities after collector-state deletion. Current logical-copy suppression and changed-source identity behavior must be distinguished from identical-input rebuild invariance.

## Evidence

[Identity audit](../IDENTITY-AUDIT.md); executable `collector-rebuild` fixtures; [failure matrix](../../../docs/collector-ingestion-tests.md).

Confirmed red regressions: CFI-007 OpenCode suppresses distinct native requests (2 facts, expected 3); CFI-008 Claude partial/final sync duplicates one request (2 facts, expected 1). CFI-009 retains weak-ID raw evidence and diagnostics only; canonical policy remains pending. See [validation](../VALIDATION.md).

## Acceptance

- Document native/fallback identity and copied-history evidence for every harness.
- Keep legitimate equal-token requests distinct, including matching source timestamps.
- Repeated/full parsing and fresh databases preserve exact canonical facts and stable identities.
- Resolve genuine red regressions through reviewed adapter semantics; retain their fixtures.
- Explicitly decide changed-payload and weak-identity behavior before publication implementation.
- Update affected tests, README, and design; obtain approval before schema/identity-contract storage changes.

## Unresolved questions

- Weak native identity?
- Changed-source replacement policy?

## Comments

Foundation task creates independent synthetic expectations; existing bugs must not be hidden by changing those expectations.
