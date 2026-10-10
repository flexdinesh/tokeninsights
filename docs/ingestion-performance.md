# Ingestion performance baseline

Sections after the SQLite/PostgreSQL baseline contain historical DuckDB measurements; they do not describe current engine performance.

## SQLite/PostgreSQL query baseline — 10 October 2026

Shared `BenchmarkQueries` ingests and processes 600 sessions before timing the same
first-page dashboard contracts. Linux amd64, Intel Core Ultra 7 155U, Go 1.26.8;
PostgreSQL 18 in a local Docker container. Twenty warm iterations, not a production
capacity result or a controlled comparison against the historical benchmarks.

| Dashboard | SQLite ms/op | PostgreSQL ms/op |
| --- | ---: | ---: |
| Sessions | 20.25 | 11.64 |
| Context | 24.82 | 15.69 |
| Tokens | 13.73 | 16.76 |

SQLite caches immutable reporting-zone rules so calendar SQL does not reload IANA
files for every row. Every measured query checks fact count and exact token totals.
Run from `packages/cli` with a live test DSN:

```sh
go test ./internal/adapters/postgres ./internal/adapters/sqlanalytics -run ^ -bench BenchmarkQueries -benchtime=20x
```

The existing capture, acceptance, publication, startup and concurrent-query
benchmarks now exercise SQLite; they remain the workload-specific tools for future
performance changes. Test Docker latency, remote-network latency and large retained
history separately before choosing deployment capacity.
Measured 9–10 October 2026 against production code at `53fee712` (PR #68).
These synthetic measurements identify follow-up work; they are not production
latency targets or evidence of a speedup over the earlier CLI measurement.

## Method

Linux amd64, Intel Core Ultra 7 155U, 14 logical CPUs, Go 1.26.8. Three separate
processes per case, one benchmark iteration per process, run sequentially.
No production source files or databases are opened. Source generation and database
opening/provisioning are outside the timed region. OS filesystem caches are not
flushed. The HTTP case uses the real hosted routes, application SQLite credentials,
dataset scoping and admission over loopback; it does not model TLS, network latency,
multiple tenants or separate-machine resource contention.

The benchmark composes real capture, collector SQLite, delivery, DuckDB and the
production two-worker dispatcher. Test-only backend wrappers count loaded records,
successful publications and stale projections. Delivery wakes the measured worker
after its response, rather than the store's unobserved worker at commit. Use the
existing local startup benchmark and the saved-query benchmark as checks against
the command-owned composition.

Elapsed stages are capture, delivery through durable acknowledgement, remaining
processing visibility lag, and first dashboard response (up to 200 rows). The first delivery
progress callback marks capture completion. `submit-ns/op` is a subset of delivery;
the remainder includes batch preparation, saved-request validation and receipt
persistence. Load-work and publication times accumulate across concurrent workers:
they overlap delivery and must not be added to elapsed stages. Visibility polling
uses the local runtime's 25 ms interval, so tiny timing differences are not precise.

Peak RSS includes fixture/opening overhead and native DuckDB memory. `B/op` measures
Go allocation volume during timed work, not live heap or RSS. CPU profiles are
separate runs. Heap allocation profiles subtract a pre-measurement profile; CPU
profiling begins after setup. Cumulative CPU percentages overlap down call stacks.

## Results

Each Pi case contains 10,000 messages, input 100/output 20 per message. Session
headers add evidence records: 10,001 / 10,050 / 10,500 accepted entries respectively.
All cases verify 1,200,000 tokens and expected session counts; the benchmark also
checks every token component. The native fixture covers all four harnesses and
retains its independent 12-fact, 1,102-token component oracle.

| Shape | Direct elapsed, median (range) | Hosted HTTP, median (range) | Direct peak RSS, median (range) | Direct Go allocations |
| --- | --- | --- | --- | --- |
| 50 × 200 messages | 7.16 s (7.04–7.25) | 7.36 s (7.31–7.86) | 432 MiB (416–435) | 2.45 GiB |
| 1 × 10,000 messages | 9.13 s (8.97–9.52) | 9.27 s (9.22–9.34) | 437 MiB (412–462) | 4.57 GiB |
| 500 × 20 messages | 12.26 s (12.22–12.27) | 12.63 s (12.58–12.67) | 358 MiB (353–361) | 2.38 GiB |
| Native conformance fixture | 164 ms (161–165) | 165 ms (164–167) | 195 MiB (182–200) | 9.4 MiB |

Direct elapsed stage medians:

| Shape | Capture | Delivery | Remaining visibility lag | First query |
| --- | --- | --- | --- | --- |
| 50 sessions | 743 ms | 6.32 s | 78 ms | 22 ms |
| 1 session | 1.16 s | 5.60 s | 2.40 s | 34 ms |
| 500 sessions | 1.02 s | 5.58 s | 5.60 s | 34 ms |

With the same 50-session / 10k-message history already open, unchanged collection
through a dashboard query takes 38 ms direct (38–48 ms) and 48 ms hosted HTTP
(47–50 ms). Appending one message takes 137 ms direct (137–143 ms) and 164 ms
HTTP (138–174 ms), including visibility and the query. It loads the affected
202-record session component once; unchanged runs submit/process no records.
These timings exclude process startup and opening storage.

The command-owned local runtime also preserved saved queries while growing 50
sessions from one to 200 messages each. Across three runs, 37–39 dashboard reads
per run averaged 31–32 ms, with the slowest at 45 ms. Queries retained all saved
sessions, returned nondecreasing totals and ended at exactly 1,200,000 tokens with
no pending processing. The query loop polls every 200 ms; its 7.39–7.76 s ingestion
duration includes that polling granularity. This is a read-responsiveness check,
not a transport or large-history query benchmark.

Existing stage benchmarks put standalone 256-record acceptance at 52 ms,
publication at 58 ms, new collector writes at 13 ms and current batch encoding at
5 ms (medians). These isolated fixtures exclude competing workers and use their
own record shapes; their times cannot be summed to predict end-to-end latency.
The small command-owned startup fixture opens storage in roughly 33–38 ms on
fresh-ingest runs, supporting the focus on ingestion rather than opening.

The earlier approximately 13 s / 374 MiB result measured a CLI subprocess using a
different Go toolchain and lifetime. It did not separate acceptance from visibility.
It is not a directly comparable before/after baseline.

## Bottlenecks and proposed order

1. **Reduce per-component database work.** With 500 sessions, every evidence record
   loads once, yet 500 publications leave another 5.6 s after acceptance. The CPU
   profile attributes 34% cumulatively to publication and 15% to work selection.
   `ReadMetadataForDataset` alone accounts for 11%: it issues four SQL statements
   on each call. The first experiment below consolidates those reads within the
   caller's snapshot while preserving every role, generation and processor-version
   check. Next measure bounded publication statements and their preparation/execution
   costs. Preserve component atomicity and the existing memory/admission limits.

2. **Give acceptance validation one owner.** In the 50-session profile,
   `evidence.DecodeBatch` accounts for 16% of CPU samples and 45% of Go allocation
   volume. Both delivery adapters fully decode/validate the request before
   `Store.Accept` does so again; collector request preparation also validates its
   own boundary. Remove redundant work inside acceptance, preserving independent
   collector/receiver trust boundaries, exact request bytes, error contracts,
   body limits, duplicate-key/private-field rejection and numeric precision.
   Measure allocations and latency independently; smaller allocation volume is
   not automatically lower peak native memory.

3. **Bound repeated work for actively arriving components.** One large session
   causes 99,601–108,305 records to load for 10,001 accepted entries, with 18–19 stale
   projections. Only 6–7 publications succeed. Investigate revision-aware work
   coalescing after the cheaper changes above. Any scheduling change needs tests
   for continuous arrivals, late ancestry, dataset fairness, bounded visibility
   delay, restart and cancellation. Waiting for global ingestion to stop would
   violate the hosted lifecycle and is not an acceptable optimization.

The 50-session block profile records about 1.95 aggregate seconds waiting in
acceptance and 2.49 in publication at the shared writer. These overlapping waits
are evidence of contention, not extra wall time. Increasing worker counts would
not address that shared bottleneck. Loopback HTTP overhead is small in this matrix;
capture and final queries are also secondary to delivery/publication here.

Make one production change per experiment. Compare all three shapes plus append,
unchanged sync and queries during ingestion. Retain gains only when repeated
samples exceed noise and the existing accounting, retry, isolation, generation,
privacy and shutdown contracts pass. No schema change is proposed.

## First optimization: metadata reads

`ReadMetadataForDataset` now uses one dataset-scoped aggregate query instead of
four statements. It retains role/version/identity/revision checks, exact active
and target generation states, the single-active-generation invariant and the
newest processor version across **all** generations, including retained ones.
Missing generations still reject. No metadata or validation result is cached.

Real-store contract tests cover invalid/missing generations, newer retained
processors, dataset isolation and a caller's read snapshot across replacement.
The new tests pass against both the original four-statement implementation and
the replacement; existing cutover/reprocess tests cover valid building states.

Isolated metadata reads, 100 iterations per sample and three samples:

| Stored generations | Before median (range) | After median (range) | Allocations per read, median |
| --- | --- | --- | --- |
| 1 | 1.441 ms (1.426–1.480) | 1.234 ms (1.165–1.259) | 340 → 195 |
| 1,000 | 1.488 ms (1.471–1.563) | 1.403 ms (1.361–1.467) | 339 → 195 |

A separate paired end-to-end comparison alternated before/after binaries, three
fresh-process samples per case. Samples taken during another worktree's heavy
verification were discarded before restarting this comparison.

| Shape, direct delivery | Before median (range) | After median (range) |
| --- | --- | --- |
| 50 sessions | 7.60 s (7.33–7.60) | 7.48 s (7.44–8.37) |
| 1 session | 9.67 s (9.65–9.73) | 9.08 s (8.96–9.46) |
| 500 sessions | 12.49 s (12.45–12.50) | 12.35 s (12.31–12.49) |

Retain this as a reduction in metadata-read cost, not a broad ingestion speedup.
The 50-session ranges overlap and the many-session improvement is small. The
single-session difference also reflects schedule-sensitive repeated processing;
the underlying component reprocessing problem remains. Peak RSS varies between
runs, so no memory-capacity improvement is claimed. Publication work and redundant
validation remain the next measured targets.

Unchanged sync remained about 49 ms in the paired comparison. Append and saved-query
checks preserved their accounting and snapshot invariants after the change. Later
append/query timings encountered renewed host activity and are excluded from
speedup claims; post-change queries averaged 33–42 ms with a 56 ms maximum in those
runs. Recheck on an otherwise idle host before setting a latency regression budget.

## Second optimization: provenance publication

The next experiment starts from `609448a` (PR #71 plus TUI progress PR #70),
using the same machine and Go 1.26.8. `BenchmarkPublication` measures the real
`PublishProjection` transaction for 10,000 facts in 1, 50 or 500 independent
components. Opening, acceptance, work loading and interpretation are untimed.
Initial cases have no published rows. Replacement cases first publish one seed
fact per component, then accept the remaining messages and replace each component
within the current generation. They do not benchmark generation rebuilding.

Each workload checks exact projected fact IDs and provenance edges, every token
component, processed outcomes, session counts, no pending scopes and published
revision advancement. The synthetic usage is 10 input, 5 output, 2 reasoning,
3 cache-read and 4 cache-write tokens per fact: 240,000 total. `ns/op` covers all
component publications; `ns/component` reports their mean. Timer pauses exclude
setup, interpretation and verification; whole-process RSS still includes them.

Temporary source overlays split publication into preparation, writer acquisition,
transaction opening, metadata reads, dependency fencing, row writes, metadata
publication and commit. Row-write instrumentation splits explicit prepare from
execution using the pinned driver's existing prepare/execute/close lifecycle.
These diagnostic timings include instrumentation and are separate from the
uninstrumented comparisons; no profiling hooks enter production.

For 500 initial components, an instrumented baseline took 7.12 s: row writes
4.96 s, metadata reads 0.81 s, commits 0.63 s, fences 0.44 s, metadata writes
0.20 s, and preparation/opening about 0.05 s each. Writer waiting was negligible
in this isolated benchmark. Within row writes, provenance conflict-ignore
execution took 1.02 s and outcomes upsert execution 1.17 s. A following uninstrumented
baseline control took 7.24 s; instrumentation cost is not distinguishable from
run variation in that pair. The instrumented candidate took 6.57 s, including
0.19 s for provenance insertion. Use the repeated uninstrumented samples below
for speedup claims. SQL execution timings
include native execution and any planning/rebinding performed there; they are
not pure row-copy timings.

Publication already deletes the component's old provenance inside its transaction.
The change deduplicates each fact's evidence IDs during preparation and uses a
plain provenance insert. This preserves the previous set semantics without asking
DuckDB to ignore duplicate conflicts on every insertion. Fact uniqueness,
dataset/generation filters, whole-component fencing, transaction boundaries and
the 128-row statement limit remain unchanged. The shared acceptance writer is
unchanged. An unused projection argument was removed from `publishRows`.

A real-store contract test covers duplicate edges beyond one SQL batch and repeat
publication; it passes against both old and new implementations. Existing tests
retain rollback/reopen/retry, cancellation, late-connected component rejection,
dataset isolation and generation cutover coverage.

Comparisons alternate baseline/candidate order across three fresh-process samples
per case. The first attempted comparison overlapped another worktree's pre-push
suite and was archived and excluded in full. The restarted runner checks for
competing verification jobs during each sample. Performance assertions remain
outside contract tests. A later overlapping test job interrupted an ingestion
native-fixture pair; that pair was archived and repeated, retaining the completed
uncontended pairs.

Isolated publication, median (range), three samples per version:

| Components | Publication | Before | After |
| --- | --- | --- | --- |
| 1 | Initial | 2.00 s (1.96–2.05) | 1.84 s (1.82–1.86) |
| 1 | Replacement | 2.04 s (1.97–2.06) | 1.81 s (1.80–1.93) |
| 50 | Initial | 2.60 s (2.57–2.62) | 2.35 s (2.35–2.38) |
| 50 | Replacement | 2.62 s (2.57–2.64) | 2.45 s (2.39–2.45) |
| 500 | Initial | 7.09 s (7.06–7.14) | 6.29 s (6.27–6.37) |
| 500 | Replacement | 7.95 s (7.76–7.96) | 6.99 s (6.93–7.09) |

The isolated medians improve about 6–12%, with separated sample ranges in each case.
For 500 components this is about 14.18 → 12.58 ms per initial publication and
15.91 → 13.97 ms per replacement. Go allocation volume is effectively unchanged
(about 159–164 MiB per full workload). Process RSS varies and includes setup;
these measurements do not establish a new memory-capacity limit.

End-to-end elapsed time, median (range), three samples per version:

| Shape | Delivery | Before | After |
| --- | --- | --- | --- |
| 1 session | Direct | 9.22 s (9.15–11.32) | 9.27 s (9.19–9.40) |
| 1 session | Hosted HTTP | 9.37 s (9.19–10.91) | 9.44 s (9.38–9.53) |
| 50 sessions | Direct | 7.95 s (7.82–8.01) | 7.59 s (7.55–9.22) |
| 50 sessions | Hosted HTTP | 7.86 s (7.77–8.14) | 7.61 s (7.42–9.30) |
| 500 sessions | Direct | 12.78 s (12.39–13.46) | 11.88 s (11.70–12.03) |
| 500 sessions | Hosted HTTP | 12.86 s (12.56–13.24) | 11.81 s (11.76–11.91) |

The clearest end-to-end gain is 500 sessions: about 7% direct and 8% hosted HTTP,
with separated ranges. Every record loads once and exactly 500 publications
complete with zero stale projections. Direct remaining visibility lag falls from
5.73 to 4.93 s; HTTP falls from 5.95 to 4.98 s. Total Go allocation volume remains
about 2.38–2.39 GiB. Median process RSS is 352 → 341 MiB direct and 343 → 334 MiB
HTTP; sample ranges overlap.

The 50-session ranges overlap, and each candidate transport has one slower sample;
these are retained, not excluded. One session has no clear elapsed improvement.
Direct single-session processing loads 101,137–102,673 records before versus
102,673–108,817 after, with 6 publications before and 6–7 after. That scheduling
variation raises median Go allocation volume from 4.88 to 5.06 GB (decimal), while
median RSS remains about 407 MiB. This optimization does not solve repeated work
under continuing arrivals or establish a universal ingestion speedup.

Unchanged sync remains about 52 ms direct and 54 → 57 ms HTTP, with no submission
or publication. HTTP append is 184 → 146 ms median, with overlapping ranges.
Direct append and concurrent saved queries received three additional paired
samples because the initial timings/RSS were variable. Across six samples,
direct append is 149 ms (140–251) before and 177 ms (137–249) after. Four pairs
improve and two regress; slower candidate runs also spend more time in unchanged
capture/load/query stages. No consistent incremental latency improvement or
regression is established; the 25 ms visibility polling interval also limits
precision. Every append loads exactly 202 records and publishes once.

Across six saved-query samples, median per-run mean query latency is 33.5 → 33.9 ms;
the largest observed response is 64.7 → 63.5 ms. All reads retain saved sessions,
nondecreasing totals and exact final accounting. Process RSS is 432 MiB median
(417–448) before and 452 MiB (405–478) after. This higher median and overlapping
ranges are retained as a measurement limitation/tradeoff, not a memory improvement.
Worker/admission limits and the DuckDB memory budget are unchanged. Native
four-harness fixture oracles pass in both modes; no native-fixture speedup is claimed.

Retain the change for the reproducible publication and many-component ingestion
gains. Acceptance validation remains the next focused optimization; scheduling
still needs a separate design backed by measurements.

## Third optimization: one receiver validation owner

Starting from `f6c6b15` (PR #73), `datastore.Store.Accept` now strictly decodes the
request once and checks the requested protocol before preparing or writing data.
The direct and HTTP adapters pass exact bytes and map typed validation errors;
they retain admission/body bounds, and HTTP retains content-type/body-read checks.
Collector preparation still validates its independent trust boundary. Neither
adapter can pass a decoded batch that bypasses receiver validation.

The Go receiver contract now takes the requested protocol explicitly. This keeps
malformed-body versus requested-version rejection ordering intact without decoding
twice. Direct/HTTP rejection contracts check stage/code/status, duplicate keys,
private fields, invalid UTF-8, invalid envelope numbers, versions, body limits,
admission-before-decoding and rejection without mutation. Exact retry bytes,
receipt hashes and native integer precision above 2^53 are preserved. The same
new external-boundary tests pass against the original implementation.

`BenchmarkReceiverAcceptance` isolates a 256-entry batch through the direct
adapter or real loopback HTTP handler, with workers inactive. New acceptance and
exact replay are separate cases; opening, fixture construction and replay warm-up
are untimed. Each result verifies accepted/pending counts, the exact request hash,
stable replay receipt and durable evidence/batch counts. HTTP authentication and
hosted admission remain covered by the full ingestion matrix, not this isolated
handler benchmark.

Same machine and Go 1.26.8, three alternating fresh-process samples per version,
ten iterations per isolated sample. Median (range):

| Adapter | Operation | Before | After | Go allocation volume per operation |
| --- | --- | --- | --- | --- |
| Direct | New | 65.91 ms (64.95–66.89) | 52.71 ms (50.77–53.63) | 18.94 → 11.52 MB |
| Direct | Replay | 34.81 ms (34.20–34.89) | 23.11 ms (22.78–23.93) | 17.54 → 10.11 MB |
| HTTP | New | 66.06 ms (65.77–66.71) | 52.74 ms (52.53–52.92) | 19.32 → 11.91 MB |
| HTTP | Replay | 37.23 ms (37.00–37.76) | 24.34 ms (23.70–24.68) | 17.90 → 10.48 MB |

New acceptance improves about 20%; replay improves 33–35%. Go allocation volume
(decimal MB) falls 38–42%. Whole-process RSS, which includes opening and warm-up,
does not show a consistent reduction; smaller allocation volume is not a native
memory-capacity claim. No storage schema or wire contract changes.

The full ingestion matrix uses three alternating fresh-process samples per version,
one iteration each, retaining exact token-component totals, session counts and
processing completion checks. No competing verification was detected in these samples.
Elapsed medians (ranges), including capture, delivery, visibility and first query:

| Shape | Adapter | Before | After |
| --- | --- | --- | --- |
| 50 sessions | Direct | 7.35 s (7.35–7.44) | 7.29 s (7.18–7.33) |
| 50 sessions | Hosted HTTP | 7.57 s (7.43–7.57) | 7.23 s (7.21–7.27) |
| 1 session | Direct | 9.14 s (8.48–9.31) | 8.16 s (8.09–8.20) |
| 1 session | Hosted HTTP | 9.21 s (9.20–9.48) | 8.17 s (8.14–8.87) |
| 500 sessions | Direct | 11.46 s (11.35–11.47) | 11.33 s (11.28–11.34) |
| 500 sessions | Hosted HTTP | 11.83 s (11.78–11.91) | 11.66 s (11.57–11.83) |

The 50/500-session cases allocate about 11% fewer Go bytes overall. Their elapsed
gains are much smaller than isolated acceptance: publication still dominates.
The single-session result also reflects changed scheduling: the candidate still
loads 90,897–99,089 records for 10,001 accepted entries, with 18–19 stale projections.
This change does not solve repeated processing of actively arriving components.

Native-fixture elapsed medians are 163 → 138 ms direct and 143 → 162 ms HTTP;
HTTP ranges overlap (139–144 versus 139–165 ms). The small fixture is sensitive to
the benchmark's 25 ms visibility polling; no uniform end-to-end speedup is claimed.
Both versions retain the independent native token oracle and seven publications.

Three additional paired samples cover unchanged sync and appending one message
to the warmed 50-session history. Unchanged sync submits/processes nothing;
append accepts one entry and publishes its 202-record component once in both
versions. Unchanged medians are 66 → 70 ms direct and 70 → 51 ms HTTP. Append
medians rise from 148 → 181 ms direct (ranges 143–173 versus 151–182 ms) and
150 → 183 ms HTTP (146–254 versus 147–203 ms). These overlapping, polling-sensitive
samples establish correct incremental behavior, not an incremental latency gain.

Saved-query runs preserve all sessions, nondecreasing totals and the exact final
total while ingesting. Per-run mean query times are 32–35 ms before and 30–36 ms
after; the slowest query is 50 versus 53 ms. Candidate runs perform 37–42 reads.
Their peak RSS median rises from 444 to 469 MiB despite lower Go allocation volume;
the matrix supports no general peak-memory reduction claim.

A separate scoped 50-session/direct CPU profile attributes 2.36 CPU-seconds
(16.48% cumulatively) to `DecodeBatch` before and 1.79 (12.85%) after, including
the independent collector boundary. Total sampled CPU is 14.32 versus 13.93 seconds.
These are one diagnostic run per version, separate from latency samples;
cumulative percentages overlap and do not establish a total-CPU speedup budget.

## Fourth optimization: delay stale component retries

Starting from `ec925c4` (PR #74), the concurrent dispatcher now preserves the
publication outcome and waits 250 ms before retrying a stale component. It passes
payload-free component exclusions to existing storage selection, which resolves
the current dependency graph before loading JSON. Independent work keeps its
dataset rotation, two-worker limit and byte admission. First attempts and work
following successful publication remain eager; serial maintenance is unchanged.

Wakeups cannot extend a deadline, and a dedicated timer makes expired hints
eligible without waiting for the one-second failure poll. Tracking is bounded to
128 components and 4,096 scope bindings, with immediate eligibility at capacity.
It retains no raw records and has no durable state. Restart drops only scheduling
hints; stale work does not become a recorded failure. Generation, membership and
revision fences remain authoritative. This bounds the deliberate scheduling wait,
not publication time for a component changing faster than it can be processed.

Deterministic virtual-time tests cover immediate first attempts, recurring retries
under continuous wakeups, idle deadline wakeup, independent dataset progress,
oversized delayed work releasing admission, successful publication and restart.
Real-store tests cover payload-free exclusions, late ancestors, alternate roots,
dataset isolation, unchanged durable backoff and eventual exact accounting.
Existing generation, failure, cancellation, recovery and pure accounting contracts
remain in place.

`BenchmarkPacedProcessing` uses real direct delivery and the shared two-worker
dispatcher against hosted DuckDB datasets. Forty 256-message batches grow one
session on a 100 ms ticker; a second dataset receives one message after batch 11.
The ticker can drop ticks under load. Opening, fixture construction and provisioning
are untimed; capture and HTTP are absent from this fixture and remain covered by
the ingestion matrix. Every run checks receipt hashes/counts/dataset binding,
all token components (1,228,800 hot tokens and 120 independent tokens), one hot
session, no pending work, and independent publication before submission finishes.
It reports first publication and independent-dataset latency separately from final
visibility. This finite stream cannot prove publication under endless changes.

The delay trial used Go 1.26.8 on the same machine, three fresh-process samples
per policy/workload, reversing policy order on the second round. Medians:

| Retry delay | Single-session loaded records | Single-session elapsed | Paced loaded records | Paced elapsed | Paced final visibility lag |
| --- | --- | --- | --- | --- | --- |
| None | 99,089 | 8.11 s | 104,193 | 6.45 s | 2.35 s |
| 100 ms | 64,529 | 7.62 s | 75,265 | 6.50 s | 2.30 s |
| 250 ms | 53,265 | 7.93 s | 50,689 | 6.11 s | 2.12 s |
| 500 ms | 37,649 | 7.67 s | 39,681 | 6.49 s | 2.51 s |

Select 250 ms for its balance of repeated work and added scheduling delay. The
500 ms policy saves more work but increases paced visibility lag. The 250 ms
single-session elapsed ranges overlap baseline (7.33–8.26 versus 7.83–8.12 s),
so this trial does not establish a universal ingestion speedup. Paced elapsed
ranges are 6.04–6.23 versus 6.45–6.46 s. Trial Go allocation medians fall from
4.51 to 3.34 GB for one session and 3.26 to 1.89 GB for paced arrivals; these are
decimal allocation volumes, not live heap. Paced peak RSS rises from 383 to 395 MiB
at the median, so no peak-memory improvement is claimed.

The selected policy then ran through a separate paired matrix: three alternating
fresh-process samples per version, one iteration each. No competing verification
was detected. Elapsed medians (ranges) include visibility and the final query:

| Shape | Adapter | Before | After |
| --- | --- | --- | --- |
| 1 session | Direct | 8.14 s (8.04–8.47) | 7.92 s (7.68–7.95) |
| 1 session | Hosted HTTP | 8.32 s (8.14–8.37) | 8.02 s (7.86–8.36) |
| 50 sessions | Direct | 7.08 s (7.08–7.35) | 6.80 s (6.71–6.89) |
| 50 sessions | Hosted HTTP | 7.24 s (7.22–7.26) | 7.06 s (6.62–7.14) |
| 500 sessions | Direct | 11.13 s (11.05–11.29) | 11.15 s (11.09–11.16) |
| 500 sessions | Hosted HTTP | 11.61 s (11.54–11.63) | 11.46 s (11.43–11.59) |
| Paced arrivals | Direct, two datasets | 6.42 s (6.28–6.48) | 6.26 s (6.25–6.29) |

The growing session loads 92,945 → 54,545 records direct and 99,089 → 52,753
over HTTP at the median; stale publications fall from 18/19 to eight. Go allocation
volume falls 23–26%. Paced loading falls 103,937 → 51,201, stale publications
21 → nine, and Go allocations 3.25 → 1.91 GB. Paced first publication stays at
115 ms median; the independent dataset publishes in 30 → 28 ms (27–49 versus
23–34 ms), while submissions are still arriving. Its final visibility lag is
2.33 → 2.26 s. Peak RSS varies across shapes, with some increases; no general
memory-capacity improvement follows from the allocation savings.

The deliberate delay has observable costs. For 50 sessions, direct delivery falls
6.25 → 5.66 s but remaining visibility lag rises 80 → 380 ms; HTTP lag rises
79 → 356 ms. Successful publications fall from 79 to 68, while stale attempts
increase from ten to 17/20. Total elapsed still improves, but staleness does not
uniformly decrease across shapes. Single-session direct lag also rises 2.38 →
2.63 s despite its lower total elapsed. Retain this as a substantial reduction
in repeated loading for growing components, with modest workload-dependent latency
gains and an explicit scheduling tradeoff.

Append still loads its 202-record component once: median elapsed is 147 → 145 ms
direct and 139 → 143 ms HTTP, with overlapping ranges. Unchanged sync submits and
processes nothing (49 → 45 ms direct, 45 → 46 ms HTTP). Saved-query means stay
roughly 32–33 ms, with maximum query latency 45 → 42 ms. Every query retains saved
sessions and nondecreasing totals; final totals remain exact.

The small native fixture takes 137 → 158 ms direct and 136 → 152 ms HTTP in
the one-iteration matrix. Both versions perform seven successful publications and
zero stale retries, so the 250 ms delay never activates. A longer control used
three alternating fresh-process samples of ten iterations each: direct medians
are 146 → 143 ms (ranges 139–148 versus 138–149 ms), HTTP 145 → 140 ms
(143–147 versus 140–152 ms). This does not reproduce a consistent slowdown;
the small fixture's 25 ms visibility polling makes single-iteration differences
too coarse to attribute to scheduler overhead. Both sets of results are retained.

## Reproduction

From the repository root, install the pinned development dependencies first.
Keep logs, profiles and compiled benchmark binaries in ignored `.scratch/`.
Use one toolchain for every compared sample; the recorded run used Go 1.26.8.

```sh
cd packages/cli
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkIngestion$' -benchtime=1x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkIngestionSavedQueries$' -benchtime=1x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkReceiverAcceptance$' -benchtime=10x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkPacedProcessing$' -benchtime=1x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkLocal(Startup|SavedQueries)$' -benchtime=3x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/datastore -run '^$' \
  -bench '^Benchmark(Acceptance|Publication)256$' -benchtime=5x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/datastore -run '^$' \
  -bench '^BenchmarkPublication$' -benchtime=1x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/datastore -run '^$' \
  -bench '^BenchmarkMetadataRead$' -benchtime=100x -count=3
env GOTOOLCHAIN=go1.26.8 go test ./internal/rawcollectorstore -run '^$' \
  -bench '^Benchmark(CaptureRecords|RawBatchEncoding)$' -benchtime=5x -count=3
```

For independent process/RSS samples, compile with `go test -c`, then execute the
binary once per selected case from `packages/cli/internal/collector` so native
fixture paths resolve. Use `-test.run`, `-test.bench`, `-test.benchtime=1x` and
`-test.count=1`; collect child-process peak RSS with an OS resource monitor.
Incremental cases warm a 10k-message store before timing, so their process peak RSS
includes warm-up and must not be described as incremental memory demand.

Profile separately from timing runs:

```sh
env GOTOOLCHAIN=go1.26.8 go test ./internal/collector -run '^$' \
  -bench '^BenchmarkIngestion$/^Sessions50$/^Direct$' -benchtime=1x -count=1 \
  -args -ingestion-profile-dir="$PWD/../../.scratch/ingestion-profile"
go tool pprof -top ../../.scratch/ingestion-profile/BenchmarkIngestion-Sessions50-Direct/cpu.pprof
go tool pprof -top -sample_index=alloc_space \
  -base ../../.scratch/ingestion-profile/BenchmarkIngestion-Sessions50-Direct/heap-before.pprof \
  ../../.scratch/ingestion-profile/BenchmarkIngestion-Sessions50-Direct/heap-after.pprof
```

The same before/after subtraction applies to block and mutex profiles. Profile
only one iteration/count per destination directory. Built-in `-cpuprofile` wraps
the whole test process, including fixture creation and warm-up; use the scoped
profile flag for this investigation. No timing thresholds belong in contract tests.

Unresolved questions: none. Further production changes remain separate experiments,
with performance targets chosen from their measured effect.
