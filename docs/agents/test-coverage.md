# Test cleanup and retained coverage

Remove a test when its assertions repeat an existing behavioral contract or only
freeze implementation spelling. Keep independent arithmetic, calendar, privacy,
security, recovery and boundary oracles even when feature tests exercise the same
code. Similar test names alone do not establish duplication.

## Removed assertions

| Location | Rationale | Retained coverage |
| --- | --- | --- |
| `tools/build/test/products-contract.test.ts` | Repeated release matrix counts, installer flag spelling, retired artifact absence and deployment source expressions. Source expressions cannot prove service isolation or engine ownership. | `release-workflow.test.ts` checks channel assets, workflow matrices and publication behavior; `build-desktop.test.ts` checks packaged layouts; `install-script.test.ts` runs installers. Real personal-network, SSH and validation-runner tests own deployment and engine behavior. |
| `tools/build/test/pre-push.test.ts` | Executed an inert hook containing only `exit 0`; this adds no feature or publication correctness evidence. | Explicit validation routing and committed-HEAD verification remain the publication boundary. |
| Naming assertions in `tools/build/test/build-desktop.test.ts` | Repeated archive name examples already consumed by release verification and installer fixtures. | Target rejection, executable/backend layouts, desktop identity, actual archive contents, source metadata and checksums remain. |
| Workflow spelling assertions in `tools/build/test/release-workflow.test.ts` | Matched `needs`/`version` text and searched for retired publishing terms without exercising publication. | Source-tag identity/immutability, workflow platform parity, generated-artifact rejection, complete checksummed assets, draft/upload/publish ordering and upload-failure behavior remain. |
| Literal mock fields in `packages/web/src/mocks/data.test.ts` | Repeated a hostname string, harness count and revision argument from fixture construction. | Every mock dashboard view still validates against generated contracts and contains rows; mock pagination still clamps. Response identity, revision/readiness and capability validation remain in `contracts.test.ts`; UI tests consume mock bootstrap, facets and status. |

## UI coverage retained

No UI suite was deleted. Pure query, date-range and formatting tests supply
independent oracles for URL state, invalid inputs, DST, fixed offsets, leap years,
calendar alignment and navigation. Browser tests exercise selected interactions;
they do not replace those edge cases.

Schema identity validation in `contracts.test.ts` and hook snapshot binding in
`api_identity.test.tsx` cover different boundaries: accepted response shape versus
whether a response belongs to the active backend/dataset/generation/revision.
Keep both. Likewise retain HTTP errors and same-origin Refresh request shape,
injected-client behavior, cancellation, stale-result rejection, saved-data recovery,
excluded-usage separation, focus and draft preservation, and component accessibility.
The retired-route cases also assert that invalid navigation performs no usage read
or capture; the browser connection cases do not duplicate that oracle.

Keep design-token and dependency guards, including palette isolation, defined CSS
properties, token scales, UI dependency ownership and development-only catalog
loading. These enforce architecture and design contracts. Keep class-merging,
chart category uniqueness and filtered-total percentage/rounding tests.

## Test environments

`packages/web/vite.config.ts` defines two Vitest projects sharing the application
configuration. Pure `.test.ts` files in web and dashboard use Node. DOM `.test.tsx`
files use JSDOM and the existing component setup, including DOM matchers and
`matchMedia`. The include patterns are disjoint; Node tests do not load component
setup. Put future DOM tests in `.test.tsx`, or explicitly configure their environment
and setup if another extension is necessary.

## Other retained boundaries

Installer security, archive safety, ownership refusal and failed-publication
rollback remain intact. API/schema generation checks retain drift rejection without
repairing files. Toolchain pins, asset digest/copy drift, safe fixture allowlists,
development lifecycle cleanup and executable validation routing remain covered.
Go accounting, storage/replay/rollback, transport and lifecycle contracts are outside
this cleanup and remain unchanged. Performance benchmarks remain opt-in.

Benchmark assertions retain independent mandatory coverage: native components and
stable identities in processor and collector golden replay/rebuild tests; exact
receipts, pending restart, generation replacement, query totals/pagination and
consistent snapshots in the real token-storage contract suite; publication identity,
provenance and rollback in datastore recovery tests; independent-session progress
and cancellation in dataengine/datastore concurrency tests. Moving benchmark runs
out of the default selection does not remove those correctness contracts.
