---
title: Local usage
weight: 2
---

Local dashboards run on your machine and read saved token usage. One viewer owns
the local storage at a time; close it before opening the other viewer.

## Browser dashboard

```sh
tokeninsights web
```

The dashboard opens at `http://127.0.0.1:8765`. The command runs in the foreground;
Ctrl+C stops its server.

To print the URL without opening a browser:

```sh
tokeninsights web --open=false
```

## Terminal dashboard

```sh
tokeninsights tui
```

Press `r` to query saved usage again. This does not start another collection.

## View saved usage

Skip collection on startup with `--sync=false`:

```sh
tokeninsights web --sync=false
tokeninsights tui --sync=false
```

Previously accepted data may still finish processing. Both viewers refresh when
new usage is published.

## Filter your view

Use date ranges and model, provider, or harness filters to narrow the displayed
usage. Filters affect what you see; they do not change which harnesses are
collected on startup.

## Refresh local sources

Restart the dashboard to collect again. If a source failed during collection,
the dashboard keeps saved usage available; restarting retries collection.
