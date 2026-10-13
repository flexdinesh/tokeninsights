CREATE SCHEMA tokeninsights_accounts;
SET LOCAL search_path TO tokeninsights_accounts;
-- PostgreSQL account schema, version 2. No token usage or source evidence.
CREATE TABLE application_metadata (
 id INTEGER PRIMARY KEY CHECK(id=1),
 schema_version INTEGER NOT NULL CHECK(schema_version=2),
 role TEXT COLLATE "C" NOT NULL CHECK(role='application'),
 database_id TEXT COLLATE "C" NOT NULL,
 instance_id TEXT COLLATE "C" NOT NULL,
 server_kind TEXT COLLATE "C" NOT NULL CHECK(server_kind='hosted')
);
CREATE TABLE users (
 user_id TEXT COLLATE "C" PRIMARY KEY, dataset_id TEXT COLLATE "C" UNIQUE NOT NULL,
 display_name TEXT COLLATE "C" NOT NULL, enabled BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 provisioning TEXT COLLATE "C" NOT NULL CHECK(provisioning IN ('pending','ready')),
 username TEXT COLLATE "C" UNIQUE NOT NULL, password_hash TEXT COLLATE "C"
);
CREATE TABLE tokens (
 token_id TEXT COLLATE "C" PRIMARY KEY, user_id TEXT COLLATE "C" NOT NULL REFERENCES users(user_id),
 digest TEXT COLLATE "C" UNIQUE NOT NULL, permissions TEXT COLLATE "C" NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ, revoked_at TIMESTAMPTZ,
 label TEXT COLLATE "C" NOT NULL
);
CREATE TABLE sessions (
 session_id TEXT COLLATE "C" PRIMARY KEY, user_id TEXT COLLATE "C" NOT NULL REFERENCES users(user_id),
 digest TEXT COLLATE "C" UNIQUE NOT NULL, created_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ
);
