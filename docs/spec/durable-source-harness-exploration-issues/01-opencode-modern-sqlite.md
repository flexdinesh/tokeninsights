# Explore OpenCode Modern SQLite Durable Sources

Type: HITL

Blocked by: None

Status: Evidence drafted

## What to explore

Research OpenCode's modern SQLite Durable Sources and define the parser requirements needed for `tokeninsights sync --harness opencode` to ingest durable token usage safely and idempotently.

TokScale lead: OpenCode uses SQLite databases under `~/.local/share/opencode/`, including `opencode.db` and channel variants such as `opencode-stable.db`.

Legacy OpenCode JSON message storage under `~/.local/share/opencode/storage/message/` is explicitly out of scope.

## Evidence packet

- [x] List the OpenCode database file names and discovery rules to support.
- [x] Document the relevant table schemas and joins, using redacted schema output only.
- [x] Document where assistant token usage lives.
- [x] Document session identity and message/fact identity.
- [x] Document provider and model extraction.
- [x] Document timestamp and optional duration extraction.
- [x] Identify dedupe risks, including forked or copied history rows.
- [x] Identify unavailable or ambiguous data that should become diagnostics.
- [x] Identify private fields or JSON paths that must never enter raw storage.
- [x] Propose minimal SQLite fixture data and expected raw/canonical/diagnostic outputs.

## Read-only evidence

### Source locations and discovery

Support modern SQLite databases under the OpenCode data directory:

- Default root: `${XDG_DATA_HOME:-~/.local/share}/opencode`.
- Candidate database names:
  - `opencode.db`.
  - `opencode-<channel>.db`, where `<channel>` is non-empty and contains only ASCII letters, digits, `.`, `_`, or `-`.
- Ignore sidecars and transient files such as `*.db-wal`, `*.db-shm`, and `*.db-journal`.
- Sort discovered DB paths before parsing so duplicate handling is deterministic.
- For explicit source overrides, accept either a matching DB file or a directory containing matching DB files.

Observed local examples, without recording full paths:

- `opencode.db`
- `opencode-local.db`

`opencode-local.db` fits the channel-suffixed filename rule with channel `local`; no separate filename rule is required.

Recommended `Source` envelope:

- `source_kind`: `opencode-sqlite`
- ingest-run `source_id`: stable hash of the DB locator
- raw-fact `source_id`: stable hash of the logical OpenCode data root, so copied facts seen in multiple channel DBs can dedupe while observations still point at per-DB ingest runs

### Relevant schema shape

Modern DBs contain a `message` table with the per-message JSON payload:

```sql
CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL,
  FOREIGN KEY (session_id) REFERENCES session(id) ON DELETE CASCADE
);
CREATE INDEX message_session_time_created_id_idx
  ON message (session_id, time_created, id);
```

Modern DBs contain a `session` table with stable session metadata and, in newer DBs, session-level aggregate token columns:

```sql
CREATE TABLE session (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  parent_id text,
  slug text NOT NULL,
  directory text NOT NULL,
  title text NOT NULL,
  version text NOT NULL,
  share_url text,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  time_archived integer,
  workspace_id text,
  path text,
  agent text,
  model text,
  cost real DEFAULT 0 NOT NULL,
  tokens_input integer DEFAULT 0 NOT NULL,
  tokens_output integer DEFAULT 0 NOT NULL,
  tokens_reasoning integer DEFAULT 0 NOT NULL,
  tokens_cache_read integer DEFAULT 0 NOT NULL,
  tokens_cache_write integer DEFAULT 0 NOT NULL,
  metadata text
);
```

Older or alternate channel DBs may omit the newer session columns after `workspace_id`.

Some DBs also contain `session_message` or `session_entry` tables:

```sql
CREATE TABLE session_message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  type text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);
```

```sql
CREATE TABLE session_entry (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  type text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);
```

Observed local counts showed assistant token rows in `message.data`; `session_message` and `session_entry` did not contain token rows in the inspected DBs. Treat those tables as out of scope for the first parser unless a fixture proves token facts live there.

### Token usage rows

Assistant token facts live in `message.data`, filtered by:

```sql
json_extract(message.data, '$.role') = 'assistant'
AND json_type(message.data, '$.tokens') IS NOT NULL
```

Observed JSON metadata shape:

```json
{
  "role": "assistant",
  "modelID": "model-id",
  "providerID": "provider-id",
  "tokens": {
    "input": 100,
    "output": 50,
    "reasoning": 10,
    "cache": {
      "read": 20,
      "write": 5
    }
  },
  "time": {
    "created": 1700000000000,
    "completed": 1700000000450
  }
}
```

Mapping to `RawTokenFact`:

| Raw field | OpenCode source |
|-----------|-----------------|
| `session_id` | `message.session_id` |
| `message_id` | embedded `$.id` when present, else `message.id` |
| `provider` | `$.providerID`, null when missing |
| `model` | `$.modelID`, null when missing |
| `occurred_at_ms` | `$.time.created` if present, else `message.time_created` |
| `usage_scope` | `message` |
| `quality` | `exact` |
| `input_tokens` | `$.tokens.input` |
| `output_tokens` | `$.tokens.output` |
| `reasoning_tokens` | `$.tokens.reasoning`, default `0` only when the `tokens` object exists |
| `cache_read_tokens` | `$.tokens.cache.read` |
| `cache_write_tokens` | `$.tokens.cache.write` |
| `total_tokens` | leave null; canonical normalization already computes a total from component columns |

Clamp negative token values to zero before writing raw facts. Negative token values should also produce a warning diagnostic because they indicate malformed source data.

Do not use `session.tokens_*` aggregates as countable facts while message-level token facts are available. They are cumulative session summaries and would double count. They may be useful later as non-countable fallback facts only if message rows are unavailable, but that requires a follow-up decision.

### Identity and dedupe

Stable identities:

- Canonical session identity: `opencode` plus `message.session_id`.
- Canonical message identity: `opencode` plus canonical session plus chosen message ID.
- Raw fact identity should include harness, logical OpenCode data-root source ID, chosen message ID, occurred timestamp, usage scope, parser version, and token components.

SQLite rows can duplicate copied history when sessions are forked or when users switch between channel DBs. The parser should suppress duplicates before writing raw token facts using a deterministic fingerprint:

- `time.created`
- `time.completed`
- `modelID`
- `providerID`
- input/output/reasoning/cache-read/cache-write token counts
- optional agent or mode when captured as metadata-only

When duplicates share the same fingerprint:

- Keep the first row in deterministic DB/path/query order.
- Prefer an embedded `$.id` over the SQLite row ID for message identity when available.
- Emit a warning or info diagnostic for suppressed fork/channel duplicates.

The first implementation should not store OpenCode cost. Cost tracking is out of scope for TokenInsights V1.

### Timestamp and timing

Use `$.time.created` as `occurred_at_ms` when present. Fall back to `message.time_created` only when `$.time.created` is absent.

`$.time.completed - $.time.created` can provide request duration in milliseconds when both values are finite and completed is greater than created. Current schema has no canonical timing fact table, so the OpenCode token parser should not store duration in `raw_token_usage.metadata_json` unless a later approved timing design needs it. Emit no diagnostic for absent duration.

### Diagnostics

Emit diagnostics for:

- `opencode_sqlite_missing_message_table`: DB does not contain the required `message` table.
- `opencode_sqlite_invalid_schema`: required `message` columns are missing.
- `opencode_sqlite_parse_error`: `message.data` is not valid JSON.
- `opencode_sqlite_missing_session`: assistant token row has no stable `message.session_id`.
- `opencode_sqlite_missing_time`: assistant token row has no usable created timestamp in JSON or column fallback.
- `opencode_sqlite_missing_tokens`: assistant row has no usable token component fields.
- `opencode_sqlite_negative_tokens`: token fields were present but negative and clamped.
- `opencode_sqlite_duplicate_suppressed`: forked or channel-copied history row was skipped by fingerprint dedupe.
- `opencode_sqlite_session_aggregate_ignored`: session-level aggregate token columns exist but message-level token facts are available.

Missing provider or model should not be a parser diagnostic. Preserve absence as null in raw facts; normalization renders `unknown`.

### Privacy review

Never store these source fields or JSON paths in raw storage or diagnostics:

- `session.directory`
- `session.path`
- `session.title`
- `session.slug`
- `session.share_url`
- `session.summary_diffs`
- `session.metadata`
- full DB path
- `message.data` wholesale
- prompt text, assistant text, tool arguments, tool output, request headers, secrets, or raw provider payloads

Allowed metadata-only values:

- DB filename or channel label, if needed for diagnostics.
- SQLite row ID only as message identity when no embedded message ID exists.
- Token component counts, provider, model, session ID, message ID, and timestamps.

### Fixture proposal

Create a minimal SQLite fixture under the existing conformance source tree, using only synthetic metadata:

```sql
CREATE TABLE session (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  slug text NOT NULL,
  directory text NOT NULL,
  title text NOT NULL,
  version text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL
);

CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);
```

Rows:

- `msg_a`: assistant token row with session `ses_a`, provider `anthropic`, model `claude-sonnet-4`, created time, completed time, and all token components.
- `msg_b`: assistant token row with session `ses_a`, missing provider/model, and token components; expected canonical provider/model `unknown`.
- `msg_user`: user row with no tokens; expected no raw fact and no diagnostic.
- `msg_no_session`: assistant token row with blank session; expected raw fact plus `missing_session` normalization diagnostic, or parser diagnostic if rejected before raw.
- `msg_fork_copy`: duplicate of `msg_a` in a second session; expected duplicate suppression diagnostic and no second countable raw fact.
- `msg_negative`: assistant token row with negative token components; expected clamped raw counts plus `opencode_sqlite_negative_tokens`.

Expected raw output:

- Countable `message` facts for valid assistant token rows only.
- `source_kind = "opencode-sqlite"`.
- Provider/model null preserved when absent.
- Total tokens null in raw facts.

Expected canonical output:

- Canonical sessions for rows with stable `session_id`.
- Canonical messages when message IDs exist.
- Provider/model `unknown` for missing raw provider/model.
- Total tokens computed from component columns.
- Repeated syncs do not insert duplicate raw or canonical token facts.

Expected diagnostics:

- Missing stable session identity.
- Duplicate suppressed for fork/channel copied rows.
- Negative tokens clamped.
- Optional session aggregate ignored when fixture includes session aggregate columns.

## Acceptance criteria

- [x] The issue proves a metadata-only mapping from OpenCode SQLite rows to `RawTokenFact`.
- [x] The issue defines stable source IDs without storing full database paths.
- [x] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [x] The issue defines how repeated syncs avoid duplicate token facts.
- [x] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [x] No parser implementation or schema change is performed as part of exploration.

## Schema assessment

The current TokenInsights schema is sufficient for OpenCode message-level token usage:

- `raw_token_usage` can represent session ID, message ID, provider/model, token components, quality, scope, and observed/occurred time.
- `canonical_token_usage` can represent normalized provider/model and computed totals.
- `normalization_diagnostics` can represent missing-session and parser warnings without storing private source content.

Follow-up work, not required for this parser:

- Canonical request-duration or TPS facts if OpenCode duration should feed the TPS tab later.
- A first-class representation for non-countable session aggregate fallback facts if future OpenCode versions omit message token rows.
