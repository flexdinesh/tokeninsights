# Storage and ingestion performance

SQLite is the sole product engine, per [ADR 0027](adr/0027-personal-backend-and-explicit-connections.md).
This document retains SQLite measurements and their limits. Earlier comparative
engine experiments remain in Git history; they do not describe supported products.
No latency SLO or production capacity is claimed.

## Reproduce

Run from the repository root with the pinned tools:

```sh
mise run bench:smoke
mkdir -p .scratch/storage-tmp
env TMPDIR="$PWD/.scratch/storage-tmp" mise run bench:storage
```

Checks run locally through managed Dagger with isolated synthetic real SQLite
fixtures, never user databases. Use disk-backed temporary storage; tmpfs timings
do not establish disk durability performance. Smoke executes benchmark families
once without timing gates. Full storage samples use three iterations and three
repetitions, one Go package at a time. Do not collect baselines concurrently with
other verification or benchmarks. Smoke timings are not full-workload samples.

## Workload and semantic contracts

| Shape | Datasets | Sessions/dataset | Facts/dataset | Generations |
| --- | ---: | ---: | ---: | ---: |
| History1K | 1 | 10 | 1,000 | 1 |
| History10K | 1 | 100 | 10,000 | 1 |
| History100K | 1 | 1,000 | 100,000 | 1 |
| Tenants10 | 10 | 100 | 10,000 | 1 |
| Retained3 | 1 | 100 | 10,000 | 3 |

Dataset-qualified test shapes preserve retained SQL isolation invariants; they
do not imply hosted account support. Pi metadata spans ninety days and four models.
Each message contributes input 10, output 5, reasoning 2, cache-read 3, cache-write 4,
total 24. Native identities deliberately repeat across dataset fixtures.
Seeding uses real acceptance, interpretation and publication. Replacement
generations use reprocessing, never direct fact inserts or fake analytics.

The shared `storagecontract` fixtures own physical opening and footprint probes.
No production schema/interface changes follow from a benchmark.

- Query cases read the first dashboard page (50 rows) with all-history summaries.
  Setup is untimed; semantic assertions remain included. These do not measure
  selective filters or complete TUI reads.
- Reopen closes/opens current token storage, validates the physical contract and
  reacquires ownership. Subsequent identity/accounting checks are untimed.
  This is neither cold OS-cache latency nor full CLI startup.
- Append accepts a unique message and processes/query-checks it serially.
  Acceptance ends at durable commit; visibility ends after a successful final query.
- QueriesDuringProcessing accepts a 256-message burst while the two-worker
  dispatcher runs, then queries until visible. Every snapshot has nondecreasing
  fact counts and exact component totals. Final immutable receipts remain terminal.

Mutating cases grow history between samples. Keep operation and iteration counts
equal for comparisons. Query percentiles cover short synthetic bursts, not production
tail latency. Independent fixture datasets retain their component oracle; reopen
retains generation, identity and totals. Native source/rebuild/replay and real
transport/restart tests remain separately required.

## Resource accounting

`B/op` and `allocs/op` measure cumulative Go allocation volume, including
assertions/sample bookkeeping, never peak heap or RSS. `store-bytes` is SQLite
token file plus WAL/SHM lengths. `growth-bytes` measures the whole sub-benchmark,
not bytes per event. Checkpoints, page reuse and allocation affect footprint;
size probes alone do not establish a leak.

## Historical SQLite baseline

10 October 2026, Linux amd64, Intel Core Ultra 7 155U (14 logical CPUs), Go 1.26.8,
AEDT. Disk-backed btrfs temporary storage, production WAL and synchronous=FULL.
No manual ANALYZE, VACUUM or cache flushing. SQLite's full matrix took 1,478 seconds,
including untimed setup. Earlier tmpfs samples were discarded. These retained
measurements are historical, not a fresh validation of the redesign.

Median of three samples, three iterations each; milliseconds:

| Engine | Shape | Sessions | Context | Tokens | Reopen |
| --- | --- | ---: | ---: | ---: | ---: |
| SQLite | History1K | 9.1 | 8.7 | 11.3 | 2.1 |
| SQLite | History10K | 140.7 | 130.8 | 145.4 | 3.6 |
| SQLite | History100K | 1603.7 | 1558.9 | 1630.9 | 3.2 |
| SQLite | Tenants10 | 145.2 | 133.7 | 148.4 | 5.7 |
| SQLite | Retained3 | 145.6 | 133.4 | 152.4 | 3.3 |

Visibility includes the successful final query. Burst p95 is the median of
per-sample p95 values, not a pooled percentile; history grows across all nine iterations.

| Engine | Shape | Append acceptance | Append visibility | Burst acceptance | Burst visibility | Burst query p95 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| SQLite | History1K | 8.4 | 139.4 | 75.0 | 3915.2 | 33.8 |
| SQLite | History10K | 17.3 | 289.7 | 105.6 | 19620.3 | 210.6 |
| SQLite | History100K | 17.4 | 1803.8 | 200.3 | 41935.7 | 2157.0 |
| SQLite | Tenants10 | 17.4 | 289.1 | 103.0 | 19653.3 | 213.7 |
| SQLite | Retained3 | 17.3 | 294.1 | 105.5 | 19799.0 | 214.4 |

Decimal MB; primary dataset final fact count is seeded count + 2,313.
Independent fixture datasets and retained generations remain.

| Engine | Shape | Seed footprint | Final footprint | Append allocated/op | Burst allocated/op |
| --- | --- | ---: | ---: | ---: | ---: |
| SQLite | History1K | 7.1 | 29.8 | 19.2 | 758.9 |
| SQLite | History10K | 39.6 | 221.9 | 25.2 | 3053.6 |
| SQLite | History100K | 350.3 | 794.5 | 84.6 | 6087.1 |
| SQLite | Tenants10 | 338.5 | 521.3 | 25.2 | 3048.5 |
| SQLite | Retained3 | 77.1 | 261.3 | 25.2 | 3050.9 |

At 100k facts, roughly 42-second median burst visibility measures continuous-read
stress, not normal dashboard polling. Multi-GB cumulative allocation does not imply
equivalent resident memory. Profile one stage before proposing indexes, caching,
preaggregation, compaction or worker changes; schema/index changes need explicit approval.

## Dashboard count optimization

10 October 2026, same host/durability/storage. Before: `ece86c6`; after: count
group identities without unused row aggregates and obtain fact count from the
existing filtered summary scan. Dataset/generation snapshots, pagination and
accounting remain identical; no index/schema/cache/publication change.

Scoped profiles/plans at 10k facts identified SQL execution as SQLite's main cost.
Removing unused distinct-count/session-peak work avoids computing row aggregates
just to count groups. Query-only reruns used fresh 10k/100k fixtures, three iterations
and three samples, without profiling or concurrent verification.

Median milliseconds, before → after:

| Engine | Facts | Sessions | Context | Tokens |
| --- | ---: | ---: | ---: | ---: |
| SQLite | 10,000 | 136.5 → 113.4 | 126.0 → 108.7 | 142.5 → 117.9 |
| SQLite | 100,000 | 1563.5 → 1333.1 | 1525.2 → 1308.0 | 1586.0 → 1372.2 |

At 100k facts, median Go allocation per SQLite read fell from 66.4 to 49.4 MB for
sessions and 96.6 to 80.5 MB for tokens. This is not a general memory optimization.
These query samples do not update mutation/burst figures or establish production
tail latency. Full-history reads still scan active facts. Contracts cover each
grouping, filters, empty counts, page clamping, complete-result limits, unknown
locations, calendars and retained generations.

## Focused investigation

Use the current package paths from repository root after generating embedded assets:

```sh
go test ./packages/core/datastore -run '^$' -bench '^BenchmarkMetadataRead$' -benchtime=100x -count=3
go test ./packages/core/collector -run '^$' -bench '^BenchmarkLocalStartup$' -benchtime=3x -count=3
go test ./packages/core/adapters/sqlanalytics -run '^$' -bench '^BenchmarkStorage$/^History10K$/^Query-sessions$' -benchtime=10x -count=3
```

Metadata benchmarks check exact generation counts. Fixture errors must fail smoke.
Use separate profiling runs: standard Go CPU/heap profiles include untimed setup.
The shared query benchmark's `TOKENINSIGHTS_QUERY_PROFILE_DIR` samples after setup;
use a fresh absolute directory and `-count=1`. Compare heap-before/heap-after for
allocation volume, never peak RSS. The ingestion suite's scoped profiling flag is
documented in `collector/ingestion_profile_test.go`. Inspect phase boundaries
before attributing a bottleneck. Keep all accounting, receipt, identity, snapshot
and generation oracles unchanged.
