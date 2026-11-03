-- Operational sync queue, version 1. Local invocation metadata; never credentials.
CREATE TABLE jobs (
 id TEXT PRIMARY KEY,
 spec TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('queued','running','accepted','failed','interrupted')),
 database_id TEXT NOT NULL DEFAULT '', dataset_id TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0, accepted INTEGER NOT NULL DEFAULT 0,
 receipts TEXT NOT NULL DEFAULT '[]',
 error_code TEXT NOT NULL DEFAULT '', created_ms INTEGER NOT NULL, updated_ms INTEGER NOT NULL
);
CREATE INDEX jobs_pending ON jobs(state,created_ms);
PRAGMA application_id = 1414091594;
PRAGMA user_version = 1;
