-- Hosted application SQLite schema, version 3. No token usage or source evidence.
CREATE TABLE application_metadata (
 id INTEGER PRIMARY KEY CHECK(id=1),
 role TEXT NOT NULL CHECK(role='application'),
 database_id TEXT NOT NULL,
 instance_id TEXT NOT NULL,
 server_kind TEXT NOT NULL CHECK(server_kind='hosted')
);
CREATE TABLE users (
 user_id TEXT PRIMARY KEY, dataset_id TEXT UNIQUE NOT NULL,
 display_name TEXT NOT NULL, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at TEXT NOT NULL,
 provisioning TEXT NOT NULL CHECK(provisioning IN ('pending','ready')),
 username TEXT UNIQUE NOT NULL, password_hash TEXT
);
CREATE TABLE tokens (
 token_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(user_id),
 digest TEXT UNIQUE NOT NULL, permissions TEXT NOT NULL,
 created_at TEXT NOT NULL, expires_at TEXT, revoked_at TEXT,
 label TEXT NOT NULL
);
CREATE TABLE sessions (
 session_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(user_id),
 digest TEXT UNIQUE NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, revoked_at TEXT
);
PRAGMA application_id = 1414091585;
PRAGMA user_version = 3;
