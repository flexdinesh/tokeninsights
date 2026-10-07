# Collector ingestion failure tests

Historical protocol-1 reference superseded by [design](design.md) and [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md). Legacy tests remain compatibility oracles; raw tests cover new contracts.

Status: implemented production SQLite/HTTP tests in PR #53. Full repository validation is recorded in [validation evidence](../.scratch/collector-ingestion/VALIDATION.md). Guarantee IDs refer to [the architecture contract](collector-server-architecture.md); task ownership and dependencies are in the [execution plan](../.scratch/collector-ingestion/EXECUTION-PLAN.md).

## Independent evidence

Fixtures are synthetic. Expected native identities, token components, countability, provenance, and totals are authored independently; parser output never regenerates goldens. The [collector rebuild fixture](../packages/cli/testdata/conformance/collector-rebuild/README.md) contains 12 facts totaling 1,102 tokens: input 800, output 148, reasoning 52, cache read 96, cache write 6.

Tests drive production adapters, normalization, publication storage, ingestion, and queries. Exact fact sets and every counter are checked across retained-source reconstruction, 100 forced collections, copied/moved artifacts, SQLite reopening, and HTTP delivery. Receipt success is also checked through REST queries. A stable total alone cannot prove correct deduplication.

[Protocol examples](../packages/cli/testdata/conformance/collector-ingestion-protocol/README.md) remain illustrative traces with symbolic IDs. The executable wire contract is [OpenAPI](openapi.yaml), backed by the strict `internal/publication` codec. Symbolic traces are test vocabulary, not request DTOs.

## Traceable failure matrix

All names below are executable production tests under `packages/cli/internal`.

| Failure | Guarantee | Production tests | Required outcome |
| --- | --- | --- | --- |
| F01 Collector database deleted | G01, G02, G05 | `ingestion.TestFailureContractFreshStreamsPreserveDistinctRequests`; `collector.TestCollectorContractRebuildAndRepeatPreserveNativeFacts` | Fresh stream/batches reproduce the same facts without amplification; equal-valued native requests remain distinct. |
| F02 Commit succeeds; response lost | G05, G06, G07, G10 | `ingestion.TestFailureContractLostResponseReopenReturnsExactReceipt`; `collector.TestCollectorContractLostAcknowledgementResumesSavedBatch` | Server facts are queryable; collector remains pending; reopening/replay returns the original receipt. |
| F03 Batch ID reused with different bytes | G05, G11 | `ingestion.TestFailureContractBatchAndFactConflictsAreAtomic` | 409, original facts and receipt unchanged. |
| F04 Same fact ID, unsupported changed payload | G05, G11 | `ingestion.TestFailureContractBatchAndFactConflictsAreAtomic` | Whole batch conflicts in either arrival order; no second contribution or last-writer replacement. |
| F05 Concurrent/repeated submissions | G05, G06 | `ingestion.TestFailureContractConcurrentDuplicateTransactions`; `ingestion.TestFailureContractDuplicateEntitiesWithinBatch`; `ingestion.TestFailureContractConcurrentBatchBodyConflict` | Same batch and different batches converge; identical intra-batch facts count once; conflicting repetition rolls back. |
| F06 Source revisions out of order; delivery gap | G05, G07, G11 | `ingestion.TestFailureContractClaudeSourceRevisionOrder`; `collector.TestCollectorContractDestinationIsolationAndReceiptBinding`; `collectorstore.TestAcknowledgementFailureRollsBackReceiptAndCursor` | Native Claude source timestamps order snapshots; stale no-op, equal-time conflict. Wrong receipt/range cannot advance contiguous progress. Journal sequence is never source revision. |
| F07 Owner isolation | G05 | `ingestion.TestFailureContractSeparateOwnerDatabases` | One owner per database; another database's receipt cannot acknowledge this upload. Multi-tenant partitions are deferred. |
| F08 Invalid references/mid-batch SQL failure | G06, G11 | `ingestion.TestFailureContractSQLiteFailureRollsBackWholeBatch`; `ingestion.TestFailureContractValidationPrivacyAndCompatibility` | Self-contained references validate against native tuples; no dangling facts, partial references, revision, or receipt. |
| F09 Unsupported publication/storage version | G09, G11 | `ingestion.TestFailureContractValidationPrivacyAndCompatibility`; `db.TestLegacySchemasRejectWithoutMutation`; serverstore role/version tests | Wire incompatibility rejects before mutation; legacy/wrong-role/unknown storage remains unchanged. |
| F10 Invalid counters/aggregate overflow | G06, G11 | `ingestion.TestFailureContractValidationPrivacyAndCompatibility`; `ingestion.TestFailureContractAggregateOverflowIsAtomic`; publication codec tests | Negative, fractional, missing, unsafe, overflow, contradictory totals reject; aggregate sums cannot exceed the safe integer domain. |
| F11 Admission/body/entry bounds; writer busy | G06, G07, G10, G11 | `ingestion.TestFailureContractAdmissionBodyAndBusyRetry`; `collectorstore.TestBatchEntryBoundAndContiguity`; `ingestion.TestFailureContractLimitsAtBoundary` | Four admissions, 1 MiB body, 256 entries; boundary and over-limit checks. Busy work receives no success and resumes manually. |
| F12 Private/raw fields | G08, G11 | `ingestion.TestFailureContractValidationPrivacyAndCompatibility`; `ingestion.TestLocationPathRejectionIsAtomicAndDoesNotPersistPrivateLabels`; `publication.TestLocationLabelsRequirePortableBasenames`; publication strict-codec tests; `collector.TestCollectorContractRawContentNeverEntersPublication` | Allowlist excludes conversation/source data; unknown/private fields and path-bearing location labels reject; safe errors never echo submitted content. |
| F13 Sources disappear | G05, G10 | `ingestion.TestFailureContractSubsetUploadRetainsHistory`; `collector.TestCollectorContractMissingSourcesRetainServerFacts` | Missing sources do not delete committed history; no retraction. |
| F14 Process crash | G03–G07, G10, G11 | `collector.TestCollectorContractCrashBoundaries`, twelve subtests below | Real killed subprocesses, SQLite/WAL reopening, and manual resume preserve atomic facts/work/progress. |

The server accepts one owner per database. F07 deliberately uses independent databases and durable database identities; it does not claim multi-user authorization. One native namespace per harness is supported initially. Profiles known to reuse native IDs require a future durable namespace contract.

## Persistence crash boundaries

`TestCollectorContractCrashBoundaries` uses test-only SQLite scalar functions and triggers, explicit progress/operation barriers, an inherited signaling pipe, and an external process kill. Production has no environment-controlled fault hooks. These are not graceful shutdown simulations. WAL files and lock inodes are preserved.

| Subtest | Verified after reopen | Manual resume |
| --- | --- | --- |
| `before_capture_commit` | No raw/canonical/journal/source cursor progress | Reparse. |
| `during_capture_transaction` | Capture and source cursor roll back together | Reparse. |
| `after_capture_commit` | Raw data and pending normalization durable | Normalize. |
| `during_canonical_journal_transaction` | Canonical facts and journal both roll back; work remains | Normalize. |
| `after_canonical_journal_commit` | Twelve canonical facts and journal entries durable | Publish. |
| `during_batch_preparation_transaction` | No partial batch or cursor advance | Prepare. |
| `after_batch_saved_before_send` | Exact saved batch exists; cursor pending | Send saved bytes. |
| `during_server_transaction` | No partial server facts or receipt | Replay saved bytes. |
| `after_server_commit_before_response` | Twelve facts and receipt durable; collector pending | Replay original receipt. |
| `after_response_before_collector_ack_commit` | Server durable; collector pending | Replay. |
| `during_collector_ack_transaction` | Receipt recording and cursor both roll back | Replay. |
| `after_collector_ack_commit` | Durable cursor reaches sequence 12 | Send only later work. |

Each subtest then reopens server storage and runs manual collection/publication to the independently expected final state. SQL-trigger failures separately exercise journal/entity atomicity, alias refresh, whole-batch rollback, and receipt/cursor rollback. Filesystem initialization and role guards have their own store tests.

These tests cover process death and deterministic transaction failures. They do not emulate physical media corruption, every filesystem durability mode, or actual power loss.

## Harness identity regressions

| Fixture | Production pipeline test | Oracle |
| --- | --- | --- |
| CFI001 Baseline | `TestCollectorRebuildFixture` | Exact twelve facts/components/provenance/diagnostics. |
| CFI002 Reparse 100 times | `TestCollectorRebuildRepeatedSyncPreservesFactsAndIdentities` | Stable values and identities at advancing clocks. |
| CFI003 Fresh database | `TestCollectorRebuildFreshDatabaseAtDifferentClocks` | Same retained input gives the same fact payloads. |
| CFI004 Raw-only reconstruction | `TestCollectorRebuildNormalizeRetainedRawWithoutSources` | Normalize without source artifacts. |
| CFI005 Moved artifacts | `TestCollectorRebuildMovingArtifactsPreservesFacts` | Native identity survives relocation. |
| CFI006 Copied artifacts | `TestCollectorRebuildCopiedArtifactsDoNotMultiplyUsage` | Copies do not multiply usage. |
| CFI007 OpenCode equal timestamps/counters | `TestCollectorRebuildOpenCodeEqualTimestampDistinctNativeRequests` | Three facts / 276 tokens; native IDs distinguish legitimate requests. |
| CFI008 Claude partial/final | `TestCollectorRebuildClaudeAppendMatchesFreshFinalParse` | One final fact / 120 tokens; entire newer snapshot replaces older. |
| CFI009 Missing Pi message IDs | `TestCollectorRebuildPiMissingIDsRetainRawEvidence` | Retain raw metadata and diagnostics; do not invent countable identity. |

Both previously failing CFI007/008 are fixed without weakening expected facts. Adjacent tests cover native request/session scope, delimiter-safe tuples, reversed records, decreasing components, stale copies, equal-time conflicts, weak-identity quarantine, and missing occurrence. Review regressions also prove local delimiter-separated identities cannot collide and filename-derived Pi/Claude sessions remain local instead of multiplying published facts after copies. Session/message time envelopes merge retained source ranges, so partial history can retain wider envelopes than a fresh final-only parse without changing a contribution.

Codex keeps its deterministic immutable event witness, including typed token-field presence. This is an explicit adapter exception, not permission to hash mutable counters into every fact identity.

## Review hardening regressions

R01–R08 supplement F01–F14; schema 16/2 and stricter response fields were
explicitly approved. The [review issue](../.scratch/collector-ingestion/issues/04-review-hardening.md)
records parallel ownership. Expected values remain independent of parser output.

| Review | Production tests | Required outcome |
| --- | --- | --- |
| R01 source timestamps (F08/F10) | `publication.TestTimestampDomainAcrossNormalizedFields`; `ingestion.TestInvalidTimestampBatchDoesNotMutateSavedData`; `db.TestCollectorCanonicalTimestampConstraints`; `pipeline.TestClaudeInvalidTimestampCannotReplaceValidRequest`; `pipeline.TestNormalizeInvalidTimestampCannotPoisonNativeSession`; `server.TestTimestampBucketsAcrossTimezones`; collector/server `TestUnbounded*SchemaRemainsUntouched` | Invalid dates remain raw-only; do not affect snapshot selection/references; encoding, SQLite and HTTP reject outside the shared millisecond range; rejected batches preserve saved state. Actual HTTP/native calendar buckets work at boundaries in six timezones; earlier schemas reject unchanged. |
| R02 weak identity (F01/F04) | `pipeline.TestClaudeWeakIdentityCannotBlockNativeInAnyRecordOrder`; `pipeline.TestClaudeWeakIdentityAcrossSyncsAndRetainedRaw`; `pipeline.TestClaudeWeakRawReparseAndFreshRebuildPreservePublication`; `pipeline.TestPiWeakIdentityCannotWidenNativeEnvelopes`; `pipeline.TestConflictingWeakClaudeRowsDoNotBlockUnrelatedNative` | All 24 record permutations, separate syncs, retained-raw normalization, copies, 100 reparses and reconstruction publish native facts once; weak session evidence creates no canonical references or blocks unrelated native usage. |
| R03 equivalent usage (F04/F06) | `pipeline.TestClaudeEquivalentOptionalCountersDoNotConflict`; existing Claude source-revision/conflict tests | Missing optional zero counters and consistent totals converge; reasoning remains exclusive; real same-time native conflicts still fail atomically. |
| R04 local auth retirement | `service.TestLegacySavedAuthenticationRequiresExplicitRestartAndPreservesReceipt`; `service.TestRunningLocalRejectsRetiredTokenOptionsWithoutReplacingOwner`; `service.TestPrivateTokenComparisonRemovedAndOwnerGuardPreserved` | Legacy credentials require explicit restart; receipt/history survive; retired token options cannot replace the owner; private owner guard remains. [System decision](system.md) supersedes public token auth. |
| R05 unhealthy lifecycle | `service.TestStopVerifiedDaemonDespiteMissingOrMalformedDatabase`; `service.TestRestartMissingDatabaseCreatesFreshAndMalformedDatabaseRemainsUntouched`; `service.TestStopDoesNotTakeOverHeldUnreachableOwnership` | Stop uses verified ownership, not query health; missing storage can restart; corrupt storage remains unchanged; unreachable owners remain protected. |
| R06 stale readers | `cli.TestInitialFilterWaitsForValidatedPublication`; `cli.TestDashboardReplacementAcceptsSnapshotAndInvalidatesOldReaders`; `cli.TestInitialDashboardIdentityRejectsEarlierFacetsAndStatus`; `cli.TestSamePublicationRevisionCannotMoveBackwards` | Initial and replacement identities invalidate earlier response generations; old facets/status cannot roll back publication or revision; valid replacement snapshots do not recursively retry. |
| R07 encoded paths | `tools/build/test/plugin-build.test.ts` | Real Pi/OpenCode builds and artifact checks support spaces/percent/hash in workspace and temporary paths; failures clean temporary output. |
| R08 response contract (F09) | `queryclient.TestQueryResponsesRequirePublicationEnvelope`; `queryclient.TestQueryEnvelopeAcceptsZeroRevisionAndExplicitUnavailableMetadata`; `server.TestQueryIdentityContractForEmptyAndUnavailableStorage`; `packages/web/src/contracts.test.ts`; `packages/web/src/api_identity.test.tsx` | Untagged/missing/null/empty analytics identities and omitted revisions reject; explicit zero succeeds; unavailable metadata cannot enable analytics; newer tagged responses remain usable. |

## Local/remote deployment tracers

`internal/deployment` builds and executes both production binaries. Each client
has isolated source files, config and Collector storage; each server has its own
SQLite file. The [Pi tracer fixture](../packages/cli/testdata/conformance/local-remote-deployment/README.md)
contains one native fact, input 100/output 20/total 120. Four-harness tests reuse
the independently authored 12-fact/1,102-token rebuild oracle.

| Case | Executable evidence | Outcome |
| --- | --- | --- |
| Configured remote, copied clients, rebuild | `TestConfigRemoteSyncTracerAndCopiedClients`; `TestAllHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild` | Exact SQL/REST components, stable IDs, no local bootstrap; shared dataset dedupes copies. |
| Remote unavailable | `TestRemoteFailureRetainsJournalAndNeverStartsLocal` | Journal retained; manual publish-only succeeds later; no fallback. |
| Destination A/B/A | `TestConfigDestinationSwitchingPreservesIndependentProgress` | Both servers receive retained history; independent cursors/receipts resume. |
| Concurrent copied/distinct clients | `TestConcurrentCopiedAndDistinctClients` | Copies count once; equal counters with distinct native IDs count separately. |
| Lost committed response | `TestRemoteCommittedResponseLostReplaysExactRequestAndReceipt` | Real upstream commit is queryable; cursor stays pending; replay uses exact saved bytes and original receipt. |
| Default local and explicit empty override | `TestConfiguredLocalDefaultAndExplicitEmptyRemoteOverride` | Wildcard local serving without token; empty flag/environment selects local. |
| Competing compositions | `TestLocalAndRemoteBinariesCannotOwnSameDatabase` | Both launch orders reject takeover; original server remains usable. |
| Config and query-only TUI | `config.TestConcurrentSettersPreserveUpdatesAndAtomicReads`; `cli.TestConfiguredRemoteQueryOnlyTUIUsesSameDestination` | Private atomic preferences; configured remote queries without collection. |

These supplement F01–F14 and the existing real process-crash tests; they do not
replace their detailed fact/reference/revision oracles. Schema 16/2 and protocol
1 remain unchanged. Backend queues, accounts, auth and remote rebinding UX remain
separate future work.

## Reproduction and maintenance

From `packages/cli`:

```sh
go test ./internal/pipeline -run 'TestCollectorRebuild|TestCanonicalPublication|TestPublication|TestClaudeMessageWithoutRequest' -count=1
go test ./internal/collectorstore ./internal/publication ./internal/serverstore
go test ./internal/ingestion -run '^TestFailureContract' -count=1
go test ./internal/collector -run '^TestCollectorContract' -count=1
go test -race ./internal/collector ./internal/collectorstore ./internal/ingestion ./internal/serverstore
```

Repository gates: `pnpm run format`, `pnpm run lint`, `pnpm run test`, `pnpm run build`, then `mise run check:push`. Fixture privacy checks remain part of root tests. Browser/TUI tests prove GET-only reload and remote query access without collector files. CLI tests exercise `tui` against saved REST data, grouped collector resets against real databases, help without storage side effects, and rejection of removed commands/flags before storage or service work.

Future regressions must name Fxx/Gxx and a fault seam, retain independent exact identity/value assertions, reopen persisted state, and specify the manual replay. Do not replace these with a simulated second ingestion engine, snapshot-generated goldens, skipped failures, or total-only assertions.

Safe diagnostics identify stage/code and saved batch ID. Journal sequence ranges, stream IDs, request hashes, and receipts are persisted locally for tracing. No raw artifacts, request bodies, full source paths, credentials, or conversation fields belong in logs.

## Unresolved questions

None for this scope. Remote provisioning, multi-tenant identity, receipt/journal retention, adapter revision rules beyond native Claude requests, and optional automatic retry remain later work.
