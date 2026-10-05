# Collector publication identity audit

Status: proposed; static audit of current implementation, not a new wire/schema contract.

## Scope and conclusions

The agreed invariant is sound: the same retained Durable Sources, accounting rules, and relevant attribution inputs must reconstruct the same published facts after collector SQLite deletion. Collector progress and upload batches are disposable; fact identity is not.

Two properties need independent assertions:

- **Rebuild determinism:** repeated interpretation produces the same IDs and canonical values.
- **Correct identity:** different legitimate facts stay separate; proven copies stay together.

A deterministically wrong collision satisfies the first property. Golden totals alone do not prove stable identity. Current fixtures generally omit canonical `semantic_key` and `recorded_at_ms` (`pipeline_test.go:33`, `pipeline_test.go:1721`); new rebuild comparisons must include them.

No production/schema changes made by this audit. No retractions, backflow, autonomous collection, or remote setup proposed.

## Existing identity boundaries

| Layer | Current implementation | Publication suitability |
| --- | --- | --- |
| Raw observation | Per-run sighting; operational run/database references | Local provenance only |
| Raw fact | `sync.go:665`: harness, source, native session/message, occurrence, scope, token components, parser | Deterministic for unchanged inputs/rules; payload/parser-dependent, unsuitable as general immutable publication identity |
| Canonical session | `normalize.go:400`: harness + native session | Good starting point; explicit namespace/encoding contract still needed |
| Canonical message | `normalize.go:422`: harness + session + message | Good when native identity exists; request identity currently not separately preserved |
| Canonical usage | `normalize.go:549`: harness + session + message + canonical timestamp + scope | Stable for ordinary unchanged sources; timestamp/weak-ID collision and changed-source gaps below |
| Batch replay | Future ingestion protocol | Distinct from fact identity: rebuilt collectors create new batches but must reproduce old fact IDs |

Database integer primary keys, ingest run IDs, observed time, collector installation IDs, and batch IDs must not become published entity identity inputs. Parser provenance and values can be published evidence/payload; they must not accidentally create a new identity for an existing logical entity.

## Findings by harness

### OpenCode: native identity is available; copy suppression is a heuristic

Evidence:

- Native SQLite row message/session IDs are authoritative: `opencode_sqlite.go:393`, `opencode_sqlite.go:402`, `opencode_sqlite.go:458`.
- V2 precedence over V1 uses native logical message key: `opencode_sqlite.go:316`, `opencode_sqlite.go:475`.
- Cross-row `DedupeKey` hashes source times, provider/model, and counters, **excluding both message and session IDs**: `opencode_sqlite.go:642`.
- Duplicate ingest suppresses later facts before canonical normalization: `sync.go:443`.
- Existing `TestOpenCodeSQLiteSuppressesDuplicateChannelRows` intentionally uses different row/session IDs with equal values: `pipeline_test.go:1560`.

Consequences:

- Fresh reconstruction of the same sources in the same discovered order can be deterministic.
- Two independent messages with equal timestamps and counters can be suppressed as copies. This is an accuracy/collision issue, not a failure caused by DB deletion.
- Moving or changing channel file discovery order can select a different representative session/message for the same heuristic group. Totals can remain equal while publication IDs differ. This requires a fixture before claiming order/path invariance.
- Native message/session + explicit origin/copy evidence is preferable for publication. Equal values alone are not proof of common origin.

Fixtures/assertions:

1. Same native message present in V1/V2: one fact, identical identity on clean DB rebuild.
2. Distinct messages with equal usage/time, no copy evidence: two facts and additive totals.
3. Proven channel copy pair: one fact; reverse discovery order and relocate source files; same selected logical identity.
4. Missing JSON timestamp with valid SQLite `time_created`: same source-derived time/ID on every rebuild.
5. Both timestamp sources absent: skip with `opencode_sqlite_missing_time`, no canonical/upload fact.

### Pi: native entry ID is good; missing entry ID permits collisions

Evidence:

- Session header preferred, filename fallback supported: `pi_jsonl.go:148`, `pi_jsonl.go:190`.
- Raw logical source hashes session identity, not directory path: `pi_jsonl.go:291`.
- Missing message ID is accepted with diagnostic: `pi_jsonl.go:279`.
- Canonical usage key includes an empty message ID plus millisecond occurrence time: `normalize.go:549`.
- Current warning test covers one missing-ID message only: `pipeline_test.go:1517`.

Consequences:

- Header + entry ID survives DB deletion and source relocation without native identity changes.
- Two missing-ID messages in one session at the same millisecond share one canonical key, even with different counters. The raw keys differ for different counters; canonical upserts then pick the later processed value instead of summing legitimate facts.
- Filename fallback is deterministic for the same file name; changing the fallback filename changes session identity. This is a changed identity input, not a same-source-byte-only guarantee.

Fixtures/assertions:

1. Native IDs distinct, values/time equal: two facts.
2. Copied native-ID file under another supported discovery path: same IDs and one logical contribution.
3. Two missing-ID records with equal time but different counters: preserve distinct usage using an approved deterministic fallback, or diagnose/reject ambiguity. Do not silently count one.
4. Header/file suffix mismatch: header wins on every rebuild; same diagnostic.
5. Incremental suffix parsing and full fresh replay: same stable IDs and final canonical values.

### Claude Code: request grouping and evolving source timestamp need a publication decision

Evidence:

- Fact message identity uses `message.id`, falling back to row `uuid`: `claude_code_jsonl.go:219`.
- Streaming groups use `message.id + requestId`, or message ID alone: `claude_code_jsonl.go:287`.
- Request ID is held in a transient slice and fact `DedupeKey`, not a distinct `RawTokenFact` field: `claude_code_jsonl.go:129`, `claude_code_jsonl.go:344`.
- Streaming merge keeps maximum counters and latest source timestamp: `claude_code_jsonl.go:303`.
- Source dedupe key includes request, timestamp, and token values: `claude_code_jsonl.go:361`.
- Raw/canonical usage identity does not include request ID independently: `sync.go:665`, `normalize.go:549`.

Consequences:

- Rebuilding the same completed transcript is deterministic under unchanged metadata.
- A sync of a partial record followed by an appended completed record can create two canonical keys because the merge selects a later occurrence time. A fresh parse of the completed transcript creates only the merged fact. This is **changed input / incremental versus full equivalence**, not a counterexample to unchanged-input rebuilding.
- Two request groups reusing the same message ID/time can collapse downstream despite being separate parse groups. Whether native harness guarantees prohibit such reuse must be explicit; unsupported contradictory identity evidence needs diagnostics.
- A missing message ID with UUID uses UUID but does not use the streaming merge function. Tests must distinguish per-record UUID from API request identity rather than assume UUID establishes common-request copies.
- Conflicting nonempty provider/model values are first-observed wins (`claude_code_jsonl.go:313`); line reordering can change attribution. Line order is source input; reversed-order invariance is a separate desired contract.

Fixtures/assertions:

1. Complete streaming copies with same native message/request: one normalized fact with pinned counters/time/ID.
2. Partial sync → append final → sync, compared with clean full parse: same final logical contributions; mark current discrepancy as changed-input regression.
3. Same message/time, different request IDs: either distinct IDs or explicit validation/diagnosis according to the chosen contract.
4. Different native messages, identical counters/time: both retained.
5. Explicit parent session/sidechain transcript copies: reference preserved origin identity without assigning a second usage identity solely from artifact path.
6. UUID fallback and missing request IDs: deterministic supported behavior and diagnostics pinned.

### Codex: snapshot hashing currently protects valid same-millisecond events

Evidence:

- Usage fact source identity hashes owning native session: `codex_jsonl.go:380`.
- Candidate message ID uses turn + occurrence time, then hash of typed presence-preserving last/cumulative source snapshot: `codex_replay.go:100`.
- Missing turn uses line + occurrence; no cumulative snapshot adds line identity: `codex_jsonl.go:566`, `codex_replay.go:103`.
- Existing same-ms snapshot test pins two different message IDs: `codex_replay_test.go:505`.
- Explicit ancestry plus turn, provider/model, and exact valid snapshots proves replay: `codex_replay.go:109`, `codex_replay.go:190`, `codex_replay.go:304`.
- Proven copy emits original fact identity/time, even when child timestamps changed: `codex_replay.go:351`.
- Missing/ambiguous ancestry retains uncertain child facts with diagnostics: `codex_replay.go:330`, `codex_replay.go:369`.

Consequences:

- Byte-identical source snapshots reconstruct the same message IDs after DB deletion. Snapshot hashes do **not** introduce nondeterminism on unchanged input.
- Removing snapshot hashes without a replacement can merge legitimate events within one turn/millisecond. Native turn ID alone is insufficient.
- Corrected source counters produce a different snapshot-derived identity. Whether that is a new event or a revision requires harness evidence; changed counters are changed input.
- Missing-turn/last-only fallback is stable for identical line ordering. Inserting unrelated lines changes line-based identity. Reordering cumulative token records can also change suppression; arbitrary record-order invariance is unsuitable for a stateful log.
- Parent availability changes the complete parser input set and can change copied-history interpretation. Identical child bytes alone do not establish unchanged-input semantics when ancestry dependencies differ.

Fixtures/assertions:

1. Same-ms distinct cumulative snapshots remain distinct after multiple fresh parses.
2. Retimestamped replay with complete available parent proof: one original identity; parent-first and child-first ingestion agree.
3. Unrelated sessions with equal counters: remain independent.
4. Missing/ambiguous/cyclic ancestry: diagnostics and conservative behavior explicit, never claimed universally duplicate-free.
5. Missing-turn/last-only records: same bytes reproduce fallback IDs; inserted-line variant documents identity limits.
6. Pending model backfill and known ancestry under incremental/full paths: same IDs/values for equivalent complete source inputs.

## Shared boundary hazards

### Occurrence-time fallback: latent risk, not active missing-time behavior

`canonicalTime` returns observation time if occurrence time is null (`normalize.go:561`). However, **all four active adapters reject facts without usable occurrence time**: OpenCode V1 `opencode_sqlite.go:384`, V2 `opencode_sqlite.go:434`; Pi `pi_jsonl.go:269`; Codex `codex_jsonl.go:345`; Claude `claude_code_jsonl.go:206`.

Therefore do not claim OpenCode presently emits clock-dependent usage IDs for absent source time. Add a normalization-boundary fixture with a directly supplied raw fact having missing occurrence time, and decide whether publication rejects it or derives supported identity without the observation clock. A future adapter must not silently inherit clock-dependent publication identity.

### Encoding ambiguity is separate from hash collisions

Current raw/canonical usage keys concatenate fields with `|`; canonical message keys use `:`. No escaping/length encoding is specified. Session/message tuples `("s|m", "x")` and `("s", "m|x")` yield the same canonical usage preimage for other equal fields. Native UUID-like constraints may exclude these values, but the collector boundary does not currently enforce that assumption.

Use deterministic typed encoding or length-prefixed fields for the proposed publication contract. Pin literal ID vectors independently of implementation; verify empty/null distinctions where they affect identity. This prevents preimage ambiguity before cryptographic hashing.

### Raw bytes alone do not pin current optional location payload

`location.go:40` inspects current Git state and derives display labels using configured home. Identical harness bytes can resolve different attribution if local checkout/remotes/home inputs change. `docs/design.md` explicitly acknowledges reused directories can attribute historical data to current checkout.

Keep attribution outside usage identity. State token-fact identity invariance separately from optional location enrichment. Tests should isolate synthetic attribution inputs, then deliberately change repository evidence to assert identity remains constant while an enrichment change is visible.

### Processing order must not choose competing payloads accidentally

Canonical normalization processes queue insertion order (`normalize.go:304`) and updates matching keys (`normalize.go:466`). That can make collisions/competing raw payloads order-dependent. Deterministic discovery order currently hides some issues; worker scheduling must remain irrelevant. Test source order permutations where source semantics are equivalent, but do not shuffle stateful records as though the input were unchanged.

## Publication test oracle

Compare normalized exports containing stable session/message/fact IDs, native identity evidence, occurrence time, canonical token components, provider/model provenance, quality/countability, and optional deterministic attribution. Exclude local row IDs, run references, observations, collector stream IDs, change sequences, delivery cursors, and batch IDs from logical equality.

Assert hand-authored totals and identities, not merely equality between two executions of the same implementation. For every baseline fixture run clean DB, repeated full refresh, fresh DB at another clock, relocated source roots, and bounded parallel preparation. Preserve native source metadata for the equivalence cases. Add adversarial and changed-source cases separately with precise expected failures.

Future end-to-end ingestion must accept rebuilt identical facts in new batches and leave row counts, reference integrity, component totals, and aggregate totals unchanged. Replay of one old batch is a separate test. Stable IDs do not repair a fact that was already incorrectly merged locally.

## Unresolved decisions

- Pi/other missing native IDs: deterministic fallback or explicit ambiguous-fact diagnostic?
- Claude message/request uniqueness: enforce harness assumption or publish request-aware identity?
- OpenCode proven copy identity: what source evidence replaces equal-value-only suppression?
- Codex mutable snapshot: supported revision evidence or explicit changed-payload conflict?
- Same stable ID with different canonical payload: reject conflict or accept a specifically proven update?
- Identity/schema incompatibility: migration must preserve uniqueness or explicitly reconcile changed identities; no silent hash-version multiplication.

These are prerequisites for claiming duplicate-safe canonical ingestion. They do not require designing retractions or automatic backfill.
