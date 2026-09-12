# API boundary and multi-source web UI

## Summary

Formalize the existing Go server as a versioned REST API, make OpenAPI authoritative, generate Go and TypeScript/Zod types, and let the browser persist and switch among local/remote TokenInsights servers.

SQLite schema and TUI behavior remain unchanged.

## Implementation

1. Add `docs/openapi.yaml` using OpenAPI 3.0.3.

   - `GET /api/v1/instance`: API/server version, hostname, timezone, capabilities, initial viewer defaults.
   - `GET /api/v1/sync`: current shared sync state.
   - `POST /api/v1/sync`: start or join server sync; return `202`.
   - `GET /api/v1/usage`: existing filtered rows, chart, summary, pagination, last-sync data.
   - `GET /api/v1/usage/facets`: provider/model/harness facets and session search.
   - Typed query parameters, enums, responses, examples, limits, errors.
   - Standard error body: `{ code, message }`.
   - Remove all unversioned endpoints; no aliases.
   - Keep OpenAPI repository-only.

2. Generate contract types.

   - Generate committed Go models with `oapi-codegen`.
   - Generate committed TypeScript types and Zod schemas with Orval.
   - Keep handlers and React Query hooks handwritten.
   - Use generated Zod schemas for runtime validation of remote responses.
   - Add `generate:api` and `check-api` scripts.
   - Fail CI when generated files differ from `docs/openapi.yaml`.
   - Generated TypeScript must pass existing no-`any`/no-assertion rules.
   - Direct Go builds continue using committed output without Node or generation tools.

3. Refactor the Go server boundary.

   - Move API transport models away from database/query structs.
   - Map canonical query results into generated response types.
   - Preserve same-transaction usage rows/chart/summary reads.
   - Preserve request deadlines, lifecycle checks, path-safe errors, shared sync coordination.
   - Add `apiVersion: "v1"`, build version, capabilities to instance metadata.
   - Return JSON for unknown API routes and correct `Allow` headers for unsupported methods.
   - Keep static React assets served by the same Go binary.
   - TUI continues reading SQLite directly.
   - Only `POST /sync` mutates state by invoking existing ingestion.

4. Add permissive API CORS.

   - `Access-Control-Allow-Origin: *`.
   - Allow `GET`, `POST`, `OPTIONS` and required content headers.
   - Return `204` for valid preflight requests.
   - Do not enable credentials.
   - Remove current cross-origin mutation rejection from API routes.

5. Add browser source management.

   - Source record: normalized base URL, hostname, API/server versions, capabilities, cached defaults.
   - Seed page origin as local source.
   - Accept valid HTTP(S) base URLs; normalize bare `host:port` to HTTP.
   - Validate `/api/v1/instance` before saving.
   - Reject unreachable, incompatible, or capability-incomplete sources.
   - Deduplicate normalized URLs.
   - Label by hostname; show URL as disambiguating metadata.
   - Persist sources and active selection in versioned `localStorage`.
   - Keep selected unavailable source active after reload; show recovery UI.
   - Allow removing remote sources; removing active source selects local.
   - Handle unavailable storage with in-memory fallback.

6. Make all browser data source-aware.

   - Pass active base URL into every request.
   - Include source identity in every TanStack Query cache key.
   - Cancel obsolete requests on source changes.
   - Preserve dates, filters, tab, sorting, pagination while switching.
   - Route Sync now, status polling, usage, facets to selected server.
   - Never display another source's cached data under selected hostname.
   - Preserve selected filters when new source yields empty results.

7. Add accessible UI.

   - Header source selector with add/remove management.
   - Keep selector usable during API failures.
   - Inline URL validation and connection errors.
   - Replace misleading LOCAL and Your machine copy with source-aware wording.
   - Follow existing Popover, spacing, responsive, focus, theme contracts.
   - Update `DESIGN.md` for source-selector pattern.

8. Update documentation.

   - Update `docs/design.md`, `README.md`, `packages/cli/README.md`.
   - Document endpoints, OpenAPI/codegen workflow, source persistence, CORS, remote Sync, browser mixed-content/TLS constraints, security exposure.
   - State sources selected individually; data not merged.

## Verification

- Contract generation and drift checks.
- Go route, method, status, error, CORS/preflight, instance metadata, concurrency, query, lifecycle tests.
- Web tests: URL normalization, validation rejection, persistence, switching with unchanged filters, cache isolation, remote Sync, removal, offline selected-source recovery.
- Playwright with two servers/distinct datasets: switching, reload persistence, remote Sync, graceful failure.
- Run `pnpm run format`, `pnpm run check-api`, `pnpm run lint`, focused tests, `pnpm run test`, `pnpm run build`, `pnpm run test:web-e2e`.
- Build from `packages/cli`; verify binary runs without Node/npm/pnpm in `PATH`.
- Manually verify `./packages/cli/bin/tokeninsights`.

## User decisions

- Remote Sync enabled.
- No authentication yet.
- CORS allowed from every origin.
- `/api/v1` only; no compatibility routes.
- Approved five-endpoint API surface.
- Any workable HTTP(S) source URL accepted.
- Sources and active selection persisted.
- Filters unchanged across source switches.
- Automatic hostname labels.
- Invalid/unreachable additions rejected.
- Later failures remain selected with graceful error UI.
- OpenAPI authoritative and repository-only.
- Go and TypeScript types generated.
- Orval pinned to 8.30.0 to satisfy repository dependency-age policy.

## Risks and tradeoffs

- Wildcard CORS plus unauthenticated Sync lets any website reaching server read usage metadata and trigger local ingestion.
- HTTP exposes metadata unencrypted; browser mixed-content/certificate rules may reject sources.
- `localStorage` is scoped to UI origin; another host/port has separate sources.
- Removing unversioned routes intentionally breaks existing clients.
- Same filters across machines aid comparison but can produce empty results.
- Code generation adds development dependencies and committed artifacts.

## Unresolved questions

None.

Critical path: OpenAPI contract -> generated models -> server/web integration. Server and source-manager work may proceed in parallel after generation.

If execution deviates, update this plan to the latest approved design and surface the deviation.
