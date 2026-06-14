# Explore Pi Session Durable Sources

Type: HITL

Blocked by: None

Status: Evidence drafted

## What to explore

Research Pi's local session Durable Sources and define parser requirements for `tokeninsights sync --harness pi`.

TokScale lead: Pi uses JSONL session data under `~/.pi/agent/sessions/`.

Oh My Pi data under `~/.omp/agent/sessions/` is explicitly out of scope.

## Evidence packet

- [x] List the Pi session root and discovery rules to support.
- [x] Document session directory and file naming patterns without storing full paths.
- [x] Document the JSONL header shape and message entry shape using redacted minimal examples.
- [x] Document where assistant token usage lives.
- [x] Document session identity and message/fact identity.
- [x] Document provider and model extraction.
- [x] Document timestamp fallback behavior when entry timestamps are missing or invalid.
- [x] Identify malformed-line and missing-field diagnostics.
- [x] Identify private fields that must never enter raw storage.
- [x] Propose minimal JSONL fixtures and expected raw/canonical/diagnostic outputs.

## Read-only evidence

### Source locations and discovery

Support Pi JSONL session files under the Pi session directory only:

- Default root: `~/.pi/agent/sessions`.
- Candidate session files: `*.jsonl` one directory below the session root.
- Observed layout: `~/.pi/agent/sessions/<encoded-cwd>/<iso-session-start>_<session-id>.jsonl`.
- Ignore Oh My Pi data under `~/.omp/agent/sessions/`.
- Ignore non-JSONL files and directories below the expected one-level workspace/session layout for the first parser.
- Sort discovered files by path before parsing so diagnostics and ingest runs are deterministic.
- For explicit source overrides, accept either a matching JSONL file or a directory containing matching Pi JSONL session files.

Observed local sample, summarized without recording full paths or transcript values:

- 173 Pi JSONL session files.
- 0 non-JSONL files under the Pi session root.
- Directory names encode the working directory. Treat them as private locators, not domain data.
- Filenames use an ISO-like timestamp prefix plus a session ID suffix. The session header `id` matched the filename suffix for all observed files.

Recommended `Source` envelope:

- `source_kind`: `pi-session-jsonl`
- ingest-run `source_id`: stable hash of the root-relative file locator, so each physical file parse has audit provenance without storing the full path
- raw-fact `source_id`: stable hash of `pi-session:<session_id>`, so copied or relocated session files dedupe by logical session rather than by path

### JSONL record shapes

Pi session files are newline-delimited JSON. Observed top-level record types:

| Record type | Count in local sample | Top-level keys |
|-------------|-----------------------|----------------|
| `session` | 173 | `type`, `version`, `id`, `timestamp`, `cwd` |
| `model_change` | 214 | `type`, `id`, `parentId`, `timestamp`, `provider`, `modelId` |
| `thinking_level_change` | 227 | `type`, `id`, `parentId`, `timestamp`, `thinkingLevel` |
| `custom` | 386 | `type`, `customType`, `data`, `id`, `parentId`, `timestamp` |
| `custom_message` | 329 | `type`, `customType`, `content`, `display`, `id`, `parentId`, `timestamp` |
| `message` | 10,413 | `type`, `id`, `parentId`, `timestamp`, `message` |

Minimal redacted session header:

```json
{
  "type": "session",
  "version": 1,
  "id": "00000000-0000-0000-0000-000000000000",
  "timestamp": "2026-01-01T00:00:00.000Z",
  "cwd": "/redacted/project"
}
```

Minimal redacted model-change entry:

```json
{
  "type": "model_change",
  "id": "evt00001",
  "parentId": "evt00000",
  "timestamp": "2026-01-01T00:00:01.000Z",
  "provider": "anthropic",
  "modelId": "claude-sonnet-4"
}
```

Minimal redacted assistant message with token usage:

```json
{
  "type": "message",
  "id": "msg00001",
  "parentId": "evt00001",
  "timestamp": "2026-01-01T00:00:02.000Z",
  "message": {
    "role": "assistant",
    "content": [],
    "api": "provider-api",
    "provider": "anthropic",
    "model": "claude-sonnet-4",
    "usage": {
      "input": 100,
      "output": 50,
      "cacheRead": 20,
      "cacheWrite": 5,
      "totalTokens": 175,
      "cost": {
        "input": 0,
        "output": 0,
        "cacheRead": 0,
        "cacheWrite": 0,
        "total": 0
      }
    },
    "stopReason": "stop",
    "timestamp": 1767225602000,
    "responseId": "redacted"
  }
}
```

Observed message role coverage:

- Assistant messages with token usage: 4,386.
- User messages without token usage: 681.
- Tool result messages without token usage: 5,346.
- All observed token usage rows were assistant messages.
- Some assistant token rows had `errorMessage` or `diagnostics` fields and still contained usable token usage. Treat token usage as countable when token fields are valid; do not reject only because an error metadata field exists.

### Token usage rows

Assistant token facts live on `message` records where:

```text
type == "message"
message.role == "assistant"
message.usage is an object
```

Observed token usage shape:

```json
{
  "input": 100,
  "output": 50,
  "cacheRead": 20,
  "cacheWrite": 5,
  "totalTokens": 175,
  "cost": {
    "input": 0,
    "output": 0,
    "cacheRead": 0,
    "cacheWrite": 0,
    "total": 0
  }
}
```

Mapping to `RawTokenFact`:

| Raw field | Pi source |
|-----------|-----------|
| `session_id` | `session.id` from the file header; fallback to filename session suffix only when the header is absent or malformed |
| `message_id` | top-level message record `id`, when present |
| `provider` | `message.provider`, null when missing |
| `model` | `message.model`, null when missing |
| `occurred_at_ms` | `message.timestamp` when it is a valid epoch millisecond number; fallback to parsed top-level message `timestamp` |
| `usage_scope` | `message` |
| `quality` | `exact` |
| `input_tokens` | `message.usage.input` |
| `output_tokens` | `message.usage.output` |
| `reasoning_tokens` | null; Pi does not expose a separate reasoning token field in observed durable usage |
| `cache_read_tokens` | `message.usage.cacheRead` |
| `cache_write_tokens` | `message.usage.cacheWrite` |
| `total_tokens` | `message.usage.totalTokens` |

Observed token fields were numeric and non-negative in the local sample. `totalTokens` equaled `input + output + cacheRead + cacheWrite` for every observed assistant token row. Preserve `totalTokens` as the source-provided exact total; canonical normalization already falls back to component sums when `total_tokens` is null.

Do not store cost. Cost tracking is out of scope for TokenInsights V1.

### Identity and dedupe

Stable identities:

- Canonical session identity: `pi` plus `session.id`.
- Canonical message identity: `pi` plus canonical session plus top-level message `id`.
- Raw fact identity: harness, logical Pi session source ID, session ID, message ID when present, occurred timestamp, usage scope, parser version, and token components.

Observed local assistant token message IDs were unique across the sampled usage rows. Message IDs were short opaque strings in the local sample; treat them as opaque IDs and do not impose UUID validation.

If a session file is copied under another encoded working-directory folder, the parser should still dedupe because raw token facts use a logical source ID derived from `session.id`, not a full path or root-relative path.

If the header `session.id` and filename suffix disagree:

- Prefer the header `session.id` for canonical identity.
- Emit `pi_jsonl_session_id_mismatch`.
- Do not store the full path, encoded directory name, or raw filename in the diagnostic.

If the header is absent but the filename suffix matches the expected session ID shape, use the filename suffix as a fallback session ID and emit `pi_jsonl_missing_session_header`.

### Timestamp behavior

Timestamp precedence for assistant token facts:

1. `message.timestamp` when it is a finite epoch millisecond number.
2. Top-level message record `timestamp` when it is a parseable timestamp string.
3. Otherwise skip the token fact and emit `pi_jsonl_missing_time`.

Do not fall back to ingest time or file modification time. Canonical token semantic keys include recorded time, so nondurable fallback times would make repeated syncs unstable.

The session header timestamp and filename timestamp are session-start metadata only. Do not use them as per-message occurrence times.

### Provider and model

For assistant token facts:

- Use `message.provider` and `message.model`.
- Preserve missing provider/model as null in raw facts.
- Normalization renders missing provider/model as `unknown`.
- Do not emit parser diagnostics only because provider or model is absent.

Observed `model_change` records also contain `provider` and `modelId`. They are useful context, but the first Pi durable parser should not require stateful provider/model reconstruction because all observed assistant token usage rows already included `message.provider` and `message.model`.

If future fixtures prove assistant token rows can omit provider/model while prior `model_change` rows provide them, add stateful fallback as a follow-up parser improvement.

### Diagnostics

Emit diagnostics for:

- `pi_jsonl_parse_error`: skipped unparsable JSON line.
- `pi_jsonl_missing_session_header`: file has no usable `session` header but filename provided a fallback session ID.
- `pi_jsonl_missing_session`: assistant token row cannot resolve a stable session ID from header or filename.
- `pi_jsonl_session_id_mismatch`: header session ID and filename session suffix disagree.
- `pi_jsonl_missing_message_id`: assistant token row has no usable message ID; ingest the fact only if session, time, and token fields are otherwise usable.
- `pi_jsonl_missing_time`: assistant token row has no usable `message.timestamp` or top-level timestamp.
- `pi_jsonl_missing_tokens`: assistant row has no usable token component fields.
- `pi_jsonl_invalid_tokens`: token fields exist but are non-numeric.
- `pi_jsonl_negative_tokens`: token fields were present but negative and clamped to zero.

Missing provider or model should not be a parser diagnostic.

Ignore non-token record types without diagnostics:

- `session`
- `model_change`
- `thinking_level_change`
- `custom`
- `custom_message`
- user `message`
- tool result `message`

### Privacy review

Never store these source fields or JSON paths in raw storage or diagnostics:

- `session.cwd`
- encoded working-directory folder names
- full JSONL paths
- raw filenames when they include user/project path context
- `message.content` wholesale
- `message.content[].text`
- `message.content[].thinking`
- `message.content[].thinkingSignature`
- `message.content[].textSignature`
- `message.content[].arguments`
- `message.toolCallId`
- `message.toolName`
- `message.responseId`
- `message.api`
- `message.usage.cost`
- `message.errorMessage`
- `message.diagnostics`
- `custom.data`
- `custom_message.content`
- `custom_message.display`
- prompt text, assistant text, tool arguments, tool output, request headers, secrets, or raw provider payloads

Allowed metadata-only values:

- token component counts
- top-level record type
- session ID
- message ID
- provider
- model
- top-level and message timestamps
- parser diagnostic code and bounded metadata such as line number, if needed

### Fixture proposal

Create minimal JSONL fixtures under the existing conformance source tree using only synthetic metadata:

```jsonl
{"type":"session","version":1,"id":"pi_s1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/redacted/project"}
{"type":"model_change","id":"evt_model","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","provider":"anthropic","modelId":"claude-sonnet-4"}
{"type":"message","id":"msg_a","parentId":"evt_model","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-sonnet-4","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":5,"totalTokens":175,"cost":{"total":0}},"stopReason":"stop","timestamp":1767225602000,"responseId":"redacted"}}
{"type":"message","id":"msg_b","parentId":"msg_a","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"assistant","content":[],"usage":{"input":10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":15},"stopReason":"stop","timestamp":1767225603000}}
{"type":"message","id":"msg_user","parentId":"msg_b","timestamp":"2026-01-01T00:00:04.000Z","message":{"role":"user","content":[],"timestamp":1767225604000}}
{"type":"message","id":"msg_tool","parentId":"msg_b","timestamp":"2026-01-01T00:00:05.000Z","message":{"role":"toolResult","content":[],"timestamp":1767225605000}}
```

Additional negative fixtures:

- Missing session header with filename fallback; expect a warning diagnostic and valid raw/canonical facts.
- Header/filename session mismatch; expect mismatch diagnostic and header session ID used.
- Assistant token row with missing provider/model; expect raw provider/model null and canonical provider/model `unknown`.
- Assistant token row with invalid or missing `message.timestamp` but valid top-level timestamp; expect top-level timestamp fallback.
- Assistant token row with no usable timestamp; expect skip plus `pi_jsonl_missing_time`.
- Assistant token row with non-numeric token fields; expect skip plus `pi_jsonl_invalid_tokens`.
- Assistant token row with negative token fields; expect clamped counts plus `pi_jsonl_negative_tokens`.
- Malformed JSONL line; expect `pi_jsonl_parse_error`.

Expected raw output:

- Countable `message` facts for valid assistant token rows only.
- `source_kind = "pi-session-jsonl"`.
- Logical raw `source_id` derived from `session.id`, not the full path.
- Provider/model null preserved when absent.
- `reasoning_tokens` null because Pi does not expose a separate durable reasoning field.
- `total_tokens` set from `message.usage.totalTokens`.
- `metadata_json` null unless a bounded line-number diagnostic requires metadata outside raw facts.

Expected canonical output:

- Canonical sessions for rows with stable `session_id`.
- Canonical messages when message IDs exist.
- Provider/model `unknown` for missing raw provider/model.
- Total tokens from `total_tokens` when present.
- Repeated syncs do not insert duplicate raw or canonical token facts.

Expected diagnostics:

- Missing stable session identity.
- Missing session header with filename fallback.
- Header/filename session mismatch.
- Missing message ID warning.
- Missing or invalid timestamp.
- Missing, invalid, or negative token fields.
- Malformed JSONL line.

## Acceptance criteria

- [x] The issue proves a metadata-only mapping from Pi JSONL entries to `RawTokenFact`.
- [x] The issue defines stable source IDs without storing full source paths.
- [x] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [x] The issue defines whether missing provider/model should preserve null in raw and normalize to `unknown`.
- [x] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [x] No parser implementation or schema change is performed as part of exploration.

## Schema assessment

The current TokenInsights schema is sufficient for Pi assistant message-level token usage:

- `raw_token_usage` can represent session ID, message ID, provider/model, token components, source total, quality, scope, and observed/occurred time.
- `canonical_token_usage` can represent normalized provider/model and source total tokens.
- `normalization_diagnostics` can represent missing-session and parser warnings without storing private source content.

Follow-up work, not required for this parser:

- Canonical request-duration or TPS facts if Pi timing should feed the TPS tab later.
- Stateful provider/model fallback from `model_change` records if future Pi versions omit `message.provider` or `message.model` on assistant token rows.
- First-class cost tracking if TokenInsights later expands beyond V1 token usage.
