# Local and remote deployment conformance

Synthetic Pi input with independently specified counters. `internal/deployment`
builds both real binaries and exercises config -> sync -> HTTP ingestion ->
SQLite receipt -> REST. Each client has isolated preferences, source files and
Collector storage. Copied histories must contribute once across clients and
Collector rebuilds; distinct native IDs contribute independently.

The existing four-harness `collector-rebuild` and F01-F14 fixtures remain the
full accounting/replay/failure oracle. This fixture adds deployment and routing
coverage rather than another implementation of ingestion.
