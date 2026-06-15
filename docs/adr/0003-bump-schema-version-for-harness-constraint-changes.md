# Bump schema version for harness constraint changes

When adding a new persisted harness ID, TokenInsights bumps the SQLite `user_version` and requires `reset-all` instead of rebuilding existing tables in place. Expanding a harness `CHECK` constraint is logically additive, but existing databases physically reject the new value, and the project prefers an explicit storage compatibility break over maintaining a migration path for local sync-first data.
