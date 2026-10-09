# Ingestion performance baseline

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
