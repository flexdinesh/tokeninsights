# 3. Keep Sparse Non-Token Viewer Tabs Empty Until Canonical Domains Exist

Execution order: 3

Type: AFK

Blocked by: None - can start immediately

## What to build

Make TPS, requests, tool calls, and tool breakdown tabs use domain-specific query boundaries instead of reusing token aggregation rows. Until those canonical domains exist, those tabs should render clean empty states, not token groups with blank metric columns.

## Acceptance criteria

- [ ] Token tab still reads countable `canonical_token_usage`.
- [ ] TPS, requests, tool calls, and tool breakdown have explicit query/load paths.
- [ ] Non-token tabs return empty rows when only canonical token facts exist.
- [ ] Tests cover tab loading for token and non-token tabs.
- [ ] Existing TPS labels remain visible and unchanged.

## Blocked by

None - can start immediately.
