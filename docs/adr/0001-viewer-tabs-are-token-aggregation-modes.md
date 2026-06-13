# Viewer tabs are token aggregation modes

The active `view` TUI uses tabs to choose how countable canonical token facts are aggregated: tokens over time, models, providers, harnesses, and sessions. TPS, request, and tool domains remain future-compatible data domains, but they are not shown as empty active tabs until durable canonical facts exist for them. This keeps the sync-first V1 viewer focused on real token data and avoids presenting unavailable metric domains as broken or empty product surfaces.
