# TokenInsights MyGo

Native Go presentation using MyGo v0.7.4 `ui`. App name **TokenInsights MyGo**;
bundle ID `com.flexdinesh.tokeninsights.mygo`. Registers `tokeninsights` and
`tokeninsights-mygo`. Electron registers its separate `tokeninsights-electron`
scheme. Existing `tokeninsights://open?url=...` links remain supported.

Local usage includes confirmed totals, date/provider/model/harness filters,
model totals table, collection/processing progress and retry. Hosted and external
URLs open in the system browser. Old trusted-origin files remain untouched.

`viewer.open` defaults to `auto`, which prefers MyGo and falls back to the browser. CLI `--open` overrides it with `mygo`,
`electron`, `browser`, `auto` or `false`. `app` remains a compatibility alias
for `mygo`; bare `--open` and `--open=true` select auto. Explicit unavailable
desktop targets return an error; `auto` tries MyGo and falls back to the browser.

`state.go` owns asynchronous loading and per-window session lifetime. `view.go`
consumes presentation values through an injected connector. `core_adapter.go`
owns localclient startup/attachment and query calls; linked URLs attach the matching existing owner without resolving default storage; `query_adapter.go` maps
application values to the view. Each window owns one backend lease. Filter
changes and refresh reuse it without capture. Retry collection requests capture
on that lease. Closing cancels outstanding I/O and releases the lease; reopening
creates a new session. The bundled `tokeninsights-backend` executable is adjacent
to the native executable and requires no host JavaScript runtime.

Tests use MyGo's headless `ui.NewTester`, including real filter selection,
custom-date validation, retry controls, stale-result protection, cancellation,
session release and reopening. Set `TOKENINSIGHTS_NATIVE_CAPTURE_DIR` for optional
PNG captures at 1180×780 and 720×520 in both themes. These captures are local
artifacts and must remain untracked.
