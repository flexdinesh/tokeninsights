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
| Query snapshots, dataset binding and pagination | `queryclient` and `analytics` tests |
| Composition, process ownership and shutdown | `localruntime`, `remoteserver` and `deployment` tests |
| Mode precedence and invalid destination rejection | `config.TestResolvedModeAndDestinationContract`, `cli.TestIngestionPreflightRejectsBeforeCapture` |
| Dependency boundaries | `architecture.TestLayerDependencies` |

Paths are relative to `packages/cli/internal`. Pure `processor` and `publication`
tests retain independent semantic oracles for accounting, native identity, ambiguity,
token components and time bounds. They complement boundary tests.

Deleted normalized-publication pipelines, legacy schemas, migration/import adapters
and old routes have no retained tests. Harness source-format tests remain: their
input variations describe real source artifacts, not obsolete product APIs.
