---
title: Getting started
weight: 1
---

Install TokenInsights on the machine where you use OpenCode, Pi, Codex, or Claude Code.

## Install

With Homebrew:

```sh
brew install flexdinesh/tap/tokeninsights
```

Or with Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

If you install with Go, make sure the directory containing the installed binary
is on your `PATH`.

## Open your first dashboard

For the browser dashboard:

```sh
tokeninsights web
```

For the terminal dashboard:

```sh
tokeninsights tui
```

Both open saved usage while collecting and processing local usage in the
background. All four supported harnesses are collected by default. Published
usage appears automatically as processing finishes.

If you have not used a supported harness yet, there may be no usage to display.
Run it normally, then restart the dashboard to collect the new data.

Continue with [local usage](../local-usage/).
