# Tool Runtime Duration Metrics - Future Design

## Summary

Sync-first V1 keeps token usage as the guaranteed canonical domain. Tool-call counts and durations remain a future canonical domain that should follow the same raw fact, observation, normalization, diagnostic, and viewer-query patterns as token usage.

## Goals

- Measure tool runtime by harness, session, provider, model, and tool name.
- Preserve started/completed/error semantics where source data exposes them.
- Support future `duration avg`, `duration mean`, and `duration median` columns.
- Tolerate harnesses that do not expose durable tool data.
- Keep raw storage metadata-only.

## Proposed Raw Semantics

A raw tool event fact should represent one observed tool lifecycle event:

- harness and source identity;
- observed and optional occurred timestamp;
- session identity, required for canonical analytics;
- optional message or turn identity;
- tool call identity when available;
- tool name, or null in raw if absent;
- provider/model when available;
- status: `started`, `completed`, `error`, or a future explicit terminal status;
- metadata-only provenance.

Started rows without a matching terminal row should count as attempted tool calls but should not contribute to duration metrics.

## Proposed Canonical Semantics

Add canonical tool fact tables rather than overloading `canonical_token_usage`:

```sql
CREATE TABLE IF NOT EXISTS canonical_tool_calls (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  recorded_at_ms INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex')),
  session_id INTEGER NOT NULL,
  message_id INTEGER,
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  tool_call_key TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'unknown',
  status TEXT NOT NULL CHECK (status IN ('started', 'completed', 'error')),
  is_countable INTEGER NOT NULL CHECK (is_countable IN (0, 1)),
  primary_raw_fact_id INTEGER NOT NULL,
  ingest_run_id INTEGER,
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY (message_id) REFERENCES canonical_messages(id) ON DELETE SET NULL,
  FOREIGN KEY (primary_raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE RESTRICT,
  FOREIGN KEY (ingest_run_id) REFERENCES ingest_runs(id) ON DELETE SET NULL
);
```

If duration can be derived reliably, add a dedicated sample table:

```sql
CREATE TABLE IF NOT EXISTS canonical_tool_runtime_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  recorded_at_ms INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex')),
  session_id INTEGER NOT NULL,
  message_id INTEGER,
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  tool_name TEXT NOT NULL DEFAULT 'unknown',
  status TEXT NOT NULL CHECK (status IN ('completed', 'error')),
  duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
  primary_raw_fact_id INTEGER NOT NULL,
  ingest_run_id INTEGER,
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY (message_id) REFERENCES canonical_messages(id) ON DELETE SET NULL,
  FOREIGN KEY (primary_raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE RESTRICT,
  FOREIGN KEY (ingest_run_id) REFERENCES ingest_runs(id) ON DELETE SET NULL
);
```

The foreign key to `raw_token_usage` is a placeholder until the raw layer has a generalized raw fact table or raw tool table.

## CLI Aggregation Plan

For total tool runtime metrics:

- attempted calls: count countable canonical tool `started` rows;
- errors: count terminal rows with `error`;
- duration average: `SUM(duration_ms) / COUNT(*)`;
- duration mean: `AVG(duration_ms)`;
- duration median: window CTE over `duration_ms`.

Group by day/hour/session plus harness, provider, and model. Tool breakdown additionally groups by `tool_name`.

Potential future columns:

```text
tool calls | errors | duration avg | duration mean | duration median
```

and for breakdown:

```text
tool | tool calls | errors | duration avg | duration mean | duration median
```

## Open Questions

- Should tool raw facts live in dedicated raw tables or a generalized raw fact envelope?
- Should blocked tool calls get a separate terminal status?
- Should cancelled tools be represented as `error`, `cancelled`, or both?
- Should durations include permission wait time, or only execution after permission is granted?
