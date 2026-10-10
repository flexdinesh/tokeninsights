# Storage and ingestion performance

SQLite is the in-process engine. Hosted deployments select SQLite or PostgreSQL 18.
This document describes the current engines only; Git history retains superseded
measurements. It does not define latency SLOs or production capacity.

## Reproduce

Run from the repository root, with the pinned development tools:

```sh
pnpm run bench:smoke
mkdir -p .scratch/storage-tmp
env TMPDIR="$PWD/.scratch/storage-tmp" pnpm run bench:storage
```

Both commands use the same disposable PostgreSQL runner as tests. Supply
`TOKENINSIGHTS_TEST_POSTGRES_DSN` with CREATEDB privileges or allow the runner to
start its pinned PostgreSQL 18 container. Choose a disk-backed TMPDIR for storage
measurements: the OS default may be tmpfs. The example uses ignored repository scratch
storage; confirm its filesystem on your machine. Every fixture creates and drops only its
own random database. All inputs are synthetic; user databases are never opened.

Smoke executes every benchmark family once. Large ingestion/publication fixtures
use smaller input counts, preserving their component, receipt, provenance and
fairness assertions; the native four-harness fixture stays unchanged. The shared
storage smoke case has two datasets, two sessions each, ten messages/session and
three generations. The paced smoke stream retains forty spaced batches and its
independent-dataset assertion, with sixteen messages/batch. Ordinary tests do not
execute benchmark bodies. Pre-push and live PostgreSQL CI run smoke explicitly,
without timing gates.

`bench:storage` runs the full matrix below, with three iterations and three
repetitions, one Go package at a time (`-p=1`). Do not run other verification or
benchmarks concurrently when collecting a baseline. Smoke timings are not samples
of the full workloads.

## Shared workload and contracts

| Shape | Datasets | Sessions/dataset | Facts/dataset | Generations |
| --- | ---: | ---: | ---: | ---: |
| History1K | 1 | 10 | 1,000 | 1 |
| History10K | 1 | 100 | 10,000 | 1 |
| History100K | 1 | 1,000 | 100,000 | 1 |
| Tenants10 | 10 | 100 | 10,000 | 1 |
| Retained3 | 1 | 100 | 10,000 | 3 |

Pi metadata spans ninety days and four models. Each native message contributes
input 10, output 5, reasoning 2, cache-read 3, cache-write 4, total 24.
All datasets deliberately reuse native identities to exercise isolation.
Seeding goes through real acceptance, interpretation and publication; retained
generations are built through reprocessing. No direct fact inserts or synthetic
analytics repository bypass the storage contract.

SQLite and PostgreSQL share the workload in `storagecontract` and the same
adapter factories as token contract tests. Physical opening and footprint probes
stay in adapter fixtures. No production interface, schema or mode policy changes.

Each shape measures:

- `Query-sessions|context|tokens`: first-page dashboard reads (page size 50),
  including all-history summaries, over one authorized dataset. Setup is untimed; semantic checks are
  included. This does not measure TUI complete-result reads or selective filters.
- `Reopen`: close and reopen existing storage, including physical validation and
  ownership reacquisition. The subsequent accounting/generation check is untimed.
  The PostgreSQL owner validates its paired token/account schemas; the SQLite
  fixture opens token storage only. Account operations are not benchmarked.
  This is not cold OS-cache latency, fresh PostgreSQL startup or full CLI startup.
- `Append`: accept one additional message in an existing session, process it
  serially and observe the final dashboard. Payload creation is untimed.
  `accept-ns/op` ends at durable acceptance; `visible-ns/op` starts there and ends
  after processing and the successful dashboard query.
- `QueriesDuringProcessing`: accept a 256-message burst across existing sessions
  while the production two-worker dispatcher runs, then query continuously until
  that burst is visible. Every snapshot must have nondecreasing fact count and
  exact component totals; final receipts must be unchanged and terminal.
  Sampled query p50/p95, query count, acceptance and visibility lag are reported.

Mutating samples append unique evidence: history grows across iterations and
repetitions. The shape names describe seeded history, not a stationary population
throughout the stream. Samples must use the same iteration/repetition counts and
operation selection for comparisons. Query percentiles describe only reads sampled
during these short bursts; they are not production tail-latency estimates.

After activity, the independent dataset must still match its original component
oracle. Reopen preserves generation, dataset identity and totals. Existing native
harness/deployment contracts remain responsible for richer source semantics,
replay after collector loss, authentication and process restart.

## Resource accounting

`B/op` and `allocs/op` report Go allocations in the timed operation, including
assertions and query-sample bookkeeping. They are allocation volume, not live heap
or peak RSS. PostgreSQL server memory is outside the Go process.

`store-bytes` is adapter-specific: SQLite token file plus WAL/SHM lengths;
PostgreSQL token table/index/TOAST relation sizes, excluding cluster WAL, shared
catalogs and the account namespace. These are different footprint definitions,
not interchangeable engine-capacity measurements. `growth-bytes` is the change
during the entire append or concurrent sub-benchmark, not bytes per operation or
logical evidence size. Checkpoints, page reuse and relation allocation affect it.

## Current measurements

Baseline: 10 October 2026, Linux amd64, Intel Core Ultra 7 155U (14 logical CPUs),
Go 1.26.8, host timezone AEDT. SQLite fixtures used a disk-backed btrfs temporary
directory with production WAL and `synchronous=FULL` settings. PostgreSQL 18.6
used the runner's pinned image, an anonymous Docker volume and default settings
(`fsync=on`, `synchronous_commit=on`,
`shared_buffers=128MB`). No manual ANALYZE, VACUUM or cache flushing was applied.
Engines ran sequentially, without other verification jobs.

PostgreSQL completed the full matrix in 1,175 seconds and SQLite in 1,478 seconds,
including untimed fixture construction: about 44 minutes combined. Smoke is the
routine verification command. SQLite was measured separately with the same
iteration/repetition counts after discarding an initial tmpfs run. Both backend
matrices passed their semantic checks. These are local workload samples,
not a capacity ranking or production SLO. Growing-history samples and default
planner/statistics behavior can produce substantial variation.

Median of three samples, each with three iterations. Times are milliseconds.

| Engine | Shape | Sessions | Context | Tokens | Reopen |
| --- | --- | ---: | ---: | ---: | ---: |
| SQLite | History1K | 9.1 | 8.7 | 11.3 | 2.1 |
| SQLite | History10K | 140.7 | 130.8 | 145.4 | 3.6 |
| SQLite | History100K | 1603.7 | 1558.9 | 1630.9 | 3.2 |
| SQLite | Tenants10 | 145.2 | 133.7 | 148.4 | 5.7 |
| SQLite | Retained3 | 145.6 | 133.4 | 152.4 | 3.3 |
| PostgreSQL | History1K | 7.9 | 4.8 | 7.0 | 22.3 |
| PostgreSQL | History10K | 43.9 | 27.0 | 57.3 | 22.3 |
| PostgreSQL | History100K | 394.9 | 248.4 | 545.7 | 25.2 |
| PostgreSQL | Tenants10 | 62.1 | 48.1 | 77.9 | 32.7 |
| PostgreSQL | Retained3 | 43.7 | 29.3 | 58.2 | 23.8 |

Append and burst visibility start at acceptance and include the final query.
Burst query p95 is the median of the three per-sample p95 values, not a pooled
percentile. Burst history grows across all nine iterations.

| Engine | Shape | Append acceptance | Append visibility | Burst acceptance | Burst visibility | Burst query p95 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| SQLite | History1K | 8.4 | 139.4 | 75.0 | 3915.2 | 33.8 |
| SQLite | History10K | 17.3 | 289.7 | 105.6 | 19620.3 | 210.6 |
| SQLite | History100K | 17.4 | 1803.8 | 200.3 | 41935.7 | 2157.0 |
| SQLite | Tenants10 | 17.4 | 289.1 | 103.0 | 19653.3 | 213.7 |
| SQLite | Retained3 | 17.3 | 294.1 | 105.5 | 19799.0 | 214.4 |
| PostgreSQL | History1K | 7.9 | 218.1 | 79.4 | 2274.6 | 19.2 |
| PostgreSQL | History10K | 9.8 | 71.0 | 49.1 | 2188.0 | 118.4 |
| PostgreSQL | History100K | 14.0 | 424.0 | 77.8 | 6860.4 | 630.2 |
| PostgreSQL | Tenants10 | 12.3 | 123.1 | 55.1 | 2173.8 | 140.6 |
| PostgreSQL | Retained3 | 9.3 | 75.3 | 48.8 | 1938.2 | 106.4 |

Footprints and median Go allocation volume use decimal MB. Seed and final sizes
use the different adapter probes described above. Final primary-dataset fact count
is seeded count + 2,313; independent datasets and retained generations remain.

| Engine | Shape | Seed footprint | Final footprint | Append allocated/op | Burst allocated/op |
| --- | --- | ---: | ---: | ---: | ---: |
| SQLite | History1K | 7.1 | 29.8 | 19.2 | 758.9 |
| SQLite | History10K | 39.6 | 221.9 | 25.2 | 3053.6 |
| SQLite | History100K | 350.3 | 794.5 | 84.6 | 6087.1 |
| SQLite | Tenants10 | 338.5 | 521.3 | 25.2 | 3048.5 |
| SQLite | Retained3 | 77.1 | 261.3 | 25.2 | 3050.9 |
| PostgreSQL | History1K | 4.2 | 34.8 | 4.8 | 154.4 |
| PostgreSQL | History10K | 37.7 | 209.0 | 4.8 | 539.4 |
| PostgreSQL | History100K | 368.1 | 634.1 | 4.8 | 1209.1 |
| PostgreSQL | Tenants10 | 364.9 | 536.4 | 4.8 | 538.3 |
| PostgreSQL | Retained3 | 84.2 | 255.5 | 4.8 | 538.9 |

Follow-up priorities:

1. Profile large-history dashboard queries: at 100k facts, SQLite views take
   roughly 1.6 seconds and PostgreSQL views 0.25–0.55 seconds. Separate query
   execution, row materialization and dashboard composition before changing SQL.
2. Profile publication and allocation volume under concurrent reads. At 100k
   facts, the median post-acceptance burst visibility is about 42 seconds for
   SQLite and 6.9 seconds for PostgreSQL. These are continuous-read stress
   measurements, not ordinary dashboard polling. Multi-GB Go allocation volume
   is cumulative allocation, not evidence of equivalent resident memory.
3. Investigate physical growth and initial/replacement publication. At 10k facts,
   both engines' measured token footprint exceeds 200 MB after only 2,313 added
   facts. During PostgreSQL's untimed multi-tenant/reprocessing setup, repeated
   activity snapshots showed the provenance DELETE in `publishRows` executing.
   Capture its query plan and a scoped profile before attributing the cost or
   choosing an index. Footprint probes alone do not establish leaks or WAL volume.

Optimize one measured stage per follow-up; keep the shared workloads and semantic
oracles unchanged so performance changes cannot hide lost or duplicated usage.

## Focused investigation

Run from `packages/cli` with the same pinned toolchain. PostgreSQL adapter commands
require the live test DSN; the root commands provision it automatically.

```sh
go test ./internal/datastore -run '^$' -bench '^BenchmarkMetadataRead$' -benchtime=100x -count=3
go test ./internal/collector -run '^$' -bench '^BenchmarkLocalStartup$' -benchtime=3x -count=3
go test ./internal/collector -run '^$' -bench '^BenchmarkIngestion$/^Native$' -benchtime=1x -count=3
go test ./internal/adapters/sqlanalytics -run '^$' -bench '^BenchmarkStorage$/^History10K$/^Query-sessions$' -benchtime=10x -count=3
```

`BenchmarkMetadataRead` uses a recursive SQLite CTE and checks the exact seeded
generation count. Benchmark smoke would reject fixture dialect errors such as the
removed `range()` call even when ordinary unit tests pass.

For CPU and allocation profiles, use a separate run, not a timing sample. Standard
`go test -cpuprofile/-memprofile` includes untimed fixture setup; do not attribute
the whole profile to dashboard latency. The ingestion suite retains its scoped
`-ingestion-profile-dir` flag for isolating capture/delivery/processing, documented
in `collector/ingestion_profile_test.go`. Inspect profile boundaries before drawing
bottleneck conclusions.

Choose one measured bottleneck per follow-up. Preserve exact accounting, isolation,
receipts, generation fences and snapshot semantics. Index/schema changes require
separate approval; retention, preaggregation and worker-count changes need evidence
from their affected workloads.
