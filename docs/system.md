# System design

Implemented [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md).
[Design](design.md) specifies storage, identities, processing and queries.

Local: tokeninsights service start, managed background process, public dashboard/
query 127.0.0.1:8765 (optional 0.0.0.0), private Unix ingestion/control.
Remote: tokeninsights-server foreground, public HTTP ingestion/query, deployment
owns supervision. Both share DuckDB core/dashboard; auth/accounts deferred.

Collector SQLite handles sanitized outbox/continuity/exact retries. Sync completes
at acceptance; server processes asynchronously. Startup never collects.
One selected destination: empty server-url local, explicit URL remote. Remote
failure never starts local. No relay/fan-out.

Client config: XDG_CONFIG_HOME or ~/.config/tokeninsights/config.json.
Flags > environment > file > defaults; explicit empty server-url selects local.
Preferences differ from runtime records; bind changes require restart.
Service commands always local regardless of client routing.

Defaults: collector.sqlite and server.duckdb. Future application data remains
separate SQLite when needed. Fresh default DuckDB imports verified sibling
server.sqlite read-only. Explicit custom imports require new target. Historical
tokeninsights.sqlite untouched. Raw/receipts/scopes commit together; projections/
outcomes commit later. Confirmed never mixes estimates. Generations preserve
published history until replacement complete; imported history stays until proven
coverage. Raw/outbox initially retained indefinitely.

No unresolved product questions. Follow-up: remote auth/ownership/rebinding,
compaction and measured scaling. See approved plan and whitelist.
