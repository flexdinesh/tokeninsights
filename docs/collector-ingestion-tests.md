# Capture, delivery and storage contracts

Current guarantees and test policy: [ADR 0010](adr/0010-current-contracts-and-boundaries.md).
Commands and tooling: [development](development.md). Tests use isolated synthetic
sources and real current stores.

| Contract | Executable coverage |
| --- | --- |
| Capture/checkpoint atomicity and privacy | `pipeline/extract_test.go`, `rawcollectorstore` tests |
| Golden native totals, collector rebuild and lost acknowledgement | `collector.TestRawCollectorNativeGoldenRebuildAndLostAcknowledgement` |
| Private source content excluded from HTTP and persisted storage | `collector.TestRawCollectorKeepsPrivateContentOutOfTransportAndStorage` |
| Direct replay and rebuild | `collector.TestDirectCollectorGoldenReplayAndRebuild` |
| Direct/HTTP acceptance, receipts and rejection parity | `collector.TestDirectAndHTTPShareAcceptanceReceiptsAndRejections` |
| Credential rotation and account switch | `collector.TestInjectedDestinationRetryRotationAndAccountSwitch` |
| Durable acceptance and duplicate replay | `datastore.TestAcceptanceReplayAndConcurrentDuplicates`, `TestPendingAcceptanceSurvivesRestart` |
| Dataset isolation with colliding native identities | `datastore/datasets_test.go` |
| Projection rollback, reopen and retry | `datastore.TestPublicationRollbackReopenAndRetryPreservesUsage` |
| Atomic generation cutover, late evidence and restart | `datastore/generation_test.go` |
| Structural contract rejection without mutation | `db.TestCollectorContractRejectsWithoutMutation`, `datastore.TestCurrentInspectionRejectsChangedContractsWithoutMutation` |
| Query snapshots, dataset binding and pagination | `storagecontract` and `adapters/sqlanalytics` tests |
| Composition, process ownership and shutdown | `localruntime`, `remoteserver` and `deployment` tests |
| Mode precedence and invalid destination rejection | `config.TestResolvedModeAndDestinationContract`, `cli.TestIngestionPreflightRejectsBeforeCapture` |
| Dependency boundaries | `architecture.TestLayerDependencies` |

Paths are relative to `packages/cli/internal`. Pure `processor` and `publication`
tests retain independent semantic oracles for accounting, native identity, ambiguity,
token components and time bounds. They complement boundary tests.

## Supported compositions

| Scenario | In-process SQLite | Hosted SQLite and PostgreSQL |
| --- | --- | --- |
| Four native harnesses, copied sources, collector rebuild and reprocessing; all token components and stable identities | `collector.TestDirectCollectorGoldenReplayAndRebuild` | `deployment.TestAllHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild` (built client/server, interrupted replacement generation, HTTP session and receipt fact IDs) |
| Committed acceptance with lost response; exact request/receipt replay | `collector.TestDirectCollectorGoldenReplayAndRebuild` | `deployment.TestRemoteCommittedResponseLostReplaysExactRequestAndReceipt` (restart before retry) |
| Failed delivery retains journal/cursor; accepted work resumes | Local queue/lifetime tests in `localruntime`; shared adapter recovery below | `deployment.TestRemoteFailureRetainsJournalAndRestartResumesAcceptedWork` |
| Concurrent copied and distinct native identities | Collector direct replay and storage concurrency contracts | `deployment.TestConcurrentCopiedAndDistinctClients` |
| Configured remote, repeat sync, copied clients and collector rebuild | Not a local routing scenario | `deployment.TestConfigRemoteSyncTracerAndCopiedClients` |
| Account provisioning, tenant isolation, spoof rejection and restart | Local default-user contracts | `remoteserver.TestHostedSharedDatabaseIsolationAndProvisioning` |

`storagecontract.RunTokens` and `RunAccounts` enforce shared semantics against real
SQLite and PostgreSQL adapters: pending reopen, atomic rollback, generation fences,
query snapshots/components/calendar boundaries, provisioning and revocation.
Engine-specific schema, transaction, ownership and physical privacy checks stay
beside adapters/collector storage. Application assertions use semantic interfaces
or HTTP; inspecting the collector's SQLite journal verifies its own durable boundary.

Deployment restart fixtures stop the server and commit through the real adapter
without a worker before restarting the same binary/endpoint. This deterministically
proves pending recovery; it is not a power-loss or database failover simulation.
Root `pnpm test` and `pnpm test:race` run both hosted backends in local pre-push
verification. Direct `go test` skips PostgreSQL without `TOKENINSIGHTS_TEST_POSTGRES_DSN`;
use the root scripts for complete verification.

Deleted normalized-publication pipelines, legacy schemas, migration/import adapters
and old routes have no retained tests. Harness source-format tests remain: their
input variations describe real source artifacts, not obsolete product APIs.
