---
title: Privacy and data
weight: 3
---

TokenInsights collects usage metadata from durable local harness artifacts.
It does not store prompt text, assistant text, tool arguments, tool output,
request headers, secrets, raw provider payloads, or full source paths in raw
analytics storage.

## Local dashboards

`tokeninsights web` and `tokeninsights tui` keep collected usage in local SQLite
databases. The browser dashboard binds to localhost by default.

Default data files live under
`${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`:

| File | Purpose |
| --- | --- |
| `collector.sqlite` | Local source progress and evidence awaiting delivery |
| `server.sqlite` | Token history and processing state |
| `app.sqlite` | Application identity and settings |

## Distributed usage

Distributed sync is explicit: configure a remote server and run
`tokeninsights sync` to submit sanitized usage evidence. Local dashboards do not
switch to a remote server based on those settings.

Publication can include directory basenames and stable location keys. Full
collector-local directory paths and private conversation content stay local.
