# Service implementation validation

Date: 2026-10-03. Native environment: Linux x86_64, Go 1.27.1 (`X:nodwarf5`). All native checks used isolated temporary HOME/XDG/source directories. No normal user database or harness sources were used.

## Shipped behavior

- Root invocation ensures the detached service and prints its URL/PID/bind. Repeated start reuses its instance, with zero new sync jobs.
- `service start|stop|restart|status` and foreground `service run` share ownership. `--host` changes only web/API binding; TUI connects locally through Unix control and reads local SQLite.
- Missing storage starts empty without ingest. Existing outdated storage stays unrepaired until requested refresh. Unknown storage is refused without modifying its bytes.
- Every normal TUI opening requests shared refresh; closing it cancels observation. `view --no-sync` remains read-only, including disabling `u`. Web opening, focus/reconnect, and filters perform reads only; Refresh explicitly requests ingest.
- Requests after capture coalesce into one follow-up. Caller-specific sync/normalize options and source roots survive online forwarding. Explicit reset cancels pending ordinary refresh, gates reads, and changes the data epoch.
- Public administration routes remain absent. Public responses omit private source paths. Schema 14, generation 5, token semantics, source identities, and recovery fingerprints remain unchanged.

## Automated checks

Passed:

- Root format, pinned Go/TypeScript lint, API generation consistency, SQLite schema consistency, and production build.
- Full `pnpm run test`: 16 build-tool tests, 42 web unit tests, every Go package.
- `go test -race ./internal/app ./internal/service ./internal/cli`.
- All 19 browser E2E tests against the built native service and isolated fixtures.
- Direct Go build from `packages/cli` using embedded committed assets.
- Darwin arm64 and Linux arm64 cross-compilation. Runtime checks were Linux only.

Focused tests exercise pre/post-capture demand, requester cancellation, one pending follow-up, failed-refresh follow-up, thousands of bounded aliases/outcomes, exclusive FIFO/options/deduplication, reset/read barriers, and log rotation. Real subprocess tests exercise inherited descriptors, concurrent startup, symlink identity, busy ports, crash recovery, foreground shutdown, source forwarding, cancellation during writer contention, cancellation during startup, and discovery across differing XDG runtime environments.

Help commands succeed without database side effects; stopped status returns exit 3 without duplicate errors. Web tests verify no mount refresh, shared pending feedback, epoch invalidation, stale responses, saved data after failures, coverage updates, direct localhost/IP origins, and URL/filter/history behavior.

## Actual binary and TUI

Reproducers:

```sh
python3 .scratch/service-architecture/verify-native.py
python3 .scratch/service-architecture/verify-tui.py
```

Both launch `./packages/cli/bin/tokeninsights` with an empty child PATH. Lifecycle, refresh, status, stop, and embedded HTML/API work without Node/npm/pnpm or shell helpers.

The PTY check holds the existing writer lock, opens the actual TUI, waits for accepted shared refresh, quits, and verifies that refresh remains active. Releasing the lock completes ingest. A `--no-sync` reopen renders saved usage without creating another job; daemon PID remains unchanged. Manual PTY verification also exercised switching to Sessions and normal quit.

The PTY exercise exposed a cancellation race: a normal quit could return a refresh-observation error. The CLI now treats cancelled local observation as successful quit; a regression test covers it. Initial service test re-execution also required explicit hidden-child dispatch in TestMain to avoid recursively running the test suite. Browser fixtures needed compatibility with omitted optional identity metadata and the new fixed month default.

## Scale measurements

Three sequential samples per endpoint; warm OS caches; native debug/default Go build. Synthetic database contains one Pi fact per session, one provider/model, and no source parsing during measurements. These are current measurements, not before/after comparisons or production latency guarantees.

| Measurement | 10,001 sessions | 100,001 sessions |
| --- | ---: | ---: |
| All-time Sessions API median | 128.01 ms | 2,044.53 ms |
| Facets API median | 24.79 ms | 245.06 ms |
| CLI status median | 7.43 ms | 7.74 ms |
| SQLite file | 5,001,216 bytes | 50,376,704 bytes |
| Daemon RSS after queries | 35,752 KiB | 156,716 KiB |

Warm root/start median was 7.04 ms; initial status median 6.74 ms. Forty status/API reads left daemon file descriptors at 11 before and after. Start/restart/status/analytics produced zero new sync jobs throughout these checks.

Existing Go aggregation materializes all matching session rows before pagination. SQL aggregation/pagination is the main measured follow-up for large histories. This release preserves that code. Any future indexes/schema changes require explicit approval.

Not measured: source bytes read, allocation profiles, multi-harness long-history ingestion, concurrent 100k-session tabs during parsing, OS sleep/resume, or a pre-change baseline. Existing continuity tests remain intact; explicit unchanged refresh can still read historical source bytes. No speedup claim is made for ingest itself.

## Implementation reconciliation

The implementation uses one validated `Action` struct and `Submit`, rather than separate request structs and submission methods for each maintenance command. `Operation` serves as the refresh receipt. Consumer interfaces remain narrow. Failure codes are `failed`, `cancelled`, and `recovery`; recovery maps back to the existing rebuild sentinel for local viewers.

Readiness values are `ready`, `metadata`, `recovery`, `rebuild`, and `unavailable`. The private `/instance` response carries identity/contracts/bind; `/status` carries readiness. Administrative compatibility requires control protocol 1; status/stop remain available for action/schema/generation skew within that protocol. Future protocol compatibility is not claimed.

Configuration replacement is the final fallible initialization step. Readiness-delivery failure is logged and leaves a fully initialized service discoverable. A failed or cancelled parent stops only its own new child and waits for exit before normally releasing admission. Existing config survives earlier initialization failures.

Queue capacity is 16 waiting actions including ordinary refresh; outcomes retain 128 completions; refresh aliases retain 256 entries with oldest-acceptance eviction and ten-minute TTL. Reset confirmation remains a CLI requirement; the private typed reset action expresses local mutation intent. No public reset route exists.

State/runtime also holds a fallback discovery record so local SSH callers can find an owner with a different XDG runtime environment. Long-lived application logs rotate at 4 MiB with three backups. Pending demand remains process-local and is lost on crash.

## Deferred work and questions

Authentication, reboot/login autostart, harness plugins, periodic/watch refresh, IPv6, durable queues, and SQL pagination changes remain out of scope.

Unresolved questions: none.

## Local checks and CI follow-up

Added Husky 9.1.7 pre-push and a project mise config pinning Go 1.27.1, Node 26.4.0, and pnpm 11.6.0. Mise delegates to root pnpm tasks. `check:push` runs format/lint, schema/API contracts, unit/conformance tests, all Go race tests, embedded-asset comparison, native build, and browser E2E without repairing tracked files. `check:ci` runs formatting, schema-copy consistency, and native build; regular/release CI omit the heavy suites and browser/frontend build. Release packaging and dev publication remain intact.

Pinned-tool installation, frozen dependency installation, Chromium setup, formatting/lint, and the minimal CI task passed locally. A real Git dry-run against a temporary bare remote rejected an intentionally unformatted Go file through the installed hook, without publishing refs. The final branch push uses the full hook without bypassing it.
