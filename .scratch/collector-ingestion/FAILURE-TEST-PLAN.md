# Executable collector/server failure plan

Status: Approved contracts implemented; production-path failure suites present. Final repository verification remains pending.

This supplements [architecture guarantees](../../docs/collector-server-architecture.md), the [current failure matrix](../../docs/collector-ingestion-tests.md), and [candidate traces](../../packages/cli/testdata/conformance/collector-ingestion-protocol/README.md). Candidate JSON is illustrative; the typed publication codec, role schemas and OpenAPI define the runtime contract. The matrix owns exact coverage and limitations; this document explains test mechanics and remaining review checks.

## Approved constraints

- Fresh `collector.sqlite` and `server.sqlite`; original `tokeninsights.sqlite` untouched. No legacy identity migration, alias bridge or silent recovery of untagged databases.
- One trusted owner per server database. Separate owner databases are tested; multi-tenant authentication and cross-owner receipt APIs are deferred.
- Typed native tuple IDs survive collector deletion. Missing native identity is retained locally with publication diagnostics, not guessed.
- Claude native request/source timestamp revisions replace one contribution; stale values no-op, equal-time conflicts reject. Unsupported changed values conflict atomically.
- One immutable pending batch per destination. Ack validates exact database/stream/batch/hash/range and advances only the contiguous saved range. Server arrival order does not establish source precedence.
- Strict explicit integer counters within 9,007,199,254,740,991, additive totals and aggregate overflow checks; 1 MiB body, 256 entries and 256-byte bounded fields.
- Normalized self-contained session/message/location references only. Unknown/private fields, duplicate JSON keys, trailing data and incompatible versions reject before mutation.
- Remote destination pins its database identity. Local replacement creates a new binding and replays retained journal from zero. Server history survives collector/source deletion.

## Independent oracles and mechanics

The native fixture passes through real adapters, normalization, journal, immutable batches, HTTP ingestion and REST queries. Its literal oracle is 12 facts, input 800, output 148, reasoning 52, cache read 96, cache write 6, total 1102. Compare native identity sets and every component; 100 full reparses and fresh collector streams must preserve them.

Protocol adversaries use explicitly constructed normalized facts: A=(80,20,0,0,0)=100 and distinct native B with equal counters independently adds 100. A changed=(80,40,0,0,0)=120 is accepted only with supported source revision evidence; A+B totals 200 and changed A+B totals 220. Expectations remain literal rather than being computed by another ingestion state machine.

Tests use temporary file-backed SQLite, real production stores/HTTP handlers, deterministic trigger failures and actual collector requests. Direct SQL inspects persisted state; it does not bypass validation to seed countable server facts. Receipt mismatch fake servers complement successful real-server delivery tests. Process kills use named pipe barriers and test-only SQLite callbacks/triggers, bounded deadlines and external kill. Reopen the same files/WAL without deleting or repairing journal data. No public fault API or production environment kill switch exists.

## F01–F14 executable mapping

| Failure | Production tests and scope |
| --- | --- |
| F01 reconstruction/replay | `TestCollectorContractRebuildAndRepeatPreserveNativeFacts`; `TestFailureContractFreshStreamsPreserveDistinctRequests`; exact native fixture, new streams and 100 reparses. |
| F02 lost response | `TestCollectorContractLostAcknowledgementResumesSavedBatch`; `TestFailureContractLostResponseReopenReturnsExactReceipt`; saved request/receipt equality after committed response loss and reopening. |
| F03 changed retry bytes | `TestFailureContractBatchAndFactConflictsAreAtomic`; `TestImmutableBatchAndIndependentDestinations`; same replay key with different exact bytes conflicts. |
| F04 changed fact | `TestFailureContractBatchAndFactConflictsAreAtomic`; `TestFailureContractClaudeSourceRevisionOrder`; unsupported mutation rejects, supported native revisions converge. |
| F05 concurrency | `TestFailureContractConcurrentDuplicateTransactions`; actual transaction uniqueness and durable original receipt. |
| F06 progress/revision order | `TestReceiptBindingRejectsEveryForeignAcknowledgement`; `TestAcknowledgementFailureRollsBackReceiptAndCursor`; Claude source-order tests. One pending batch is the implemented collector contract; a fabricated second unacknowledged batch cannot bypass it. |
| F07 owner isolation | `TestFailureContractSeparateOwnerDatabases`; trusted single-owner composition, no uploader owner selector. Deferred multi-tenant authorization is not claimed. |
| F08 references/receipt atomicity | `TestFailureContractSQLiteFailureRollsBackWholeBatch`; native reference validation and whole-batch rollback; references accompany each fact. |
| F09 compatibility | `TestFailureContractValidationPrivacyAndCompatibility`; `TestServerRejectsLegacyCollectorAndFutureSchemaWithoutMutation`; role/version rejection preserves stored history. |
| F10 numeric failures | `TestStrictBatchDecoderRejectsInvalidAndPrivateFields`; `TestNumericIdentityAndRevisionValidation`; `TestFailureContractAggregateOverflowIsAtomic`. |
| F11 bounds/busy retry | `TestFailureContractAdmissionBodyAndBusyRetry`; `TestBatchEntryBoundAndContiguity`; bounded admission/body and actual SQLite contention. |
| F12 privacy | `TestFailureContractValidationPrivacyAndCompatibility`; native publication sentinel checks in collector contract tests; source safety fixtures. |
| F13 retained history | `TestCollectorContractMissingSourcesRetainServerFacts`; `TestFailureContractSubsetUploadRetainsHistory`; collector reset boundaries. |
| F14 crashes | `TestCollectorContractCrashBoundaries`; twelve external-kill barriers inspect intermediate durable state and converge to exact native facts/totals. |

Focused package test outcomes belong in [VALIDATION.md](VALIDATION.md). Test existence and illustrative JSON snapshots are not claims that the final full suite, mutation review or every future remote scenario was verified.

## F14 seam checklist

`TestCollectorContractCrashBoundaries` implements all twelve barriers below with the native 12-fact/1102-token fixture. Each case starts independently; server rollback/preservation tests separately seed unrelated history. Native facts appear once after recovery. Check intermediate persisted state before resuming so an eventual correct total cannot hide an unsafe acknowledgement or lost queue.

| Observed child barrier | After kill/reopen | Required next manual action |
| --- | --- | --- |
| Before capture commit | No target raw facts or advanced source continuity marker | Capture again |
| During capture transaction, after first write | No target partial raw/observations/normalization work or advanced marker; failure audit, if separately committed, cannot claim capture success | Capture again |
| After capture commit | Captured raw evidence, matching source progress, normalization work durable; target server facts absent | Normalize and deliver |
| During canonical/journal transaction, after canonical write | No target canonical mutation or associated journal entry; normalization work remains pending | Normalize and deliver |
| After canonical/journal commit | Canonical and corresponding journal snapshots both durable; target server facts absent | Prepare/deliver |
| During immutable batch save | No delivery cursor advance; any committed batch has its complete immutable body and retry identity | Reuse saved batch or prepare from pending journal |
| After saved batch, before send | Exact retry identity/body retained; target server facts absent | Send saved batch |
| During server transaction, after first entity write | No target partial facts/session/receipt; collector batch pending | Replay saved batch |
| After server commit, before response | A+B and receipt durable/queryable; collector remains pending | Replay saved batch |
| After response, before collector ack transaction | Same as previous seam; received bytes alone cannot imply durable progress | Replay saved batch |
| During collector ack transaction, after first state write | Batch acknowledgement and contiguous destination cursor are both rolled back or both committed; never mismatch | Replay only if still pending |
| After collector ack commit | Destination cursor corresponds to durable matching server receipt; acknowledged batch not resent by ordinary sync | Deliver only later pending work |

## Additional implemented boundaries

- **Destination independence:** `TestCollectorContractDestinationIsolationAndReceiptBinding` publishes the same native facts independently to two actual databases.
- **Server replacement:** `TestRemoteBindingPinsDatabaseAndNeverStartsLocal` rejects remote replacement; `TestLocalReplacementReplaysJournalWithIndependentBinding` replays into a fresh local database without inheriting a cursor.
- **Immutable retry/capabilities:** `TestReceiptMismatchKeepsExactBatchForNextManualSync` preserves bytes through mismatched receipts and incompatible capabilities, then accepts a later valid acknowledgement.
- **Source failures/offline collection:** collection and delivery outcomes remain separate. Previously normalized journal work can publish despite capture failure; manual publish-only needs no source discovery.
- **Schema/path isolation:** wrong-role/legacy storage rejects before mutation; same inode, symlink and dangling aliases reject before collection or creation.
- **Native tuple vectors:** `TestIdentityNativeTupleVectors` pins Unicode/delimiter/native identities without installation/batch/clock fields.
- **Diagnostics:** fixed stage/code and saved batch correlation expose delivery failures without echoing URLs, credentials or incoming raw bodies.

## Review and remaining verification

The root owner maintains precise runnable coverage in the failure matrix and actual commands/results in validation. Run root format/lint before full tests, schema/API checks, native build/runtime, race and affected browser gates. Plugin fake-executable tests do not prove real-host installation, event timing or durable source flush.

Controlled mutation review remains optional additional evidence: disable uniqueness; advance ack before server commit; save canonical without journal; alter saved retry values; partially commit before validation; allow forbidden fields. Record experiments only if actually performed and revert all changes before verification. The current suites must execute production implementations, not a parallel simulated ingestion engine.

## Unresolved questions

None blocking the approved contract. Later scope: remote provisioning/multi-tenant authorization, retention and isolated native-host plugin verification.
