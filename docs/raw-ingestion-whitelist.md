# Raw ingestion whitelist

Approved direction: [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md).
Implemented in the evidence contract and collector extractor. This list comes from fields recognized by the current
TokenInsights adapters, not from collecting private user transcripts. It governs
the implemented evidence contract, distinct from retained legacy normalized requests.

## Rules

Extract only the scalar leaves listed below plus their required object containers.
Listing `usage`, `tokens`, `payload` or `message` never permits the complete
object. Preserve absent fields, explicit null, zero, aliases and original numeric
values without clamping, subtraction, summation or inference. Preserve native
timestamp values/units; normalization happens on the server. Numeric extraction
must avoid floating-point rounding. Preserve safe scalar type mismatches for
diagnosis, but replace unexpected object/array values with a fixed type-error
marker; do not upload their contents. Do not forward arbitrary diagnostic text.

Bound strings and records. Native IDs, models and providers are expected metadata
strings, not free text; values containing paths, URLs, credentials, control
characters or other disallowed structure must be withheld with a fixed reason
code, never silently truncated into another identity. Field allowlisting reduces
privacy exposure; it cannot guarantee legitimate model labels or IDs contain no
personal information. Unknown fields are dropped locally. Server validation
rejects unknown transport fields before storing anything.

Select usage-bearing assistant records and the context records listed below.
Missing identity, time or unusable counters on an otherwise eligible record must
not discard all safe evidence; the server retains and diagnoses it. Keep context
linked to its source observation/order so incremental batches retain attribution.
Extract both supported aliases independently, including contradictory values.

## OpenCode: SQLite

Source: [opencode_sqlite.go](../packages/cli/internal/pipeline/opencode_sqlite.go).

| Source record | Allowed transmitted leaves |
| --- | --- |
| V1 `message`, assistant role | Columns `id`, `session_id`, `time_created`; `data.role`, `data.modelID`, `data.providerID` |
| V2 `session_message`, assistant type | Columns `id`, `session_id`, `type`, `time_created`; `data.model.id`, `data.model.providerID` |
| Either message representation | `data.tokens.input`, `.output`, `.reasoning`, `.cache.read`, `.cache.write`; `data.time.created`, `.completed` |
| Session/project context | `session.id`, `session.project_id`; referenced `project.id`, `project.vcs` restricted to the recognized `git` marker |

The `data` column is never sent as a whole. Native millisecond timestamps stay
unchanged. Preserve V1/V2 origin and both safe snapshots where the tables overlap;
the server applies the supported native-session/message precedence, rather than
counting both. Current parsing recognizes completion time; retention does not
activate a new timing metric. `session.directory` is local-only enrichment input.
Project IDs are metadata identifiers subject to the ID validation rule above.

## Pi: JSONL

Source: [pi_jsonl.go](../packages/cli/internal/pipeline/pi_jsonl.go).

| Source record | Allowed transmitted leaves |
| --- | --- |
| Session header | `type` = `session`, native `id` |
| Assistant message with usage | `type` = `message`, root `id`, root `timestamp`; `message.role`, `.timestamp`, `.provider`, `.model` |
| Usage leaves | `message.usage.input`, `.output`, `.reasoning`, `.cacheRead`, `.cacheWrite`, `.totalTokens` |

Keep both the root RFC3339 timestamp and native message millisecond timestamp
where supplied; do not choose or convert them in the Collector. Header `cwd` is
local-only. Filename-derived session fallback is not a native header ID: retain
a fixed missing-header reason and opaque local source-instance reference instead
of uploading the filename as a claimed stable session ID. Message content, cost
objects and unrecognized session fields are excluded.

## Claude Code: JSONL

Source:
[claude_code_jsonl.go](../packages/cli/internal/pipeline/claude_code_jsonl.go).

| Source record | Allowed transmitted leaves |
| --- | --- |
| Assistant record with usage | `type` = `assistant`, root `timestamp`, `uuid`, `sessionId`, `session_id`, `requestId`, `request_id`; `message.role`, `message.id` |
| Attribution aliases | `message.provider`, `.provider_id`, `.providerID`; `message.model`, `.model_id`, `.modelID` |
| Usage leaves | `message.usage.input_tokens`, `.output_tokens`, `.cache_read_input_tokens`, `.cache_creation_input_tokens`, `.total_tokens` |
| Reasoning leaves | `message.usage.output_tokens_details.thinking_tokens`, `.reasoning_tokens` |

Preserve source timestamp strings and complete snapshots separately; streaming
merge and equal-time conflict handling move to the server. UUID fallback is
evidence, not a universal request dedupe key. Preserve missing provider as absent;
`maybe-anthropic` is server inference. Root `cwd` and filename fallback remain
local-only. `message.content`, tool fields and other transcript metadata are
excluded. No new parent/subagent fields are implied by this whitelist.

## Codex: JSONL

Sources: [codex_jsonl.go](../packages/cli/internal/pipeline/codex_jsonl.go) and
[codex_replay.go](../packages/cli/internal/pipeline/codex_replay.go).

| Source record | Allowed transmitted leaves |
| --- | --- |
| Every selected record | Root `type`, `timestamp` |
| `session_meta` | `payload.id`, `.model_provider`, `.forked_from_id`, `.parent_thread_id`; `payload.source.subagent.thread_spawn.parent_thread_id` |
| `turn_context` | `payload.turn_id`, `.model` |
| `event_msg` / `task_started` | `payload.type`, `.turn_id` |
| `event_msg` / `token_count` | `payload.type`; the usage leaves below under both `payload.info.last_token_usage` and `.total_token_usage` |
| Each usage snapshot | `input_tokens`, `cached_input_tokens`, `cache_read_input_tokens`, `output_tokens`, `reasoning_output_tokens`, `total_tokens` |

The adapter additionally inspects whether `payload.source.subagent` is non-null,
even without a usable parent. Preserve this as a fixed presence/type marker in
the extraction envelope; never send an arbitrary `source` or `subagent` object.
Parent aliases remain separate, including conflicting values. Preserve the
complete original last/cumulative counter snapshots and both cache aliases.
Cumulative usage is copy/context evidence, not another additive contribution.

Original record order and contextual associations are required because some
token rows precede model/turn context and copied ancestry may rewrite timestamps.
Neither a line number nor a matching snapshot alone proves global usage identity.
Root filenames and `payload.cwd` remain local-only, as does
`payload.git.repository_url`. `response_item`, conversations, prompts, tools,
rate limits and every unlisted payload field are excluded.

## Safe extraction envelope and local-only enrichment

Allowed operational metadata: contract/extractor version, harness/source-format
enum, opaque source-instance and lineage references, observation ordinal,
native-ID presence/type markers, Collector stream/batch identity and sequence,
capture time, sanitized-evidence/request checksum, and fixed diagnostic codes.
These support delivery and interpretation; they do not become consumption IDs.
Do not transmit filesystem paths, inode/mtime refresh state, arbitrary host
labels, environment values or unsanitized parse errors. Captured context is sent
as referenced evidence rather than flattened into inferred facts.

Current location handling is in
[location.go](../packages/cli/internal/pipeline/location.go), with normalized
publication in
[publication](../packages/cli/internal/publication/). It reads local directories,
Git checkout metadata and recorded remote URLs. Those inputs stay local. If
location grouping is retained, the raw envelope may carry explicitly marked
extraction enrichment: opaque directory/repository keys, basename-only bounded
display labels and a fixed origin enum (`harness`, `git-remote`, `git-common-dir`,
`opencode-project`, or unavailable). Credentials, URL owners/paths and full
directory labels are never transmitted. The existing sanitized directory label
is not sufficient when it contains a path; use its basename for this protocol.

Opaque location keys are grouping hints, not usage or ownership identities.
Path-based local keys need not match across hosts. Preserve native references
and competing safe enrichments; the server resolves conflicts without using
arrival order. Hashing does not make metadata anonymous, and basename labels can
still identify a project. No arbitrary `metadata_json` extension is permitted.

## Excluded data and replay limit

Never retain/upload prompt or assistant text, tool arguments/output, content
blocks, complete provider/request payloads, request headers, API keys, secrets,
full source/working-directory paths, full repository URLs or raw malformed lines.
An unexpected shape at an allowed leaf is not permission to capture its children.

This whitelist supports the interpretations already implemented by TokenInsights.
Future parsing that needs an excluded field requires an explicit privacy/contract
decision and new collection; historical server evidence cannot recover fields
never captured. Server replay corrects processing, not defective extraction.
