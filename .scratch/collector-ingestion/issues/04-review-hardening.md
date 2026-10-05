# Harden collector/server review failures

Status: ready-for-agent
Implementation: Implemented and verified. All changes belong to PR #53.

User approved collector/server schema 16/2 and stricter OpenAPI on 5 October
2026. No earlier PR databases contain user history; fresh reconstruction is
accepted. No preserving migration, legacy aliases or receipt rewriting.

## Ownership and acceptance

| Review | Owner | Required regression outcome |
| --- | --- | --- |
| R01 timestamps | Ingestion + normalization | Shared bounded epoch milliseconds before source merge, references and ingestion; canonical SQL rejects invalid times; valid sibling data survives; mixed invalid batches change no durable state. |
| R02 weak session identity | Harness | Pi/Claude filename evidence raw-only; weak Claude conflicts cannot block native data; all source, queue and reconstruction orders agree. |
| R03 equivalent Claude usage | Harness | Compare canonical values; omitted zero counters dedupe; reasoning removed once; true same-time native conflicts preserve previous data. |
| R04 changed credentials | Service | Same/omitted token reuses; differing explicit token rejects; private comparison never reveals credentials; restart rotates actual HTTP authorization. |
| R05 unhealthy shutdown | Service | Verified owner control independent of DB health; missing/corrupt storage stops; missing restart creates fresh DB; corrupt restart preserves file and refuses startup. |
| R06 stale TUI facets | Query | Tagged responses, centralized identity transitions, obsolete request generations discarded; no rollback or recursive refresh loop. |
| R07 plugin paths | Tooling | Both builders/checks work in escaped paths and clean temporary output on failure. |
| R08 untagged responses | Query + web | Required identities/readiness/revision; omitted revision differs from zero; unavailable metadata is observable but cannot enable analytics. |

## Verification

Regression fixtures/oracles precede fixes. Use production parsers, file-backed
SQLite, real HTTP/control endpoints and deterministic delayed responses.
Preserve exact CFI001 native fact/component oracle, F01–F14 and twelve crash seams.
Record executable test names in docs/collector-ingestion-tests.md and actual
commands/results in ../VALIDATION.md. Full format/lint/unit/race/schema/API/
artifact/native/browser gates run before normal push; independent final review.

## Unresolved questions

None. Schema changes approved; all earlier PR data may be rebuilt.
