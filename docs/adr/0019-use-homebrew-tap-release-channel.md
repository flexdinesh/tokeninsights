# Use a Homebrew tap release channel

Superseded where conflicting by [ADR 0027](0027-personal-backend-and-explicit-connections.md).
Current product: one personal SQLite backend, explicit local/SSH/temporary TCP
connections. Hosted/accounts/PostgreSQL/sync/plugins and conflicting listener,
opener or distribution choices below are historical. Applicable accounting,
privacy, identity, transaction and lifecycle principles remain.
The original decision below is preserved as history.

Superseded for current distribution/desktop behavior by [ADR 0024](0024-shared-backend-and-installable-products.md). Historical decisions remain below.

Renumbered from duplicate 0003; decision unchanged.

TokenInsights uses the generic `flexdinesh/homebrew-tap` repository as its Homebrew distribution channel, with `brew install flexdinesh/tap/tokeninsights` as the primary stable install command. The TokenInsights release workflow publishes prebuilt GitHub Release archives from the existing `packages/cli/vX.Y.Z` tag, then opens a pull request to update `Formula/tokeninsights.rb` in the tap using a narrowly scoped `HOMEBREW_TAP_TOKEN` secret. This keeps Go module-prefixed tags as the single release identity while giving users a low-friction Homebrew install path and preserving review/CI before tap updates land.
