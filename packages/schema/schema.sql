PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS ingest_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL UNIQUE,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  collector TEXT NOT NULL,
  parser TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
  started_at_ms INTEGER NOT NULL,
  completed_at_ms INTEGER,
  error_message TEXT,
  raw_fact_count INTEGER NOT NULL DEFAULT 0,
  observation_count INTEGER NOT NULL DEFAULT 0,
  canonical_count INTEGER NOT NULL DEFAULT 0,
  diagnostic_count INTEGER NOT NULL DEFAULT 0,
  CHECK (completed_at_ms IS NULL OR completed_at_ms >= started_at_ms)
);

CREATE INDEX IF NOT EXISTS ingest_runs_harness_time_idx ON ingest_runs (harness, started_at_ms);
CREATE INDEX IF NOT EXISTS ingest_runs_status_time_idx ON ingest_runs (status, started_at_ms);

CREATE TABLE IF NOT EXISTS raw_token_usage (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  raw_fact_key TEXT NOT NULL UNIQUE,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  source_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  collector TEXT NOT NULL,
  parser TEXT NOT NULL,
  observed_at_ms INTEGER NOT NULL,
  occurred_at_ms INTEGER,
  session_id TEXT,
  message_id TEXT,
  provider TEXT,
  model TEXT,
  usage_scope TEXT NOT NULL,
  quality TEXT NOT NULL CHECK (quality IN ('exact', 'derived', 'estimated')),
  input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_tokens INTEGER,
  cache_read_tokens INTEGER,
  cache_write_tokens INTEGER,
  total_tokens INTEGER,
  metadata_json TEXT,
  CHECK (input_tokens IS NULL OR input_tokens >= 0),
  CHECK (output_tokens IS NULL OR output_tokens >= 0),
  CHECK (reasoning_tokens IS NULL OR reasoning_tokens >= 0),
  CHECK (cache_read_tokens IS NULL OR cache_read_tokens >= 0),
  CHECK (cache_write_tokens IS NULL OR cache_write_tokens >= 0),
  CHECK (total_tokens IS NULL OR total_tokens >= 0)
);

CREATE INDEX IF NOT EXISTS raw_token_usage_harness_source_idx ON raw_token_usage (harness, source_id);
CREATE INDEX IF NOT EXISTS raw_token_usage_session_idx ON raw_token_usage (harness, session_id);
CREATE INDEX IF NOT EXISTS raw_token_usage_observed_idx ON raw_token_usage (observed_at_ms);

CREATE TABLE IF NOT EXISTS raw_observations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ingest_run_id INTEGER NOT NULL,
  raw_fact_id INTEGER NOT NULL,
  observed_at_ms INTEGER NOT NULL,
  observation_key TEXT NOT NULL UNIQUE,
  FOREIGN KEY (ingest_run_id) REFERENCES ingest_runs(id) ON DELETE CASCADE,
  FOREIGN KEY (raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS raw_observations_run_idx ON raw_observations (ingest_run_id);
CREATE INDEX IF NOT EXISTS raw_observations_fact_idx ON raw_observations (raw_fact_id);

CREATE TABLE IF NOT EXISTS canonical_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  session_id TEXT NOT NULL,
  first_seen_at_ms INTEGER NOT NULL,
  last_seen_at_ms INTEGER NOT NULL,
  primary_raw_fact_id INTEGER,
  FOREIGN KEY (primary_raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE SET NULL,
  CHECK (last_seen_at_ms >= first_seen_at_ms)
);

CREATE INDEX IF NOT EXISTS canonical_sessions_harness_session_idx ON canonical_sessions (harness, session_id);

CREATE TABLE IF NOT EXISTS canonical_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  session_id INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  harness_message_id TEXT NOT NULL,
  occurred_at_ms INTEGER,
  primary_raw_fact_id INTEGER,
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY (primary_raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS canonical_messages_session_idx ON canonical_messages (session_id);

CREATE TABLE IF NOT EXISTS canonical_token_usage (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  recorded_at_ms INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  session_id INTEGER NOT NULL,
  message_id INTEGER,
  provider TEXT NOT NULL DEFAULT 'unknown',
  provider_source TEXT NOT NULL DEFAULT 'unknown' CHECK (provider_source IN ('explicit', 'inferred', 'unknown')),
  model TEXT NOT NULL DEFAULT 'unknown',
  usage_scope TEXT NOT NULL,
  quality TEXT NOT NULL CHECK (quality IN ('exact', 'derived', 'estimated')),
  is_countable INTEGER NOT NULL CHECK (is_countable IN (0, 1)),
  input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  reasoning_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
  cache_read_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
  cache_write_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
  total_tokens INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
  primary_raw_fact_id INTEGER NOT NULL,
  ingest_run_id INTEGER,
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY (message_id) REFERENCES canonical_messages(id) ON DELETE SET NULL,
  FOREIGN KEY (primary_raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE RESTRICT,
  FOREIGN KEY (ingest_run_id) REFERENCES ingest_runs(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS canonical_token_usage_time_idx ON canonical_token_usage (recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_session_time_idx ON canonical_token_usage (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_harness_time_idx ON canonical_token_usage (harness, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_provider_model_time_idx ON canonical_token_usage (provider, model, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_countable_time_idx ON canonical_token_usage (is_countable, recorded_at_ms);

CREATE TABLE IF NOT EXISTS normalization_diagnostics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  diagnostic_key TEXT NOT NULL UNIQUE,
  recorded_at_ms INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  raw_fact_id INTEGER,
  ingest_run_id INTEGER,
  severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'error')),
  code TEXT NOT NULL,
  message TEXT NOT NULL,
  metadata_json TEXT,
  FOREIGN KEY (raw_fact_id) REFERENCES raw_token_usage(id) ON DELETE SET NULL,
  FOREIGN KEY (ingest_run_id) REFERENCES ingest_runs(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS normalization_diagnostics_harness_time_idx ON normalization_diagnostics (harness, recorded_at_ms);
CREATE INDEX IF NOT EXISTS normalization_diagnostics_code_idx ON normalization_diagnostics (code);

PRAGMA user_version = 5;
