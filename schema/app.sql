-- Application SQLite schema, version 1. No token usage or source evidence.
CREATE TABLE application_metadata (
 id INTEGER PRIMARY KEY CHECK(id=1),
 role TEXT NOT NULL CHECK(role='application'),
 database_id TEXT NOT NULL,
 instance_id TEXT NOT NULL,
 server_kind TEXT NOT NULL CHECK(server_kind IN ('personal','hosted')),
 legacy_accounts_imported INTEGER NOT NULL DEFAULT 0 CHECK(legacy_accounts_imported IN (0,1))
);
CREATE TABLE users (
 user_id TEXT PRIMARY KEY, dataset_id TEXT UNIQUE NOT NULL,
 display_name TEXT NOT NULL, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at TEXT NOT NULL,
 provisioning TEXT NOT NULL CHECK(provisioning IN ('pending','ready'))
);
CREATE TABLE tokens (
 token_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(user_id),
 digest TEXT UNIQUE NOT NULL, permissions TEXT NOT NULL,
 created_at TEXT NOT NULL, expires_at TEXT, revoked_at TEXT
);
CREATE TABLE sessions (
 session_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(user_id),
 source_token_id TEXT NOT NULL REFERENCES tokens(token_id) ON DELETE CASCADE,
 digest TEXT UNIQUE NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, revoked_at TEXT
);
PRAGMA application_id = 1414091585;
PRAGMA user_version = 1;
