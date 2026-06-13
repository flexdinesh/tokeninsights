# Explore Codex Session Durable Sources

Type: HITL

Blocked by: None

Status: Implemented

## What to explore

Research Codex's local session Durable Sources and define parser requirements for `tokeninsights sync --harness codex`.

TokScale lead: Codex uses JSONL session data under `~/.codex/sessions/`.

Codex auth files, account stores, keychain data, remote usage/quota APIs, and token refresh flows are out of scope.

## Evidence packet

- [x] List the Codex session roots and discovery rules to support.
- [x] Document session file naming patterns without storing full paths.
- [x] Document relevant JSONL event types using redacted minimal examples.
- [x] Document where token usage lives, including `last_token_usage` and `total_token_usage` if present.
- [x] Document stateful model and provider extraction.
- [x] Document session identity and turn/message/fact identity.
- [x] Document which events are countable and which are context, replay, or non-human input.
- [x] Document fork, replay, resumed-session, cumulative-counter, and headless `exec` behavior.
- [x] Identify stale snapshot, counter regression, and double-counting diagnostics.
- [x] Identify private fields that must never enter raw storage.
- [x] Propose minimal JSONL fixtures and expected raw/canonical/diagnostic outputs.

## Read-only evidence

### Source locations and discovery

Support Codex JSONL session files under the Codex session directory only:

- Default root: `${CODEX_HOME:-~/.codex}/sessions`.
- Observed layout: `~/.codex/sessions/<year>/<month>/<day>/rollout-<iso-like-start>_<session-id>.jsonl` in older files and `rollout-<iso-like-start>-<session-id>.jsonl` in current files. The local sample used date directories such as `2026/06/13`.
- Candidate session files: `*.jsonl` under the date-partitioned session root.
- Prefer files whose basename starts with `rollout-` and ends in `.jsonl` for default discovery. For explicit source overrides, accept either a matching JSONL file or a directory containing matching Codex JSONL session files.
- Ignore Codex auth, account, model cache, memory, state, goal, log, shell snapshot, plugin, and history files outside `sessions/`.
- Sort discovered files by path before parsing so diagnostics and ingest runs are deterministic.

Observed local sample during exploration, summarized without recording full paths or transcript values:

- 218 Codex JSONL session files.
- Session files were partitioned by date below the session root.
- Session files contained top-level JSONL records with `type`, `timestamp`, and `payload` fields.
- The first record in every sampled file was `session_meta`.
- The first `session_meta.payload.id` matched the filename session suffix in every sampled file.
- Some fork/subagent files contained later `session_meta` records with different `payload.id` values. Treat the first session metadata ID as the owning file session and later metadata as fork/replay context.

Recommended `Source` envelope:

- `source_kind`: `codex-session-jsonl`
- ingest-run `source_id`: stable hash of the root-relative file locator, so each physical file parse has audit provenance without storing the full path
- raw-fact `source_id`: stable hash of `codex-session:<first session_meta.payload.id>`, falling back to a stable hash of the filename session suffix only when the first session metadata ID is missing

### JSONL record shapes

Observed top-level record types:

| Record type | Role in token parser |
|-------------|----------------------|
| `session_meta` | Session identity, provider, source/originator, fork markers, and private working-directory metadata. |
| `turn_context` | Current model and turn context state. |
| `event_msg` with `payload.type == "token_count"` | Token usage snapshots. This is the primary token source. |
| `event_msg` with `payload.type == "task_started"` | Turn ID boundary for token-count identity when present. |
| `event_msg` with `payload.type == "task_complete"` or `turn_aborted` | Turn boundary end; no token fact by itself. |
| `event_msg` with `payload.type == "user_message"` | Human-turn context only; message text must not be stored. |
| `response_item` | Transcript, tool-call, reasoning, and output records. Ignore for token ingestion. |
| `compacted` and `event_msg.context_compacted` | Context compaction/replay signal. Ignore as token facts. |

Minimal redacted session metadata:

```json
{
  "timestamp": "2026-01-01T00:00:00.000Z",
  "type": "session_meta",
  "payload": {
    "id": "00000000-0000-0000-0000-000000000000",
    "timestamp": "2026-01-01T00:00:00.000Z",
    "source": "cli",
    "originator": "codex_cli_rs",
    "model_provider": "openai",
    "cli_version": "redacted",
    "cwd": "/redacted/project"
  }
}
```

Minimal redacted turn context:

```json
{
  "timestamp": "2026-01-01T00:00:01.000Z",
  "type": "turn_context",
  "payload": {
    "turn_id": "turn_0001",
    "model": "gpt-5.5",
    "effort": "high",
    "cwd": "/redacted/project"
  }
}
```

Minimal redacted token-count event:

```json
{
  "timestamp": "2026-01-01T00:00:02.000Z",
  "type": "event_msg",
  "payload": {
    "type": "token_count",
    "info": {
      "model_context_window": 400000,
      "last_token_usage": {
        "input_tokens": 100,
        "cached_input_tokens": 20,
        "output_tokens": 50,
        "reasoning_output_tokens": 10,
        "total_tokens": 150
      },
      "total_token_usage": {
        "input_tokens": 300,
        "cached_input_tokens": 100,
        "output_tokens": 120,
        "reasoning_output_tokens": 30,
        "total_tokens": 420
      }
    },
    "rate_limits": {}
  }
}
```

### Token usage rows

Token usage lives on:

```text
type == "event_msg"
payload.type == "token_count"
payload.info is an object
```

Observed usage objects contained:

- `payload.info.last_token_usage`
- `payload.info.total_token_usage`
- `input_tokens`
- `cached_input_tokens`
- `output_tokens`
- `reasoning_output_tokens`
- `total_tokens`

`last_token_usage` is the per-snapshot increment source. `total_token_usage` is a cumulative session snapshot and must not be inserted as a countable raw fact because summing it would double count. Use cumulative totals only to suppress repeated snapshots and detect stale/regressed counters.

Mapping to `RawTokenFact` for countable token facts:

| Raw field | Codex source |
|-----------|--------------|
| `session_id` | current logical session ID, initialized from first `session_meta.payload.id`; updated only when a forked child becomes the active logical session after its own `turn_context` |
| `message_id` | stable synthetic token fact ID derived from session ID, active `turn_id` when present, token event timestamp, model, and token components |
| `provider` | `session_meta.payload.model_provider`, null when missing |
| `model` | latest `turn_context.payload.model`; fallback to token event model fields if present; null when unresolved |
| `occurred_at_ms` | parsed top-level token event `timestamp` |
| `usage_scope` | `message` for countable `last_token_usage` facts |
| `quality` | `exact` when token fields are valid |
| `input_tokens` | `last_token_usage.input_tokens - cache_read_tokens`, clamped to zero |
| `output_tokens` | `last_token_usage.output_tokens` |
| `reasoning_tokens` | `last_token_usage.reasoning_output_tokens` |
| `cache_read_tokens` | `last_token_usage.cached_input_tokens`; also accept `cache_read_input_tokens` if present and use the larger non-negative value |
| `cache_write_tokens` | null; Codex does not expose durable cache-write tokens in observed session logs |
| `total_tokens` | `last_token_usage.total_tokens` only if it is usable as a source total; otherwise leave null and let canonical total sum the normalized components |

Important token-total nuance:

- In the local sample, `total_tokens` usually equaled `input_tokens + output_tokens`, not `input_tokens + cached_input_tokens + output_tokens + reasoning_output_tokens`.
- To avoid inflated canonical totals, the parser should prefer normalized components over source `total_tokens` unless a fixture proves the source total has the same semantics TokenInsights wants to display.
- Component normalization should treat `cached_input_tokens` as the cache-read portion of input. For example, `input_tokens = 100` and `cached_input_tokens = 20` should become raw `input_tokens = 80`, `cache_read_tokens = 20`, not `input_tokens = 100`, `cache_read_tokens = 20`.

Clamp negative token values to zero before writing raw facts. Negative token values should also produce a warning diagnostic because they indicate malformed source data.

### Stateful interpretation model

Codex parsing must be stateful per file:

1. Initialize session state from the first `session_meta` record.
2. Track provider from `session_meta.payload.model_provider`.
3. Track the active model from `turn_context.payload.model`.
4. Track active `turn_id` from `turn_context.payload.turn_id` or `event_msg.task_started.payload.turn_id` when present.
5. For each token-count event, parse `last_token_usage` as the candidate countable fact and parse `total_token_usage` as the cumulative counter.
6. If cumulative totals are identical to the previous cumulative totals for the same logical session, suppress the event as a duplicate snapshot.
7. If cumulative totals increase, ingest the `last_token_usage` fact and advance the cumulative baseline.
8. If cumulative totals are missing but `last_token_usage` is present, ingest `last_token_usage` with a warning diagnostic because duplicate suppression is weaker.
9. If cumulative totals regress, do not automatically sum the new `last_token_usage`. Treat likely stale near-regressions as skipped duplicate/stale snapshots. Treat clear reset behavior as a warning and start a new cumulative baseline only when the fixture-defined behavior proves it is safe.
10. If a token-count event appears before a model is known, hold it until a later model-bearing event if possible. If no model appears before an unrelated event boundary or EOF, ingest with raw model null and emit an unresolved-model diagnostic.

This model intentionally avoids deriving token deltas from `total_token_usage` as the normal path. Codex cumulative snapshots can represent replay, resume, compaction, or repeated status updates; `last_token_usage` is the token fact, while `total_token_usage` is the guardrail.

### Identity and dedupe

Stable identities:

- Canonical session identity: `codex` plus logical session ID.
- Canonical message identity: `codex` plus canonical session plus stable synthetic token fact ID.
- Raw fact identity: harness, logical Codex session source ID, logical session ID, active turn ID when present, occurred timestamp, model, usage scope, parser version, normalized token components, and cumulative total fingerprint when available.

Token-count events do not expose top-level event IDs in the observed local sample. Do not use line number alone as canonical identity. Line number may be diagnostic metadata only.

Recommended duplicate suppression fingerprint:

- logical session ID
- active turn ID when present
- event timestamp
- provider
- model
- normalized `last_token_usage` token components
- `total_token_usage` token components when present

When duplicate token-count snapshots share the same cumulative total fingerprint:

- Keep the first event in deterministic file/line order.
- Emit `codex_jsonl_duplicate_token_snapshot`.
- Do not insert a second raw token fact or canonical fact.

### Timestamp behavior

Timestamp precedence for countable token facts:

1. Top-level token-count event `timestamp` when it is a parseable timestamp string.
2. Otherwise skip the token fact and emit `codex_jsonl_missing_time`.

Do not fall back to ingest time or file modification time. Canonical token semantic keys include recorded time, so nondurable fallback times would make repeated syncs unstable.

The session filename timestamp and `session_meta.payload.timestamp` are session-start metadata only. Do not use them as per-token occurrence times.

### Provider and model

Provider extraction:

- Use `session_meta.payload.model_provider`.
- Preserve missing provider as null in raw facts.
- Normalization renders missing provider as `unknown`.

Model extraction:

- Use the latest `turn_context.payload.model`.
- If future token-count events expose `payload.info.model`, `payload.info.model_name`, `payload.model`, or `payload.model_info.slug`, use those as narrower event-local model values when non-empty.
- Preserve unresolved model as null in raw facts.
- Normalization renders missing model as `unknown`.
- Emit a warning diagnostic only when a token-count event had to be ingested before model state could be resolved.

### Fork, replay, resume, compaction, and headless behavior

Fork and replay:

- Some sampled files had multiple `session_meta` records with different IDs. These were associated with fork/subagent context and parent replay.
- If a `session_meta` record has `payload.forked_from_id`, or `payload.source.subagent.thread_spawn.parent_thread_id`, mark the file as a forked child.
- While waiting for the child session's own `turn_context`, skip replayed parent token-count rows and remember the inherited cumulative baseline for duplicate suppression.
- After the child session starts its own turn, continue skipping token snapshots that are within or equal to the inherited baseline. Resume counting only once cumulative totals move beyond inherited replay.
- Emit diagnostics for skipped inherited snapshots and ambiguous fork transitions, but do not fail the whole file.

Resume and repeated snapshots:

- Repeated snapshots with unchanged cumulative totals are duplicate status updates. Suppress them.
- A resumed session's first token-count event may include cumulative history. Use `last_token_usage`, not full `total_token_usage`, for the countable fact.

Compaction:

- `compacted` and `event_msg.context_compacted` records are not token facts.
- If cumulative counters regress around compaction while `last_token_usage` is present, treat likely stale snapshots as non-countable/skipped to avoid counting the same increment twice.
- If a clear counter reset is fixture-proven, ingest the next valid `last_token_usage` fact and reset the cumulative baseline with a diagnostic.

Headless `codex exec`:

- `session_meta.payload.source == "exec"` or `originator == "codex_exec"` identifies headless `exec` sessions.
- Headless sessions still use the same structured `event_msg.token_count` path when present.
- If a headless file contains older unstructured usage records, support them only as an explicit follow-up fixture path. Do not let generic JSONL fallback parse unrelated Codex session records.

### Diagnostics

Emit diagnostics for:

- `codex_jsonl_parse_error`: skipped unparsable JSON line.
- `codex_jsonl_missing_session_meta`: file has no first `session_meta` record and no usable filename session suffix.
- `codex_jsonl_session_id_mismatch`: first `session_meta.payload.id` and filename session suffix disagree.
- `codex_jsonl_multiple_session_meta`: file contains later `session_meta` records with different IDs; informational unless fork/replay handling becomes ambiguous.
- `codex_jsonl_missing_session`: token-count row cannot resolve a stable logical session ID.
- `codex_jsonl_missing_time`: token-count row has no usable top-level timestamp.
- `codex_jsonl_missing_model`: token-count row was ingested before model state could be resolved.
- `codex_jsonl_missing_tokens`: token-count row has no usable `last_token_usage` or cumulative-delta fallback.
- `codex_jsonl_invalid_tokens`: token fields exist but are non-numeric.
- `codex_jsonl_negative_tokens`: token fields were present but negative and clamped to zero.
- `codex_jsonl_duplicate_token_snapshot`: repeated token-count snapshot was suppressed by cumulative fingerprint.
- `codex_jsonl_stale_token_snapshot`: cumulative totals regressed in a way that looked like an out-of-order or stale snapshot and was skipped.
- `codex_jsonl_counter_regression`: cumulative totals regressed in a way that could not be safely classified.
- `codex_jsonl_total_without_last`: only `total_token_usage` was present; parser used cumulative delta when safe or skipped when unsafe.
- `codex_jsonl_last_without_total`: only `last_token_usage` was present; parser ingested it with weaker duplicate protection.
- `codex_jsonl_fork_replay_skipped`: inherited parent/fork token-count row was skipped.
- `codex_jsonl_unsupported_headless_usage`: older unstructured headless usage shape was present but not parsed by the first implementation.

Missing provider or model should preserve null in raw facts. Missing provider should not be a parser diagnostic. Missing model is diagnostic-worthy only when stateful reconstruction was attempted and still unresolved.

### Privacy review

Never store these source fields or JSON paths in raw storage or diagnostics:

- full JSONL paths
- raw filenames when they include local session locator details
- `session_meta.payload.cwd`
- `session_meta.payload.git`
- `session_meta.payload.base_instructions`
- `session_meta.payload.instructions`
- `session_meta.payload.thread_source` when it exposes private workspace context
- `turn_context.payload.cwd`
- `turn_context.payload.summary`
- `turn_context.payload.user_instructions`
- `turn_context.payload.developer_instructions`
- `turn_context.payload.workspace_roots`
- `event_msg.user_message.payload.message`
- `event_msg.user_message.payload.text_elements`
- `event_msg.agent_message.payload.message`
- `event_msg.agent_message.payload.last_agent_message`
- `event_msg.agent_reasoning.payload.text`
- `event_msg.task_complete.payload.last_agent_message`
- `event_msg.exec_command_end.payload.command`
- `event_msg.exec_command_end.payload.cwd`
- `event_msg.exec_command_end.payload.stdout`
- `event_msg.exec_command_end.payload.stderr`
- `event_msg.exec_command_end.payload.formatted_output`
- `event_msg.patch_apply_end.payload.stdout`
- `event_msg.patch_apply_end.payload.stderr`
- `response_item.payload.content`
- `response_item.payload.arguments`
- `response_item.payload.input`
- `response_item.payload.output`
- `response_item.payload.encrypted_content`
- `response_item.payload.summary`
- request headers, secrets, raw provider payloads, prompt text, assistant text, tool arguments, and tool output

Allowed metadata-only values:

- token component counts
- top-level record type and payload type
- session ID
- active turn ID
- synthetic message/fact ID
- provider
- model
- top-level timestamp
- bounded parser diagnostic code and line number
- whether a row was suppressed as duplicate, stale, replay, or unsupported

### Fixture proposal

Create minimal JSONL fixtures under the existing conformance source tree using only synthetic metadata:

```jsonl
{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex_s1","timestamp":"2026-01-01T00:00:00.000Z","source":"cli","originator":"codex_cli_rs","model_provider":"openai","cwd":"/redacted/project"}}
{"timestamp":"2026-01-01T00:00:01.000Z","type":"turn_context","payload":{"turn_id":"turn_1","model":"gpt-5.5","cwd":"/redacted/project"}}
{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":150},"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":150}}}}
{"timestamp":"2026-01-01T00:00:03.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":35},"total_token_usage":{"input_tokens":130,"cached_input_tokens":30,"output_tokens":55,"reasoning_output_tokens":10,"total_tokens":185}}}}
{"timestamp":"2026-01-01T00:00:04.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":35},"total_token_usage":{"input_tokens":130,"cached_input_tokens":30,"output_tokens":55,"reasoning_output_tokens":10,"total_tokens":185}}}}
```

Expected output for the base fixture:

- Two countable raw `message` facts.
- The third token-count line is suppressed as `codex_jsonl_duplicate_token_snapshot`.
- Provider `openai`.
- Model `gpt-5.5`.
- First fact normalized components: input `80`, cache read `20`, output `50`, reasoning `10`.
- Second fact normalized components: input `20`, cache read `10`, output `5`, reasoning `0`.
- Raw `total_tokens` should be null unless implementation explicitly proves Codex `total_tokens` semantics are compatible with TokenInsights canonical totals.

Additional fixture cases:

- Missing provider in `session_meta`; expect raw provider null and canonical provider `unknown`.
- Missing model until a later `turn_context`; expect pending token facts to bind to the later model when safe.
- Missing model through EOF; expect raw model null, canonical model `unknown`, and `codex_jsonl_missing_model`.
- Missing or invalid top-level timestamp; expect skipped fact plus `codex_jsonl_missing_time`.
- Non-numeric token fields; expect skipped fact plus `codex_jsonl_invalid_tokens`.
- Negative token fields; expect clamped raw counts plus `codex_jsonl_negative_tokens`.
- `last_token_usage` present without `total_token_usage`; expect countable fact plus `codex_jsonl_last_without_total`.
- `total_token_usage` present without `last_token_usage`; expect safe cumulative delta only when a previous baseline exists, otherwise skip or non-countable fallback plus `codex_jsonl_total_without_last`.
- Cumulative counter regression near a previous watermark; expect skipped stale snapshot plus `codex_jsonl_stale_token_snapshot`.
- Clear cumulative reset after compaction; expect baseline reset behavior and `codex_jsonl_counter_regression`.
- Forked child with parent replay before child `turn_context`; expect replay rows skipped and only child-owned token facts counted.
- Forked child with inherited cumulative baseline after child `turn_context`; expect inherited snapshot skipped until totals move beyond the baseline.
- Headless `exec` session with structured token-count records; expect normal countable facts.
- Headless older unstructured usage line; expect `codex_jsonl_unsupported_headless_usage` unless implementation explicitly adds a fixture-backed parser branch.
- Malformed JSONL line; expect `codex_jsonl_parse_error`.

Expected raw output:

- Countable `message` facts for valid Codex token-count increments only.
- `source_kind = "codex-session-jsonl"`.
- Logical raw `source_id` derived from logical session identity, not the full path.
- Provider/model null preserved when unresolved.
- `cache_write_tokens` null.
- `metadata_json` null unless bounded line-number or suppression metadata is needed.

Expected canonical output:

- Canonical sessions for rows with stable logical session IDs.
- Canonical messages for synthetic token fact IDs.
- Provider/model `unknown` for missing raw provider/model.
- Total tokens computed from normalized components unless raw `total_tokens` has proven compatible semantics.
- Repeated syncs do not insert duplicate raw or canonical token facts.

Expected diagnostics:

- Missing stable session identity.
- Filename/session ID mismatch.
- Multiple session metadata IDs.
- Missing or unresolved model.
- Missing or invalid timestamp.
- Missing, invalid, or negative token fields.
- Duplicate, stale, regressed, replayed, or unsupported headless usage rows.
- Malformed JSONL line.

## Acceptance criteria

- [x] The issue proves a metadata-only mapping from Codex JSONL events to `RawTokenFact`.
- [x] The issue defines a stateful token interpretation model before implementation.
- [x] The issue defines stable source IDs without storing full source paths.
- [x] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [x] The issue defines diagnostics for ambiguous state transitions or unsupported event patterns.
- [x] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [x] Parser implementation was completed after exploration without a schema change.

## Schema assessment

The current TokenInsights schema is sufficient for Codex token-count usage:

- `raw_token_usage` can represent logical session ID, synthetic token fact/message ID, provider/model, normalized token components, quality, scope, and observed/occurred time.
- `canonical_token_usage` can represent normalized provider/model and component-summed totals.
- `normalization_diagnostics` can represent missing-session and parser warnings without storing private source content.

Follow-up work, not required for this parser:

- Canonical request-duration or TPS facts if Codex `task_complete.duration_ms` or `time_to_first_token_ms` should feed the TPS tab later.
- A first-class raw/canonical representation for non-countable cumulative snapshots if future UX needs to audit Codex session totals directly.
- Fixture-backed support for older unstructured headless `codex exec` usage lines if current structured `event_msg.token_count` coverage is insufficient.
