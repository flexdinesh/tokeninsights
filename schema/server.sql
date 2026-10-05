PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS server_metadata (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 database_id TEXT NOT NULL UNIQUE,
 owner_id TEXT NOT NULL,
 identity_version INTEGER NOT NULL CHECK (identity_version = 1),
 semantics_version INTEGER NOT NULL CHECK (semantics_version = 1),
 revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0 AND revision <= 9007199254740991),
 last_ingestion_at_ms INTEGER NOT NULL DEFAULT 0 CHECK (last_ingestion_at_ms >= 0 AND last_ingestion_at_ms <= 9007199254740991),
 created_at_ms INTEGER NOT NULL CHECK (created_at_ms >= 0 AND created_at_ms <= 9007199254740991)
);

CREATE TABLE IF NOT EXISTS usage_locations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  directory_key TEXT,
  directory_name TEXT,
  repository_key TEXT,
  repository_name TEXT,
  repository_source TEXT
);

CREATE INDEX IF NOT EXISTS usage_locations_repository_idx ON usage_locations (repository_key);
CREATE INDEX IF NOT EXISTS usage_locations_directory_idx ON usage_locations (directory_key);

CREATE TABLE IF NOT EXISTS canonical_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  session_id TEXT NOT NULL,
  first_seen_at_ms INTEGER NOT NULL CHECK (typeof(first_seen_at_ms) = 'integer' AND first_seen_at_ms BETWEEN 0 AND 253402214399999),
  last_seen_at_ms INTEGER NOT NULL CHECK (typeof(last_seen_at_ms) = 'integer' AND last_seen_at_ms BETWEEN 0 AND 253402214399999),
  CHECK (last_seen_at_ms >= first_seen_at_ms)
);

CREATE INDEX IF NOT EXISTS canonical_sessions_harness_session_idx ON canonical_sessions (harness, session_id);

CREATE TABLE IF NOT EXISTS canonical_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  session_id INTEGER NOT NULL,
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  harness_message_id TEXT NOT NULL,
  occurred_at_ms INTEGER NOT NULL CHECK (typeof(occurred_at_ms) = 'integer' AND occurred_at_ms BETWEEN 0 AND 253402214399999),
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS canonical_messages_session_idx ON canonical_messages (session_id);

CREATE TABLE IF NOT EXISTS canonical_token_usage (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  semantic_key TEXT NOT NULL UNIQUE,
  recorded_at_ms INTEGER NOT NULL CHECK (typeof(recorded_at_ms) = 'integer' AND recorded_at_ms BETWEEN 0 AND 253402214399999),
  harness TEXT NOT NULL CHECK (harness IN ('opencode', 'pi', 'codex', 'claude-code')),
  session_id INTEGER NOT NULL,
  message_id INTEGER,
  provider TEXT NOT NULL DEFAULT 'unknown',
  provider_source TEXT NOT NULL DEFAULT 'unknown' CHECK (provider_source IN ('explicit', 'inferred', 'unknown')),
  model TEXT NOT NULL DEFAULT 'unknown',
  usage_scope TEXT NOT NULL,
  quality TEXT NOT NULL CHECK (quality IN ('exact', 'derived', 'estimated')),
  is_countable INTEGER NOT NULL CHECK (is_countable IN (0, 1)),
  input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0 AND input_tokens <= 9007199254740991),
  output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0 AND output_tokens <= 9007199254740991),
  reasoning_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0 AND reasoning_tokens <= 9007199254740991),
  cache_read_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0 AND cache_read_tokens <= 9007199254740991),
  cache_write_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0 AND cache_write_tokens <= 9007199254740991),
  total_tokens INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0 AND total_tokens <= 9007199254740991),
  payload_hash TEXT NOT NULL,
  revision_rule TEXT NOT NULL DEFAULT '',
  revision_value INTEGER NOT NULL DEFAULT 0 CHECK (typeof(revision_value) = 'integer' AND revision_value BETWEEN 0 AND 253402214399999),
  location_id INTEGER,
  FOREIGN KEY (session_id) REFERENCES canonical_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY (message_id) REFERENCES canonical_messages(id) ON DELETE SET NULL,
  FOREIGN KEY (location_id) REFERENCES usage_locations(id),
  CHECK (total_tokens = input_tokens + output_tokens + reasoning_tokens + cache_read_tokens + cache_write_tokens)
);

CREATE INDEX IF NOT EXISTS canonical_token_usage_time_idx ON canonical_token_usage (recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_session_time_idx ON canonical_token_usage (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_harness_time_idx ON canonical_token_usage (harness, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_provider_model_time_idx ON canonical_token_usage (provider, model, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_countable_time_idx ON canonical_token_usage (is_countable, recorded_at_ms);
CREATE INDEX IF NOT EXISTS canonical_token_usage_location_time_idx ON canonical_token_usage (location_id, recorded_at_ms);

CREATE TABLE IF NOT EXISTS ingestion_receipts (
 stream_id TEXT NOT NULL,
 batch_id TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 first_sequence INTEGER NOT NULL CHECK (first_sequence > 0),
 last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
 receipt_json TEXT NOT NULL CHECK (json_valid(receipt_json)),
 committed_at_ms INTEGER NOT NULL CHECK (committed_at_ms >= 0 AND committed_at_ms <= 9007199254740991),
 inserted_count INTEGER NOT NULL CHECK (inserted_count >= 0 AND inserted_count <= 9007199254740991),
 updated_count INTEGER NOT NULL CHECK (updated_count >= 0 AND updated_count <= 9007199254740991),
 noop_count INTEGER NOT NULL CHECK (noop_count >= 0 AND noop_count <= 9007199254740991),
 revision INTEGER NOT NULL CHECK (revision >= 0 AND revision <= 9007199254740991),
 PRIMARY KEY (stream_id, batch_id)
);

CREATE TABLE IF NOT EXISTS ingestion_producers (
 stream_id TEXT PRIMARY KEY,
 hostname TEXT NOT NULL DEFAULT '',
 last_ingestion_at_ms INTEGER NOT NULL CHECK (last_ingestion_at_ms >= 0 AND last_ingestion_at_ms <= 9007199254740991)
);

PRAGMA application_id = 1414091606;
PRAGMA user_version = 2;
