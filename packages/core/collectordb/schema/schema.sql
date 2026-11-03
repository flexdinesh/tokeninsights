PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS evidence_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  stream_id TEXT NOT NULL UNIQUE,
  extractor_version INTEGER NOT NULL CHECK (extractor_version = 1),
  created_at_ms INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS evidence_sources (
  source_key TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  lineage TEXT NOT NULL,
  format TEXT NOT NULL,
  extractor_version INTEGER NOT NULL,
  byte_offset INTEGER NOT NULL CHECK (byte_offset >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  prefix_hash TEXT NOT NULL,
  context_json TEXT NOT NULL CHECK (json_valid(context_json)),
  updated_at_ms INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS evidence_outbox (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  observation_key TEXT NOT NULL UNIQUE,
  harness TEXT NOT NULL,
  record_json TEXT NOT NULL CHECK (json_valid(record_json)),
  created_at_ms INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS evidence_destinations (
  destination_id TEXT PRIMARY KEY,
  endpoint TEXT NOT NULL,
  database_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256),
  acknowledged_sequence INTEGER NOT NULL DEFAULT 0,
  acknowledged_at_ms INTEGER
);
CREATE TABLE IF NOT EXISTS evidence_batches (
  batch_id TEXT PRIMARY KEY,
  destination_id TEXT NOT NULL REFERENCES evidence_destinations(destination_id),
  stream_id TEXT NOT NULL,
  database_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256),
  protocol_version INTEGER NOT NULL DEFAULT 3 CHECK (protocol_version = 3),
  first_sequence INTEGER NOT NULL,
  last_sequence INTEGER NOT NULL,
  request_hash TEXT NOT NULL,
  request_bytes BLOB NOT NULL,
  receipt_bytes BLOB,
  acknowledged_at_ms INTEGER,
  CHECK ((receipt_bytes IS NULL) = (acknowledged_at_ms IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS evidence_batches_pending_idx ON evidence_batches(destination_id) WHERE receipt_bytes IS NULL;
CREATE TRIGGER IF NOT EXISTS evidence_outbox_immutable_update BEFORE UPDATE ON evidence_outbox BEGIN SELECT RAISE(ABORT, 'evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS evidence_outbox_immutable_delete BEFORE DELETE ON evidence_outbox BEGIN SELECT RAISE(ABORT, 'evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS evidence_batches_immutable BEFORE UPDATE OF batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes ON evidence_batches BEGIN SELECT RAISE(ABORT, 'evidence request is immutable'); END;
PRAGMA application_id = 1414091587;
-- Durable capture quarantine. Hashes only; no source paths or native content.
CREATE TABLE IF NOT EXISTS evidence_quarantine (
  source_key TEXT PRIMARY KEY CHECK (length(source_key) = 64 AND source_key NOT GLOB '*[^0-9a-f]*'),
  format TEXT NOT NULL CHECK (format IN ('codex-jsonl', 'pi-jsonl', 'claude-code-jsonl', 'opencode-sqlite')),
  signature TEXT NOT NULL CHECK (length(signature) = 64 AND signature NOT GLOB '*[^0-9a-f]*'),
  parser_version INTEGER NOT NULL CHECK (typeof(parser_version) = 'integer' AND parser_version > 0),
  code TEXT NOT NULL CHECK (code IN ('source_record_limit', 'invalid_source_record')),
  byte_offset INTEGER NOT NULL CHECK (typeof(byte_offset) = 'integer' AND byte_offset >= 0),
  recorded_at_ms INTEGER NOT NULL CHECK (typeof(recorded_at_ms) = 'integer' AND recorded_at_ms >= 0)
);
PRAGMA user_version = 20;
