# 3. Keep Sparse Non-Token Viewer Tabs Empty Until Canonical Domains Exist

Superseded by [ADR 0001](../../../docs/adr/0001-viewer-tabs-are-token-aggregation-modes.md). Non-token viewer tabs were an interim sparse-domain boundary; the active viewer surface now uses token aggregation tabs and hides TPS, request, and tool domains until durable canonical facts exist for them.

Execution order: 3

Type: AFK

Blocked by: None - can start immediately

## What to build

Make TPS, requests, tool calls, and tool breakdown tabs use domain-specific query boundaries instead of reusing token aggregation rows. Until those canonical domains exist, those tabs should render clean empty states, not token groups with blank metric columns.

## Acceptance criteria

- [x] Token tab still reads countable `canonical_token_usage`.
- [x] TPS, requests, tool calls, and tool breakdown have explicit query/load paths.
- [x] Non-token tabs return empty rows when only canonical token facts exist.
- [x] Tests cover tab loading for token and non-token tabs.
- [x] Existing TPS labels remain visible and unchanged.

## Blocked by

None - can start immediately.
