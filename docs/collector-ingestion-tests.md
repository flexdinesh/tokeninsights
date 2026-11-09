# Capture, acceptance and storage contracts

Current composition: [ADR 0027](adr/0027-personal-backend-and-explicit-connections.md).
Semantic boundary principles: [ADR 0010](adr/0010-current-contracts-and-boundaries.md).
Commands: [development](development.md). Tests use isolated synthetic sources and
real collector/token SQLite stores.

| Contract | Executable coverage |
| --- | --- |
| Capture/checkpoint atomicity, metadata-only privacy | `pipeline/extract_test.go`, `rawcollectorstore` |
| Native golden totals, collector rebuild, lost acknowledgement | `collector.TestRawCollectorNativeGoldenRebuildAndLostAcknowledgement` |
| Private content excluded from acceptance and persistence | `collector.TestRawCollectorKeepsPrivateContentOutOfTransportAndStorage` |
| Direct replay, copied source identity and rebuild | `collector.TestDirectCollectorGoldenReplayAndRebuild` |
| Direct receipts and rejection | `collector.TestDirectAcceptanceReceiptsAndRejections` |
| External submission rejected before capture | `collector.TestExternalDestinationRejectsBeforeCapture` |
| Durable acceptance, replay/concurrency and pending reopen | `datastore` and `storagecontract` |
| Publication rollback, cancellation, reopen and retry | `datastore.TestPublicationRollbackReopenAndRetryPreservesUsage` |
| Generation cutover, late evidence and restart | `datastore/generation_test.go` |
| Physical contract rejection without mutation | `collectordb`, `datastore`, `localstorage` |
| Query snapshot, binding, pagination and five token components | `storagecontract`, `adapters/sqlanalytics` |
| Shared capture and bounded stronger follow-up | `localbackend/capture_test.go` |
| Final lease shutdown/reopen and reconnect grace | `localbackend/socket_test.go`, `localbackend/lifetime_test.go` |
| Compatible startup and protocol rejection | `localclient`, `localprotocol` |
| Real OpenSSH | `deployment.TestSSHEndpointDeployment` |
| TLS authentication, pin/token rejection and expiry | `remoteaccess/gateway_test.go`, `tcptransport` |
| Redacted bridge metadata, Refresh and independent lifetime | `remoteaccess.TestGatewayBridgeMetadataRefreshAndIndependentLifetime`, `dashboardproxy` |
| Native secret handoff and desktop acknowledgement | `desktoplaunch` and installed distribution/desktop validation |
| Dependency boundaries | `architecture.TestLayerDependencies` |

Paths are relative to packages/core. Pure `processor` and `publication` tests
retain independent semantic oracles for native identity, ambiguity, token
components, provider attribution and calendar/time bounds. Boundary tests do not
replace them.

## Storage and transport verification

`storagecontract` uses real SQLite fixtures with semantic operations and durable
reopen. Acceptance commits evidence/receipts/scopes together; projection commits
facts/estimates/provenance/outcomes together. Failure injection must prove rollback,
retained receipt bytes, stable IDs/components and exactly-once eventual totals.
Inspecting the collector journal verifies its own durable boundary; application
assertions use semantic operations and query results.

Personal SQL retains dataset-qualified keys and fixed historical role literals.
Tests may exercise those storage invariants without promising hosted accounts,
multiple product modes or another storage engine. Source-format fixtures describe
real artifacts; they remain supported despite retired product APIs.

Local container validation must retain real OpenSSH and isolated network access,
including TCP on the serving machine, bad pins/tokens, expired strings and exact
protocol mismatch. Prove query/Refresh parity across local/SSH/TCP and shared
owner/startup/final lease behavior. Use installed apps for native handoff/unavailable
app and Node-free Go runtime checks.

Before PR creation/update: full `mise run check:pr` and
`mise run build:docs` on clean committed HEAD. Root owns aggregate validation
during concurrent implementation.
